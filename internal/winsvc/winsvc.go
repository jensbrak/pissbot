//go:build windows

// Package winsvc provides helpers for installing, removing, and running
// pissbot as a native Windows service via the Service Control Manager.
package winsvc

import (
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	SvcName        = "PissBot"
	SvcDisplayName = "PISS Bot (Public IP Server Service)"
	SvcDescription = "Discord bot that reports the machine's public IP address on demand via !piss."
)

// Starter is implemented by App in the main package.
type Starter interface {
	Start() error
	Stop()
}

// handler adapts App to the svc.Handler interface required by the SCM.
type handler struct {
	app    Starter
	logger *slog.Logger
}

// Execute is called by the SCM when the service is started.
// It must block until a Stop or Shutdown command is received.
func (h *handler) Execute(_ []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown

	status <- svc.Status{State: svc.StartPending}

	if err := h.app.Start(); err != nil {
		h.logger.Error("app failed to start", "error", err)
		// Return svcSpecificEC=true, exitCode=1 to signal failure to the SCM.
		return true, 1
	}

	status <- svc.Status{State: svc.Running, Accepts: accepted}
	h.logger.Info("service is running")

	for c := range r {
		switch c.Cmd {
		case svc.Stop, svc.Shutdown:
			h.logger.Info("stop signal received from SCM")
			status <- svc.Status{State: svc.StopPending}
			h.app.Stop()
			return false, 0
		default:
			// Log and ignore unrecognised commands; the SCM should not send
			// anything we haven't declared in Accepts, but be defensive.
			h.logger.Warn("ignoring unexpected SCM command", "cmd", c.Cmd)
		}
	}

	return false, 0
}

// RunService hands control to the Windows SCM.
// Set isDebug=true to run the service handler in-process without the SCM
// (useful for testing the service logic from a normal console session).
func RunService(name string, isDebug bool, app Starter, logger *slog.Logger) error {
	h := &handler{app: app, logger: logger}
	if isDebug {
		return debug.Run(name, h)
	}
	return svc.Run(name, h)
}

// ─── SCM management helpers ──────────────────────────────────────────────────
// All functions below require an elevated (Administrator) process.

// Install registers the service with the SCM. exePath must be the absolute
// path to the pissbot executable. The service is configured for automatic
// start so it launches on every system boot.
func Install(exePath string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to SCM: %w", err)
	}
	defer m.Disconnect()

	// Prevent accidental double-install.
	if s, err := m.OpenService(SvcName); err == nil {
		s.Close()
		return fmt.Errorf("service %q already exists — run -uninstall first", SvcName)
	}

	s, err := m.CreateService(SvcName, exePath, mgr.Config{
		DisplayName: SvcDisplayName,
		Description: SvcDescription,
		StartType:   mgr.StartAutomatic,
	})
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer s.Close()

	// Register an Event Log source so Windows Event Viewer knows about us.
	if err := eventlog.InstallAsEventCreate(SvcName, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
		_ = s.Delete() // roll back on failure
		return fmt.Errorf("register event log source: %w", err)
	}

	return nil
}

// Uninstall removes the service from the SCM. The service must be stopped first.
func Uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(SvcName)
	if err != nil {
		return fmt.Errorf("service %q not found: %w", SvcName, err)
	}
	defer s.Close()

	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	// Non-fatal: the service is gone even if Event Log cleanup fails.
	_ = eventlog.Remove(SvcName)
	return nil
}

// Start asks the SCM to start the service.
func Start() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(SvcName)
	if err != nil {
		return fmt.Errorf("open service: %w", err)
	}
	defer s.Close()

	return s.Start()
}

// Stop sends a stop command to the SCM and blocks until the service has
// reached the Stopped state or the 15-second deadline is exceeded.
func Stop() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(SvcName)
	if err != nil {
		return fmt.Errorf("open service: %w", err)
	}
	defer s.Close()

	status, err := s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("send stop control: %w", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for status.State != svc.Stopped {
		if time.Now().After(deadline) {
			return fmt.Errorf("service did not stop within 15 s")
		}
		time.Sleep(300 * time.Millisecond)
		if status, err = s.Query(); err != nil {
			return fmt.Errorf("query service status: %w", err)
		}
	}
	return nil
}
