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

- `ipservice` — **done.** `internal/ipservice/ipservice_test.go` covers
  `LoadSettings` (7 subtests: valid config, zero defaults, empty/missing
  sources, invalid regex, invalid JSON, file not found) and `GetPublicIP`
  (8 subtests: happy path, fallback, all-fail, round-robin distribution, empty
  body, regex extraction, regex no-match, byte-limit truncation). Uses
  `net/http/httptest.NewServer` — no mocking framework, no network calls.
- `bot` — **done.** `internal/bot/bot_test.go` covers `handlePiss` via the
  `messageSender` interface (added alongside tests): formatted reply, scheme
  stripping from display text, correct channel targeting, message reference,
  no-ping behaviour, and error reply. White-box (`package bot`) to reach the
  unexported handler directly.
- `winsvc` — `RunService` accepts `isDebug=true` to run the service handler
  in-process without a real SCM; install/uninstall/start/stop require a live
  Windows SCM and are integration-test territory.
- `platform` — `IsService` and `ServiceLogger` are thin wrappers; smoke tests
  confirm the right paths are chosen.

**Current tradeoff.** `ipservice` and `bot` message handling are covered.
`onMessage` filtering (bot-self skip, case-insensitive match) and `platform`
path selection are verified only by running the bot — the former relies on
`discordgo.Session.State` which requires a live gateway connection to
initialise, making it impractical to unit-test.

**Implementation tradeoffs.** Test coverage for the Discord gateway interaction
(`bot.Open`, `bot.Close`, the `onMessage` dispatch chain) is impractical
without a real Discord connection or a significant mock of the `discordgo`
session — that boundary is best left to manual smoke testing. Windows SCM tests
require elevation and a real service manager; they belong in a separate
integration-test binary or are accepted as manual-only.

