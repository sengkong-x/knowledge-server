package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/sengkong/knowledge-server/internal/activevault"
	"github.com/sengkong/knowledge-server/internal/index"
	"github.com/sengkong/knowledge-server/internal/notes"
	"github.com/sengkong/knowledge-server/internal/settings"
	"github.com/sengkong/knowledge-server/internal/vault"
	"github.com/sengkong/knowledge-server/web"
	"github.com/yuin/goldmark"
)

// layoutTemplate wraps a page's content in the full HTML document: theme
// CSS (driven by ActiveVault's current theme, switchable at runtime — see
// ADR-0011), vendored HTMX/Alpine.js, the vault-picker nav (Ticket 07), and
// a live-update script that reacts to the /events SSE ping (ADR-0009) by
// re-fetching the current page's content in place.
//
// Page content lives inside #page-content, deliberately separate from
// {{.Nav}}: every HTMX-driven update in this app (the SSE live-update
// script below, searchUITemplate's form) targets #page-content rather than
// <body>, specifically so the nav survives those swaps instead of being
// wiped out by them — <body> previously had nothing else in it, so
// targeting the whole body was harmless before this ticket added the nav.
var layoutTemplate = template.Must(template.New("layout").Parse(`<!doctype html>
<html data-theme="{{.Theme}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<link rel="stylesheet" href="/themes/base.css">
<link rel="stylesheet" href="/themes/{{.Theme}}.css">
<script src="/vendor/htmx.min.js"></script>
<script src="/vendor/alpine.min.js" defer></script>
</head>
<body>
{{.Nav}}
<div id="page-content">
{{.Content}}
</div>
<script>
document.body.addEventListener("htmx:responseError", function (evt) {
  var targetSel = evt.detail.elt.getAttribute("data-error-target");
  if (!targetSel) return;
  var el = document.querySelector(targetSel);
  if (!el) return;
  var message = "Request failed.";
  try {
    var body = JSON.parse(evt.detail.xhr.responseText);
    if (body.error) message = body.error;
  } catch (e) {}
  el.textContent = message;
});
new EventSource("/events").onmessage = function () {
  htmx.ajax("GET", window.location.pathname + window.location.search, {target: "#page-content", swap: "innerHTML"});
};
</script>
</body>
</html>`))

type layoutView struct {
	Theme   string
	Title   string
	Content template.HTML
	Nav     template.HTML
}

// The icon inner-paths below are each used at more than one size/context
// across the templates in this file (e.g. the vault icon in both the nav
// picker and the empty state), so they're named constants rather than
// inline markup repeated verbatim at each call site.
const (
	iconPathBook   = `<path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/>`
	iconPathVault  = `<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z"/>`
	iconPathSearch = `<circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/>`
	iconPathCheck  = `<path d="M20 6 9 17l-5-5"/>`
	iconPathPlus   = `<path d="M12 5v14M5 12h14"/>`
	iconPathClose  = `<path d="M18 6 6 18M6 6l12 12"/>`
	iconPathEdit   = `<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/>`
	iconPathFile   = `<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/>`
	iconPathClock  = `<circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>`
)

// svgIcon renders an inline SVG icon at the given size and stroke weight —
// a shared shape for every icon in this file's templates rather than
// duplicating the surrounding <svg> attributes at each call site.
func svgIcon(size int, strokeWidth, innerPath string) template.HTML {
	return template.HTML(fmt.Sprintf(
		`<svg width="%d" height="%d" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="%s" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">%s</svg>`,
		size, size, strokeWidth, innerPath))
}

// pathLabel is the short, scannable form of a Vault path shown in the nav —
// its parent directory name rather than the full absolute path, which reads
// as debug output when dumped verbatim into chrome. The full path is still
// available via each element's title attribute for anyone who needs it.
func pathLabel(path string) string {
	base := filepath.Base(path)
	if base == "." || base == string(filepath.Separator) {
		return path
	}
	return base
}

// tagHue is one of tagColorClasses hue buckets a tag's chip color is hashed
// into (see hueOf) — a named type rather than a bare int, since it stands
// for "which of the six tag colors", not an arbitrary number.
type tagHue int

// tagColorClasses is the number of hue buckets tags are hashed into (see
// hueOf) — matched by the .tag-0..tag-{{tagColorClasses-1}} rules in
// base.css/light.css/dark.css.
const tagColorClasses tagHue = 6

