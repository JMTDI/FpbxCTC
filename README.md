# FpbxCTC — FreePBX Click-to-Call for Windows ![GitHub Downloads (latest release)](https://img.shields.io/github/downloads/JMTDI/FpbxCTC/latest/total)

A lightweight Windows desktop app **and** browser extension that forward calls to a FreePBX-compatible PBX via its Click-to-Call REST API.

## Components

| Component | Description |
|---|---|
| **`FpbxCTC.exe`** | Windows desktop app — registers as the default `tel:` link handler |
| **`FpbxCTC-Setup.exe`** | Windows installer (auto-registers everything, Start Menu, Add/Remove Programs) |
| **`browser-extension/`** | Chrome / Edge / Brave / Vivaldi MV3 extension — right-click any selected number to dial, with settings sync from the desktop app |
| **`pbx_files/`** | Server-side files to deploy on your FusionPBX server |

---

## How it works

```
User selects / clicks a number
        │
        ├─ tel: link  →  Windows launches FpbxCTC.exe tel:+15551234567
        └─ selected text  →  browser extension right-click → "Call with FpbxCTC"
                │
                ▼
GET https://<domain>/ctc.php?api=1&key=<api_key>&agent=<agent>&dest=<number>
                │
                ▼
        PBX rings agent extension → agent answers → bridges to destination
```

---

## Features

- Native Windows GUI settings window (no browser, no Electron)
- Automatic `tel:` protocol registration on install / cleanup on uninstall
- Browser extension with right-click context menu for any selected phone number
- **Sync from Desktop App** button in extension popup — pulls settings from the desktop app via Native Messaging
- Supports Chrome, Edge, Brave, and Vivaldi
- Instant OS notification on call success or failure
- Config saved to `%APPDATA%\FpbxCTC\config.json` (desktop) and `chrome.storage.sync` (extension)
- Professional installer with Start Menu shortcut, Add/Remove Programs entry, and full uninstall

---

## PBX server setup

Copy the files from `pbx_files/` to your FusionPBX server:

| File | Destination |
|---|---|
| `ctc.php` | `/var/www/fspbx/public/ctc.php` |
| `ctc.lua` | `/usr/share/freeswitch/scripts/ctc.lua` |

Then edit the placeholders at the top of each file:

```php
// ctc.php
define('GATEWAY', 'YOUR-GATEWAY-UUID-HERE');
```

```lua
-- ctc.lua
local GATEWAY    = "YOUR-GATEWAY-UUID-HERE"
local DOMAIN     = "YOUR-PBX-DOMAIN-HERE"  -- the FusionPBX domain your agent extensions belong to
local CID_NUMBER = "15550000000"   -- your outbound caller ID
```

Your gateway UUID is in FusionPBX → **Accounts → Gateways** → click your gateway → copy the UUID from the URL.

`DOMAIN` must match the FusionPBX domain the **agent** extension is registered under (e.g. `pbx.example.com`). The agent leg is dialed internally as `user/<agent>@<domain>` so it rings the agent's desk phone directly — only the destination leg goes out through the external `GATEWAY`.

---

## Quick start (installer)

1. Build or download `FpbxCTC-Setup.exe` and run it
2. The installer automatically:
   - Copies files to `C:\Program Files\FpbxCTC\`
   - Registers the `tel:` protocol handler
   - Registers the Native Messaging host for Chrome / Edge / Brave / Vivaldi
   - Creates a Start Menu shortcut
3. Launch **FpbxCTC** from the Start Menu
4. Fill in **Domain**, **API Key**, **Agent Number** and click **Save Settings**
5. Click **Install Browser Extension** → pick your browser → restart it
6. The extension loads automatically; click **Sync from Desktop App** in the popup to copy your settings

---

## Browser extension — load unpacked (dev)

1. Run `build.bat` (generates icons)
2. Open `chrome://extensions` (or `edge://extensions`, `brave://extensions`)
3. Enable **Developer mode** → **Load unpacked** → select the `browser-extension/` folder
4. Click the FpbxCTC icon → enter settings **or** click **Sync from Desktop App**

---

## Building from source

### Prerequisites

| Tool | Version | Link |
|---|---|---|
| Go | 1.22 + | https://go.dev/dl/ |
| Rust + Cargo | stable | https://rustup.rs/ |
| Visual Studio 2022 | Desktop C++ workload | https://visualstudio.microsoft.com/ |

Install the installer CLI once:

```powershell
cargo install installrs --locked
```

### Desktop app only

```bat
build.bat
```

### Desktop app + installer

```bat
build_installer.bat
```

Both scripts automatically prepend Go and Cargo to `PATH`.

**What the build scripts do:**
1. `tools/mkico` — `FpbxCTC.png` → `FpbxCTC.ico`
2. `tools/mkicons` — `FpbxCTC.png` → `browser-extension/icons/` (16 / 32 / 48 / 128 px)
3. `github.com/akavel/rsrc` — embeds the ICO into `rsrc.syso`
4. `go build` → `FpbxCTC.exe`
5. *(installer only)* `installrs build` → `FpbxCTC-Setup.exe`

---

## Project structure

```
FpbxCTC/
├── main.go                        # Entry point — NM mode / tel: call mode / settings GUI
├── config.go                      # Config struct, load/save to %APPDATA%
├── caller.go                      # Number sanitisation + HTTP API call
├── registry.go                    # tel: handler + Native Messaging host registration
├── settings.go                    # Native GUI settings window (gonutz/wui)
├── browser_install.go             # Browser detection, shortcut patching, install/uninstall
├── FpbxCTC.png                    # Source icon (all other icon formats generated from this)
├── go.mod / go.sum
├── build.bat                      # Build FpbxCTC.exe (+ generate icons)
├── build_installer.bat            # Build FpbxCTC-Setup.exe
├── tools/
│   ├── mkico/main.go              # PNG → ICO (pure Go, no CGO)
│   ├── mkicons/main.go            # PNG → 16/32/48/128 px PNGs
│   └── mkzip/main.go              # Zip browser-extension/ for distribution
├── installer/
│   ├── Cargo.toml                 # Rust installer crate (installrs)
│   └── src/lib.rs                 # Install / uninstall logic
├── browser-extension/
│   ├── manifest.json              # MV3 — Chrome, Edge, Brave, Vivaldi
│   ├── background.js              # Service worker: context menu + API call + notifications
│   ├── icons/                     # Generated by build.bat (gitignored)
│   └── popup/
│       ├── popup.html             # Settings UI + Sync button
│       ├── popup.css              # Dark theme
│       └── popup.js               # Load/save via chrome.storage.sync + Native Messaging sync
└── pbx_files/
    ├── ctc.php                    # FusionPBX web UI + API endpoint
    └── ctc.lua                    # FreeSwitch Lua script (call flow)
```

---

## Configuration

**Desktop** — `%APPDATA%\FpbxCTC\config.json` (never committed):

```json
{
  "domain": "pbx.example.com",
  "api_key": "your-api-key-here",
  "agent_number": "1001"
}
```

**Browser extension** — stored in `chrome.storage.sync` (syncs across devices when signed in).  
Use **Sync from Desktop App** in the popup to pull settings from the desktop app via Native Messaging.

---

## Uninstalling

Run **Uninstall FpbxCTC** from Add/Remove Programs (or `C:\Program Files\FpbxCTC\Uninstall FpbxCTC.exe`).

The uninstaller will:
- Remove `--load-extension` from all patched browser shortcuts
- Remove the Native Messaging host registry entries
- Unregister the `tel:` protocol handler
- Delete all installed files

---

## License

MIT — see [LICENSE](LICENSE).
