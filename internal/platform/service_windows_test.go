//go:build windows

// Tests in this file are Windows-only and will not run on Linux or other
// platforms. Run them on a Windows machine or add a Windows runner to CI.
// They cover log rotation and the Windows file logger; SCM integration
// (IsService, RunAsService, HandleSCMFlags) requires Administrator privileges
// and a live SCM, so those remain untested here.
package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── rotateLogs ────────────────────────────────────────────────────────────────

func TestRotateLogs(t *testing.T) {
	t.Run("no-op when log absent", func(t *testing.T) {
		dir := t.TempDir()
		rotateLogs(filepath.Join(dir, "pissbot.log")) // must not panic
	})

	t.Run("no-op when below threshold", func(t *testing.T) {
		dir := t.TempDir()
		logPath := filepath.Join(dir, "pissbot.log")
		if err := os.WriteFile(logPath, []byte("small"), 0644); err != nil {
			t.Fatal(err)
		}
		rotateLogs(logPath)
		if _, err := os.Stat(logPath); err != nil {
			t.Error("log file should remain when below threshold")
		}
	})

	t.Run("rotates oversized log", func(t *testing.T) {
		dir := t.TempDir()
		logPath := filepath.Join(dir, "pissbot.log")
		f, err := os.Create(logPath)
		if err != nil {
			t.Fatal(err)
		}
		// Truncate to just over the threshold; NTFS allocates lazily so this
		// is fast and uses minimal real disk I/O.
		if err := f.Truncate(maxLogBytes + 1); err != nil {
			t.Fatal(err)
		}
		f.Close()

		rotateLogs(logPath)

		if _, err := os.Stat(logPath); err == nil {
			t.Error("original log should have been renamed away")
		}
		if _, err := os.Stat(logPath + ".1"); err != nil {
			t.Error("backup log.1 should exist after rotation")
		}
	})

	t.Run("overwrites existing backup on second rotation", func(t *testing.T) {
		// Go's os.Rename on Windows uses MoveFileExW with MOVEFILE_REPLACE_EXISTING,
		// so rotation succeeds even when a .1 backup already exists. A prior
		// review flagged this as a potential bug; this test pins the verified
		// behaviour so any future regression is caught immediately.
		dir := t.TempDir()
		logPath := filepath.Join(dir, "pissbot.log")

		if err := os.WriteFile(logPath+".1", []byte("previous backup"), 0644); err != nil {
			t.Fatal(err)
		}

		f, err := os.Create(logPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(maxLogBytes + 1); err != nil {
			t.Fatal(err)
		}
		f.Close()

		rotateLogs(logPath)

		if _, err := os.Stat(logPath); err == nil {
			t.Error("original log should have been renamed away even when backup already exists")
		}
	})
}

// ── defaultLogPath ────────────────────────────────────────────────────────────

func TestDefaultLogPath(t *testing.T) {
	t.Run("uses ProgramData env var", func(t *testing.T) {
		t.Setenv("ProgramData", `C:\custom`)
		got := defaultLogPath()
		if !strings.HasPrefix(got, `C:\custom`) {
			t.Errorf("got %q, want path under C:\\custom", got)
		}
		if !strings.HasSuffix(got, "pissbot.log") {
			t.Errorf("got %q, want path ending in pissbot.log", got)
		}
	})

	t.Run("falls back to C:\\ProgramData when env unset", func(t *testing.T) {
		t.Setenv("ProgramData", "")
		got := defaultLogPath()
		if !strings.HasPrefix(got, `C:\ProgramData`) {
			t.Errorf("got %q, want path under C:\\ProgramData", got)
		}
	})
}

// ── ServiceLogger ─────────────────────────────────────────────────────────────

func TestServiceLogger(t *testing.T) {
	t.Run("creates directory and opens log", func(t *testing.T) {
		dir := t.TempDir()
		logPath := filepath.Join(dir, "subdir", "pissbot.log")

		logger, closeLog, err := ServiceLogger(logPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer closeLog()

		if logger == nil {
			t.Error("logger should not be nil")
		}
		if _, err := os.Stat(logPath); err != nil {
			t.Errorf("log file should exist: %v", err)
		}
	})
}
