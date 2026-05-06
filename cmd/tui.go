package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	qrterminal "github.com/mdp/qrterminal/v3"
)

// Animation and UI state
type tickMsg time.Time
type logMsg string
type devEndedMsg struct {
	err error
}

type model struct {
	publicURL      string
	appName        string
	port           string
	devOutputChan  <-chan string
	devOutput      []string
	devProcessDone <-chan error
	devExitCode    int
	cleanupFunc    func()
	startTime      time.Time
	frame          int
	width          int
	height         int
	scrollOffset   int
	autoScroll     bool
	showHelp       bool
	showQR         bool
	tunnelStatus   string
	devStatus      string
}

// Colors and styles
var (
	primaryColor   = lipgloss.Color("#00D9FF")
	accentColor    = lipgloss.Color("#FF3E96")
	successColor   = lipgloss.Color("#00FF9F")
	warningColor   = lipgloss.Color("#FFB900")
	errorColor     = lipgloss.Color("#FF3E3E")
	mutedColor     = lipgloss.Color("#6B7280")
	bgColor        = lipgloss.Color("#1a1a1a")
	borderColor    = lipgloss.Color("#3B82F6")
	textColor      = lipgloss.Color("#E5E7EB")

	// Header style
	headerStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(primaryColor).
		Background(bgColor).
		Padding(0, 1)

	// Panel styles
	panelStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		MarginBottom(1)

	activePanelStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Padding(1, 2).
		MarginBottom(1)

	// Status badge styles
	statusBadge = lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1).
		MarginRight(1)

	successBadge = statusBadge.Copy().
		Foreground(lipgloss.Color("#000")).
		Background(successColor)

	warningBadge = statusBadge.Copy().
		Foreground(lipgloss.Color("#000")).
		Background(warningColor)

	errorBadge = statusBadge.Copy().
		Foreground(lipgloss.Color("#FFF")).
		Background(errorColor)

	// Log styles
	logLineStyle = lipgloss.NewStyle().
		Foreground(textColor)

	errorLogStyle = lipgloss.NewStyle().
		Foreground(errorColor).
		Bold(true)

	warningLogStyle = lipgloss.NewStyle().
		Foreground(warningColor)

	successLogStyle = lipgloss.NewStyle().
		Foreground(successColor)

	// Footer style
	footerStyle = lipgloss.NewStyle().
		Foreground(mutedColor).
		Background(bgColor).
		Padding(0, 1)

	keyStyle = lipgloss.NewStyle().
		Foreground(accentColor).
		Bold(true)

	helpStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Padding(1, 2).
		Foreground(textColor)
)

func (m model) Init() tea.Cmd {
	return tea.Batch(
		streamDevOutput(m.devOutputChan),
		waitForDevProcessEnd(m.devProcessDone),
		tickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.cleanupFunc()
			return m, tea.Quit
		case "?", "h":
			m.showHelp = !m.showHelp
			m.showQR = false // Close QR if help is opened
		case "r", "u":
			m.showQR = !m.showQR
			m.showHelp = false // Close help if QR is opened
		case "esc":
			m.showHelp = false
			m.showQR = false
		case "down", "j":
			if !m.showHelp && !m.showQR && len(m.devOutput) > 0 {
				m.autoScroll = false
				m.scrollOffset++
			}
		case "up", "k":
			if !m.showHelp && !m.showQR && m.scrollOffset > 0 {
				m.autoScroll = false
				m.scrollOffset--
			}
		case "g":
			if !m.showHelp && !m.showQR {
				m.autoScroll = false
				m.scrollOffset = 0
			}
		case "G":
			if !m.showHelp && !m.showQR {
				m.autoScroll = true
				m.scrollOffset = len(m.devOutput)
			}
		case "space":
			if !m.showHelp && !m.showQR {
				m.autoScroll = !m.autoScroll
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tickMsg:
		m.frame++
		return m, tickCmd()
	case logMsg:
		m.devOutput = append(m.devOutput, string(msg))
		if m.autoScroll {
			m.scrollOffset = len(m.devOutput)
		}
		return m, streamDevOutput(m.devOutputChan)
	case devEndedMsg:
		m.devStatus = "exited"
		m.devExitCode = exitCodeFromError(msg.err)
		m.cleanupFunc()
		return m, tea.Quit
	}
	return m, nil
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus()
		}
	}

	return 1
}

