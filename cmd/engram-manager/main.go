// Command engram-manager is the primary CLI tool for inspecting and managing Engram memories.
//
// Usage:
//
//	engram-manager              # Launch interactive TUI (default)
//	engram-manager tui          # Launch interactive TUI explicitly
//	engram-manager status       # Quick CLI health and metric summary (non-interactive)
//	engram-manager web          # Placeholder for upcoming web dashboard
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"engram-manager/internal/api"
	"engram-manager/internal/cli"
	"engram-manager/internal/store"
	"engram-manager/internal/tui"
)

func usage() {
	fmt.Fprintf(os.Stderr, `engram-manager — Monitor and memory manager for Engram

Usage:
  engram-manager [flags] [command]

Commands:
  tui         Launch interactive terminal UI (default if no command provided)
  status      Print database health, active sessions, and metrics to stdout
  web         Start web dashboard (upcoming)

Flags:
`)
	flag.PrintDefaults()
}

func main() {
	var (
		dbPath  string
		apiAddr string
	)

	flag.StringVar(&dbPath, "db", store.DefaultPath, "path to engram.db")
	flag.StringVar(&apiAddr, "api", api.DefaultAddr, "address of `engram serve`, for writes")
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	cmd := "tui"
	if len(args) > 0 {
		cmd = args[0]
	}

	st, err := store.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engram-manager: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	switch cmd {
	case "tui":
		client := api.New(apiAddr)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := client.EnsureServer(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "note:", err)
			fmt.Fprintln(os.Stderr, "note: read-only features still work; write commands will need the server")
		}
		cancel()

		if err := tui.Run(st, client); err != nil {
			fmt.Fprintf(os.Stderr, "engram-manager tui: %v\n", err)
			os.Exit(1)
		}

	case "status":
		if err := cli.PrintStatus(os.Stdout, st); err != nil {
			fmt.Fprintf(os.Stderr, "engram-manager status: %v\n", err)
			os.Exit(1)
		}

	case "web", "gui", "dashboard":
		fmt.Println("Web dashboard / GUI is currently under development.")
		fmt.Printf("For now, you can use the interactive terminal UI with `engram-manager tui`.\n")

	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(1)
	}
}
