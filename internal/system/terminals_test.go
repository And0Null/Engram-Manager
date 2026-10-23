package system

import "testing"

func TestInspectTerminals(t *testing.T) {
	status := InspectTerminals()
	if status.Herdr.SocketPath == "" {
		t.Errorf("expected socket path to be non-empty")
	}
	// Verify that processes is non-nil slice
	if status.Processes == nil {
		t.Errorf("expected non-nil processes slice")
	}
}