// hueOf hashes a tag name to one of tagColorClasses buckets so the same tag
// always renders in the same hue across pages, without maintaining an
// explicit tag->color registry as new tags get added to notes.
func hueOf(tag string) tagHue {
	h := fnv.New32a()
	h.Write([]byte(tag))
	return tagHue(h.Sum32() % uint32(tagColorClasses))
}

// renderTags renders each tag as a hue-colored chip — shared by the browse
// and search result cards rather than each duplicating the same
// range-and-hash markup shape.
func renderTags(tags []string) template.HTML {
	var buf bytes.Buffer
	for _, t := range tags {
		fmt.Fprintf(&buf, `<span class="tag tag-%d">%s</span>`, hueOf(t), template.HTMLEscapeString(t))
	}
	return template.HTML(buf.String())
}

// pluralCount renders "N noun" or "N nouns" — the shared shape behind the
// browse and search page-meta lines, which otherwise differ (one names a
// vault, the other a query) too much to share a whole template snippet.
func pluralCount(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// formatDate renders a note's Created frontmatter as a short, scannable
// date for the entry-card footer. Created is a required frontmatter field
// (parser.go rejects notes missing it before they ever reach the index),
// so every entry passed here is guaranteed non-zero.
func formatDate(t time.Time) string {
	return t.Format("Jan 2, 2006")
}

// navTemplate is the vault-picker + theme-toggle chrome shown on every
// page (there was no nav anywhere before this ticket). The picker lists
// VaultHistory (from GET /settings) as switch triggers, plus an "Add new
// vault..." toggle revealing a free-text path input — an Alpine x-data
// dropdown, since Alpine's already vendored for exactly this kind of small
// interactive widget (see web/vendor/alpine.min.js).
//
// Every action here (hx-put="/vault", hx-put="/theme") sets hx-swap="none"
// and relies on the server setting an HX-Refresh response header on
// success (see the PUT /vault and PUT /theme handlers below) to trigger a
// full page reload — simpler and more correct than trying to swap in a
// freshly rendered page fragment, and it naturally re-renders both the nav
// (now reflecting the new vault/theme) and the content in one browser
// navigation. On failure (no HX-Refresh header, a JSON error body instead),
// the htmx:responseError listener in layoutTemplate reads the triggering
// element's data-error-target attribute and writes the error message into
// that element — see #vault-error below.
//
// When no vault is selected (HasVault false), the picker starts expanded
// (x-data's initial "open" value), since Ticket 07 requires it be the
// empty state's primary call to action, not a collapsed control the user
// has to discover.
var navTemplate = template.Must(template.New("nav").Funcs(template.FuncMap{"base": pathLabel}).Parse(`<nav class="site-nav">
<div class="nav-start">
<span class="brand">` + string(svgIcon(20, "2", iconPathBook)) + ` Knowledge Server</span>
<ul class="nav-links">
<li><a href="/" class="{{if eq .Active "browse"}}active{{end}}">Browse</a></li>
<li><a href="/search/ui" class="{{if eq .Active "search"}}active{{end}}">Search</a></li>
<li><a href="/graph/ui" class="{{if eq .Active "graph"}}active{{end}}">Graph</a></li>
</ul>
</div>
<div class="nav-end">
<div class="vault-picker" x-data="{open: {{if .HasVault}}false{{else}}true{{end}}}">
<button type="button" @click="open = !open" class="id-chip" aria-haspopup="true" :aria-expanded="open" {{if .HasVault}}title="{{.CurrentPath}}"{{end}}>
` + string(svgIcon(14, "2", iconPathVault)) + `
<span>{{if .HasVault}}{{base .CurrentPath}}{{else}}No vault selected{{end}}</span>
<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m6 9 6 6 6-6"/></svg>
</button>
<div x-show="open" x-transition>
<p class="vault-menu-label">Switch vault</p>
<ul>
{{$current := .CurrentPath}}{{range .History}}<li class="vault-item-row"><button hx-put="/vault" hx-vals='{"path": "{{.}}"}' hx-swap="none" data-error-target="#vault-error" class="menu-row{{if eq . $current}} vault-item-active{{end}}" title="{{.}}">` + string(svgIcon(15, "2", iconPathVault)) + `<span>{{base .}}</span>{{if eq . $current}}` + string(svgIcon(15, "2", iconPathCheck)) + `{{end}}</button><button type="button" hx-delete="/vault" hx-vals='{"path": "{{.}}"}' hx-swap="none" data-error-target="#vault-error" class="vault-item-remove" aria-label="Remove {{base .}} from vault history" title="Remove from history">` + string(svgIcon(13, "2", iconPathClose)) + `</button></li>
{{end}}</ul>
<div x-data="vaultBrowser()">
<button type="button" @click="openModal()" class="menu-row">` + string(svgIcon(15, "2", iconPathPlus)) + `<span>Add new vault&hellip;</span></button>
<template x-teleport="body">
<div x-show="open" x-cloak class="vault-browser-modal" @keydown.escape.window="closeModal()" @click.self="closeModal()">
<div class="vault-browser-panel" role="dialog" aria-modal="true" aria-label="Add new vault">
<div class="vault-browser-breadcrumb">
<div class="vault-browser-path-segments" role="navigation" x-show="!editingPath" aria-label="Current path">
<template x-for="seg in segments" :key="seg.path">
<span class="vault-browser-path-segment-wrap"><button type="button" @click="load(seg.path)" x-text="seg.name" class="vault-browser-path-segment"></button><span class="vault-browser-path-sep" x-show="seg.sep">/</span></span>
</template>
</div>
<input type="text" x-show="editingPath" x-model="pathInput" @keydown.enter="jumpTo(pathInput)" @blur="editingPath = false" class="vault-browser-path-input" aria-label="Vault path">
<button type="button" class="icon-button" x-show="!editingPath" @click="startEditingPath()" aria-label="Type a path directly" title="Type a path directly">` + string(svgIcon(14, "2", iconPathEdit)) + `</button>
<button type="button" class="button-primary" @click="useFolder()" :disabled="pending">Use this folder</button>
</div>
<ul class="vault-browser-list">
<template x-for="dir in directories" :key="dir">
<li><button type="button" @click="navigate(dir)" class="menu-row">` + string(svgIcon(14, "2", iconPathVault)) + `<span x-text="dir"></span></button></li>
</template>
<template x-if="!pending && directories.length === 0"><li class="vault-browser-empty">No subfolders here.</li></template>
</ul>
<p class="vault-browser-error" x-show="error" x-text="error" role="alert"></p>
</div>
</div>
</template>
</div>
</div>
</div>
<script src="/js/vault-browser.js"></script>
<button class="icon-button" hx-put="/theme" hx-vals='{"theme": "{{.NextTheme}}"}' hx-swap="none" aria-label="{{if eq .Theme "dark"}}Switch to light mode{{else}}Switch to dark mode{{end}}">
{{if eq .Theme "dark"}}<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>{{else}}<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/></svg>{{end}}
</button>
<span id="vault-error" role="alert"></span>
</div>
</nav>`))

type navView struct {
	HasVault    bool
	CurrentPath string
	History     []string
	Theme       string
	NextTheme   string
	Active      string
}

func renderNav(av *activevault.ActiveVault, active string) (template.HTML, error) {
	path, _, _, _, hasVault := av.Snapshot()
	theme := av.Theme()

	s, err := settings.Load()
	if err != nil {
		return "", err
	}

	nextTheme := "dark"
	if theme == "dark" {
		nextTheme = "light"
	}

	return renderFragment(navTemplate, navView{
		HasVault:    hasVault,
		CurrentPath: path,
		History:     s.VaultHistory,
		Theme:       theme,
		NextTheme:   nextTheme,
		Active:      active,
	})
}

// render writes content wrapped in the full page layout (including the nav)
// for a normal browser navigation, or just content on its own for an HTMX
// request (identified by the HX-Request header) — the live-update script
// above swaps the latter into #page-content without re-loading <head> or
// the nav.
func render(w http.ResponseWriter, r *http.Request, av *activevault.ActiveVault, title string, content template.HTML, active string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("HX-Request") == "true" {
		w.Write([]byte(content))
		return
	}

	nav, err := renderNav(av, active)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	layoutTemplate.Execute(w, layoutView{Theme: av.Theme(), Title: title, Content: content, Nav: nav})
}

func renderFragment(tmpl *template.Template, data any) (template.HTML, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// noVaultSelectedTemplate is the fallback content for HTML routes when no
// Active Vault is selected (a first-class, expected state — the server
// boots into it and stays in it until the picker UI selects a vault). The
// nav's own picker (rendered expanded in this state, see navTemplate) is
// the primary call to action; this is just the content-area companion.
var noVaultSelectedTemplate = template.Must(template.New("noVault").Parse(`<div class="empty-state">
` + string(svgIcon(40, "1.5", iconPathVault)) + `
<p>No vault selected.</p>
<p>Pick one from the vault picker above to get started.</p>
</div>`))

// noVaultSelected renders the "no vault selected" fallback for an HTML
// route so a missing Active Vault never reaches a nil provider/store/state
// and panics.
func noVaultSelected(w http.ResponseWriter, r *http.Request, av *activevault.ActiveVault) {
	content, _ := renderFragment(noVaultSelectedTemplate, nil)
	render(w, r, av, "No vault selected", content, "")
}

// noVaultSelectedJSON is the equivalent fallback for API routes.
func noVaultSelectedJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	json.NewEncoder(w).Encode(map[string]string{"error": "no vault selected"})
}

// noteDetailTemplate renders a note's title, goldmark-rendered body, and its
// Graph neighbors as related-note links. Body is produced by goldmark from
// trusted, locally-authored Vault content and is intentionally not escaped
// further; Title and Neighbors are escaped normally.
var noteDetailTemplate = template.Must(template.New("note").Parse(`<article class="note">
<a href="/">&larr; Back to browse</a>
<h1>{{.Title}}</h1>
{{.Body}}
{{if .Neighbors}}<h2>Related notes</h2>
<ul>
{{range .Neighbors}}<li><a href="/notes/{{.}}" class="id-chip">{{.}}</a></li>
{{end}}</ul>{{end}}
</article>`))

type noteDetailView struct {
	Title     string
	Body      template.HTML
	Neighbors []string
}

// entryCardView is the shape rendered by the shared "entryCard" template
// below — both browseTemplate (from index.IndexEntry) and searchUITemplate
// (from searchResultResponse, which also carries a Snippet) convert into
// this common view rather than each maintaining its own copy of the card
// markup, since the two were otherwise identical but for the snippet line.
type entryCardView struct {
	ID      string
	Title   string
	Tags    []string
	Created time.Time
	Snippet string
}

func entryCardFromIndex(e index.IndexEntry) entryCardView {
	return entryCardView{ID: e.ID, Title: e.Title, Tags: e.Tags, Created: e.Created}
}

func entryCardFromSearch(r searchResultResponse) entryCardView {
	return entryCardView{ID: r.ID, Title: r.Title, Tags: r.Tags, Created: r.Created, Snippet: r.Snippet}
}

// entryCardTemplateSrc defines the "entryCard" associated template shared by
// browseTemplate and searchUITemplate: a file icon + title, tags as the
// colorful "badges" that actually carry meaning (categorization), and the ID
// + Created date below a hairline divider as plain muted metadata — the two
// pieces of per-note info that used to render as visually-identical pills
// (id-chip vs tag) no longer compete for the same "badge" reading.
var entryCardTemplateSrc = `{{define "entryCard"}}<li class="entry-card"><a href="/notes/{{.ID}}" class="entry-card-title">` + string(svgIcon(16, "2", iconPathFile)) + `<span>{{.Title}}</span></a>
{{if .Snippet}}<p class="entry-card-snippet">{{.Snippet}}</p>{{end}}
{{if .Tags}}<div class="entry-card-tags">{{tags .Tags}}</div>{{end}}
<div class="entry-card-footer"><span class="entry-card-id">{{.ID}}</span><span class="entry-card-date">` + string(svgIcon(12, "2", iconPathClock)) + `{{formatDate .Created}}</span></div>
</li>{{end}}`

// entryCardFuncs is shared by both browseTemplate and searchUITemplate so
// each can convert its own row type (index.IndexEntry / searchResultResponse)
// into entryCardView before invoking the "entryCard" associated template.
var entryCardFuncs = template.FuncMap{
	"tags":                renderTags,
	"pluralCount":         pluralCount,
	"formatDate":          formatDate,
	"entryCardFromIndex":  entryCardFromIndex,
	"entryCardFromSearch": entryCardFromSearch,
}

// browseTemplate lists notes as links to their detail page.
var browseTemplate = template.Must(template.Must(template.New("browse").Funcs(entryCardFuncs).Parse(entryCardTemplateSrc)).Parse(`<h1>Browse</h1>
{{if .Entries}}<p class="page-meta">{{pluralCount (len .Entries) "note"}} in <span class="id-chip">{{.VaultLabel}}</span></p>
<ul class="entry-list">
{{range .Entries}}{{template "entryCard" (entryCardFromIndex .)}}
{{end}}</ul>{{else}}<div class="empty-state">
` + string(svgIcon(40, "1.5", iconPathBook)) + `
<p>This vault has no notes yet.</p>
</div>{{end}}`))

type browseView struct {
	Entries    []index.IndexEntry
	VaultLabel string
}

// searchUITemplate renders a search form plus any matching results.
var searchUITemplate = template.Must(template.Must(template.New("searchUI").Funcs(entryCardFuncs).Parse(entryCardTemplateSrc)).Parse(`<h1>Search</h1>
<form hx-get="/search/ui" hx-target="#page-content" class="search-bar">
` + string(svgIcon(16, "2", iconPathSearch)) + `
<input type="text" name="q" value="{{.Query}}" placeholder="Search notes..." aria-label="Search notes" autofocus>
<button type="submit" class="button-primary">Search</button>
</form>
{{if .Results}}<p class="page-meta">{{pluralCount (len .Results) "result"}} for &ldquo;{{.Query}}&rdquo;</p>
<ul class="entry-list">
{{range .Results}}{{template "entryCard" (entryCardFromSearch .)}}
{{end}}</ul>{{else if .Query}}<div class="empty-state">
` + string(svgIcon(40, "1.5", iconPathSearch)) + `
<p>No notes match &ldquo;{{.Query}}&rdquo;.</p>
</div>{{end}}`))

type searchUIView struct {
	Query   string
	Results []searchResultResponse
}

// graphUITemplate is the Cytoscape.js graph visualization shell. Cytoscape
// itself and the script that fetches /graph/data into it are vendored
// frontend assets (see ADR-0007's companion asset-vendoring deliverable),
// not written here.
const graphUITemplate = `<div class="graph-panel-header">
<h1>Graph</h1>
<p>Notes linked by <code>related</code>, laid out by connectivity.</p>
</div>
<div id="cy"></div>
<script src="/vendor/cytoscape.min.js"></script>
<script src="/js/graph.js" data-source="/graph/data"></script>`

type healthResponse struct {
	VaultPath string `json:"vault_path"`
	NoteCount int    `json:"note_count"`
}

type searchResultResponse struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Path    string    `json:"path"`
	Tags    []string  `json:"tags"`
	Snippet string    `json:"snippet"`
	Created time.Time `json:"created"`
}

