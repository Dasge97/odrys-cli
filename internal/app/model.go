package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Dasge97/odrys-cli/internal/backend"
)

type screenMode string

const (
	modeHome    screenMode = "home"
	modeHelp    screenMode = "help"
	modeSession screenMode = "session"
)

var logoLines = []string{
	" ▄█████▄  ██████▄  ██████▄  ▀██  ██▀  ▄██████",
	"██▀   ▀██ ██   ▀██ ██   ▀██   ████   ██▀      ",
	"██     ██ ██    ██ ██████▀     ██    ▀████▄   ",
	"██▄   ▄██ ██   ▄██ ██  ██      ██        ▀██  ",
	" ▀█████▀  ██████▀  ██   ██     ██   ██████▀   ",
}

type doctorLoadedMsg struct {
	payload backend.DoctorPayload
	err     error
}

type runFinishedMsg struct {
	result backend.RunResult
	err    error
}

type scanFinishedMsg struct {
	output string
	err    error
}

type model struct {
	root         string
	service      *backend.Service
	mode         screenMode
	width        int
	height       int
	ready        bool
	busy         bool
	errText      string
	infoText     string
	homeInput    textinput.Model
	sessionInput textinput.Model
	viewport     viewport.Model
	providerName string
	providerModel string
	workspace    string
	sessionAt    string
	transcript   []string
	previousMode screenMode
}

func NewModel(root string) tea.Model {
	home := textinput.New()
	home.Placeholder = "Describe lo que quieres hacer"
	home.CharLimit = 4000
	home.Width = 42
	home.Prompt = ""
	home.Focus()

	session := textinput.New()
	session.Placeholder = "Escribe una instruccion o /help"
	session.CharLimit = 4000
	session.Width = 72
	session.Prompt = ""

	vp := viewport.New(80, 20)
	vp.SetContent("")

	return &model{
		root:          root,
		service:       backend.NewService(root),
		mode:          modeHome,
		homeInput:     home,
		sessionInput:  session,
		viewport:      vp,
		providerName:  "mock",
		providerModel: "odrys-mock-1",
		workspace:     ".",
	}
}

func (m *model) Init() tea.Cmd {
	return m.loadDoctorCmd()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = typed.Width
		m.height = typed.Height
		m.ready = true
		m.resize()
		return m, nil

	case doctorLoadedMsg:
		if typed.err != nil {
			m.errText = typed.err.Error()
			return m, nil
		}
		m.providerName = fallback(typed.payload.Provider.Name, m.providerName)
		m.providerModel = fallback(typed.payload.Provider.Model, m.providerModel)
		m.workspace = fallback(typed.payload.Workspace.Path, m.workspace)
		return m, nil

	case runFinishedMsg:
		m.busy = false
		if typed.err != nil {
			m.appendBlock(m.errorBubble(typed.err.Error()))
			return m, nil
		}
		m.appendBlock(m.runBubble(typed.result))
		return m, nil

	case scanFinishedMsg:
		m.busy = false
		if typed.err != nil {
			m.appendBlock(m.errorBubble(typed.err.Error()))
			return m, nil
		}
		m.appendBlock(m.systemBubble("Workspace", typed.output))
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case modeHome:
			return m.updateHome(typed)
		case modeHelp:
			return m.updateHelp(typed)
		case modeSession:
			return m.updateSession(typed)
		}
	}

	var cmd tea.Cmd
	switch m.mode {
	case modeHome:
		m.homeInput, cmd = m.homeInput.Update(msg)
	case modeSession:
		m.sessionInput, cmd = m.sessionInput.Update(msg)
	}
	return m, cmd
}

func (m *model) View() string {
	if !m.ready {
		return "Cargando Odrys..."
	}

	switch m.mode {
	case modeHelp:
		return m.helpView()
	case modeSession:
		return m.sessionView()
	default:
		return m.homeView()
	}
}

func (m *model) updateHome(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "enter":
		value := strings.TrimSpace(m.homeInput.Value())
		m.homeInput.SetValue("")
		if value == "" {
			return m, nil
		}
		if value == "/exit" || value == "/quit" {
			return m, tea.Quit
		}
		if value == "/help" {
			m.previousMode = modeHome
			m.mode = modeHelp
			return m, nil
		}
		return m.startGoal(value)
	}

	var cmd tea.Cmd
	m.homeInput, cmd = m.homeInput.Update(msg)
	return m, cmd
}

