//go:build windows

package platform

import (
	"fmt"
	"log/slog"
	"os"

	"golang.org/x/sys/windows/svc"

	"github.com/jensbrak/pissbot/internal/winsvc"
)

// maxLogBytes is the file size at which the log is rotated on service startup.
const maxLogBytes int64 = 10 << 20 // 10 MiB

// ServiceLogger opens the log file at logPath (rotating it first if it exceeds
// maxLogBytes) and returns a slog.Logger writing to that file, plus a closer
// that must be called when the process exits.
func ServiceLogger(logPath string) (*slog.Logger, func(), error) {
	rotateLogs(logPath)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log %q: %w", logPath, err)
	}
	logger := slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return logger, func() { f.Close() }, nil
}

// rotateLogs renames logPath to logPath+".1" when the file exceeds maxLogBytes,
// making room for a fresh log file. Only one backup generation is kept.
// Failures are silently ignored — a stale large log is better than no log.
func rotateLogs(logPath string) {
	info, err := os.Stat(logPath)
	if err != nil || info.Size() < maxLogBytes {
		return
	}
	_ = os.Rename(logPath, logPath+".1")
}

// IsService reports whether the process was launched by the Windows SCM.
func IsService() (bool, error) {
	return svc.IsWindowsService()
}

// RunAsService hands control to the Windows SCM and blocks until the service
// is stopped.
func RunAsService(app Starter, logger *slog.Logger) error {
	return winsvc.RunService(winsvc.SvcName, false, app, logger)
}

// HandleSCMFlags processes Windows service sub-commands. It returns true if a
// flag was handled, in which case the caller should exit.
func HandleSCMFlags(install, uninstall, start, stop bool) (bool, error) {
	switch {
	case install:
		exePath, err := os.Executable()
		if err != nil {
			return true, fmt.Errorf("resolve exe path: %w", err)
		}
		if err := winsvc.Install(exePath); err != nil {
			return true, fmt.Errorf("install: %w", err)
		}
		fmt.Printf("Service %q installed successfully.\n", winsvc.SvcName)
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  1. Set the DiscordToken environment variable at the SYSTEM level")
		fmt.Println("     (System Properties → Advanced → Environment Variables → System variables)")
		fmt.Printf("  2. pissbot.exe -start   (or: net start %s)\n", winsvc.SvcName)
		return true, nil

	case uninstall:
		if err := winsvc.Uninstall(); err != nil {
			return true, fmt.Errorf("uninstall: %w", err)
		}
		fmt.Printf("Service %q uninstalled successfully.\n", winsvc.SvcName)
		return true, nil

	case start:
		if err := winsvc.Start(); err != nil {
			return true, fmt.Errorf("start: %w", err)
		}
		fmt.Printf("Service %q started.\n", winsvc.SvcName)
		return true, nil

	case stop:
		if err := winsvc.Stop(); err != nil {
			return true, fmt.Errorf("stop: %w", err)
		}
		fmt.Printf("Service %q stopped.\n", winsvc.SvcName)
		return true, nil
	}

	return false, nil
}