func (m model) View() string {
	if m.showHelp {
		return m.helpView()
	}

	if m.showQR {
		return m.qrView()
	}

	// Build the UI
	var sections []string

	// Header with ASCII art
	sections = append(sections, m.headerView())

	// Status panels
	sections = append(sections, m.statusView())

	// Main content area
	sections = append(sections, m.logsView())

	// Footer
	sections = append(sections, m.footerView())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m model) headerView() string {
	logo := `
   ▄▄▄▄▄▄▄ ▄     ▄ ▄▄▄▄▄▄▄ ▄▄▄▄▄▄▄▄ ▄▄▄▄▄▄▄ ▄▄▄▄▄▄▄ ▄▄▄▄▄▄▄ ▄▄▄▄▄▄▄ 
  █       █ █   █ █       █        █       █       █       █       █
  █   ▄   █ ██ ██ █    ▄▄▄█    ▄   █    ▄  █   ▄   █  ▄▄▄▄▄█  ▄▄▄▄▄█
  █  █ █  █   █   █   █▄▄▄█   █▄█  █   █▄█ █  █▄█  █ █▄▄▄▄▄█ █▄▄▄▄▄ 
  █  █▄█  █       █    ▄▄▄█   ▄  ▄▄█    ▄▄▄█  ▄▄▄  █▄▄▄▄▄  █▄▄▄▄▄  █
  █       ██     ██   █▄▄▄█  █ █  ██   █   █  █ █  █▄▄▄▄▄█ █▄▄▄▄▄█ █
  █▄▄▄▄▄▄▄█ █▄▄▄█ █▄▄▄▄▄▄▄█▄▄█  █▄▄█▄▄▄█   █▄▄█ █▄▄█▄▄▄▄▄▄▄█▄▄▄▄▄▄▄█`

	logoStyled := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#3B82F6", Dark: "#60A5FA"}).
		Bold(true).
		Render(logo)

	uptime := time.Since(m.startTime).Round(time.Second)
	subtitle := lipgloss.NewStyle().
		Foreground(mutedColor).
		Render(fmt.Sprintf("  Local Development Tunnel • Uptime: %s", uptime))

	return lipgloss.JoinVertical(lipgloss.Left, logoStyled, subtitle, "")
}

func (m model) statusView() string {
	// Animated indicator
	spinner := []string{"◐", "◓", "◑", "◒"}
	spinnerIcon := spinner[m.frame/3%len(spinner)]

	// Tunnel status
	tunnelIcon := "●"
	tunnelColor := successColor
	if m.tunnelStatus == "error" {
		tunnelIcon = "✗"
		tunnelColor = errorColor
	}

	tunnelStatusBadge := lipgloss.NewStyle().
		Foreground(tunnelColor).
		Bold(true).
		Render(fmt.Sprintf("%s TUNNEL", tunnelIcon))

	// Dev server status
	devIcon := spinnerIcon
	devColor := successColor
	if m.devStatus == "exited" {
		devIcon = "✗"
		devColor = errorColor
	}

	devStatusBadge := lipgloss.NewStyle().
		Foreground(devColor).
		Bold(true).
		Render(fmt.Sprintf("%s SERVER", devIcon))

	// Left panel - Tunnel info
	tunnelTitle := lipgloss.NewStyle().
		Foreground(primaryColor).
		Bold(true).
		Render("🌐 Cloudflare Tunnel")

	urlBox := lipgloss.NewStyle().
		Background(lipgloss.Color("#2D3748")).
		Foreground(successColor).
		Bold(true).
		Padding(0, 1).
		MarginTop(1).
		Render(m.publicURL)

	qrHint := lipgloss.NewStyle().
		Foreground(mutedColor).
		Italic(true).
		MarginTop(1).
		Render("📱 Press r to view QR code")

	tunnelContent := lipgloss.JoinVertical(
		lipgloss.Left,
		tunnelStatusBadge,
		"",
		tunnelTitle,
		urlBox,
		qrHint,
	)

	tunnelPanel := panelStyle.Render(tunnelContent)

	// Right panel - App info
	appTitle := lipgloss.NewStyle().
		Foreground(accentColor).
		Bold(true).
		Render("⚡ Application Info")

	appInfo := []string{
		devStatusBadge,
		"",
		fmt.Sprintf("App:  %s", lipgloss.NewStyle().Foreground(primaryColor).Bold(true).Render(m.appName)),
		fmt.Sprintf("Port: %s", lipgloss.NewStyle().Foreground(warningColor).Bold(true).Render(m.port)),
		fmt.Sprintf("Logs: %s", lipgloss.NewStyle().Foreground(successColor).Bold(true).Render(fmt.Sprintf("%d lines", len(m.devOutput)))),
	}

	appContent := lipgloss.JoinVertical(lipgloss.Left, appInfo...)

	appPanel := panelStyle.Render(lipgloss.JoinVertical(lipgloss.Left, appTitle, "", appContent))

	// Join panels side by side
	return lipgloss.JoinHorizontal(lipgloss.Top, tunnelPanel, "  ", appPanel)
}

