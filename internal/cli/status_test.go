package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"engram-manager/internal/store"
)

func TestPrintStatus(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	p := home + "/.engram/engram.db"
	if _, err := os.Stat(p); err != nil {
		t.Skip("no engram db")
	}

	st, err := store.Open(p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	var buf bytes.Buffer
	if err := PrintStatus(&buf, st); err != nil {
		t.Fatalf("PrintStatus: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Engram Health & Status") {
		t.Errorf("missing header in output:\n%s", out)
	}
	if !strings.Contains(out, "Memories:") || !strings.Contains(out, "Sessions:") {
		t.Errorf("missing counts in output:\n%s", out)
	}
}
