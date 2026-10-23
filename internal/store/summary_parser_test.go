package store

import (
	"reflect"
	"testing"
)

func TestParseSessionSummary_Canonical(t *testing.T) {
	raw := `
## Goal
Audit the repository center-control-manager and build an integration plan.

## Accomplished
1. Cloned and audited code under ponytail standards.
2. Extracted 5 high-value capabilities.
- Formulated single-binary architecture plan.

## Key Decisions
- Use isolated radar.db for backlog tasks.
- Keep engram.db strictly read-only.

## Discoveries
* Herdr socket can be queried safely via net.DialTimeout.

## Next Steps
1. Implement Phase 1 SQLite store.
2. Add web REST handlers.
`

	parsed := ParseSessionSummary(raw)

	expectedGoal := "Audit the repository center-control-manager and build an integration plan."
	if parsed.Goal != expectedGoal {
		t.Errorf("Goal mismatch: got %q, want %q", parsed.Goal, expectedGoal)
	}

	if len(parsed.Accomplished) != 3 {
		t.Fatalf("Accomplished count mismatch: got %d, want 3", len(parsed.Accomplished))
	}
	if parsed.Accomplished[0] != "Cloned and audited code under ponytail standards." {
		t.Errorf("Accomplished[0] mismatch: %q", parsed.Accomplished[0])
	}

	if len(parsed.Decisions) != 2 {
		t.Fatalf("Decisions count mismatch: got %d, want 2", len(parsed.Decisions))
	}
	if parsed.Decisions[0] != "Use isolated radar.db for backlog tasks." {
		t.Errorf("Decisions[0] mismatch: %q", parsed.Decisions[0])
	}

	if len(parsed.Discoveries) != 1 {
		t.Fatalf("Discoveries count mismatch: got %d, want 1", len(parsed.Discoveries))
	}
	if parsed.Discoveries[0] != "Herdr socket can be queried safely via net.DialTimeout." {
		t.Errorf("Discoveries[0] mismatch: %q", parsed.Discoveries[0])
	}

	if len(parsed.NextSteps) != 2 {
		t.Fatalf("NextSteps count mismatch: got %d, want 2", len(parsed.NextSteps))
	}
	if parsed.NextSteps[0] != "Implement Phase 1 SQLite store." {
		t.Errorf("NextSteps[0] mismatch: %q", parsed.NextSteps[0])
	}
}

func TestParseSessionSummary_AlternativeHeadersAndSpanish(t *testing.T) {
	raw := `
### Objetivo
Migrar base de datos.

### Logros
- Completada la tabla radar_tasks.
- Migración ejecutada sin errores.

### Próximos pasos
- Diseñar interfaz gráfica en web.
`

	parsed := ParseSessionSummary(raw)
	if parsed.Goal != "Migrar base de datos." {
		t.Errorf("Goal: got %q", parsed.Goal)
	}
	if len(parsed.Accomplished) != 2 {
		t.Errorf("Accomplished count: got %d, want 2", len(parsed.Accomplished))
	}
	if len(parsed.NextSteps) != 1 {
		t.Errorf("NextSteps count: got %d, want 1", len(parsed.NextSteps))
	}
}

func TestParseSessionSummary_UnstructuredFallback(t *testing.T) {
	raw := "This is just a simple one-line session summary with no markdown headers."
	parsed := ParseSessionSummary(raw)

	if parsed.Goal != raw {
		t.Errorf("expected fallback to Goal: got %q, want %q", parsed.Goal, raw)
	}
	if len(parsed.Accomplished) != 0 || len(parsed.NextSteps) != 0 {
		t.Errorf("expected empty lists on unstructured text")
	}
}

func TestParseSessionSummary_Empty(t *testing.T) {
	parsed := ParseSessionSummary("   \n\t  ")
	if parsed.Goal != "" {
		t.Errorf("expected empty goal, got %q", parsed.Goal)
	}
	if !reflect.DeepEqual(parsed.Accomplished, []string{}) {
		t.Errorf("expected empty slice, got %+v", parsed.Accomplished)
	}
}
