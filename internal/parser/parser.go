package parser

// Note is a single parsed Markdown note with its YAML frontmatter.
// Implementation arrives in ticket 02 alongside NoteStore (see ADR-0001).
type Note struct {
	ID    string
	Title string
	Body  string
}

// NoteStore is declared ahead of its implementation, which arrives in
// ticket 02 once the parser exists (see ADR-0001).
type NoteStore interface {
	List() ([]Note, error)
	Load(id string) (*Note, error)
}