func (m model) logsView() string {
	title := lipgloss.NewStyle().
		Foreground(primaryColor).
		Bold(true).
		Render("📋 Development Server Logs")

	scrollInfo := ""
	if !m.autoScroll {
		scrollInfo = lipgloss.NewStyle().
			Foreground(warningColor).
			Render(" [SCROLL PAUSED]")
	}

	header := lipgloss.JoinHorizontal(lipgloss.Left, title, scrollInfo)

	// Calculate visible log lines
	maxLines := 15
	if m.height > 40 {
		maxLines = 20
	}

	start := len(m.devOutput) - maxLines
	if start < 0 {
		start = 0
	}
	if !m.autoScroll && m.scrollOffset < len(m.devOutput) {
		start = m.scrollOffset - maxLines
		if start < 0 {
			start = 0
		}
	}

	visibleLogs := m.devOutput[start:]
	if len(visibleLogs) > maxLines {
		visibleLogs = visibleLogs[len(visibleLogs)-maxLines:]
	}

	var styledLogs []string
	for i, line := range visibleLogs {
		lineNum := lipgloss.NewStyle().
			Foreground(mutedColor).
			Width(4).
			Render(fmt.Sprintf("%3d ", start+i+1))

		styledLine := m.styleLogLine(line)
		styledLogs = append(styledLogs, lineNum+styledLine)
	}

	if len(styledLogs) == 0 {
		styledLogs = append(styledLogs, lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true).
			Render("  Waiting for dev server output..."))
	}

	logsContent := strings.Join(styledLogs, "\n")
	logsPanel := activePanelStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, "", logsContent))

	return logsPanel
}

func (m model) styleLogLine(line string) string {
	lower := strings.ToLower(line)

	// Error patterns
	if strings.Contains(lower, "error") || strings.Contains(lower, "fail") || strings.Contains(lower, "✗") {
		return errorLogStyle.Render("✗ " + line)
	}

	// Warning patterns
	if strings.Contains(lower, "warn") || strings.Contains(lower, "warning") {
		return warningLogStyle.Render("⚠ " + line)
	}

	// Success patterns
	if strings.Contains(lower, "success") || strings.Contains(lower, "compiled") || 
	   strings.Contains(lower, "✓") || strings.Contains(lower, "ready") {
		return successLogStyle.Render("✓ " + line)
	}

	// Info patterns
	if strings.Contains(lower, "http://") || strings.Contains(lower, "https://") {
		return lipgloss.NewStyle().Foreground(primaryColor).Render("→ " + line)
	}

	return logLineStyle.Render("  " + line)
}

