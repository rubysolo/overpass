# overpass

[![Go Report Card](https://goreportcard.com/badge/github.com/whiteso/overpass)](https://goreportcard.com/report/github.com/whiteso/overpass)

A CLI tool for local front-end development with Cloudflare tunnels, designed to streamline development workflows when working with deployed servers that require local overrides.

## Overview

`overpass` automates the process of:

- Starting a Cloudflare tunnel to bypass PNA (Private Network Access) restrictions
- Generating a browser-compatible manifest file for local overrides
- Running the local development server
- Providing a real-time terminal UI to monitor the workflow

## Prerequisites

Before using `overpass`, ensure you have the following installed:

- **cloudflared** - [Cloudflare Tunnel daemon](https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/installation/)
- **Go** 1.25.7 or later (for building from source)
- **Yarn** - Package manager for running the dev server

## Installation

### Build from Source

```bash
git clone https://github.com/whiteso/overpass.git
cd overpass
go build -o overpass .
```

The binary will be available at `./overpass`. Move it to your PATH if desired.

## Usage

```bash
overpass <app-name>
```

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--path` | `-p` | Path to the monorepo root directory (defaults to current directory) |

### Arguments

| Argument | Description |
|----------|-------------|
| `<app-name>` | The name of the application to run (e.g., `ph-portal`). Must correspond to a directory in `apps/` |

## Configuration

`overpass` **requires** a configuration file at `.overpass.yml` in the monorepo root.

### Required `.overpass.yml`:

```yaml
# Overpass configuration
manifestPathTemplate: "{{.LocalOverrides}}/applications/{{.App}}/app/manifest.js"
```

### Template Variables

The `manifestPathTemplate` supports these variables:

| Variable        | Description                              |
|-----------------|------------------------------------------|
| `{{.Monorepo}}`  | Absolute path to the monorepo root       |
| `{{.LocalOverrides}}` | The `local-overrides` directory name  |
| `{{.App}}`       | The application name (from CLI argument) |

### Custom Path Example

To output the manifest to a custom location:

```yaml
manifestPathTemplate: "{{.Monorepo}}/overrides/{{.App}}/manifest.js"
```

This generates: `/absolute/path/to/monorepo/overrides/<app-name>/manifest.js`


## Monorepo Structure Requirements

`overpass` expects the following directory structure:

```
monorepo/
├── apps/
│   └── <app-name>/
│       ├── webpack.config.js    # Port discovery (devServer.port)
│       └── vite.config.mts      # Alternative: Vite port config
```

### Port Discovery

The tool automatically discovers the development server port by parsing:

- `apps/<app-name>/webpack.config.js` - looks for `devServer.port`
- `apps/<app-name>/vite.config.mts` - looks for `server.port`

### Manifest Template

The manifest template (embedded in the binary) uses placeholders that are replaced at runtime:

- `{{ APP }}` - Replaced with the application name
- `{{ BASE_URL }}` - Replaced with the Cloudflare tunnel URL

## Terminal UI

The TUI provides:

- **Live log streaming** from the development server with syntax highlighting
- **Tunnel status** with the public Cloudflare URL
- **Application info** (name, port, log count)
- **QR code** for mobile access (press `r`)
- **Help screen** (press `?`)

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | Quit and cleanup |
| `?` / `h` | Toggle help screen |
| `r` | Show QR code |
| `↑` / `k` | Scroll logs up |
| `↓` / `j` | Scroll logs down |
| `space` | Toggle auto-scroll |
| `g` | Jump to top of logs |
| `G` | Jump to bottom (enable auto-scroll) |
| `esc` | Close modals |

## How It Works

1. **Validates the monorepo path** - Ensures required directories exist (`apps/`, `local-overrides/`)
2. **Discovers the port** - Parses webpack or vite config to find the dev server port
3. **Starts cloudflared** - Creates a tunnel to `localhost:<port>` and captures the public URL
4.  **Generates manifest** - Creates the manifest file using the path defined in `.overpass.yml` with the tunnel URL
5. **Starts dev server** - Runs `yarn start <app-name>` in the monorepo root
6. **Launches TUI** - Displays the interactive terminal interface
7. **Graceful shutdown** - On exit, kills all background processes and restores the manifest file via `git restore`

## Development

### Running from Source

```bash
go run .
```

### Testing with the Fake Monorepo

A test monorepo structure is included in `fake-monorepo/`:

```bash
cd fake-monorepo
../../overpass ph-portal
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

### Development Setup

1. Install Go 1.25.7 or later
2. Clone the repository
3. Run `go mod tidy` to install dependencies
4. Make your changes
5. Test with the fake monorepo or your own monorepo setup

## Troubleshooting

| Issue | Solution |
|-------|----------|
| "apps directory not found" | Run from monorepo root or use `--path` flag |
| "app directory not found" | Verify the app name matches a directory in `apps/` |
| "local-overrides directory not found" | Ensure `local-overrides/` exists in the monorepo |
| "Error loading configuration" | `.overpass.yml` is required in the monorepo root. Ensure `manifestPathTemplate` is set. See Configuration section. |
| "could not find port" | Check webpack.config.js or vite.config.mts exists with valid port |

## License

MIT License - see LICENSE file for details.

## Technology Stack

- **Language:** Go 1.25.7
- **CLI Framework:** [Cobra](https://github.com/spf13/cobra)
- **TUI Framework:** [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- **TUI Styling:** [Lipgloss](https://github.com/charmbracelet/lipgloss)
- **QR Code:** [qrterminal](https://github.com/mdp/qrterminal)