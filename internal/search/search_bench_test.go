package search

import (
	"fmt"
	"testing"

	"github.com/sengkong/knowledge-server/internal/notes"
	"github.com/sengkong/knowledge-server/internal/vault"
	"github.com/sengkong/knowledge-server/internal/vaultfixture"
)

// BenchmarkQuery reports Query latency against a fixture sized to represent
// a realistic personal vault. The spec's 50ms target is verified by reading
// ns/op from `go test -bench=.` output, not asserted here (see ADR-0005 and
// the ticket 04 grilling decisions on why this isn't a hard CI assertion).
func BenchmarkQuery(b *testing.B) {
	root := b.TempDir()
	const noteCount = 3000
	for i := range noteCount {
		rel := fmt.Sprintf("notes/note-%d.md", i)
		content := fmt.Sprintf(`---
title: Note about topic %d
created: 2026-07-12
---
This note discusses distributed systems, hybrid logical clocks, and consensus protocols in example %d.
`, i, i)
		vaultfixture.WriteNote(b, root, rel, content)
	}

	provider := vault.NewLocalVaultProvider(root)
	store := notes.NewVaultNoteStore(provider)
	ss, _, err := Build(provider, store)
	if err != nil {
		b.Fatalf("Build returned error: %v", err)
	}

	for b.Loop() {
		ss.Query("logical clocks")
	}
}