func (m model) footerView() string {
	shortcuts := []string{
		keyStyle.Render("q/Ctrl+C") + " quit",
		keyStyle.Render("?/h") + " help",
		keyStyle.Render("r") + " QR code",
		keyStyle.Render("↑/k") + " scroll up",
		keyStyle.Render("↓/j") + " scroll down",
		keyStyle.Render("space") + " toggle auto-scroll",
		keyStyle.Render("g") + " top",
		keyStyle.Render("G") + " bottom",
	}

	return footerStyle.Render("  " + strings.Join(shortcuts, " • ") + "  ")
}

func (m model) helpView() string {
	title := lipgloss.NewStyle().
		Foreground(primaryColor).
		Bold(true).
		Align(lipgloss.Center).
		Width(60).
		Render("OVERPASS HELP")

	helpText := `
Navigation:
  ↑ / k           Scroll logs up (pauses auto-scroll)
  ↓ / j           Scroll logs down
  g               Jump to top of logs
  G               Jump to bottom of logs (enables auto-scroll)
  space           Toggle auto-scroll mode

Actions:
  q / Ctrl+C      Quit and cleanup
  ? / h           Toggle this help screen
  r               Show QR code for tunnel URL
  esc             Close modals

Features:
  • Live log streaming with syntax highlighting
  • Auto-scroll mode (enabled by default)
  • Color-coded log levels (errors, warnings, success)
  • Real-time tunnel status monitoring
  • Uptime tracking
  • QR code for mobile access

Tips:
  • The tunnel URL is automatically generated
  • Logs are color-coded for easy scanning
  • Use auto-scroll to follow live output
  • Press space to pause and review logs
  • Scan QR code to access from mobile devices`

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		"",
		lipgloss.NewStyle().Foreground(textColor).Render(helpText),
		"",
		lipgloss.NewStyle().
			Foreground(mutedColor).
			Render("Press ? or h to close this help screen"),
	)

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		helpStyle.Render(content),
	)
}

func (m model) qrView() string {
	title := lipgloss.NewStyle().
		Foreground(primaryColor).
		Bold(true).
		Align(lipgloss.Center).
		Width(50).
		Render("🌐 TUNNEL QR CODE")

	urlDisplay := lipgloss.NewStyle().
		Foreground(successColor).
		Bold(true).
		Align(lipgloss.Center).
		Width(50).
		Render(m.publicURL)

	// Generate QR code to buffer
	var buf bytes.Buffer
	qrConfig := qrterminal.Config{
		Level:     qrterminal.M,
		Writer:    &buf,
		BlackChar: qrterminal.BLACK,
		WhiteChar: qrterminal.WHITE,
		QuietZone: 2,
	}
	qrterminal.GenerateWithConfig(m.publicURL, qrConfig)

	qrCode := lipgloss.NewStyle().
		Foreground(textColor).
		Align(lipgloss.Center).
		Render(buf.String())

	hint := lipgloss.NewStyle().
		Foreground(mutedColor).
		Align(lipgloss.Center).
		Width(50).
		Render("Scan with your mobile device • Press r or esc to close")

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		"",
		urlDisplay,
		"",
		qrCode,
		"",
		hint,
	)

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Padding(2, 4).
		Background(bgColor)

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		modalStyle.Render(content),
	)
}

func initialModel(publicURL, appName, port string, devOutputChan <-chan string, devProcessDone <-chan error, cleanupFunc func()) model {
	return model{
		publicURL:      publicURL,
		appName:        appName,
		port:           port,
		devOutputChan:  devOutputChan,
		devProcessDone: devProcessDone,
		cleanupFunc:    cleanupFunc,
		startTime:      time.Now(),
		autoScroll:     true,
		tunnelStatus:   "active",
		devStatus:      "running",
		width:          120, // Default width, will be updated by WindowSizeMsg
		height:         40,  // Default height, will be updated by WindowSizeMsg
	}
}

func streamDevOutput(devOutputChan <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-devOutputChan
		if !ok {
			return nil
		}
		return logMsg(line)
	}
}

func waitForDevProcessEnd(devProcessDone <-chan error) tea.Cmd {
	return func() tea.Msg {
		err, ok := <-devProcessDone
		if !ok {
			return devEndedMsg{}
		}
		return devEndedMsg{err: err}
	}
}
