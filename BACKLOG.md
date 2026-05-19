# pissbot — backlog

Possible future work. Items here have been consciously deferred, not forgotten.
Each entry states what it is, why it makes sense, the current tradeoff, and the
tradeoffs a future implementation would have to navigate.

---

## Platform-conditional CLI flags

**What.** Split flag registration across build-tag files (`flags_windows.go`,
`flags_other.go`) so that `pissbot -help` only surfaces flags that are
meaningful on the current platform. A Linux user would not see `-install`,
`-start`, `-stop`, `-uninstall`, or `-log`; a Windows user would not see notes
saying flags have no effect.

**Why it makes sense.** Users configure the bot on the platform they are
running. Seeing flags that silently do nothing — or descriptions that mention
another OS — is noise that erodes trust in the tool.

**Current tradeoff.** All flags are registered unconditionally in `main.go`.
The descriptions note "Windows only" / "no effect on Linux" as a textual
mitigation. This keeps the code simple at the cost of a cluttered `-help`
output on both platforms.

**Implementation tradeoffs.** Requires splitting the flag block across
platform-specific files (build tags), or doing a runtime `runtime.GOOS` check.
Either approach adds indirection. Flag *values* that are used in shared code
(e.g. `*flagSettings`) need to be accessible regardless of where they are
registered, which means either package-level vars or a small struct passed
between the platform file and `main`.

---

## Windows Event Log integration

**What.** Write lifecycle and error events (service started, service stopped,
fatal errors) to the Windows Event Log in addition to the existing file log.
The Event Log source is already registered during `-install`
(`eventlog.InstallAsEventCreate` in `winsvc.go`) but nothing currently writes
to it.

**Why it makes sense.** Windows administrators expect to find service events in
Event Viewer. It is the system-level audit surface for Windows services, has
built-in retention management, and survives service crashes (writes are
synchronous). The infrastructure is already half-wired.

**Current tradeoff.** The service is invisible to Event Viewer during normal
operation. An admin investigating a problem would have to know to look at the
log file in `%ProgramData%\pissbot\` rather than the standard tool.

**Implementation tradeoffs.** The Event Log API (`eventlog.Info(eid, msg)`,
`.Error(eid, msg)`) is not an `io.Writer`, so `slog.NewTextHandler` cannot
target it directly. A custom `slog.Handler` is needed (~60–80 lines to
implement correctly, including `WithAttrs` and `WithGroup`). The natural
end-state is a fan-out handler that mirrors to both Event Log (lifecycle/errors
only, to avoid polluting the log with every `!piss` hit) and the file (verbose
operational detail). That fan-out handler is an additional moving part to
maintain. The design should be additive — Event Log as a second destination,
not a replacement for the file log.

---

## Testing

**What.** A test suite following Go conventions: `*_test.go` files alongside
the packages they cover, table-driven tests, no external test framework.

**Why it makes sense.** Go's testing toolchain is part of the standard
distribution (`go test ./...`, `-race`, `-cover`) and requires no setup. The
codebase has clear seams that are already testable:

- `ipservice` — pure HTTP logic; `net/http/httptest.NewServer` provides a
  local mock endpoint with no mocking framework needed.
- `bot` — `IPGetter` is already an interface, so a lightweight test double
  can be substituted for the real IP service without any extra infrastructure.
- `winsvc` — `RunService` accepts `isDebug=true` to run the service handler
  in-process without a real SCM; install/uninstall/start/stop require a live
  Windows SCM and are integration-test territory.
- `platform` — `IsService` and `ServiceLogger` are thin wrappers; smoke tests
  confirm the right paths are chosen.

**Current tradeoff.** Correctness is verified only by running the bot. A
refactor or dependency update can silently break IP parsing, command matching,
or log rotation with no automated signal.

**Implementation tradeoffs.** Test coverage for the Discord gateway interaction
(`bot.Open`, `bot.Close`, message dispatch) is impractical without a real
Discord connection or a significant mock of the `discordgo` session — that
boundary is best left to manual smoke testing. Windows SCM tests require
elevation and a real service manager; they belong in a separate integration-test
binary or are accepted as manual-only. The practical target is full coverage of
`ipservice` and meaningful coverage of `bot` message handling logic.

---

## CI and build artefacts

**What.** A GitHub Actions workflow that builds, tests, and — on a tagged
release — publishes signed binaries for Windows (amd64) and Linux (amd64,
arm64). [goreleaser](https://goreleaser.com) is the idiomatic Go tool for the
release step: it handles cross-compilation, archive packaging, checksum files,
and GitHub Release creation from a single config file.

**Why it makes sense.** Go's cross-compilation story is unusually clean
(`GOOS`/`GOARCH` env vars, no C toolchain needed for this project), so
producing both platform binaries from a single Linux CI runner is trivial.
goreleaser integrates naturally with the existing `-ldflags="-X main.version=..."`
version injection — it substitutes the git tag automatically. Without CI,
releases are manual, untested builds from a developer machine.

**Current tradeoff.** No automated test or build signal on push. Release
binaries are produced manually. The `-ldflags` version string must be set by
hand and can be forgotten, leaving binaries reporting `dev`.

**Implementation tradeoffs.** goreleaser adds a `.goreleaser.yaml` config file
and a dependency on the goreleaser tool in the CI environment (available as a
standard GitHub Action). The workflow needs a `GITHUB_TOKEN` secret for
publishing releases — present by default in GitHub Actions, no extra setup.
Code signing for the Windows binary (Authenticode) is a separate concern that
requires a paid certificate; it is worth knowing it exists but is out of scope
for a personal-use bot.
