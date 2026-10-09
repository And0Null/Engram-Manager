// Command engram-tui provides direct, dedicated execution of the terminal UI.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"engram-manager/internal/api"
	"engram-manager/internal/store"
	"engram-manager/internal/tui"
)

func main() {
	var (
		dbPath  = flag.String("db", store.DefaultPath, "path to engram.db")
		apiAddr = flag.String("api", api.DefaultAddr, "address of `engram serve`, for writes")
	)
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engram-tui: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	client := api.New(*apiAddr)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := client.EnsureServer(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "note:", err)
		fmt.Fprintln(os.Stderr, "note: read-only features still work; write commands will need the server")
	}
	cancel()

	if err := tui.Run(st, client); err != nil {
		fmt.Fprintf(os.Stderr, "engram-tui: %v\n", err)
		os.Exit(1)
	}
}