func (m *model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		if m.previousMode == modeSession {
			m.mode = modeSession
			m.sessionInput.Focus()
			return m, nil
		}
		m.mode = modeHome
		m.homeInput.Focus()
		return m, nil
	}
	return m, nil
}

func (m *model) updateSession(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "pgup", "shift+up":
		m.viewport.LineUp(8)
		return m, nil
	case "pgdown", "shift+down":
		m.viewport.LineDown(8)
		return m, nil
	case "up":
		m.viewport.LineUp(1)
		return m, nil
	case "down":
		m.viewport.LineDown(1)
		return m, nil
	case "enter":
		if m.busy {
			return m, nil
		}
		value := strings.TrimSpace(m.sessionInput.Value())
		m.sessionInput.SetValue("")
		if value == "" {
			return m, nil
		}
		return m.submitSession(value)
	}

	var inputCmd tea.Cmd
	var viewportCmd tea.Cmd
	m.sessionInput, inputCmd = m.sessionInput.Update(msg)
	m.viewport, viewportCmd = m.viewport.Update(msg)
	return m, tea.Batch(inputCmd, viewportCmd)
}

func (m *model) startGoal(goal string) (tea.Model, tea.Cmd) {
	m.mode = modeSession
	m.previousMode = modeHome
	m.sessionInput.Focus()
	m.sessionAt = time.Now().UTC().Format(time.RFC3339)
	m.transcript = nil
	m.appendBlock(m.userBubble(goal))
	m.appendBlock(m.agentBadge("Cocinero", m.providerModel, m.providerName))
	m.busy = true
	return m, m.runGoalCmd(goal)
}

func (m *model) submitSession(value string) (tea.Model, tea.Cmd) {
	if strings.HasPrefix(value, "/") {
		switch {
		case value == "/help":
			m.previousMode = modeSession
			m.mode = modeHelp
			return m, nil
		case value == "/clear":
			m.transcript = nil
			m.viewport.SetContent("")
			return m, nil
		case value == "/doctor":
			m.appendBlock(m.systemBubble("Doctor", fmt.Sprintf("provider: %s\nmodel: %s\nworkspace: %s", m.providerName, m.providerModel, m.workspace)))
			return m, nil
		case value == "/scan":
			m.busy = true
			m.appendBlock(m.systemBubble("Workspace", "Escaneando workspace..."))
			return m, m.scanCmd()
		case value == "/exit", value == "/quit":
			return m, tea.Quit
		default:
			m.appendBlock(m.errorBubble("Comando no soportado en esta v1 de Go. Usa /help para ver lo disponible."))
			return m, nil
		}
	}

	m.appendBlock(m.userBubble(value))
	m.appendBlock(m.agentBadge("Cocinero", m.providerModel, m.providerName))
	m.busy = true
	return m, m.runGoalCmd(value)
}

func (m *model) runGoalCmd(goal string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		_ = ctx
		result, err := m.service.Run(goal)
		return runFinishedMsg{result: result, err: err}
	}
}

func (m *model) scanCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = ctx
		output, err := m.service.Scan()
		if err != nil {
			return scanFinishedMsg{err: err}
		}
		raw, marshalErr := json.MarshalIndent(output, "", "  ")
		if marshalErr != nil {
			return scanFinishedMsg{err: marshalErr}
		}
		return scanFinishedMsg{output: string(raw), err: nil}
	}
}

func (m *model) loadDoctorCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = ctx
		payload, err := m.service.Doctor()
		return doctorLoadedMsg{payload: payload, err: err}
	}
}

func (m *model) appendBlock(block string) {
	if block == "" {
		return
	}
	m.transcript = append(m.transcript, block)
	m.viewport.SetContent(strings.Join(m.transcript, "\n"))
	m.viewport.GotoBottom()
}

