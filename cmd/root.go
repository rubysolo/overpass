package cmd

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"text/template"

	"github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// Config holds overpass configuration from .overpass.yml
type Config struct {
	// ManifestPathTemplate is a Go template string that defines the manifest output path.
	// Available variables: .Monorepo (monorepo root), .LocalOverrides (local-overrides dir), .App (app name)
	ManifestPathTemplate string `yaml:"manifestPathTemplate"`
}

// loadConfig reads .overpass.yml from the monorepo root
func loadConfig(monoRepoPath string) (*Config, error) {
	configPath := filepath.Join(monoRepoPath, ".overpass.yml")
	data, err := os.ReadFile(configPath)
	if err != nil {
	if os.IsNotExist(err) {
			return nil, fmt.Errorf(".overpass.yml not found at %s.\n\nCreate this file with required field 'manifestPathTemplate', for example:\nmanifestPathTemplate: \"{{.LocalOverrides}}/applications/{{.App}}/app/manifest.js\"\n\nUse template variables: {{.Monorepo}}, {{.LocalOverrides}}, {{.App}}.\nSee README.md for full documentation.", configPath)
		}
		return nil, fmt.Errorf("failed to read .overpass.yml: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse .overpass.yml: %w.\n\nEnsure the file is valid YAML. Example:\nmanifestPathTemplate: \"{{.LocalOverrides}}/applications/{{.App}}/app/manifest.js\"", err)
	}

	if config.ManifestPathTemplate == "" {
		return nil, fmt.Errorf(".overpass.yml missing required field 'manifestPathTemplate'.\n\nExample:\nmanifestPathTemplate: \"{{.LocalOverrides}}/applications/{{.App}}/app/manifest.js\"")
	}

	return &config, nil
}

// expandPathTemplate expands a template string with the given data
func expandPathTemplate(templateStr string, data map[string]string) (string, error) {
	tmpl, err := template.New("path").Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("invalid template syntax: %w", err)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to expand template: %w", err)
	}
	return buf.String(), nil
}

const (
	failedToKillCloudflaredMsg = "failed to kill cloudflared process: %v"
	localOverridesDirName      = "local-overrides"
)

// validateMonorepoPath checks if the given path appears to be a valid monorepo
func validateMonorepoPath(path, appName string) error {
	appsDir := filepath.Join(path, "apps")
	appDir := filepath.Join(appsDir, appName)
	localOverridesDir := filepath.Join(path, localOverridesDirName)

	if _, err := os.Stat(appsDir); os.IsNotExist(err) {
		return fmt.Errorf("apps directory not found at %s", appsDir)
	}

	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		return fmt.Errorf("app directory not found at %s (apps/%s does not exist)", appDir, appName)
	}

	if _, err := os.Stat(localOverridesDir); os.IsNotExist(err) {
		return fmt.Errorf("local-overrides directory not found at %s", localOverridesDir)
	}

	return nil
}

// cleanup performs graceful shutdown of background processes and restores the manifest file.
func cleanup(monoRepoPath, appName string, cfCmd, devCmd *exec.Cmd, config *Config) {
	fmt.Println("\nShutting down...")
	if devCmd != nil && devCmd.Process != nil {
		if err := syscall.Kill(-devCmd.Process.Pid, syscall.SIGKILL); err != nil {
			log.Printf("failed to kill dev server process: %v", err)
		}
	}
	if cfCmd != nil && cfCmd.Process != nil {
		if err := syscall.Kill(-cfCmd.Process.Pid, syscall.SIGKILL); err != nil {
			log.Printf(failedToKillCloudflaredMsg, err)
		}
	}
	if err := restoreManifest(monoRepoPath, appName, config); err != nil {
		log.Printf("failed to restore manifest file: %v", err)
	}
}

func captureTerminalState() string {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return ""
	}

	stty := exec.Command("stty", "-g")
	stty.Stdin = os.Stdin
	output, err := stty.Output()
	if err != nil {
		log.Printf("failed to capture terminal state: %v", err)
		return ""
	}

	return strings.TrimSpace(string(output))
}

