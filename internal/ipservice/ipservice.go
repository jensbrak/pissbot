// Package ipservice resolves the machine's public IP from a rotating list of
// external sources. On each call the next source in the list is tried first;
// if it fails the remaining sources are attempted in order so that a single
// bad endpoint never blocks a response.
package ipservice

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// Settings is the structure of settings.json.
type Settings struct {
	IPSources             []string `json:"ip_sources"`
	RequestTimeoutSeconds int      `json:"request_timeout_seconds"`
	// ResponseRegex is an optional regular expression applied to the raw
	// response body. When set, the first match is used as the IP address,
	// allowing sources that wrap the IP in surrounding text (e.g. Loopia's
	// "Current IP Address: x.x.x.x"). When empty, the trimmed body is used
	// as-is, which is correct for plain-text-only endpoints.
	ResponseRegex string `json:"response_regex"`
	// ResponseMaxBytes caps how many bytes are read from each source response.
	// Defaults to 256, which is ample for any terse IP echo service while
	// rejecting full HTML pages outright.
	ResponseMaxBytes int64 `json:"response_max_bytes"`
}

// LoadSettings reads and validates the settings file at path.
// It returns an error if response_regex is set but does not compile.
func LoadSettings(path string) (*Settings, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	var s Settings
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, fmt.Errorf("parse %q: %w", path, err)
	}
	if len(s.IPSources) == 0 {
		return nil, fmt.Errorf("%q: ip_sources must not be empty", path)
	}
	if s.RequestTimeoutSeconds <= 0 {
		s.RequestTimeoutSeconds = 5
	}
	if s.ResponseMaxBytes <= 0 {
		s.ResponseMaxBytes = 256
	}
	if s.ResponseRegex != "" {
		if _, err := regexp.Compile(s.ResponseRegex); err != nil {
			return nil, fmt.Errorf("%q: invalid response_regex: %w", path, err)
		}
	}
	return &s, nil
}

// Service fetches the public IP from a rotating list of external endpoints.
// It is safe for concurrent use.
type Service struct {
	sources  []string
	idx      atomic.Uint64
	client   *http.Client
	logger   *slog.Logger
	regex    *regexp.Regexp // nil when response_regex is not set
	maxBytes int64
}

// New creates a Service from the given settings.
// It returns an error if the response_regex fails to compile (LoadSettings
// already validates this, so an error here indicates a programming mistake).
func New(s *Settings, logger *slog.Logger) (*Service, error) {
	svc := &Service{
		sources:  s.IPSources,
		maxBytes: s.ResponseMaxBytes,
		client: &http.Client{
			Timeout: time.Duration(s.RequestTimeoutSeconds) * time.Second,
			// Disable keep-alives so stale connections to IP sources don't
			// cause false failures during long uptimes.
			Transport: &http.Transport{
				DisableKeepAlives: true,
			},
		},
		logger: logger,
	}
	if s.ResponseRegex != "" {
		re, err := regexp.Compile(s.ResponseRegex)
		if err != nil {
			return nil, fmt.Errorf("compile response_regex: %w", err)
		}
		svc.regex = re
		logger.Info("response regex active", "pattern", s.ResponseRegex)
	}
	return svc, nil
}

// GetPublicIP returns the machine's public IP address and the URL of the
// source that provided it.
//
// Each call atomically advances the round-robin index so that sources are
// used evenly across successive calls. If the selected source fails, the
// remaining sources are tried in order. An error is returned only if every
// source fails.
func (s *Service) GetPublicIP(ctx context.Context) (ip, source string, err error) {
	n := uint64(len(s.sources))

	// Atomically grab a slot. Each concurrent caller gets its own starting
	// position, ensuring true round-robin without any lock.
	start := s.idx.Add(1) - 1

	for i := uint64(0); i < n; i++ {
		source = s.sources[(start+i)%n]

		ip, err = s.fetch(ctx, source)
		if err != nil {
			s.logger.Warn("IP source unavailable", "source", source, "error", err)
			continue
		}

		if i > 0 {
			s.logger.Info("fell back to alternate IP source", "source", source, "attempt", i+1)
		}
		return ip, source, nil
	}

	return "", "", fmt.Errorf("all %d IP sources failed", n)
}

func (s *Service) fetch(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "pissbot/1.0")
	req.Header.Set("Accept", "text/plain")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, s.maxBytes))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	text := strings.TrimSpace(string(body))
	if text == "" {
		return "", fmt.Errorf("empty response body")
	}

	if s.regex != nil {
		match := s.regex.FindString(text)
		if match == "" {
			return "", fmt.Errorf("response_regex found no match in response")
		}
		return match, nil
	}

	return text, nil
}
