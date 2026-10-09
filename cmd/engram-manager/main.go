// Command engram-manager is the primary CLI tool for inspecting and managing Engram memories.
//
// Usage:
//
//	engram-manager              # Launch interactive TUI (default)
//	engram-manager tui          # Launch interactive TUI explicitly
//	engram-manager status       # Quick CLI health and metric summary (non-interactive)
//	engram-manager web          # Launch embedded web dashboard (default: 127.0.0.1:7438)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"engram-manager/internal/api"
	"engram-manager/internal/cli"
	"engram-manager/internal/store"
	"engram-manager/internal/tui"
	"engram-manager/internal/web"
)

func usage() {
	fmt.Fprintf(os.Stderr, `engram-manager — Monitor and memory manager for Engram

Usage:
  engram-manager [flags] [command]

Commands:
  tui         Launch interactive terminal UI (default if no command provided)
  status      Print database health, active sessions, and metrics to stdout
  web         Start embedded web dashboard (listens on 127.0.0.1:7438 by default)

Flags:
`)
	flag.PrintDefaults()
}

func main() {
	var (
		dbPath  string
		apiAddr string
		webAddr string
	)

	defaultWeb := "127.0.0.1:7438"
	if p := os.Getenv("ENGRAM_DASH_PORT"); p != "" {
		defaultWeb = "127.0.0.1:" + p
	} else if p := os.Getenv("PORT"); p != "" {
		defaultWeb = "127.0.0.1:" + p
	}

	flag.StringVar(&dbPath, "db", store.DefaultPath, "path to engram.db")
	flag.StringVar(&apiAddr, "api", api.DefaultAddr, "address of `engram serve`, for writes")
	flag.StringVar(&webAddr, "addr", defaultWeb, "address for web dashboard (engram-manager web)")
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
		if len(args) > 1 {
			webFlags := flag.NewFlagSet("web", flag.ExitOnError)
			webFlags.StringVar(&webAddr, "addr", webAddr, "address for web dashboard to listen on")
			var port string
			webFlags.StringVar(&port, "port", "", "port to listen on (e.g. 7438)")
			_ = webFlags.Parse(args[1:])
			if port != "" {
				if !strings.Contains(port, ":") {
					webAddr = "127.0.0.1:" + port
				} else {
					webAddr = port
				}
			}
		}

		client := api.New(apiAddr)
		srv, err := web.NewServer(st, client)
		if err != nil {
			fmt.Fprintf(os.Stderr, "engram-manager web: %v\n", err)
			os.Exit(1)
		}

		httpServer := &http.Server{
			Addr:         webAddr,
			Handler:      srv.Handler(),
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		}

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		go func() {
			fmt.Printf("Engram Manager Web Dashboard listening on http://%s\n", webAddr)
			fmt.Printf("Database: %s (mode=ro)\n", st.Path())
			fmt.Printf("API backend: %s\n", apiAddr)
			fmt.Println("Press Ctrl+C to stop.")
			if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				fmt.Fprintf(os.Stderr, "server error: %v\n", err)
				os.Exit(1)
			}
		}()

		<-sigChan
		fmt.Println("\nShutting down web server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(os.Stderr, "shutdown error: %v\n", err)
		}
		fmt.Println("Stopped.")

	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(1)
	}
}
