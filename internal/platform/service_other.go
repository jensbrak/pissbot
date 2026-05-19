//go:build !windows

package platform

import "log/slog"

// IsService always returns false on non-Windows platforms.
func IsService() (bool, error) { return false, nil }

// RunAsService is unreachable on non-Windows since IsService always returns
// false, but must be defined to satisfy the compiler on all platforms.
func RunAsService(_ Starter, _ *slog.Logger) error { return nil }

// HandleSCMFlags is a no-op on non-Windows platforms. The -install, -start,
// -stop and -uninstall flags are accepted but silently ignored.
func HandleSCMFlags(install, uninstall, start, stop bool) (bool, error) { return false, nil }