type neighborsResponse struct {
	Neighbors []string `json:"neighbors"`
}

type pathResponse struct {
	Path  []string `json:"path"`
	Found bool     `json:"found"`
}

type orphansResponse struct {
	Orphans []string `json:"orphans"`
}

type graphNodeResponse struct {
	ID        string   `json:"id"`
	Neighbors []string `json:"neighbors"`
}

type graphDataResponse struct {
	Nodes []graphNodeResponse `json:"nodes"`
}

type settingsResponse struct {
	VaultPath    string   `json:"vault_path"`
	Theme        string   `json:"theme"`
	VaultHistory []string `json:"vault_history"`
}

type browseDirectoriesResponse struct {
	Path        string   `json:"path"`
	Directories []string `json:"directories"`
}

type switchVaultRequest struct {
	Path string `json:"path"`
}

type switchThemeRequest struct {
	Theme string `json:"theme"`
}

// parseVaultPath reads the "path" value from a PUT /vault request body.
// Ticket 06 speced a JSON body ({"path": "..."}), but Ticket 07's picker UI
// submits via plain htmx hx-vals/hx-include, which htmx sends form-encoded
// (application/x-www-form-urlencoded) unless a separate JSON-encoding
// extension is vendored — so this accepts either: a JSON body first, and if
// that doesn't parse into a non-empty Path, falls back to treating the body
// as form-encoded. Existing JSON API callers are unaffected.
func parseVaultPath(r *http.Request) (string, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}

	var req switchVaultRequest
	if err := json.Unmarshal(body, &req); err == nil && req.Path != "" {
		return req.Path, nil
	}

	values, err := url.ParseQuery(string(body))
	if err != nil {
		return "", fmt.Errorf("parsing request body: %w", err)
	}
	return values.Get("path"), nil
}

