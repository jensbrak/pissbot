//go:build !windows && !linux

package platform

import (
	"log/slog"
	"os"
)

// ServiceLogger returns a logger writing to stdout. logPath is accepted for
// API consistency with the Windows implementation but is not used.
func ServiceLogger(_ string) (*slog.Logger, func(), error) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return logger, func() {}, nil
}

// IsService always returns false on non-Windows, non-Linux platforms.
func IsService() (bool, error) { return false, nil }

// RunAsService is unreachable since IsService always returns false, but must
// be defined to satisfy the compiler on all platforms.
func RunAsService(_ Starter, _ *slog.Logger) error { return nil }

// HandleSCMFlags is a no-op. The -install, -start, -stop and -uninstall flags
// are accepted but silently ignored on this platform.
func HandleSCMFlags(install, uninstall, start, stop bool) (bool, error) { return false, nil }
