//go:build linux

// Tests in this file are Linux-only and will not run on Windows or other
// platforms. Run them in a Linux environment or a Linux CI runner.
// They cover systemd-detection and sd_notify. SCM flags (HandleSCMFlags) are
// a no-op on Linux, so there is nothing meaningful to test there.
package platform

import (
	"net"
	"testing"
	"time"
)

// ── IsService ─────────────────────────────────────────────────────────────────

func TestIsService_linux(t *testing.T) {
	t.Run("false when INVOCATION_ID unset", func(t *testing.T) {
		t.Setenv("INVOCATION_ID", "")
		ok, err := IsService()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Error("IsService() should be false when INVOCATION_ID is empty")
		}
	})

	t.Run("true when INVOCATION_ID set", func(t *testing.T) {
		t.Setenv("INVOCATION_ID", "abc123def456")
		ok, err := IsService()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Error("IsService() should be true when INVOCATION_ID is set")
		}
	})
}

// ── sdNotify ──────────────────────────────────────────────────────────────────

func TestSdNotify_noSocket(t *testing.T) {
	// When NOTIFY_SOCKET is unset, sdNotify should be a silent no-op.
	t.Setenv("NOTIFY_SOCKET", "")
	sdNotify("READY=1") // must not panic or return an error
}

// ── ServiceLogger ─────────────────────────────────────────────────────────────

func TestServiceLogger_linux(t *testing.T) {
	// On Linux the logger writes to stdout; logPath is ignored.
	logger, closeLog, err := ServiceLogger("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer closeLog()

	if logger == nil {
		t.Error("logger should not be nil")
	}
}

// ── sdNotify with socket ──────────────────────────────────────────────────────

func TestSdNotify_withSocket(t *testing.T) {
	// Create a real Unix datagram socket to verify sdNotify actually writes.
	dir := t.TempDir()
	socketPath := dir + "/notify.sock"

	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: socketPath, Net: "unixgram"})
	if err != nil {
		t.Skipf("cannot create unix socket (may not be available in this environment): %v", err)
	}
	defer conn.Close()

	t.Setenv("NOTIFY_SOCKET", socketPath)
	sdNotify("READY=1")

	buf := make([]byte, 64)
	conn.SetReadDeadline(time.Now().Add(time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("no data received from sdNotify: %v", err)
	}
	if got := string(buf[:n]); got != "READY=1" {
		t.Errorf("sdNotify sent %q, want %q", got, "READY=1")
	}
}
