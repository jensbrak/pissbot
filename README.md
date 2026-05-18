# pissbot — Public IP Server Service

A Discord bot that responds to `!piss` with the machine's current public IP
address. It supports running interactively or as a native Windows service so
it starts automatically on boot.

---

## Prerequisites

| Requirement | Notes |
|---|---|
| Go 1.21+ | <https://go.dev/dl/> |
| Windows 11 (build target) | Can be cross-compiled from any OS |
| A Discord bot token | See *Discord setup* below |

---

## Discord setup

### 1. Create the application and bot

1. Go to <https://discord.com/developers/applications> and click **New Application**.
2. Name it (e.g. *pissbot*), then navigate to **Bot** in the left sidebar.
3. Click **Add Bot** → confirm.
4. Under **Token**, click **Reset Token** and copy the value. This is your `DiscordToken`.

### 2. Enable the Message Content intent (required)

Still on the **Bot** page, scroll to **Privileged Gateway Intents** and enable:

- ✅ **Message Content Intent**

Without this the bot receives message events but cannot read the text body,
so `!piss` will never match.

### 3. Invite the bot to your server

Go to **OAuth2 → URL Generator**. Select:

- Scopes: `bot`
- Bot permissions: `Send Messages`, `Read Message History`

Open the generated URL in a browser and choose your server.

---

## Configuration — `settings.json`

Copy `settings.example.json` to `settings.json` and edit to taste.
Place `settings.json` in the same directory as the executable (or pass
`-settings <path>` to override).

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

## Build

```powershell
# From the project root (works on any OS via cross-compilation):
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o pissbot.exe .
```

On Windows natively:
```powershell
go mod tidy                          # first time: fetch dependencies
go build -ldflags="-s -w" -o pissbot.exe .
```

`-ldflags="-s -w"` strips debug info, reducing the binary size by ~30%.

---

## Running in console mode

Useful for testing. Set `DiscordToken` in your current session, then:

```powershell
$env:DiscordToken = "your-token-here"
.\pissbot.exe
```

Press **Ctrl+C** to stop.

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
(e.g. `C:\Services\pissbot\`) *before* installing, because the SCM stores
the exe path at install time.

```powershell
# Run as Administrator from the directory containing pissbot.exe:
.\pissbot.exe -install
```

### 3. Start / stop

```powershell
.\pissbot.exe -start
.\pissbot.exe -stop

# Alternatively, the built-in net commands work too:
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

## Log file

When running as a service, logs are written to `pissbot.log` in the same
directory as the executable. The file is appended to on each start and uses
a human-readable structured text format:

```
time=2024-05-01T12:00:00.000Z level=INFO msg="service process started" settings=C:\Services\pissbot\settings.json
time=2024-05-01T12:00:01.000Z level=INFO msg="connected to Discord" user=pissbot#1234 guilds=1 session_id=...
time=2024-05-01T12:05:22.000Z level=INFO msg="!piss received" user=Alice#0001 channel=123... guild=456...
time=2024-05-01T12:05:22.000Z level=INFO msg="serving public IP" ip=203.0.113.42 source=https://api.ipify.org
```

The log file is not rotated automatically. For long-running deployments,
periodically archive or truncate it.

---

## Updating

1. Stop the service: `.\pissbot.exe -stop`
2. Replace `pissbot.exe` with the new build.
3. Start the service: `.\pissbot.exe -start`

The service registration does not need to be re-done unless the exe path changes.

---

## Project layout

```
pissbot/
├── main.go                      # entry point, App lifecycle, CLI flags
├── go.mod
├── settings.example.json        # template — copy to settings.json and edit
├── LICENSE
├── internal/
│   ├── bot/
│   │   └── bot.go               # Discord session, !piss handler
│   ├── ipservice/
│   │   └── ipservice.go         # round-robin IP fetching
│   └── winsvc/
│       └── winsvc.go            # Windows SCM integration
└── README.md
```

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/bwmarrin/discordgo` | Discord WebSocket gateway & REST client |
| `golang.org/x/sys` | Windows Service Control Manager bindings |

The standard library covers everything else (HTTP client, JSON, logging,
signal handling, atomic counters). No logging framework, no DI container,
no config library.
