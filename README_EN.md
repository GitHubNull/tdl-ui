# tdl UI

> A Telegram media downloader desktop client based on [tdl](https://github.com/GitHubNull/tdl)

English | [简体中文](README.md)

tdl UI is a GUI transformation of the CLI tool tdl: it reuses tdl's battle-tested download engine and session system, wraps them in a clean desktop interface built with [Wails v2](https://wails.io) + [Vue 3](https://vuejs.org) + [PrimeVue 4](https://primevue.org), and embeds the [Yaegi](https://github.com/traefik/yaegi) Go script engine for flexible download filtering, renaming and task automation.

## Features

- **Account login**: code login, QR code login, one-click Telegram Desktop session import
- **Media download**: paste message links for batch download, multi-threaded with resumable transfers (same engine as tdl CLI)
- **Task management**: real-time progress, pause / resume / cancel
- **Script engine** (Yaegi, Go syntax):
  - `Filter` / `Rename` — skip files by condition, customize file names
  - `OnTaskStart` / `OnFileDone` / `OnTaskDone` — task lifecycle hooks
- **Proxy support**: SOCKS5 / HTTP
- **Clean UI**: light / dark themes (follow system), single portable binary

## Screenshots

> (Placeholders: Login / Download / Tasks / Scripts / Settings)

## Installation

Download `tdl-ui.exe` (Windows x64) from Releases and run it directly — no installation required.

### Build from source

Requirements: Go 1.23+, Node.js 20+, pnpm, [Wails CLI v2](https://wails.io/docs/gettingstarted/installation).

```bash
git clone --recurse-submodules <repo-url>
cd tdl_UI/src
wails build -ldflags "-s -w" -trimpath
# Output: src/build/bin/tdl-ui.exe
```

> Size note: the binary is ~50 MB due to the embedded gotd (Telegram MTProto) and Yaegi interpreter.
> To reduce size, compress with [UPX](https://upx.github.io/): `upx --best tdl-ui.exe` (roughly halves the size at a small startup cost).

## Quick Start

1. Open the app and log in to Telegram on the **Account** page (QR code recommended)
2. Paste message links (one per line) on the **Download** page, pick a directory, and create a task
3. Watch real-time progress on the **Tasks** page; pause / resume (resumable) / cancel anytime
4. (Optional) Write Go scripts on the **Scripts** page for filtering, renaming and automation hooks

## Documentation

| Series | Description |
| --- | --- |
| [Tutorials](doc/tutorials/README.md) | Graded by difficulty: install & login → downloads & settings → advanced scripting |
| [Developer docs](doc/dev-human/README.md) | For human developers: environment, architecture, extension guides |
| [AI agent docs](doc/dev-ai/README.md) | For AI coding agents: constraints, modification recipes, upgrade playbooks |

> Documentation is currently written in Chinese.

## Project Layout

```
├── ref/tdl      # tdl upstream submodule (read-only, sync-only)
├── src/         # Wails app source (Go backend + Vue frontend)
├── doc/         # documentation
└── tmp/         # temporary files (not committed)
```

## License & Disclaimer

This project reuses code from [tdl](https://github.com/GitHubNull/tdl) (as a Git submodule referenced via `go.mod replace`) and is therefore released under [AGPL-3.0](LICENSE).

Please read the [disclaimer](DISCLAIMER.md) before use: this tool is for lawful purposes only; comply with the Telegram Terms of Service and your local laws.
