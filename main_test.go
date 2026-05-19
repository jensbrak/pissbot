package main

import (
	"strings"
	"testing"
)

func TestResolveSettingsPath(t *testing.T) {
	t.Run("returns explicit path unchanged", func(t *testing.T) {
		const explicit = "/some/absolute/path/settings.json"
		if got := resolveSettingsPath(explicit); got != explicit {
			t.Errorf("got %q, want %q", got, explicit)
		}
	})

	t.Run("default resolves relative to executable directory", func(t *testing.T) {
		got := resolveSettingsPath("")
		if !strings.HasSuffix(got, "settings.json") {
			t.Errorf("got %q, expected path ending in settings.json", got)
		}
		// The result must not be a bare filename — it should be an absolute or
		// at least directory-qualified path derived from os.Executable(), not
		// the last-resort fallback.
		if got == "settings.json" {
			t.Error("resolveSettingsPath returned bare filename fallback; os.Executable() may have failed")
		}
	})
}