// updateSettings loads the current settings, applies with to them, and
// saves the result — the load-mutate-save shape shared by every handler
// that persists a Settings change (PUT/DELETE /vault, PUT /theme).
func updateSettings(with func(settings.Settings) settings.Settings) error {
	saved, err := settings.Load()
	if err != nil {
		return err
	}
	return settings.Save(with(saved))
}

// parseTheme is parseVaultPath's counterpart for PUT /theme.
func parseTheme(r *http.Request) (string, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}

	var req switchThemeRequest
	if err := json.Unmarshal(body, &req); err == nil && req.Theme != "" {
		return req.Theme, nil
	}

	values, err := url.ParseQuery(string(body))
	if err != nil {
		return "", fmt.Errorf("parsing request body: %w", err)
	}
	return values.Get("theme"), nil
}

// New builds the http.Handler for a running instance. Every route reads the
// Active Vault fresh per request via av.Snapshot()/av.Theme() rather than
// closing over a vault/theme captured once at construction time, since
// ADR-0011 lets av.Switch/SetTheme change what's active at any point in the
// server's lifetime.
func New(av *activevault.ActiveVault) http.Handler {
	mux := http.NewServeMux()

	assets := http.FileServerFS(web.FS)
	mux.Handle("GET /vendor/", assets)
	mux.Handle("GET /themes/", assets)
	mux.Handle("GET /js/", assets)
	mux.Handle("GET /fonts/", assets)

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		path, _, _, s, ok := av.Snapshot()
		if !ok {
			noVaultSelected(w, r, av)
			return
		}

		tag := r.URL.Query().Get("tag")

		var entries []index.IndexEntry
		if tag != "" {
			entries = s.ByTag(tag)
		} else {
			entries = s.IndexAll()
		}

		content, err := renderFragment(browseTemplate, browseView{Entries: entries, VaultLabel: pathLabel(path)})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		render(w, r, av, "Browse", content, "browse")
	})

	mux.HandleFunc("GET /search/ui", func(w http.ResponseWriter, r *http.Request) {
		_, _, _, s, ok := av.Snapshot()
		if !ok {
			noVaultSelected(w, r, av)
			return
		}

		q := r.URL.Query().Get("q")

		var results []searchResultResponse
		if q != "" {
			for _, m := range s.Query(q) {
				entry, ok := s.ByID(m.ID)
				if !ok {
					continue
				}
				results = append(results, searchResultResponse{
					ID:      entry.ID,
					Title:   entry.Title,
					Path:    entry.Path,
					Tags:    entry.Tags,
					Snippet: m.Snippet,
					Created: entry.Created,
				})
			}
		}

		content, err := renderFragment(searchUITemplate, searchUIView{Query: q, Results: results})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		render(w, r, av, "Search", content, "search")
	})

	mux.HandleFunc("GET /graph/ui", func(w http.ResponseWriter, r *http.Request) {
		render(w, r, av, "Graph", template.HTML(graphUITemplate), "graph")
	})

	// /events: an SSE connection subscribes to the currently active vault's
	// State for content-change pings (ADR-0009). A vault switch discards
	// that State and swaps in a new one (Ticket 05), which would otherwise
	// leave any already-open /events connection subscribed to a State
	// nobody will ever notify again — a silent "stops updating" gap rather
	// than a visible failure. Resolution (deliberately chosen over the
	// alternative of leaving it silently stale): also subscribe to
	// av.SubscribeSwitch(), and end the stream the moment a switch happens.
	// The browser's EventSource auto-reconnects on a closed connection,
	// which re-runs this handler, re-Snapshots, and subscribes fresh to the
	// new State — so a switch costs one reconnect, not a silently stopped
	// feed.
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		_, _, _, s, hasVault := av.Snapshot()

		var stateCh <-chan struct{}
		unsubState := func() {}
		if hasVault {
			stateCh, unsubState = s.Subscribe()
		}
		defer unsubState()

		switchCh, unsubSwitch := av.SubscribeSwitch()
		defer unsubSwitch()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-switchCh:
				return
			case <-stateCh:
				// Generic "something changed" ping, no per-note payload
				// (see ADR-0009) — every listening view re-fetches its own
				// current content rather than reasoning about what changed.
				fmt.Fprint(w, "data: change\n\n")
				flusher.Flush()
			}
		}
	})

	mux.HandleFunc("GET /assets/{path...}", func(w http.ResponseWriter, r *http.Request) {
		// No path-traversal guard needed here: ServeMux itself redirects any
		// request path containing ".." to its cleaned equivalent before a
		// handler ever runs (see net/http's ServeMux docs), and
		// VaultProvider.ReadAsset rejects any escape of the Vault root on
		// its own behalf, since it's the abstraction that owns filesystem
		// safety, not this transport-layer handler.
		_, provider, _, _, ok := av.Snapshot()
		if !ok {
			http.NotFound(w, r)
			return
		}

		reqPath := r.PathValue("path")

		data, err := provider.ReadAsset(reqPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.ServeContent(w, r, reqPath, time.Time{}, bytes.NewReader(data))
	})

	mux.HandleFunc("GET /notes/{id...}", func(w http.ResponseWriter, r *http.Request) {
		_, _, store, s, ok := av.Snapshot()
		if !ok {
			noVaultSelected(w, r, av)
			return
		}

		id := r.PathValue("id")
		note, err := store.Load(id)
		if err != nil {
			if errors.Is(err, notes.ErrNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var body bytes.Buffer
		if err := goldmark.Convert([]byte(note.Body), &body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// A note missing from the Graph (e.g. it failed to parse during the
		// last Build) simply has no related notes to show, rather than
		// failing the whole page.
		neighbors, _ := s.Neighbors(id)

		content, err := renderFragment(noteDetailTemplate, noteDetailView{
			Title:     note.Title,
			Body:      template.HTML(body.String()),
			Neighbors: neighbors,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		render(w, r, av, note.Title, content, "")
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		path, provider, _, _, ok := av.Snapshot()

		w.Header().Set("Content-Type", "application/json")
		if !ok {
			json.NewEncoder(w).Encode(healthResponse{})
			return
		}

		notes, err := provider.ListNotes()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(healthResponse{
			VaultPath: path,
			NoteCount: len(notes),
		})
	})

	mux.HandleFunc("GET /search", func(w http.ResponseWriter, r *http.Request) {
		_, _, _, s, ok := av.Snapshot()
		if !ok {
			noVaultSelectedJSON(w)
			return
		}

		q := r.URL.Query().Get("q")
		tag := r.URL.Query().Get("tag")
		if q == "" && tag == "" {
			http.Error(w, "missing q or tag parameter", http.StatusBadRequest)
			return
		}

		// snippets holds the matched-text excerpt per note ID when q is
		// given; tag-only results have no snippet, since they didn't come
		// from a text query.
		var candidates []index.IndexEntry
		snippets := make(map[string]string)
		if q != "" {
			for _, m := range s.Query(q) {
				if entry, ok := s.ByID(m.ID); ok {
					candidates = append(candidates, entry)
					snippets[m.ID] = m.Snippet
				}
			}
		} else {
			candidates = s.ByTag(tag)
		}

		results := make([]searchResultResponse, 0, len(candidates))
		for _, entry := range candidates {
			if q != "" && tag != "" && !entry.HasTag(tag) {
				continue
			}
			results = append(results, searchResultResponse{
				ID:      entry.ID,
				Title:   entry.Title,
				Path:    entry.Path,
				Tags:    entry.Tags,
				Snippet: snippets[entry.ID],
				Created: entry.Created,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	})

	mux.HandleFunc("GET /graph/neighbors", func(w http.ResponseWriter, r *http.Request) {
		_, _, _, s, ok := av.Snapshot()
		if !ok {
			noVaultSelectedJSON(w)
			return
		}

		id := r.URL.Query().Get("id")
		neighbors, err := s.Neighbors(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(neighborsResponse{Neighbors: neighbors})
	})

	mux.HandleFunc("GET /graph/path", func(w http.ResponseWriter, r *http.Request) {
		_, _, _, s, ok := av.Snapshot()
		if !ok {
			noVaultSelectedJSON(w)
			return
		}

		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		path, found, err := s.ShortestPath(from, to)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pathResponse{Path: path, Found: found})
	})

	mux.HandleFunc("GET /graph/orphans", func(w http.ResponseWriter, r *http.Request) {
		_, _, _, s, ok := av.Snapshot()
		if !ok {
			noVaultSelectedJSON(w)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(orphansResponse{Orphans: s.Orphans()})
	})

	mux.HandleFunc("GET /graph/data", func(w http.ResponseWriter, r *http.Request) {
		_, _, _, s, ok := av.Snapshot()
		if !ok {
			noVaultSelectedJSON(w)
			return
		}

		entries := s.GraphAll()
		nodes := make([]graphNodeResponse, 0, len(entries))
		for _, entry := range entries {
			neighbors := make([]string, 0, len(entry.Neighbors))
			for n := range entry.Neighbors {
				neighbors = append(neighbors, n)
			}
			sort.Strings(neighbors)
			nodes = append(nodes, graphNodeResponse{ID: entry.ID, Neighbors: neighbors})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(graphDataResponse{Nodes: nodes})
	})

	mux.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) {
		s, err := settings.Load()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settingsResponse{
			VaultPath:    s.VaultPath,
			Theme:        s.Theme,
			VaultHistory: s.VaultHistory,
		})
	})

	// PUT, not POST: switching the vault sets the one current selection
	// (an idempotent "this is now the active vault" state change) rather
	// than creating a new resource — same reasoning for PUT /theme below.
	// No existing route in this table sets a precedent either way (all are
	// GET), so this is a fresh choice, documented here rather than left
	// implicit.
	//
	// On success, the response carries HX-Refresh: true rather than any
	// content — the Ticket 07 picker's htmx triggers all set hx-swap="none"
	// and rely on this header to make the browser do a full page reload,
	// which re-renders both the nav (now reflecting the new vault/theme)
	// and the content in one navigation, rather than trying to hand-swap a
	// freshly rendered page fragment into place. A non-htmx/JSON API caller
	// simply gets 200 with an empty body and can ignore the header.
	mux.HandleFunc("PUT /vault", func(w http.ResponseWriter, r *http.Request) {
		path, err := parseVaultPath(r)
		if err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if err := av.Switch(path); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		canonical, _, _, _, _ := av.Snapshot()
		if err := updateSettings(func(s settings.Settings) settings.Settings { return s.WithVault(canonical) }); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("HX-Refresh", "true")
		w.WriteHeader(http.StatusOK)
	})

	// DELETE /vault: removes path from the picker's history and deletes its
	// disposable Engines cache. If path is the currently active vault, this
	// also clears the active selection entirely (forced switch to "no vault
	// selected" — see ActiveVault.RemoveVault and Settings.WithoutVault) so
	// removal never silently falls back to another history entry.
	mux.HandleFunc("DELETE /vault", func(w http.ResponseWriter, r *http.Request) {
		path, err := parseVaultPath(r)
		if err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		canonical, err := av.RemoveVault(path)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		if err := updateSettings(func(s settings.Settings) settings.Settings { return s.WithoutVault(canonical) }); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("HX-Refresh", "true")
		w.WriteHeader(http.StatusOK)
	})

	// GET /vault/browse: powers the "Add new vault" modal's directory
	// browser (a plain server-side listing, since browsers can never expose
	// an absolute filesystem path from a native picker — see ADR-0011
	// addendum). Returns the canonicalized path alongside its immediate
	// subdirectories so the client always has a well-formed path to submit
	// back, even if the query param it sent wasn't canonical.
	mux.HandleFunc("GET /vault/browse", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			path = home
		}

		canonical, err := vault.CanonicalPath(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := vault.ValidateRoot(canonical); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		dirs, err := vault.ListDirectories(canonical)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(browseDirectoriesResponse{Path: canonical, Directories: dirs})
	})

	mux.HandleFunc("PUT /theme", func(w http.ResponseWriter, r *http.Request) {
		theme, err := parseTheme(r)
		if err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		av.SetTheme(theme)

		if err := updateSettings(func(s settings.Settings) settings.Settings { return s.WithTheme(theme) }); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("HX-Refresh", "true")
		w.WriteHeader(http.StatusOK)
	})

	return mux
}
