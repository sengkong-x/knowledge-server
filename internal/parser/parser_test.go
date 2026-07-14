package parser

import (
	"testing"
	"time"
)

func TestParse_ParsesFullFrontmatter(t *testing.T) {
	raw := []byte(`---
title: Hybrid Logical Clock
tags:
  - distributed-system
  - consistency
aliases:
  - HLC
related:
  - vector-clock
  - lamport-clock
status: evergreen
created: 2026-07-12
---

# Overview

Hybrid Logical Clock combines physical time and a logical counter.
`)

	note, err := Parse("distributed-systems/hlc", raw)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if note.ID != "distributed-systems/hlc" {
		t.Errorf("ID = %q, want %q", note.ID, "distributed-systems/hlc")
	}
	if note.Title != "Hybrid Logical Clock" {
		t.Errorf("Title = %q, want %q", note.Title, "Hybrid Logical Clock")
	}
	if len(note.Tags) != 2 || note.Tags[0] != "distributed-system" || note.Tags[1] != "consistency" {
		t.Errorf("Tags = %v, want [distributed-system consistency]", note.Tags)
	}
	if len(note.Aliases) != 1 || note.Aliases[0] != "HLC" {
		t.Errorf("Aliases = %v, want [HLC]", note.Aliases)
	}
	if len(note.Related) != 2 || note.Related[0] != "vector-clock" || note.Related[1] != "lamport-clock" {
		t.Errorf("Related = %v, want [vector-clock lamport-clock]", note.Related)
	}
	if note.Status != "evergreen" {
		t.Errorf("Status = %q, want %q", note.Status, "evergreen")
	}
	want := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	if !note.Created.Equal(want) {
		t.Errorf("Created = %v, want %v", note.Created, want)
	}
	wantBody := "# Overview\n\nHybrid Logical Clock combines physical time and a logical counter."
	if note.Body != wantBody {
		t.Errorf("Body = %q, want %q", note.Body, wantBody)
	}
}

func TestParse_AcceptsStatusAsSingleElementList(t *testing.T) {
	raw := []byte(`---
title: Vector Clock
created: 2026-07-12
status:
  - evergreen
---
Body.
`)

	note, err := Parse("vector-clock", raw)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if note.Status != "evergreen" {
		t.Errorf("Status = %q, want %q", note.Status, "evergreen")
	}
}

func TestParse_RejectsStatusWithMultipleValues(t *testing.T) {
	raw := []byte(`---
title: Vector Clock
created: 2026-07-12
status:
  - evergreen
  - needs-review
---
Body.
`)

	_, err := Parse("vector-clock", raw)
	if err == nil {
		t.Fatal("Parse returned nil error for multi-value status, want error")
	}
}

func TestParse_AcceptsTimestampWithTimeOfDay(t *testing.T) {
	raw := []byte(`---
title: Vector Clock
created: 2026-07-12T14:30:00Z
---
Body.
`)

	note, err := Parse("vector-clock", raw)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	want := time.Date(2026, 7, 12, 14, 30, 0, 0, time.UTC)
	if !note.Created.Equal(want) {
		t.Errorf("Created = %v, want %v", note.Created, want)
	}
}

func TestParse_ErrorsOnMissingTitle(t *testing.T) {
	raw := []byte(`---
created: 2026-07-12
---
Body.
`)

	_, err := Parse("linux/process", raw)
	if err == nil {
		t.Fatal("Parse returned nil error for missing title, want error")
	}
}

func TestParse_ErrorsOnMissingCreated(t *testing.T) {
	raw := []byte(`---
title: Process
---
Body.
`)

	_, err := Parse("linux/process", raw)
	if err == nil {
		t.Fatal("Parse returned nil error for missing created, want error")
	}
}

func TestParse_ErrorsOnMissingFrontmatterEntirely(t *testing.T) {
	raw := []byte("# Just a heading\n\nNo frontmatter here.\n")

	_, err := Parse("linux/process", raw)
	if err == nil {
		t.Fatal("Parse returned nil error for missing frontmatter, want error")
	}
}

func TestParse_ErrorsOnUnclosedFrontmatter(t *testing.T) {
	raw := []byte("---\ntitle: Process\ncreated: 2026-07-12\n\nBody without a closing delimiter.\n")

	_, err := Parse("linux/process", raw)
	if err == nil {
		t.Fatal("Parse returned nil error for unclosed frontmatter, want error")
	}
}

func TestParse_IgnoresUnknownFrontmatterFields(t *testing.T) {
	raw := []byte(`---
title: Process
created: 2026-07-12
priority: high
---
Body.
`)

	note, err := Parse("linux/process", raw)
	if err != nil {
		t.Fatalf("Parse returned error for unknown field: %v", err)
	}
	if note.Title != "Process" {
		t.Errorf("Title = %q, want %q", note.Title, "Process")
	}
}

func TestParse_TrimsBodyWhitespace(t *testing.T) {
	raw := []byte("---\ntitle: Process\ncreated: 2026-07-12\n---\n\n\n# Process\n\nBody text.\n\n\n")

	note, err := Parse("linux/process", raw)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	want := "# Process\n\nBody text."
	if note.Body != want {
		t.Errorf("Body = %q, want %q", note.Body, want)
	}
}

func TestParse_ErrorsOnMalformedYAML(t *testing.T) {
	raw := []byte("---\ntitle: [unterminated\ncreated: 2026-07-12\n---\nBody.\n")

	_, err := Parse("linux/process", raw)
	if err == nil {
		t.Fatal("Parse returned nil error for malformed YAML, want error")
	}
}
