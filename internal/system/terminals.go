package system

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DefaultHerdrSocket is the standard UNIX domain socket for Herdr.
const DefaultHerdrSocket = "~/.config/herdr/herdr.sock"

// HerdrStatus reports availability and active agents of Herdr.
type HerdrStatus struct {
	Available  bool        `json:"available"`
	SocketPath string      `json:"socket_path"`
	Agents     []AgentInfo `json:"agents"`
}

// AgentInfo describes an agent registered in Herdr.
type AgentInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Project string `json:"project,omitempty"`
}

// ProcessInfo describes a running coding agent process.
type ProcessInfo struct {
	PID     int    `json:"pid"`
	Name    string `json:"name"`
	Command string `json:"command"`
}

// TerminalMonitorStatus bundles Herdr and active agent process inspection.
type TerminalMonitorStatus struct {
	Herdr     HerdrStatus   `json:"herdr"`
	Processes []ProcessInfo `json:"processes"`
	Active    int           `json:"active_count"`
}

// InspectTerminals inspects Herdr socket and active agent processes.
func InspectTerminals() TerminalMonitorStatus {
	res := TerminalMonitorStatus{
		Herdr: HerdrStatus{
			Available:  false,
			SocketPath: expandPath(DefaultHerdrSocket),
			Agents:     []AgentInfo{},
		},
		Processes: []ProcessInfo{},
	}

	// 1. Probe Herdr socket
	sockPath := res.Herdr.SocketPath
	conn, err := net.DialTimeout("unix", sockPath, 300*time.Millisecond)
	if err == nil {
		res.Herdr.Available = true
		_ = conn.Close()

		// If herdr binary exists, query agents
		ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
		defer cancel()

		if herdrBin, err := exec.LookPath("herdr"); err == nil {
			cmd := exec.CommandContext(ctx, herdrBin, "agent", "list", "--json")
			if out, err := cmd.Output(); err == nil {
				var agents []AgentInfo
				if err := json.Unmarshal(out, &agents); err == nil {
					res.Herdr.Agents = agents
				}
			}
		}
	}

	// 2. Discover active agent processes (pi, gentle-pi, herdr)
	procs := discoverAgentProcesses()
	res.Processes = procs
	res.Active = len(procs)

	return res
}

func discoverAgentProcesses() []ProcessInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()

	// Use pgrep with command line
	cmd := exec.CommandContext(ctx, "pgrep", "-a", "-f", "pi|herdr")
	out, err := cmd.Output()
	if err != nil {
		return []ProcessInfo{}
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var result []ProcessInfo
	myPid := os.Getpid()

	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		parts := strings.SplitN(l, " ", 2)
		if len(parts) < 2 {
			continue
		}
		pid, err := strconv.Atoi(parts[0])
		if err != nil || pid == myPid {
			continue
		}

		cmdLine := parts[1]
		// Filter out unrelated pgrep or grep commands
		if strings.Contains(cmdLine, "pgrep") || strings.Contains(cmdLine, "grep") {
			continue
		}

		name := filepath.Base(strings.Fields(cmdLine)[0])
		result = append(result, ProcessInfo{
			PID:     pid,
			Name:    name,
			Command: cmdLine,
		})
	}

	return result
}
