// vaultBrowser() backs the nav's "Add new vault" modal (server.go's
// navTemplate). Browsers can't expose an absolute filesystem path from a
// native folder picker, so this walks GET /vault/browse (a plain
// os.ReadDir-based listing, see internal/vault.ListDirectories) one
// directory at a time instead, with an editable breadcrumb for typing/
// pasting a path directly. "Use this folder" submits the currently
// displayed path to PUT /vault (the same switch flow the history list
// uses), which sets HX-Refresh on success — hence the plain location.reload
// rather than an htmx swap.
function vaultBrowser() {
  return {
    open: false,
    path: "",
    pathInput: "",
    directories: [],
    editingPath: false,
    pending: false,
    error: "",

    // segments splits the current absolute path into clickable breadcrumb
    // pieces (each cumulative from root), so navigating *up* is a plain
    // click on an ancestor segment — same interaction as descending into a
    // subfolder — rather than requiring the text-edit path meant for power
    // users jumping somewhere arbitrary.
    get segments() {
      const parts = this.path.split("/").filter(Boolean);
      const segments = [{ name: "/", path: "/" }];
      let acc = "";
      for (const part of parts) {
        acc += "/" + part;
        segments.push({ name: part, path: acc });
      }
      segments.forEach((seg, i) => {
        seg.last = i === segments.length - 1;
        // The root segment's own label is already "/", so it doesn't need
        // a separator glyph after it too (that rendered as a "/ /" double
        // slash) — every other non-last segment still gets one.
        seg.sep = !seg.last && i !== 0;
      });
      return segments;
    },

    openModal() {
      this.open = true;
      this.load("");
    },

    closeModal() {
      this.open = false;
      this.editingPath = false;
    },

    startEditingPath() {
      this.pathInput = this.path;
      this.editingPath = true;
    },

    navigate(dir) {
      const next = this.path.endsWith("/") ? this.path + dir : this.path + "/" + dir;
      this.load(next);
    },

    jumpTo(path) {
      this.editingPath = false;
      this.load(path);
    },

    load(path) {
      this.pending = true;
      this.error = "";
      const url = path ? "/vault/browse?path=" + encodeURIComponent(path) : "/vault/browse";
      fetch(url)
        .then((res) => {
          if (!res.ok) {
            return res.text().then((text) => {
              throw new Error(text || "Failed to list directory.");
            });
          }
          return res.json();
        })
        .then((data) => {
          this.path = data.path;
          this.directories = data.directories || [];
        })
        .catch((err) => {
          this.error = err.message;
        })
        .finally(() => {
          this.pending = false;
        });
    },

    useFolder() {
      this.pending = true;
      this.error = "";
      fetch("/vault", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path: this.path }),
      })
        .then((res) => {
          if (!res.ok) {
            return res.json().then((body) => {
              throw new Error((body && body.error) || "Failed to switch vault.");
            });
          }
          window.location.reload();
        })
        .catch((err) => {
          this.error = err.message;
          this.pending = false;
        });
    },
  };
}
