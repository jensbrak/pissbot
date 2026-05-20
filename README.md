# pissbot — Public IP Server Service

A Discord bot that responds to `!piss` with the machine's current public IP
address. Runs interactively in a console or as a native service that starts
automatically on boot — Windows SCM and Linux systemd are both supported.

pissbot is useful whenever you host services at home on a dynamic IP and need
a way to share the current address with users. The simplest case is skipping
DNS entirely — just ask the bot. It also bridges the gap when DNS is in place
but hasn't caught up to a recent IP change. Either way, pissbot must run on
the same network as the service — it reports the public IP of the machine it's
running on.

---

## Getting started

### 1. Download

Download the latest binary for your platform from the
[Releases](https://github.com/jensbrak/pissbot/releases) page and extract the
archive.

| Platform | Archive | Binary |
|---|---|---|
| Windows (x86-64) | `pissbot_<version>_windows_amd64.zip` | `pissbot.exe` |
| Linux (x86-64) | `pissbot_<version>_linux_amd64.tar.gz` | `pissbot` |
| Linux (ARM64 / Raspberry Pi 3+) | `pissbot_<version>_linux_arm64.tar.gz` | `pissbot` |

### 2. Configure

Copy `settings.example.json` (included in the archive) to `settings.json` and
place it next to the executable. Edit to taste — see
[Configuration](#configuration--settingsjson) for all options.

### 3. Discord bot

You need a Discord bot with:

- **Message Content Intent** enabled (Bot → Privileged Gateway Intents)
- **Permissions:** Send Messages, Read Message History

Set the bot token as the `DiscordToken` environment variable before running.

### 4. Run

**Windows (PowerShell):**

```powershell
$env:DiscordToken = "your-token-here"
.\pissbot.exe
```

**Linux:**

```bash
export DiscordToken="your-token-here"
./pissbot
```

Press **Ctrl+C** to stop. To have the bot start automatically on boot, see
[Running as a Windows service](#running-as-a-windows-service) or
[Running as a Linux service](#running-as-a-linux-service).

---

## Configuration — `settings.json`

Place `settings.json` next to the executable, or pass `-settings <path>` to
override the location.

```json
{
  "ip_sources": [
    "https://api.ipify.org",
    "https://ifconfig.me/ip",
    "https://icanhazip.com",
    "https://api4.my-ip.io/ip",
    "https://checkip.amazonaws.com"
  ],
  "request_timeout_seconds": 5,
  "response_max_bytes": 256,
  "response_regex": "\\b(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\\b"
}
```

| Field | Default | Description |
|---|---|---|
| `ip_sources` | — | List of IP echo endpoints. At least one required. Sources are tried in round-robin order; if one fails the next is tried automatically. |
| `request_timeout_seconds` | `5` | Per-request HTTP timeout in seconds. |
| `response_max_bytes` | `256` | Maximum bytes read from each source response. Prevents reading full HTML pages from misbehaving endpoints. |
| `response_regex` | `""` | Optional regex applied to the response body; the first match is used as the IP. When empty, the trimmed response body is used as-is. The value above extracts an IPv4 address and is recommended for sources that embed the IP in surrounding text. |

Settings are read at startup. Restart the bot to pick up changes.

---

## Running as a Windows service

The service starts automatically on boot and runs under the SYSTEM account.
All management commands require an **elevated (Administrator) prompt**.

### 1. Set the environment variable at the system level

Because the SYSTEM account does not inherit user environment variables,
`DiscordToken` must be a **System** variable:

```powershell
# Run as Administrator:
[Environment]::SetEnvironmentVariable("DiscordToken", "your-token-here", "Machine")
```

Verify:

```powershell
[Environment]::GetEnvironmentVariable("DiscordToken", "Machine")
```

> **Note:** changes to system environment variables are not picked up by
> already-running processes. The service reads the variable on startup.

### 2. Install

Copy `pissbot.exe` and `settings.json` to their permanent location
(e.g. `C:\Services\pissbot\`) *before* installing — the SCM stores the exe
path at install time.

```powershell
# Run as Administrator from the directory containing pissbot.exe:
.\pissbot.exe -install
```

Logs are written to `%ProgramData%\pissbot\pissbot.log` by default
(e.g. `C:\ProgramData\pissbot\pissbot.log`). The directory is created
automatically on first start. Any flags passed at install time are baked
into the service registration and used on every subsequent start:

```powershell
# Custom settings and log paths — baked in permanently at install time:
.\pissbot.exe -install -settings D:\conf\settings.json -log D:\logs\pissbot.log
```

### 3. Start / stop

```powershell
.\pissbot.exe -start
.\pissbot.exe -stop

# The built-in net commands also work:
net start PissBot
net stop PissBot
```

### 4. Uninstall

Stop the service first, then uninstall:

```powershell
.\pissbot.exe -stop
.\pissbot.exe -uninstall
```

---

## Running as a Linux service

pissbot integrates with systemd using `sd_notify` (`Type=notify`), so systemd
waits for a confirmed Discord connection before considering the service started.

### 1. Create a dedicated user

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin pissbot
```

### 2. Install the binary and configuration

```bash
sudo install -o root -g root -m 755 pissbot /usr/local/bin/pissbot

sudo mkdir -p /etc/pissbot
sudo install -o root -g pissbot -m 640 settings.json /etc/pissbot/settings.json
```

### 3. Create the token file

```bash
sudo install -o root -g pissbot -m 640 /dev/null /etc/pissbot/env
echo "DiscordToken=your-token-here" | sudo tee /etc/pissbot/env > /dev/null
```

> **Security:** `/etc/pissbot/env` is readable only by root and the `pissbot`
> group. Do not store the token in the unit file — it would be visible in
> `systemctl show`.

### 4. Create the unit file

```bash
sudo tee /etc/systemd/system/pissbot.service > /dev/null << 'EOF'
[Unit]
Description=pissbot — Public IP Server Service
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
ExecStart=/usr/local/bin/pissbot -settings /etc/pissbot/settings.json
EnvironmentFile=/etc/pissbot/env
Restart=on-failure
RestartSec=5s
User=pissbot
Group=pissbot
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF
```

### 5. Enable and start

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now pissbot
```

### 6. Uninstall

```bash
sudo systemctl disable --now pissbot
sudo rm /etc/systemd/system/pissbot.service
sudo systemctl daemon-reload
sudo rm /usr/local/bin/pissbot
sudo rm -rf /etc/pissbot
sudo userdel pissbot
```

---

## Logs

| Context | Destination | Rotation |
|---|---|---|
| Windows service | `%ProgramData%\pissbot\pissbot.log` (override with `-log <path>`) | Renamed to `.log.1` on startup when > 10 MiB (one backup kept) |
| Linux service | stdout → journald | Managed by journald (`SystemMaxUse` in `journald.conf`) |
| Console (both) | stdout | — |

On Linux, use `journalctl -u pissbot` to view logs.

---

## Updating

**Windows:**

```powershell
.\pissbot.exe -stop
# Replace pissbot.exe with the new build.
.\pissbot.exe -start
```

The service registration does not need to be re-done unless the exe path changes.

**Linux:**

```bash
sudo systemctl stop pissbot
sudo install -o root -g root -m 755 pissbot /usr/local/bin/pissbot
sudo systemctl start pissbot
```

---

## Building from source

### Prerequisites

| Requirement | Notes |
|---|---|
| Go 1.21+ | <https://go.dev/dl/> |
| Windows or Linux | Windows SCM and Linux systemd both supported |

### Build

Builds are managed with [Task](https://taskfile.dev). Install it once:

```bash
go install github.com/go-task/task/v3/cmd/task@latest
```

> **Linux:** `go install` places binaries in `~/go/bin/`, which may not be in
> your `PATH`. Some distributions also ship an unrelated package named `task`
> — if the command exists but behaves unexpectedly, that's likely the cause.
> Either run `~/go/bin/task` directly, add `~/go/bin` to your `PATH`, or see
> [taskfile.dev](https://taskfile.dev/installation/) for alternative install
> methods.

| Command | What it does |
|---|---|
| `task build` | Fast dev build for the current platform (`-version` reports `dev`) |
| `task build:windows` | Cross-compiled Windows release binary → `dist/pissbot.exe` |
| `task build:linux` | Cross-compiled Linux release binary → `dist/pissbot` |
| `task release` | Both platform release binaries (tag first — see *Releasing* below) |
| `task test` | Run the test suite (`go test ./...`) |
| `task check` | `go vet` + `go test` (vet then test) |
| `task tidy` | `go mod tidy` |

Run `task` with no arguments to list all available tasks.

Release builds inject the version from the nearest git tag via
`git describe --tags --always --dirty`, so the binary reports `v1.0.0`,
`v1.0.0-3-gabcd123`, or `v1.0.0-3-gabcd123-dirty` depending on whether the
working tree is clean and on a tagged commit.

**First-time setup:**

```bash
go mod tidy   # fetch dependencies
task build    # confirm the toolchain works
```

### Releasing

Releases use [semantic versioning](https://semver.org) with a `v` prefix
(`v1.2.3`).

1. Ensure the working tree is clean and all changes are committed.
2. Create a tag and push it — the release workflow runs automatically and
   publishes binaries to the GitHub Release page:

   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

   To build locally without CI, `task release` produces the same binaries
   in `dist/`.

---

## VS Code

The repo includes a `.vscode/` directory with workspace configuration.
VS Code will prompt to install the recommended extensions on first open.

| Extension | Purpose |
|---|---|
| `golang.go` | Go language support — IntelliSense, navigation, debugging, testing |
| `redhat.vscode-yaml` | YAML support for `Taskfile.yml` |

**Format on save** is enabled using `goimports`, which formats code and
automatically manages import statements on every save.

**Debugging** — two launch configurations are provided:

- *pissbot (console)* — runs the bot in console mode. `DiscordToken` is
  inherited from the environment VS Code was launched from, and `-settings`
  points at `settings.json` in the workspace root. Ensure both exist before
  starting a debug session.
- *test (current package)* — debugs the tests in whichever package the
  active file belongs to.

**Note on build tag warnings** — `gopls` analyses only the files that apply
to the current platform. On Windows, `service_linux.go` will show errors;
on Linux, `service_windows.go` will. This is expected and does not affect
the build.

---

## Project layout

```
pissbot/
├── main.go                          # entry point, App lifecycle, CLI flags
├── go.mod
├── Taskfile.yml                     # build, test, and release tasks
├── .vscode/                         # VS Code workspace config (extensions, settings, debug)
├── settings.example.json            # template — copy to settings.json and edit
├── LICENSE
├── internal/
│   ├── bot/
│   │   └── bot.go                   # Discord session, !piss handler
│   ├── ipservice/
│   │   └── ipservice.go             # round-robin IP fetching with regex extraction
│   ├── platform/
│   │   ├── platform.go              # Starter interface (shared by all platforms)
│   │   ├── service_windows.go       # Windows SCM integration, file logging with rotation
│   │   ├── service_linux.go         # systemd sd_notify integration, stdout logging
│   │   └── service_other.go         # no-op stubs for other platforms
│   └── winsvc/
│       └── winsvc.go                # Windows SCM low-level helpers (install/start/stop)
└── README.md
```

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/bwmarrin/discordgo` | Discord WebSocket gateway & REST client |
| `golang.org/x/sys` | Windows SCM bindings (Windows builds only at runtime) |

The standard library covers everything else: HTTP client, JSON, structured
logging (`log/slog`), signal handling, sd_notify via Unix datagram sockets.
No logging framework, no DI container, no config library.