func (m *model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	m.viewport.Width = max(20, m.width-8)
	availableHeight := m.height - 9
	if availableHeight < 8 {
		availableHeight = 8
	}
	m.viewport.Height = availableHeight
	homeWidth := 46
	if m.width < 72 {
		homeWidth = max(30, m.width-12)
	}
	m.homeInput.Width = homeWidth - 4
	m.sessionInput.Width = max(24, m.width-12)
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var (
	pageStyle = lipgloss.NewStyle().Background(lipgloss.Color("#1a1a1a")).Foreground(lipgloss.Color("#f2f2f2"))

	logoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f2f2f2")).
			Bold(true)

	homeFrameStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#4d8dff")).
			Background(lipgloss.Color("#1f1f1f")).
			Padding(1, 2).
			Width(46)

	homeMetaAgentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4d8dff"))
	homeMetaTextStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#f0f0f0"))
	homeMetaMutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	homeHelpStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#e6e600")).MarginTop(1)

	sessionHeaderStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("#3f3f3f")).
				Background(lipgloss.Color("#171717")).
				Padding(0, 1).
				Width(0)

	sessionViewportStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#121212")).
				Padding(0, 1)

	sessionViewportShellStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#1c1c1c")).
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("#2e2e2e")).
				Padding(0, 1)

	composerAccentStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#4d8dff")).
				Width(1)

	composerShellStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#0b0b0b")).
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("#2d2d2d"))

	composerBodyStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#0b0b0b")).
				Padding(0, 1, 0, 1).
				Width(0)

	composerInputStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#f7f7f7"))

	composerInputFieldStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#111111")).
				Foreground(lipgloss.Color("#f7f7f7")).
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("#333333")).
				Padding(0, 1)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8a8a8a")).
			PaddingLeft(1)

	userCardStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#222222")).
			Foreground(lipgloss.Color("#f4f7fb")).
			Padding(0, 1)

	eventBlockStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#212121")).
			Foreground(lipgloss.Color("#dddddd")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#303030")).
			Padding(0, 1)

	systemCardStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#212121")).
			Foreground(lipgloss.Color("#efefef")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#313131")).
			Padding(0, 1)

	titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#39d98a")).Bold(true)
	infoStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#f2f2f2"))
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6b6b")).Bold(true)
	cyanStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#36c7ff"))
	mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#8f8f8f"))
)

func (m *model) homeView() string {
	logo := logoStyle.Render(strings.Join(logoLines, "\n"))
	meta := lipgloss.JoinHorizontal(
		lipgloss.Top,
		homeMetaAgentStyle.Render("Cocinero"),
		homeMetaTextStyle.Render("  "+m.providerModel),
		homeMetaMutedStyle.Render("  "+m.providerName),
	)
	box := homeFrameStyle.Render(m.homeInput.View() + "\n" + meta)
	help := homeHelpStyle.Render("/help")
	blockWidth := max(lipgloss.Width(logo), lipgloss.Width(box))
	logoRow := lipgloss.PlaceHorizontal(blockWidth, lipgloss.Center, logo)
	boxRow := lipgloss.PlaceHorizontal(blockWidth, lipgloss.Center, box)
	helpRow := lipgloss.PlaceHorizontal(blockWidth, lipgloss.Center, help)

	content := lipgloss.JoinVertical(lipgloss.Left, logoRow, "", boxRow, helpRow)
	return pageStyle.Width(m.width).Height(m.height).Render(
		lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content),
	)
}

func (m *model) helpView() string {
	title := titleStyle.Render("Odrys Help")
	body := infoStyle.Render(strings.Join([]string{
		"",
		"Esta vista ya esta pensada como pantalla separada.",
		"",
		"Comandos disponibles:",
		"/help",
		"/doctor",
		"/scan",
		"/clear",
		"/exit",
		"",
		"Esc o q para volver.",
	}, "\n"))

	panel := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#5c5c5c")).
		Background(lipgloss.Color("#141414")).
		Padding(1, 2).
		Width(max(40, m.width-20)).
		Render(title + "\n" + body)

	return pageStyle.Width(m.width).Height(m.height).Render(
		lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel),
	)
}

