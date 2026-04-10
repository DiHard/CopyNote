# CopyNote

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Svelte](https://img.shields.io/badge/Svelte-5-FF3E00?logo=svelte&logoColor=white)](https://svelte.dev/)
[![Tailwind CSS](https://img.shields.io/badge/Tailwind_CSS-4-06B6D4?logo=tailwindcss&logoColor=white)](https://tailwindcss.com/)
[![WebView2](https://img.shields.io/badge/WebView2-Runtime-0078D4?logo=microsoftedge&logoColor=white)](https://developer.microsoft.com/en-us/microsoft-edge/webview2/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A lightweight Windows tray utility for storing and instantly copying frequently used text snippets &mdash; emails, addresses, IDs, templates, and anything you paste often.

[Русская версия / Russian version](README.ru.md)

## Features

- **One-click copy** &mdash; click any entry to copy its value to the clipboard
- **CRUD management** &mdash; create, edit, delete, and reorder entries
- **Instant search** &mdash; filter entries by label or value as you type
- **System tray integration** &mdash; lives in the notification area, toggle with a left click
- **Auto-hide on focus loss** &mdash; click outside and the window quietly tucks away
- **Single instance** &mdash; launching again brings the existing window to front
- **Adaptive tray icon** &mdash; automatically switches between light and dark variants when you change the Windows theme
- **Portable** &mdash; single `.exe`, no installation required
- **Lightweight** &mdash; ~7 MB binary, ~40 MB RAM in idle

## Requirements

- **Windows 10 (1809+) or Windows 11**
- **WebView2 Runtime** &mdash; pre-installed on Windows 11 and most up-to-date Windows 10 machines. If missing, download the [Evergreen Bootstrapper](https://developer.microsoft.com/en-us/microsoft-edge/webview2/#download).

## Quick start

Download the latest `copynote.exe` from [Releases](https://github.com/DiHard/CopyNote/releases) and run it. That's it &mdash; no installer, no dependencies beyond WebView2.

The app starts minimized to the system tray. Left-click the tray icon to open.

## Building from source

### Prerequisites

| Tool | Version |
|------|---------|
| [Go](https://go.dev/dl/) | 1.23+ |
| [Node.js](https://nodejs.org/) | 18+ (only for building the frontend) |

### Steps

```bash
# 1. Clone
git clone https://github.com/DiHard/CopyNote.git
cd CopyNote

# 2. Build the frontend (Svelte + Tailwind → single inlined HTML)
cd web
npm install
npm run build
cd ..

# 3. Build the Go binary
go build -ldflags="-H=windowsgui -s -w" -o copynote.exe .
```

The resulting `copynote.exe` is fully self-contained (the frontend is embedded via `//go:embed`).

### Regenerating the app icon

The tray/exe icon is generated from code (Lucide "copy" outline):

```bash
go run tools/genicon/main.go          # writes assets/icon-dark.ico + icon-light.ico
./bin/rsrc.exe -ico "assets/icon-dark.ico,assets/icon-light.ico" -arch amd64 -o resource_windows_amd64.syso
```

## Data storage

| What | Where |
|------|-------|
| Entries (JSON) | `%APPDATA%\CopyNote\data.json` |
| WebView2 cache | `%LOCALAPPDATA%\CopyNote\WebView2\` |

No data is stored next to the executable &mdash; safe to put it anywhere.

## Tech stack

| Layer | Technology |
|-------|-----------|
| Backend | Go (stdlib + [go-webview2](https://github.com/jchv/go-webview2)) |
| Frontend | Svelte 5, TypeScript, Tailwind CSS 4 |
| Bundler | Vite + vite-plugin-singlefile |
| UI host | Microsoft Edge WebView2 |
| System integration | Win32 API via `golang.org/x/sys/windows` (no cgo) |

## Keyboard shortcuts

| Key | Context | Action |
|-----|---------|--------|
| `Escape` | Main window | Hide to tray |
| `Escape` | Any modal | Close modal |
| `Enter` | Create/Edit form | Save |
| `Enter` | Delete confirmation | Confirm |
| `Tab` | Entry list | Navigate between entries |

## License

[MIT](LICENSE)
