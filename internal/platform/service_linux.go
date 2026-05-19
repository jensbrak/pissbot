//go:build linux

package platform

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
)

// ServiceLogger returns a logger that writes to stdout. Under systemd, stdout
// is forwarded to journald automatically, which handles timestamping, rotation,
// and indexed querying — no log file or rotation logic is needed here.
// logPath is accepted for API consistency with the Windows implementation but
// is not used.
func ServiceLogger(_ string) (*slog.Logger, func(), error) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return logger, func() {}, nil
}

// IsService reports whether the process was launched by systemd.
// systemd sets INVOCATION_ID for every service it manages (all Type= variants).
func IsService() (bool, error) {
	return os.Getenv("INVOCATION_ID") != "", nil
}

// RunAsService starts the application, notifies systemd of readiness via
// sd_notify (when NOTIFY_SOCKET is set), then blocks until SIGTERM or SIGINT
// is received before gracefully stopping.
func RunAsService(app Starter, logger *slog.Logger) error {
	if err := app.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	sdNotify("READY=1")
	logger.Info("service is running")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	logger.Info("stop signal received", "signal", sig)

	sdNotify("STOPPING=1")
	app.Stop()
	return nil
}

// HandleSCMFlags is a no-op on Linux. systemd services are managed via
// systemctl and .service unit files, not CLI sub-commands.
func HandleSCMFlags(install, uninstall, start, stop bool) (bool, error) {
	return false, nil
}

// sdNotify sends a notification to systemd via NOTIFY_SOCKET. It is a no-op
// when the socket is not set (Type=simple units or non-systemd environments).
func sdNotify(state string) {
	socket := os.Getenv("NOTIFY_SOCKET")
	if socket == "" {
		return
	}
	conn, err := net.Dial("unixgram", socket)
	if err != nil {
		return
	}
	defer conn.Close()
	_, _ = conn.Write([]byte(state))
}