func restoreTerminalState(state string) {
	if state == "" || !term.IsTerminal(int(os.Stdin.Fd())) {
		return
	}

	stty := exec.Command("stty", state)
	stty.Stdin = os.Stdin
	stty.Stdout = os.Stdout
	stty.Stderr = os.Stderr
	if err := stty.Run(); err != nil {
		log.Printf("failed to restore terminal state: %v", err)
	}
}

// forceResetTerminalState performs a best-effort reset of terminal mode/state.
func forceResetTerminalState(savedState string) {
	if savedState != "" {
		restoreTerminalState(savedState)
	} else if term.IsTerminal(int(os.Stdin.Fd())) {
		stty := exec.Command("stty", "sane")
		stty.Stdin = os.Stdin
		stty.Stdout = os.Stdout
		stty.Stderr = os.Stderr
		_ = stty.Run()
	}

	// Ensure attributes/cursor/alt-screen state are restored.
	fmt.Fprint(os.Stdout, "\x1b[0m\x1b[?25h\x1b[?1049l\r")
}

func discoverPort(monoRepoPath, appName string) (string, error) {
	appPath := filepath.Join(monoRepoPath, "apps", appName)
	webpackConfigPath := filepath.Join(appPath, "webpack.config.js")
	viteConfigPath := filepath.Join(appPath, "vite.config.mts")

	if content, err := os.ReadFile(webpackConfigPath); err == nil {
		re := regexp.MustCompile(`port:\s*(\d+)`)
		matches := re.FindSubmatch(content)
		if len(matches) >= 2 {
			return string(matches[1]), nil
		}
	}

	if content, err := os.ReadFile(viteConfigPath); err == nil {
		re := regexp.MustCompile(`(?s)server\s*:\s*\{.*?port\s*:\s*(\d+)`)
		matches := re.FindSubmatch(content)
		if len(matches) >= 2 {
			return string(matches[1]), nil
		}
		return "", fmt.Errorf("could not find server.port in %s", viteConfigPath)
	}

	return "", fmt.Errorf("could not find port in %s or %s", webpackConfigPath, viteConfigPath)
}

func startCloudflared(port string) (<-chan string, *exec.Cmd, error) {
	cmd := exec.Command("cloudflared", "tunnel", "--url", fmt.Sprintf("http://localhost:%s", port))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	urlChan := make(chan string)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "trycloudflare.com") {
				fields := strings.Fields(line)
				for _, field := range fields {
					if strings.HasPrefix(field, "https://") {
						urlChan <- field
						return
					}
				}
			}
		}
	}()

	return urlChan, cmd, nil
}

func generateManifest(monoRepoPath, appName, publicURL string, config *Config) error {
	if manifestTemplate == "" {
		return fmt.Errorf("embedded manifest template is empty")
	}

	modifiedContent := strings.ReplaceAll(manifestTemplate, "{{ APP }}", appName)
	modifiedContent = strings.ReplaceAll(modifiedContent, "{{ BASE_URL }}", publicURL)

	// Build manifest output path from template
	templateData := map[string]string{
		"Monorepo":         monoRepoPath,
		"LocalOverrides":   localOverridesDirName,
		"App":              appName,
	}
	relPath, err := expandPathTemplate(config.ManifestPathTemplate, templateData)
	if err != nil {
		return fmt.Errorf("failed to expand manifest path template: %w", err)
	}
	outputPath := filepath.Join(monoRepoPath, relPath)

	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	if err := os.WriteFile(outputPath, []byte(modifiedContent), 0644); err != nil {
		return fmt.Errorf("failed to write manifest file: %w", err)
	}

	return nil
}

func startDevServer(monoRepoPath, appName string) (*exec.Cmd, <-chan string, error) {
	cmd := exec.Command("yarn", "start", appName)
	cmd.Dir = monoRepoPath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stderr = cmd.Stdout // Merge stderr into stdout

	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	outputChan := make(chan string)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			outputChan <- scanner.Text()
		}
		close(outputChan)
	}()

	return cmd, outputChan, nil
}

