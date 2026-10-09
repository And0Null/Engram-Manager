// Package cli implements headless command-line subcommands (e.g. status)
// that print diagnostic and store health information directly to stdout.
package cli

import (
	"fmt"
	"io"
	"strings"

	"engram-manager/internal/store"
)

// PrintStatus writes a clean, styled summary of Engram health to out.
func PrintStatus(out io.Writer, st *store.Store) error {
	h, err := st.Health()
	if err != nil {
		return fmt.Errorf("read store health: %w", err)
	}
	dbSize, walSize, err := st.DBSize()
	if err != nil {
		return fmt.Errorf("read db size: %w", err)
	}
	projects, err := st.Projects()
	if err != nil {
		return fmt.Errorf("read projects: %w", err)
	}

	fmt.Fprintln(out, "Engram Health & Status")
	fmt.Fprintf(out, "Database: %s (%s, WAL: %s)\n", st.Path(), humanSize(dbSize), humanSize(walSize))
	fmt.Fprintf(out, "Memories: %d live (%d total)\n", h.LiveObservations, h.TotalObservations)
	fmt.Fprintf(out, "Sessions: %d (%d active)\n", h.TotalSessions, h.ActiveSessions)
	fmt.Fprintf(out, "Prompts:  %d across %d projects\n", h.TotalPrompts, h.Projects)
	if emb, err := st.EmbeddingStats(); err == nil && emb.Available {
		modelDisplay := emb.Model
		if idx := strings.LastIndex(modelDisplay, "/"); idx != -1 {
			modelDisplay = modelDisplay[idx+1:]
		}
		fmt.Fprintf(out, "Vectors:  %d / %d (%.0f%%, %dd %s, %d pending)\n",
			emb.TotalEmbedded, emb.LiveCount, emb.CoveragePct, emb.Dimensions, modelDisplay, emb.PendingCount)
	}
	fmt.Fprintln(out)

	// Concerns
	var concerns []string
	if h.AmbiguousSessions > 0 {
		concerns = append(concerns, fmt.Sprintf("%d sessions share a project with another active session", h.AmbiguousSessions))
	}
	if h.ProjectsWithoutSession > 0 {
		concerns = append(concerns, fmt.Sprintf("%d projects have memories with no owning session", h.ProjectsWithoutSession))
	}
	if h.OrphanSessions > 0 {
		concerns = append(concerns, fmt.Sprintf("%d memories point at nonexistent sessions", h.OrphanSessions))
	}
	if h.DeletedCount > 0 {
		concerns = append(concerns, fmt.Sprintf("%d soft-deleted memories remain in the database", h.DeletedCount))
	}

	if len(concerns) > 0 {
		fmt.Fprintln(out, "Warnings:")
		for _, c := range concerns {
			fmt.Fprintf(out, "  [!] %s\n", c)
		}
	} else {
		fmt.Fprintln(out, "Health: OK (all invariants hold, 0 operational drifts)")
	}

	if len(projects) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Top Projects:")
		shown := projects
		if len(shown) > 5 {
			shown = shown[:5]
		}
		for _, p := range shown {
			fmt.Fprintf(out, "  %-24s %4d obs  (%2d ses)\n", p.Name, p.Observations, p.Sessions)
		}
	}

	return nil
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
