package parser

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Note is a single parsed Markdown note with its YAML frontmatter.
type Note struct {
	ID      string
	Title   string
	Tags    []string
	Aliases []string
	Related []string
	Status  string
	Created time.Time
	Body    string
}

const delimiter = "---"

// frontmatter mirrors the YAML block at the top of a note. Status is decoded
// as yaml.Node so it can accept either a scalar or a single-element list.
type frontmatter struct {
	Title   string    `yaml:"title"`
	Tags    []string  `yaml:"tags"`
	Aliases []string  `yaml:"aliases"`
	Related []string  `yaml:"related"`
	Status  yaml.Node `yaml:"status"`
	Created time.Time `yaml:"created"`
}

// Parse turns raw note bytes (frontmatter + Markdown body) into a Note.
// id is the note's identifier, as produced by VaultProvider.ListNotes.
func Parse(id string, raw []byte) (*Note, error) {
	yamlBlock, body, err := splitFrontmatter(raw)
	if err != nil {
		return nil, fmt.Errorf("note %q: %w", id, err)
	}

	var fm frontmatter
	if err := yaml.Unmarshal(yamlBlock, &fm); err != nil {
		return nil, fmt.Errorf("note %q: parsing frontmatter: %w", id, err)
	}

	if fm.Title == "" {
		return nil, fmt.Errorf("note %q: missing required field %q", id, "title")
	}
	if fm.Created.IsZero() {
		return nil, fmt.Errorf("note %q: missing required field %q", id, "created")
	}

	status, err := decodeStatus(fm.Status)
	if err != nil {
		return nil, fmt.Errorf("note %q: %w", id, err)
	}

	return &Note{
		ID:      id,
		Title:   fm.Title,
		Tags:    fm.Tags,
		Aliases: fm.Aliases,
		Related: fm.Related,
		Status:  status,
		Created: fm.Created,
		Body:    strings.TrimSpace(body),
	}, nil
}

// splitFrontmatter separates the YAML frontmatter block from the Markdown
// body. A note with no frontmatter block yields an empty YAML block, which
// Parse then rejects for missing required fields.
func splitFrontmatter(raw []byte) (yamlBlock []byte, body string, err error) {
	text := string(raw)

	if !strings.HasPrefix(text, delimiter) {
		return nil, text, nil
	}

	rest := text[len(delimiter):]
	rest = strings.TrimPrefix(rest, "\n")

	before, after, found := strings.Cut(rest, "\n"+delimiter)
	if !found {
		return nil, text, nil
	}

	return []byte(before), after, nil
}

// decodeStatus accepts status as either a scalar or a single-element list.
func decodeStatus(node yaml.Node) (string, error) {
	switch node.Kind {
	case 0:
		return "", nil
	case yaml.ScalarNode:
		return node.Value, nil
	case yaml.SequenceNode:
		if len(node.Content) == 0 {
			return "", nil
		}
		if len(node.Content) > 1 {
			return "", fmt.Errorf("field %q: expected a single value, got %d", "status", len(node.Content))
		}
		return node.Content[0].Value, nil
	default:
		return "", fmt.Errorf("field %q: unsupported YAML type", "status")
	}
}
