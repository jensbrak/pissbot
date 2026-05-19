package ipservice_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jensbrak/pissbot/internal/ipservice"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

func writeSettings(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func newService(t *testing.T, sources []string) *ipservice.Service {
	t.Helper()
	svc, err := ipservice.New(&ipservice.Settings{
		IPSources:             sources,
		RequestTimeoutSeconds: 5,
		ResponseMaxBytes:      256,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func newServiceWithRegex(t *testing.T, sources []string, regex string) *ipservice.Service {
	t.Helper()
	svc, err := ipservice.New(&ipservice.Settings{
		IPSources:             sources,
		RequestTimeoutSeconds: 5,
		ResponseMaxBytes:      256,
		ResponseRegex:         regex,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// ── LoadSettings ──────────────────────────────────────────────────────────────

func TestLoadSettings(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		path := writeSettings(t, `{
			"ip_sources": ["http://a.example.com", "http://b.example.com"],
			"request_timeout_seconds": 3,
			"response_max_bytes": 128
		}`)
		s, err := ipservice.LoadSettings(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(s.IPSources) != 2 {
			t.Errorf("sources: got %d, want 2", len(s.IPSources))
		}
		if s.RequestTimeoutSeconds != 3 {
			t.Errorf("timeout: got %d, want 3", s.RequestTimeoutSeconds)
		}
		if s.ResponseMaxBytes != 128 {
			t.Errorf("max bytes: got %d, want 128", s.ResponseMaxBytes)
		}
	})

	t.Run("zero values default to safe minimums", func(t *testing.T) {
		path := writeSettings(t, `{"ip_sources":["http://example.com"]}`)
		s, err := ipservice.LoadSettings(path)
		if err != nil {
			t.Fatal(err)
		}
		if s.RequestTimeoutSeconds != 5 {
			t.Errorf("timeout: got %d, want 5", s.RequestTimeoutSeconds)
		}
		if s.ResponseMaxBytes != 256 {
			t.Errorf("max bytes: got %d, want 256", s.ResponseMaxBytes)
		}
	})

	t.Run("empty ip_sources", func(t *testing.T) {
		path := writeSettings(t, `{"ip_sources":[]}`)
		if _, err := ipservice.LoadSettings(path); err == nil {
			t.Error("expected error for empty ip_sources")
		}
	})

	t.Run("missing ip_sources", func(t *testing.T) {
		path := writeSettings(t, `{}`)
		if _, err := ipservice.LoadSettings(path); err == nil {
			t.Error("expected error for missing ip_sources")
		}
	})

	t.Run("invalid response_regex", func(t *testing.T) {
		path := writeSettings(t, `{"ip_sources":["http://example.com"],"response_regex":"[invalid"}`)
		if _, err := ipservice.LoadSettings(path); err == nil {
			t.Error("expected error for invalid response_regex")
		}
	})

	t.Run("unknown fields rejected", func(t *testing.T) {
		path := writeSettings(t, `{"ip_sources":["http://a.example.com"],"response_regxe":"\\d+"}`)
		if _, err := ipservice.LoadSettings(path); err == nil {
			t.Error("expected error for unknown field response_regxe")
		}
	})

	t.Run("negative request_timeout_seconds defaults to 5", func(t *testing.T) {
		path := writeSettings(t, `{"ip_sources":["http://example.com"],"request_timeout_seconds":-1}`)
		s, err := ipservice.LoadSettings(path)
		if err != nil {
			t.Fatal(err)
		}
		if s.RequestTimeoutSeconds != 5 {
			t.Errorf("timeout: got %d, want 5", s.RequestTimeoutSeconds)
		}
	})

	t.Run("negative response_max_bytes defaults to 256", func(t *testing.T) {
		path := writeSettings(t, `{"ip_sources":["http://example.com"],"response_max_bytes":-1}`)
		s, err := ipservice.LoadSettings(path)
		if err != nil {
			t.Fatal(err)
		}
		if s.ResponseMaxBytes != 256 {
			t.Errorf("max bytes: got %d, want 256", s.ResponseMaxBytes)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		path := writeSettings(t, `not json`)
		if _, err := ipservice.LoadSettings(path); err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		if _, err := ipservice.LoadSettings(filepath.Join(t.TempDir(), "missing.json")); err == nil {
			t.Error("expected error for missing file")
		}
	})
}

// ── GetPublicIP ───────────────────────────────────────────────────────────────

func TestGetPublicIP(t *testing.T) {
	t.Run("returns IP and source URL", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "1.2.3.4")
		}))
		defer srv.Close()

		svc := newService(t, []string{srv.URL})
		ip, source, err := svc.GetPublicIP(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ip != "1.2.3.4" {
			t.Errorf("ip: got %q, want %q", ip, "1.2.3.4")
		}
		if source != srv.URL {
			t.Errorf("source: got %q, want %q", source, srv.URL)
		}
	})

	t.Run("falls back when first source fails", func(t *testing.T) {
		bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer bad.Close()

		good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "5.6.7.8")
		}))
		defer good.Close()

		svc := newService(t, []string{bad.URL, good.URL})
		ip, _, err := svc.GetPublicIP(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ip != "5.6.7.8" {
			t.Errorf("ip: got %q, want %q", ip, "5.6.7.8")
		}
	})

	t.Run("error when all sources fail", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		svc := newService(t, []string{srv.URL})
		if _, _, err := svc.GetPublicIP(context.Background()); err == nil {
			t.Error("expected error when all sources fail")
		}
	})

	t.Run("round-robin distributes calls evenly", func(t *testing.T) {
		var hits [2]int
		servers := make([]*httptest.Server, 2)
		for i := range servers {
			i := i // capture for closure
			servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits[i]++
				fmt.Fprint(w, "1.1.1.1")
			}))
			defer servers[i].Close()
		}

		svc := newService(t, []string{servers[0].URL, servers[1].URL})
		for i := 0; i < 4; i++ {
			if _, _, err := svc.GetPublicIP(context.Background()); err != nil {
				t.Fatal(err)
			}
		}
		if hits[0] != 2 || hits[1] != 2 {
			t.Errorf("uneven distribution: server[0]=%d server[1]=%d, want 2 each", hits[0], hits[1])
		}
	})

	t.Run("empty response body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Status 200, no body.
		}))
		defer srv.Close()

		svc := newService(t, []string{srv.URL})
		if _, _, err := svc.GetPublicIP(context.Background()); err == nil {
			t.Error("expected error for empty response body")
		}
	})

	t.Run("regex extracts IP from surrounding text", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "Your IP is 9.10.11.12 today")
		}))
		defer srv.Close()

		svc := newServiceWithRegex(t, []string{srv.URL}, `\d+\.\d+\.\d+\.\d+`)
		ip, _, err := svc.GetPublicIP(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ip != "9.10.11.12" {
			t.Errorf("ip: got %q, want %q", ip, "9.10.11.12")
		}
	})

	t.Run("regex with no match returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "no ip address here")
		}))
		defer srv.Close()

		svc := newServiceWithRegex(t, []string{srv.URL}, `\d+\.\d+\.\d+\.\d+`)
		if _, _, err := svc.GetPublicIP(context.Background()); err == nil {
			t.Error("expected error when regex finds no match")
		}
	})

	t.Run("whitespace-only response treated as empty", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "   \n   ")
		}))
		defer srv.Close()

		svc := newService(t, []string{srv.URL})
		if _, _, err := svc.GetPublicIP(context.Background()); err == nil {
			t.Error("expected error for whitespace-only response body")
		}
	})

	t.Run("cancelled context fails immediately", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "1.2.3.4")
		}))
		defer srv.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		svc := newService(t, []string{srv.URL})
		if _, _, err := svc.GetPublicIP(ctx); err == nil {
			t.Error("expected error with already-cancelled context")
		}
	})

	t.Run("malformed source URL returns error", func(t *testing.T) {
		svc := newService(t, []string{"://not-a-url"})
		if _, _, err := svc.GetPublicIP(context.Background()); err == nil {
			t.Error("expected error for malformed source URL")
		}
	})

	t.Run("body truncated at max bytes prevents reading buried IP", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// IP is buried well past the byte limit.
			fmt.Fprintf(w, "%s1.2.3.4", strings.Repeat("x", 300))
		}))
		defer srv.Close()

		svc, err := ipservice.New(&ipservice.Settings{
			IPSources:             []string{srv.URL},
			RequestTimeoutSeconds: 5,
			ResponseMaxBytes:      10,
			ResponseRegex:         `\d+\.\d+\.\d+\.\d+`,
		}, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := svc.GetPublicIP(context.Background()); err == nil {
			t.Error("expected error: IP buried past max bytes should not be found")
		}
	})
}