func (m *model) sessionView() string {
	headerWidth := max(20, m.width-6)
	header := sessionHeaderStyle.Width(headerWidth).Render("# New session - " + fallback(m.sessionAt, time.Now().UTC().Format(time.RFC3339)))

	mainViewportInner := sessionViewportStyle.Width(max(20, m.width-10)).Render(m.viewport.View())
	mainViewport := sessionViewportShellStyle.Width(max(20, m.width-6)).Render(mainViewportInner)
	footerLeft := "esc interrupt"
	if m.busy {
		footerLeft = "ejecutando..."
	}
	footer := footerStyle.Width(max(20, m.width-6)).Render(footerLeft)

	composerMeta := lipgloss.JoinHorizontal(
		lipgloss.Top,
		homeMetaAgentStyle.Render("Cocinero"),
		homeMetaTextStyle.Render("  "+m.providerModel),
		homeMetaMutedStyle.Render("  "+m.providerName),
	)

	composerBodyWidth := max(20, m.width-9)
	inputField := composerInputFieldStyle.Width(max(12, composerBodyWidth-6)).Render(composerInputStyle.Render(m.sessionInput.View()))
	composerBody := composerBodyStyle.Width(composerBodyWidth).Render(inputField + "\n" + composerMeta)
	composer := composerShellStyle.Width(max(20, m.width-6)).Render(
		lipgloss.JoinHorizontal(lipgloss.Top, composerAccentStyle.Height(lipgloss.Height(composerBody)).Render(""), composerBody),
	)

	body := lipgloss.JoinVertical(lipgloss.Left, header, "", mainViewport, "", composer, "", footer)
	body = lipgloss.JoinVertical(lipgloss.Left, header, mainViewport, composer, footer)
	return pageStyle.Width(m.width).Height(m.height).Render(
		lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, lipgloss.NewStyle().Margin(1, 2, 0, 2).Render(body)),
	)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (m *model) bubbleWidth(limit int) int {
	return max(24, min(limit, m.width-18))
}

func (m *model) userBubble(text string) string {
	bubble := userCardStyle.Width(m.bubbleWidth(28)).Render(text)
	return lipgloss.NewStyle().Width(max(20, m.width-10)).Align(lipgloss.Right).Render(bubble)
}

func (m *model) agentBadge(name, model, provider string) string {
	return eventBlockStyle.Width(m.bubbleWidth(38)).Render(cyanStyle.Render("◻ "+name) + mutedStyle.Render(" · "+model+" "+provider))
}

func (m *model) systemBubble(title, text string) string {
	return systemCardStyle.Width(m.bubbleWidth(76)).Render(titleStyle.Render(title) + "\n" + infoStyle.Render(text))
}

func (m *model) errorBubble(text string) string {
	return systemCardStyle.Width(m.bubbleWidth(76)).BorderForeground(lipgloss.Color("#6b3434")).Render(errorStyle.Render("Error") + "\n" + infoStyle.Render(text))
}

func (m *model) runBubble(result backend.RunResult) string {
	lines := []string{
		titleStyle.Render("Run completado"),
		infoStyle.Render("goal: " + result.Goal),
		infoStyle.Render(fmt.Sprintf("planned: %t", result.Planned)),
		infoStyle.Render("log: " + result.LogPath),
	}

	for _, task := range result.Tasks {
		lines = append(lines, "")
		lines = append(lines, cyanStyle.Render("Tarea: ")+infoStyle.Render(task.Task))
		lines = append(lines, infoStyle.Render("Cocinero: "+task.Executor.Summary))
		if len(task.AppliedOperations) > 0 {
			lines = append(lines, infoStyle.Render("Operaciones aplicadas:"))
			for _, operation := range task.AppliedOperations {
				switch operation.Tool {
				case "write":
					lines = append(lines, infoStyle.Render("- write "+operation.Path))
				case "apply_patch":
					lines = append(lines, infoStyle.Render("- apply_patch"))
				default:
					lines = append(lines, infoStyle.Render("- "+operation.Tool))
				}
			}
		}
		lines = append(lines, infoStyle.Render("Auditor: "+task.Reviewer.Summary))
	}

	return systemCardStyle.Width(m.bubbleWidth(80)).Render(strings.Join(lines, "\n"))
}
