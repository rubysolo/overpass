# `overpass` CLI Development Plan

## 1. Objective

The `overpass` tool is a Go-based command-line utility designed to simplify local development and testing of front-end applications that require local overrides for deployed servers. It automates the process of using a Cloudflare tunnel to bypass PNA (Private Network Access) restrictions, generates a browser-compatible manifest file, and runs the local development server.

## 2. CLI Usage

The tool will be invoked as follows:

```sh
overpass <app-name>
```

-   `<app-name>`: The name of the application to run (e.g., `ph-portal`). This is a mandatory argument.

## 3. Core Features & Logic

The tool will execute the following steps in order:

1.  **Parse Arguments:**
    -   Extract the `<app-name>` from the command-line arguments.

2.  **Discover Port:**
    -   Automatically determine the application's development server port.
    -   **Action:** Read the file at `apps/<app-name>/webpack.config.js`.
    -   **Action:** Parse the file content to find the value of `devServer.port` using a regular expression.
    -   If the port cannot be determined, exit with a clear error message.

3.  **Start Cloudflare Tunnel:**
    -   **Action:** Execute the command `cloudflared tunnel --url http://localhost:<discovered-port>` as a background process.
    -   **Action:** Capture the public URL from the `cloudflared` output.

4.  **Generate Manifest File:**
    -   **Action:** Read the manifest path template from `.overpass.yml` (require field `manifestPathTemplate`).
    -   **Action:** Read the content of the template file located at `local-overrides/manifest.template.js`.
    -   **Action:** Perform the following replacements on the content:
        -   Replace the placeholder `{{ APP }}` with the `<app-name>` from the command line.
        -   Replace the placeholder `{{ BASE_URL }}` with the public tunnel URL.
    -   **Action:** Expand the path template using variables: `.Monorepo`, `.LocalOverrides`, `.App`.
    -   **Action:** Ensure the destination directory exists, creating it if necessary.
    -   **Action:** Write the modified content to the output path.

5.  **Start Local Dev Server:**
    -   **Action:** Execute the command `yarn start <app-name>` as a background process.
    -   **Action:** Capture the `stdout` and `stderr` streams of this process to be displayed in the TUI.

6.  **Launch Terminal UI (TUI):**
    -   **Action:** Display an interactive TUI built with Bubble Tea.
    -   The UI will feature:
        -   A panel showing the Cloudflare Tunnel status and public URL.
        -   A scrolling panel showing the live log output from the local dev server.
        -   Status indicators for each component.

7.  **Graceful Shutdown:**
    -   **Action:** On receiving an interrupt signal (Ctrl+C), the tool will initiate a cleanup sequence.
    -   The sequence is:
        1.  Terminate the `yarn start` dev server process.
        2.  Terminate the `cloudflared` tunnel process.
        3.  Restore the manifest file to its original state by running `git restore`.

## 4. Configuration Summary

| Item                    | Value                                                                            |
| ----------------------- | -------------------------------------------------------------------------------- |
| Dev Server Command      | `yarn start <app-name>`                                                          |
| Port Discovery File     | `apps/<app-name>/webpack.config.js`                                              |
| Config File             | `.overpass.yml` (monorepo root)                                                  |
| Manifest Template       | `local-overrides/manifest.template.js`                                           |
| Manifest Output Path    | Configured via `manifestPathTemplate` in `.overpass.yml` (required, no default) |
| App Name Placeholder    | `{{ APP }}`                                                                      |
| URL Placeholder         | `{{ BASE_URL }}`                                                                 |
| Path Template Variables | `{{.Monorepo}}`, `{{.LocalOverrides}}`, `{{.App}}`                              |

## 5. Technology Stack

-   **Language:** Go
-   **CLI Framework:** `github.com/spf13/cobra`
-   **TUI Framework:** `github.com/charmbracelet/bubbletea`
-   **TUI Styling:** `github.com/charmbracelet/lipgloss`