func restoreManifest(monoRepoPath, appName string, config *Config) error {
	templateData := map[string]string{
		"Monorepo":         monoRepoPath,
		"LocalOverrides":   localOverridesDirName,
		"App":              appName,
	}
	relPath, err := expandPathTemplate(config.ManifestPathTemplate, templateData)
	if err != nil {
		return fmt.Errorf("failed to expand manifest path template: %w", err)
	}
	manifestPath := filepath.Join(monoRepoPath, relPath)
	cmd := exec.Command("git", "restore", manifestPath)
	cmd.Dir = monoRepoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restore manifest file: %s, %w", output, err)
	}
	return nil
}


// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "overpass <app-name>",
	Short: "A CLI tool for local front-end development with Cloudflare tunnels.",
	Long: `The overpass tool simplifies local development and testing of front-end applications
that require local overrides for deployed servers. It automates the process of using
a Cloudflare tunnel to bypass PNA (Private Network Access) restrictions, generates a
browser-compatible manifest file, and runs the local development server.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		appName := args[0]

		// Get monorepo path from flag or use current directory
		monoRepoPath, _ := cmd.Flags().GetString("path")
		if monoRepoPath == "" {
			var err error
			monoRepoPath, err = os.Getwd()
			if err != nil {
				fmt.Println("Error getting current directory:", err)
				os.Exit(1)
			}
		}

		// Validate monorepo path
		if err := validateMonorepoPath(monoRepoPath, appName); err != nil {
			fmt.Println("Error validating monorepo path:", err)
			fmt.Printf("Please ensure you are in the monorepo root directory or specify the correct path with --path\n")
			os.Exit(1)
		}

		// Load configuration
		config, err := loadConfig(monoRepoPath)
		if err != nil {
			fmt.Println("Error loading configuration:", err)
			os.Exit(1)
		}

		port, err := discoverPort(monoRepoPath, appName)
		if err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}

		urlChan, cfCmd, err := startCloudflared(port)
		if err != nil {
			fmt.Println("Error starting cloudflared:", err)
			os.Exit(1)
		}

		var publicURL string
		cfProcessDone := make(chan struct{})
		go func() {
			cfCmd.Wait()
			close(cfProcessDone)
		}()

		select {
		case publicURL = <-urlChan:
			// Tunnel URL received, continue silently
		case <-cfProcessDone:
			fmt.Println("Error: cloudflared process exited unexpectedly")
			os.Exit(1)
		}

		if err := generateManifest(monoRepoPath, appName, publicURL, config); err != nil {
			fmt.Println("Error generating manifest:", err)
			if err := syscall.Kill(-cfCmd.Process.Pid, syscall.SIGKILL); err != nil {
				log.Fatalf(failedToKillCloudflaredMsg, err)
			}
			os.Exit(1)
		}

		devCmd, devOutputChan, err := startDevServer(monoRepoPath, appName)
		if err != nil {
			fmt.Println("Error starting dev server:", err)
			if err := syscall.Kill(-cfCmd.Process.Pid, syscall.SIGKILL); err != nil {
				log.Fatalf(failedToKillCloudflaredMsg, err)
			}
			os.Exit(1)
		}

		devProcessDone := make(chan error, 1)
		go func() {
			devProcessDone <- devCmd.Wait()
			close(devProcessDone)
		}()

		savedTerminalState := captureTerminalState()
		cleanupOnce := sync.OnceFunc(func() {
			cleanup(monoRepoPath, appName, cfCmd, devCmd, config)
		})
		defer func() {
			cleanupOnce()
			forceResetTerminalState(savedTerminalState)
		}()

		// Start TUI with alt screen
		p := tea.NewProgram(
			initialModel(publicURL, appName, port, devOutputChan, devProcessDone, cleanupOnce),
			tea.WithAltScreen(),
		)
		finalModel, err := p.Run()
		if err != nil {
			log.Printf("Error running TUI: %v", err)
			os.Exit(1)
		}

		if finalState, ok := finalModel.(model); ok && finalState.devExitCode != 0 {
			os.Exit(finalState.devExitCode)
		}
	},
}


// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringP("path", "p", "", "Path to the monorepo root directory (defaults to current directory)")
}
