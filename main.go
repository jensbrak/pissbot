// pissbot — Public IP Server Service
//
// A Discord bot that replies to !piss with the machine's current public IP
// address. It can run interactively as a console app or be installed as a
// native Windows service for unattended 24/7 operation.
//
// Usage:
//
//	pissbot                  # console mode (Ctrl+C to stop)
//	pissbot -install         # install as Windows service (requires elevation)
//	pissbot -start           # start the installed service
//	pissbot -stop            # stop the running service
//	pissbot -uninstall       # remove the service (requires elevation)
//	pissbot -settings <path> # override the settings.json location
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/jensbrak/pissbot/internal/bot"
	"github.com/jensbrak/pissbot/internal/ipservice"
	"github.com/jensbrak/pissbot/internal/platform"
)

// ─── App ─────────────────────────────────────────────────────────────────────

// App owns the application lifecycle and satisfies platform.Starter so it can
// be driven by either the platform service manager or a plain OS signal in
// console mode.
type App struct {
	bot    *bot.Bot
	logger *slog.Logger
}

// newApp reads configuration and wires all dependencies together.
func newApp(settingsPath string, logger *slog.Logger) (*App, error) {
	token := os.Getenv("DiscordToken")
	if token == "" {
		return nil, fmt.Errorf("environment variable DiscordToken is not set")
	}

	settings, err := ipservice.LoadSettings(settingsPath)
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}
	logger.Info("settings loaded",
		"path", settingsPath,
		"sources", len(settings.IPSources),
		"timeout_sec", settings.RequestTimeoutSeconds,
	)

	ipSvc, err := ipservice.New(settings, logger)
	if err != nil {
		return nil, fmt.Errorf("create IP service: %w", err)
	}

	b, err := bot.New(token, ipSvc, logger)
	if err != nil {
		return nil, fmt.Errorf("create bot: %w", err)
	}

	return &App{bot: b, logger: logger}, nil
}

// Start connects the bot to the Discord gateway. Implements platform.Starter.
func (a *App) Start() error {
	return a.bot.Open()
}

// Stop disconnects the bot from the Discord gateway. Implements platform.Starter.
func (a *App) Stop() {
	if err := a.bot.Close(); err != nil {
		a.logger.Error("error during shutdown", "error", err)
	}
}

// ─── Entry point ─────────────────────────────────────────────────────────────

func main() {
	var (
		flagInstall   = flag.Bool("install", false, "install as a Windows service (requires elevation)")
		flagUninstall = flag.Bool("uninstall", false, "uninstall the Windows service (requires elevation)")
		flagStart     = flag.Bool("start", false, "start the Windows service")
		flagStop      = flag.Bool("stop", false, "stop the Windows service")
		flagSettings  = flag.String("settings", "", "path to settings.json (default: <exe directory>/settings.json)")
	)
	flag.Parse()

	settingsPath := resolveSettingsPath(*flagSettings)

	// Handle platform service management commands (no-op on non-Windows).
	handled, err := platform.HandleSCMFlags(*flagInstall, *flagUninstall, *flagStart, *flagStop)
	if err != nil {
		fatalf("%v", err)
	}
	if handled {
		return
	}

	// Detect whether we were launched by the platform service manager.
	inService, err := platform.IsService()
	if err != nil {
		fatalf("detect service context: %v", err)
	}

	if inService {
		runAsService(settingsPath)
	} else {
		runAsConsole(settingsPath)
	}
}

// ─── Run modes ────────────────────────────────────────────────────────────────

func runAsConsole(settingsPath string) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	logger.Info("starting in console mode")

	app, err := newApp(settingsPath, logger)
	if err != nil {
		logger.Error("startup failed", "error", err)
		os.Exit(1)
	}

	if err := app.Start(); err != nil {
		logger.Error("failed to connect to Discord", "error", err)
		os.Exit(1)
	}
	logger.Info("bot is running — press Ctrl+C to stop")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down…")
	app.Stop()
	logger.Info("done")
}

func runAsService(settingsPath string) {
	// When running under a service manager there is no console. Log to a file
	// in the same directory as the executable so output is always findable.
	exePath, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	logPath := filepath.Join(filepath.Dir(exePath), "pissbot.log")

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		// Without a log file we cannot report the error anywhere useful.
		os.Exit(1)
	}
	defer logFile.Close()

	logger := slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	logger.Info("service process started", "settings", settingsPath, "log", logPath)

	app, err := newApp(settingsPath, logger)
	if err != nil {
		logger.Error("startup failed", "error", err)
		os.Exit(1)
	}

	if err := platform.RunAsService(app, logger); err != nil {
		logger.Error("service error", "error", err)
		os.Exit(1)
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// resolveSettingsPath returns the explicit path if provided, otherwise looks
// for settings.json in the same directory as the running executable. Using
// the exe directory (rather than the working directory) ensures the correct
// file is found whether the bot is run from a shortcut, the service manager,
// or a shell.
func resolveSettingsPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	exePath, err := os.Executable()
	if err != nil {
		return "settings.json" // last-resort fallback to CWD
	}
	return filepath.Join(filepath.Dir(exePath), "settings.json")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ERROR: "+format+"\n", args...)
	os.Exit(1)
}
