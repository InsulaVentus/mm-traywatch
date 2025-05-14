# mm‑traywatch
A tiny menu‑bar for Mattermost written in Go + [Fyne](https://github.com/fyne-io/fyne) that shows a color‑coded dot indicating status:

Dot	Means
🔴red	Unread direct message and/or @‑mention
🔵blue	Unread posts
blank	No unread

# Installation
> Requires Go (to build)
```bash
git clone git@github.com:InsulaVentus/mm-traywatch.git
cd mm-traywatch
go build # creates ./mm-traywatch
```

# Configuration
## 1. Create a personal access token (PAT)
**_NOTE:_** A System Admin might need to give your account permission to create an access token.
See this howto: https://developers.mattermost.com/integrate/reference/personal-access-token/#create-a-personal-access-token

## 2. Create a `config.yaml`
```yaml
# macOS ~/Library/Application Support/mm-traywatch/config.yaml

host:  chat.example.com       # without https://
pat:   xxxxxxxxxxxxxxxxxxxxx
theme: dark                   # light | dark
```

## 3. First run
```bash
./mm-traywatch
```
The menu icon should now be visible in the menu bar.

## Environment overrides
All config values can be overridden by environment variable(s)

| Var                   | Purpose                            |
|-----------------------|------------------------------------|
| `MM_TRAYWATCH_PAT`    | Overrides pat                      |
| `MM_TRAYWATCH_HOST`   | Overrides host                     |
| `MM_TRAYWATCH_THEME`  | `light` or `dark`                  |
| `MM_TRAYWATCH_CONFIG` | Full path to alternate config.yaml |


# Running as a login item (macOS)
## 1. Create a `launchd` property list (`.plist`) file
Save as `~/Library/LaunchAgents/com.mm-traywatch.plist`
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
    <key>Label</key>
    <string>com.mm-traywatch</string>
    
    <!-- Full path to the binary you built -->
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/mm-traywatch</string>
    </array>

    <key>StandardOutPath</key>
    <string>~/Library/Logs/mm-traywatch.log</string>
    <key>StandardErrorPath</key>
    <string>~/Library/Logs/mm-traywatch.log</string>

    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>LimitLoadToSessionType</key>
    <string>Aqua</string>
</dict>
</plist>
```
_(Adjust the path in <ProgramArguments> to wherever you placed the binary, e.g. /Applications/mm-traywatch.)_

## 2. Load and enable it
```bash
launchctl load  -w  ~/Library/LaunchAgents/com.mm-traywatch.plist
```

## 3. Stop or remove
```bash
launchctl unload -w ~/Library/LaunchAgents/com.mm-traywatch.plist
```
