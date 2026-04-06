package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Dasge97/odrys-cli/internal/backend"
	"github.com/Dasge97/odrys-cli/internal/serverclient"
)

type screenMode string

const (
	modeHome    screenMode = "home"
	modeHelp    screenMode = "help"
	modeSessions screenMode = "sessions"
	modeModels  screenMode = "models"
	modeProviders screenMode = "providers"
	modeOpenAIAuth screenMode = "openai_auth"
	modeOpenAIKey screenMode = "openai_key"
	modeOpenAIDevice screenMode = "openai_device"
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

type chatFinishedMsg struct {
	result backend.ChatResult
	err    error
}

type scanFinishedMsg struct {
	output string
	err    error
}

type sessionsLoadedMsg struct {
	items []backend.SessionSummary
	err   error
}

type approvalRequestMsg struct {
	prompt backend.PermissionPrompt
	goal   string
}

type helpMenuItem struct {
	key         string
	title       string
	description string
}

type modelMenuItem struct {
	provider string
	model    string
	title    string
	detail   string
	group    string
}

type providerMenuItem struct {
	key         string
	title       string
	description string
}

type authMenuItem struct {
	key         string
	title       string
	description string
}

type openAIConnectFinishedMsg struct {
	model string
	err   error
}

type openAIDeviceStartedMsg struct {
	session backend.OpenAIDeviceCodeSession
	err     error
}

type openAIDevicePolledMsg struct {
	result backend.OpenAIDeviceCodePollResult
	err    error
}

type serverStatusMsg struct {
	available bool
	openai    backend.OpenAIAuthStatus
}

type clipboardResultMsg struct {
	ok  bool
	err error
}

type model struct {
	root         string
	service      *backend.Service
	serverClient *serverclient.Client
	mode         screenMode
	width        int
	height       int
	ready        bool
	busy         bool
	errText      string
	infoText     string
	homeInput    textinput.Model
	sessionInput textinput.Model
	openAIKeyInput textinput.Model
	viewport     viewport.Model
	currentAgentName string
	providerName string
	providerModel string
	workspace    string
	sessionAt    string
	transcript   []string
	previousMode screenMode
	sessionID    string
	sessionAutoResume bool
	sessions     []backend.SessionSummary
	helpCursor   int
	sessionCursor int
	modelCursor  int
	providerCursor int
	authCursor   int
	openAIDevice *backend.OpenAIDeviceCodeSession
	openAIDeviceAuthID string
	serverAvailable bool
	openAIStatus backend.OpenAIAuthStatus
	pendingApproval *backend.PermissionPrompt
	pendingGoal  string
	permissionOverrides map[string]map[string]string
}

func NewModel(root string) tea.Model {
	home := textinput.New()
	home.Placeholder = "Describe lo que quieres hacer"
	home.CharLimit = 4000
	home.Width = 42
	home.Prompt = ""
	home.Focus()

	session := textinput.New()
	session.Placeholder = "Escribe una instruccion"
	session.CharLimit = 4000
	session.Width = 72
	session.Prompt = ""

	openAIKey := textinput.New()
	openAIKey.Placeholder = "Pega tu OPENAI_API_KEY"
	openAIKey.CharLimit = 4000
	openAIKey.Width = 56
	openAIKey.Prompt = ""
	openAIKey.EchoMode = textinput.EchoPassword
	openAIKey.EchoCharacter = '•'

	vp := viewport.New(80, 20)
	vp.SetContent("")

	return &model{
		root:          root,
		service:       backend.NewService(root),
		serverClient:  serverclient.New(),
		mode:          modeHome,
		homeInput:     home,
		sessionInput:  session,
		openAIKeyInput: openAIKey,
		viewport:      vp,
		currentAgentName: "Odrys",
		providerName:  "mock",
		providerModel: "odrys-mock-1",
		workspace:     ".",
		permissionOverrides: map[string]map[string]string{},
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.loadDoctorCmd(), m.loadSessionsCmd(), m.probeServerCmd())
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
		m.sessionAutoResume = typed.payload.Session.AutoResume
		return m, nil

	case serverStatusMsg:
		m.serverAvailable = typed.available
		if typed.available {
			m.openAIStatus = typed.openai
			if m.providerName == "mock" && typed.openai.Connected {
				if typed.openai.Method == "device_code" || typed.openai.Method == "oauth" {
					m.providerName = "openai"
					m.providerModel = defaultOpenAIAccountModel()
				} else {
					m.providerName = "openai"
					m.providerModel = "gpt-4.1-mini"
				}
				_ = m.service.SaveProvider(backend.ProviderConfig{
					Name:  m.providerName,
					Model: m.providerModel,
				})
			}
		}
		return m, nil

	case clipboardResultMsg:
		if typed.ok {
			m.errText = ""
			m.infoText = "Enlace copiado al portapapeles."
			return m, nil
		}
		if typed.err != nil {
			m.errText = typed.err.Error()
		}
		return m, nil

	case sessionsLoadedMsg:
		if typed.err == nil {
			m.sessions = typed.items
			if m.sessionCursor >= len(m.sessions) && len(m.sessions) > 0 {
				m.sessionCursor = len(m.sessions) - 1
			}
			if len(m.sessions) == 0 {
				m.sessionCursor = 0
			}
			if m.sessionID == "" && len(typed.items) > 0 {
				m.sessionID = typed.items[0].ID
			}
		}
		return m, nil

	case approvalRequestMsg:
		m.busy = false
		m.pendingApproval = &typed.prompt
		m.pendingGoal = typed.goal
		m.appendBlock(m.approvalBubble(typed.prompt))
		return m, nil

	case runFinishedMsg:
		m.busy = false
		if typed.err != nil {
			m.appendBlock(m.errorBubble(typed.err.Error()))
			return m, nil
		}
		if typed.result.SessionID != "" {
			m.sessionID = typed.result.SessionID
		}
		m.appendBlock(m.runBubble(typed.result))
		return m, m.loadSessionsCmd()

	case chatFinishedMsg:
		m.busy = false
		if typed.err != nil {
			m.appendBlock(m.errorBubble(typed.err.Error()))
			return m, nil
		}
		if typed.result.SessionID != "" {
			m.sessionID = typed.result.SessionID
		}
		m.appendBlock(m.assistantBubble(m.currentAgentName, typed.result.Reply))
		return m, m.loadSessionsCmd()

	case scanFinishedMsg:
		m.busy = false
		if typed.err != nil {
			m.appendBlock(m.errorBubble(typed.err.Error()))
			return m, nil
		}
		m.appendBlock(m.systemBubble("Workspace", typed.output))
		return m, nil

	case openAIConnectFinishedMsg:
		m.busy = false
		if typed.err != nil {
			m.errText = typed.err.Error()
			if m.previousMode == modeSession {
				m.mode = modeSession
				m.sessionInput.Focus()
				m.appendBlock(m.errorBubble(typed.err.Error()))
				return m, nil
			}
			m.mode = modeOpenAIKey
			m.openAIKeyInput.Focus()
			return m, nil
		}
		m.errText = ""
		m.infoText = ""
		m.providerName = "openai"
		m.providerModel = fallback(typed.model, "gpt-4.1-mini")
		m.openAIStatus = backend.OpenAIAuthStatus{
			Connected: true,
			Method:    "api_key",
			Provider: backend.ProviderConfig{
				Name:  "openai",
				Model: m.providerModel,
			},
			DeviceCodeAvailable: true,
		}
		m.openAIKeyInput.SetValue("")
		if m.previousMode == modeSession {
			m.mode = modeSession
			m.sessionInput.Focus()
			m.appendBlock(m.systemBubble("OpenAI conectado", fmt.Sprintf("provider: %s\nmodel: %s", m.providerName, m.providerModel)))
			return m, nil
		}
		m.mode = modeHome
		m.homeInput.Focus()
		return m, nil

	case openAIDeviceStartedMsg:
		m.busy = false
		if typed.err != nil {
			m.errText = typed.err.Error()
			if m.previousMode == modeSession {
				m.mode = modeSession
				m.sessionInput.Focus()
				m.appendBlock(m.errorBubble(typed.err.Error()))
				return m, nil
			}
			m.mode = modeOpenAIAuth
			return m, nil
		}
		m.errText = ""
		m.infoText = "Si no se abre el navegador, copia la URL y mete el codigo manualmente."
		m.openAIDevice = &typed.session
		m.openAIDeviceAuthID = typed.session.DeviceAuthID
		m.mode = modeOpenAIDevice
		target := fallback(typed.session.LaunchURL, fallback(typed.session.AuthorizationURL, typed.session.VerificationURL))
		return m, tea.Batch(m.openBrowserCmd(target), m.pollOpenAIDeviceCmd(typed.session))

	case openAIDevicePolledMsg:
		if typed.err != nil {
			m.busy = false
			m.errText = typed.err.Error()
			if m.previousMode == modeSession {
				m.mode = modeSession
				m.sessionInput.Focus()
				m.appendBlock(m.errorBubble(typed.err.Error()))
				return m, nil
			}
			m.mode = modeOpenAIDevice
			return m, nil
		}
		switch typed.result.Status {
		case "pending":
			if m.openAIDevice != nil {
				return m, m.pollOpenAIDeviceCmd(*m.openAIDevice)
			}
			return m, nil
		case "success":
			m.busy = false
			m.errText = ""
			m.infoText = ""
			m.providerName = "openai"
			if m.openAIDevice != nil && strings.TrimSpace(m.openAIDevice.Model) != "" {
				m.providerModel = m.openAIDevice.Model
			}
			if !isOpenAICodexModel(m.providerModel) {
				m.providerModel = defaultOpenAIAccountModel()
				_ = m.service.SaveProvider(backend.ProviderConfig{
					Name:  "openai",
					Model: m.providerModel,
				})
			}
			m.openAIStatus = backend.OpenAIAuthStatus{
				Connected: true,
				Method:    "device_code",
				Provider: backend.ProviderConfig{
					Name:  "openai",
					Model: m.providerModel,
				},
				DeviceCodeAvailable: true,
			}
			m.openAIDevice = nil
			m.openAIDeviceAuthID = ""
			if m.previousMode == modeSession {
				m.mode = modeSession
				m.sessionInput.Focus()
				message := fmt.Sprintf("provider: %s\nmodel: %s", m.providerName, m.providerModel)
				if strings.TrimSpace(typed.result.Email) != "" {
					message += "\ncuenta: " + typed.result.Email
				}
				m.appendBlock(m.systemBubble("OpenAI conectado", message))
				return m, nil
			}
			m.mode = modeHome
			m.homeInput.Focus()
			return m, nil
		case "expired":
			m.openAIDevice = nil
			m.openAIDeviceAuthID = ""
			m.busy = false
			m.errText = "El codigo de OpenAI ha expirado."
			if m.previousMode == modeSession {
				m.mode = modeSession
				m.sessionInput.Focus()
				m.appendBlock(m.errorBubble("El codigo de OpenAI ha expirado."))
				return m, nil
			}
			m.mode = modeOpenAIAuth
			return m, nil
		default:
			return m, nil
		}

	case tea.KeyMsg:
		switch m.mode {
		case modeHome:
			return m.updateHome(typed)
		case modeHelp:
			return m.updateHelp(typed)
		case modeSessions:
			return m.updateSessions(typed)
		case modeModels:
			return m.updateModels(typed)
		case modeProviders:
			return m.updateProviders(typed)
		case modeOpenAIAuth:
			return m.updateOpenAIAuth(typed)
		case modeOpenAIKey:
			return m.updateOpenAIKey(typed)
		case modeOpenAIDevice:
			return m.updateOpenAIDevice(typed)
		case modeSession:
			return m.updateSession(typed)
		}
	}

	var cmd tea.Cmd
	switch m.mode {
	case modeHome:
		m.homeInput, cmd = m.homeInput.Update(msg)
	case modeOpenAIKey:
		m.openAIKeyInput, cmd = m.openAIKeyInput.Update(msg)
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
	case modeSessions:
		return m.sessionsView()
	case modeModels:
		return m.modelsView()
	case modeProviders:
		return m.providersView()
	case modeOpenAIAuth:
		return m.openAIAuthView()
	case modeOpenAIKey:
		return m.openAIKeyView()
	case modeOpenAIDevice:
		return m.openAIDeviceView()
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
	case "ctrl+l":
		m.previousMode = modeHome
		m.mode = modeModels
		return m, nil
	case "ctrl+p", "ctrl+o", "f1":
		m.previousMode = modeHome
		m.mode = modeHelp
		return m, nil
	case "enter":
		value := strings.TrimSpace(m.homeInput.Value())
		m.homeInput.SetValue("")
		if value == "" {
			return m, nil
		}
		if value == "exit" || value == "quit" || value == "/exit" || value == "/quit" {
			return m, tea.Quit
		}
		if value == "/sessions" {
			m.previousMode = modeHome
			m.mode = modeSessions
			return m, m.loadSessionsCmd()
		}
		return m.startChat(value)
	}

	var cmd tea.Cmd
	m.homeInput, cmd = m.homeInput.Update(msg)
	return m, cmd
}

func (m *model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.helpCursor > 0 {
			m.helpCursor--
		}
		return m, nil
	case "down", "j":
		if m.helpCursor < len(m.helpMenuItems())-1 {
			m.helpCursor++
		}
		return m, nil
	case "esc", "q":
		return m.closeOverlay()
	case "enter":
		return m.executeHelpSelection()
	}
	return m, nil
}

func (m *model) updateSessions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.sessionCursor > 0 {
			m.sessionCursor--
		}
		return m, nil
	case "down", "j":
		if m.sessionCursor < len(m.sessions)-1 {
			m.sessionCursor++
		}
		return m, nil
	case "r":
		return m, m.loadSessionsCmd()
	case "esc", "q":
		return m.closeOverlay()
	case "enter":
		if len(m.sessions) == 0 {
			return m.closeOverlay()
		}
		if m.sessionCursor < 0 {
			m.sessionCursor = 0
		}
		if m.sessionCursor >= len(m.sessions) {
			m.sessionCursor = len(m.sessions) - 1
		}
		if err := m.loadSessionIntoView(m.sessions[m.sessionCursor].ID); err != nil {
			m.mode = modeSession
			m.appendBlock(m.errorBubble(err.Error()))
			return m, nil
		}
		m.mode = modeSession
		m.sessionInput.Focus()
		return m, nil
	}
	return m, nil
}

func (m *model) updateModels(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.modelMenuItems()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.modelCursor > 0 {
			m.modelCursor--
		}
		return m, nil
	case "down", "j":
		if m.modelCursor < len(items)-1 {
			m.modelCursor++
		}
		return m, nil
	case "esc", "q":
		return m.closeOverlay()
	case "enter":
		if len(items) == 0 {
			return m.closeOverlay()
		}
		if m.modelCursor < 0 {
			m.modelCursor = 0
		}
		if m.modelCursor >= len(items) {
			m.modelCursor = len(items) - 1
		}
		selected := items[m.modelCursor]
		if selected.provider == "__providers__" {
			m.mode = modeProviders
			return m, nil
		}
		if selected.provider == "openai" && !m.isOpenAIConnected() {
			m.previousMode = m.previousModeFallback()
			m.mode = modeOpenAIAuth
			m.errText = "OpenAI no esta conectado todavia."
			return m, nil
		}
		if err := m.service.SaveProvider(backend.ProviderConfig{
			Name:  selected.provider,
			Model: selected.model,
		}); err != nil {
			if m.previousMode == modeSession {
				m.mode = modeSession
				m.sessionInput.Focus()
				m.appendBlock(m.errorBubble(err.Error()))
				return m, nil
			}
			m.mode = modeHome
			m.homeInput.Focus()
			return m, nil
		}
		m.providerName = selected.provider
		m.providerModel = selected.model
		if m.previousMode == modeSession {
			m.mode = modeSession
			m.sessionInput.Focus()
			m.appendBlock(m.systemBubble("Modelo activo", fmt.Sprintf("provider: %s\nmodel: %s", m.providerName, m.providerModel)))
			return m, nil
		}
		m.mode = modeHome
		m.homeInput.Focus()
		return m, nil
	}
	return m, nil
}

func (m *model) updateProviders(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.providerMenuItems()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.providerCursor > 0 {
			m.providerCursor--
		}
		return m, nil
	case "down", "j":
		if m.providerCursor < len(items)-1 {
			m.providerCursor++
		}
		return m, nil
	case "esc", "q":
		m.mode = modeModels
		return m, nil
	case "enter":
		if len(items) == 0 {
			m.mode = modeModels
			return m, nil
		}
		selected := items[m.providerCursor]
		switch selected.key {
		case "mock":
			if err := m.service.SaveProvider(backend.ProviderConfig{Name: "mock", Model: "odrys-mock-1"}); err != nil {
				return m, nil
			}
			m.providerName = "mock"
			m.providerModel = "odrys-mock-1"
			m.mode = modeModels
			return m, nil
		case "openai":
			if m.isOpenAIConnected() {
				model := "gpt-4.1-mini"
				if m.openAIStatus.Method == "device_code" || m.openAIStatus.Method == "oauth" {
					model = defaultOpenAIAccountModel()
				}
				m.providerName = "openai"
				m.providerModel = model
				if err := m.service.SaveProvider(backend.ProviderConfig{Name: "openai", Model: model}); err != nil {
					m.errText = err.Error()
					return m, nil
				}
				m.mode = modeModels
				m.infoText = "OpenAI activado con la sesion existente."
				m.errText = ""
				return m, nil
			}
			m.errText = ""
			m.mode = modeOpenAIAuth
			return m, m.probeServerCmd()
		default:
			return m, nil
		}
	}
	return m, nil
}

func (m *model) updateOpenAIAuth(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.openAIAuthItems()
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.authCursor > 0 {
			m.authCursor--
		}
		return m, nil
	case "down", "j":
		if m.authCursor < len(items)-1 {
			m.authCursor++
		}
		return m, nil
	case "esc", "q":
		m.mode = modeProviders
		return m, nil
	case "enter":
		selected := items[m.authCursor]
		switch selected.key {
		case "api_key":
			m.mode = modeOpenAIKey
			m.openAIKeyInput.Focus()
			return m, nil
		case "browser_code":
			if !m.serverAvailable {
				m.errText = "ChatGPT Plus/Pro requiere odrys-core activo."
				m.infoText = "Levanta odrys-core y vuelve a intentarlo."
				return m, nil
			}
			if !m.openAIStatus.DeviceCodeAvailable {
				m.errText = fallback(m.openAIStatus.DeviceCodeMessage, "ChatGPT Plus/Pro aun no esta disponible en odrys-core.")
				m.infoText = ""
				return m, nil
			}
			m.errText = ""
			m.infoText = ""
			m.busy = true
			return m, m.startOpenAIDeviceCmd(defaultOpenAIAccountModel())
		default:
			return m, nil
		}
	}
	return m, nil
}

func (m *model) updateOpenAIKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeOpenAIAuth
		return m, nil
	case "enter":
		value := strings.TrimSpace(m.openAIKeyInput.Value())
		if value == "" {
			return m, nil
		}
		m.busy = true
		return m, m.connectOpenAICmd(value, "gpt-4.1-mini")
	}

	var cmd tea.Cmd
	m.openAIKeyInput, cmd = m.openAIKeyInput.Update(msg)
	return m, cmd
}

func (m *model) updateOpenAIDevice(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "c":
		if m.openAIDevice != nil {
			target := fallback(m.openAIDevice.LaunchURL, fallback(m.openAIDevice.AuthorizationURL, m.openAIDevice.VerificationURL))
			if strings.TrimSpace(target) != "" {
				return m, m.copyToClipboardCmd(target)
			}
		}
		return m, nil
	case "o":
		if m.openAIDevice != nil {
			target := fallback(m.openAIDevice.LaunchURL, fallback(m.openAIDevice.AuthorizationURL, m.openAIDevice.VerificationURL))
			if strings.TrimSpace(target) != "" {
				return m, m.openBrowserCmd(target)
			}
		}
		return m, nil
	case "esc", "q":
		m.openAIDevice = nil
		m.mode = modeOpenAIAuth
		return m, nil
	}
	return m, nil
}

func (m *model) updateSession(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pendingApproval != nil {
		switch strings.ToLower(msg.String()) {
		case "y", "enter":
			m.allowPending()
			m.pendingApproval = nil
			m.busy = true
			return m, m.runGoalCmd(m.pendingGoal)
		case "n", "esc":
			m.pendingApproval = nil
			m.pendingGoal = ""
			m.appendBlock(m.errorBubble("Permiso rechazado por el usuario."))
			return m, nil
		}
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "ctrl+l":
		m.previousMode = modeSession
		m.mode = modeModels
		return m, nil
	case "ctrl+p", "ctrl+o", "f1":
		m.previousMode = modeSession
		m.mode = modeHelp
		return m, nil
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

func (m *model) startChat(goal string) (tea.Model, tea.Cmd) {
	m.mode = modeSession
	m.previousMode = modeHome
	m.sessionInput.Focus()
	m.normalizeOpenAIModelForCurrentAuth()
	if m.sessionAutoResume && m.sessionID != "" {
		_ = m.loadSessionIntoView(m.sessionID)
	} else {
		m.sessionAt = time.Now().UTC().Format(time.RFC3339)
		m.transcript = nil
		m.viewport.SetContent("")
	}
	m.appendBlock(m.userBubble(goal))
	m.currentAgentName = "Odrys"
	m.appendBlock(m.agentBadge(m.currentAgentName, m.providerModel, m.providerName))
	m.busy = true
	return m, m.runChatCmd(goal)
}

func (m *model) submitSession(value string) (tea.Model, tea.Cmd) {
	if value == "exit" || value == "quit" {
		return m, tea.Quit
	}
	if strings.HasPrefix(value, "/") {
		switch {
		case strings.HasPrefix(value, "/worker "):
			task := strings.TrimSpace(strings.TrimPrefix(value, "/worker "))
			if task == "" {
				m.appendBlock(m.errorBubble("Uso: /worker <tarea>"))
				return m, nil
			}
			m.normalizeOpenAIModelForCurrentAuth()
			m.currentAgentName = "Cocinero"
			m.appendBlock(m.userBubble(task))
			m.appendBlock(m.agentBadge(m.currentAgentName, m.providerModel, m.providerName))
			m.busy = true
			return m, m.runGoalCmd(task)
		case value == "/clear":
			m.transcript = nil
			m.viewport.SetContent("")
			return m, nil
		case value == "/doctor":
			m.appendBlock(m.systemBubble("Doctor", fmt.Sprintf("provider: %s\nmodel: %s\nworkspace: %s", m.providerName, m.providerModel, m.workspace)))
			return m, nil
		case value == "/sessions":
			m.previousMode = modeSession
			m.mode = modeSessions
			return m, m.loadSessionsCmd()
		case strings.HasPrefix(value, "/resume "):
			target := strings.TrimSpace(strings.TrimPrefix(value, "/resume "))
			if target == "" || target == "latest" {
				if len(m.sessions) == 0 {
					m.appendBlock(m.errorBubble("No hay sesiones para reanudar."))
					return m, nil
				}
				target = m.sessions[0].ID
			}
			if target == "" {
				m.appendBlock(m.errorBubble("Uso: /resume <session_id>"))
				return m, nil
			}
			if err := m.loadSessionIntoView(target); err != nil {
				m.appendBlock(m.errorBubble(err.Error()))
			}
			return m, nil
		case value == "/scan":
			m.busy = true
			m.appendBlock(m.systemBubble("Workspace", "Escaneando workspace..."))
			return m, m.scanCmd()
		case value == "/exit", value == "/quit":
			return m, tea.Quit
		default:
			m.appendBlock(m.errorBubble("Comando no soportado en esta v1 de Go. Usa ctrl+p para abrir el menu."))
			return m, nil
		}
	}

	m.normalizeOpenAIModelForCurrentAuth()
	m.currentAgentName = "Odrys"
	m.appendBlock(m.userBubble(value))
	m.appendBlock(m.agentBadge(m.currentAgentName, m.providerModel, m.providerName))
	m.busy = true
	return m, m.runChatCmd(value)
}

func (m *model) runGoalCmd(goal string) tea.Cmd {
	return func() tea.Msg {
		result, err := m.service.RunWithOptions(goal, backend.RunOptions{
			SessionID: m.sessionID,
			Overrides: m.permissionOverrides,
		})
		var permissionErr *backend.PermissionRequiredError
		if errors.As(err, &permissionErr) {
			return approvalRequestMsg{prompt: permissionErr.Prompt, goal: goal}
		}
		return runFinishedMsg{result: result, err: err}
	}
}

func (m *model) runChatCmd(goal string) tea.Cmd {
	return func() tea.Msg {
		result, err := m.service.ChatWithOptions(goal, backend.RunOptions{
			SessionID: m.sessionID,
			Overrides: m.permissionOverrides,
		})
		return chatFinishedMsg{result: result, err: err}
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

func (m *model) loadSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		var (
			items []backend.SessionSummary
			err error
		)
		if m.serverAvailable {
			items, err = m.serverClient.ListSessions()
		} else {
			items, err = m.service.Sessions(8)
		}
		return sessionsLoadedMsg{items: items, err: err}
	}
}

func (m *model) connectOpenAICmd(apiKey, model string) tea.Cmd {
	return func() tea.Msg {
		var err error
		if m.serverAvailable {
			err = m.serverClient.ConnectOpenAIAPIKey(apiKey, model)
		} else {
			err = m.service.ConnectOpenAI(apiKey, model)
		}
		return openAIConnectFinishedMsg{model: model, err: err}
	}
}

func (m *model) startOpenAIDeviceCmd(model string) tea.Cmd {
	return func() tea.Msg {
		var (
			session backend.OpenAIDeviceCodeSession
			err error
		)
		if m.serverAvailable {
			var authID string
			authID, session, err = m.serverClient.StartOpenAIDevice(model)
			session.DeviceAuthID = authID
		} else {
			session, err = m.service.StartOpenAIDeviceCode(model)
		}
		return openAIDeviceStartedMsg{session: session, err: err}
	}
}

func (m *model) openBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if strings.TrimSpace(url) == "" {
			return nil
		}
		for _, name := range []string{"xdg-open", "open"} {
			if err := exec.Command(name, url).Start(); err == nil {
				return nil
			}
		}
		return nil
	}
}

func (m *model) copyToClipboardCmd(value string) tea.Cmd {
	return func() tea.Msg {
		for _, candidate := range []struct {
			name string
			args []string
		}{
			{name: "clip.exe"},
			{name: "wl-copy"},
			{name: "xclip", args: []string{"-selection", "clipboard"}},
			{name: "pbcopy"},
		} {
			cmd := exec.Command(candidate.name, candidate.args...)
			cmd.Stdin = strings.NewReader(value)
			if err := cmd.Run(); err == nil {
				return clipboardResultMsg{ok: true}
			}
		}
		return clipboardResultMsg{err: errors.New("no pude copiar al portapapeles en este entorno")}
	}
}

func (m *model) pollOpenAIDeviceCmd(session backend.OpenAIDeviceCodeSession) tea.Cmd {
	interval := session.IntervalSeconds
	if interval <= 0 {
		interval = 5
	}
	return tea.Tick(time.Duration(interval)*time.Second, func(time.Time) tea.Msg {
		var (
			result backend.OpenAIDeviceCodePollResult
			err error
		)
		if m.serverAvailable && strings.TrimSpace(session.DeviceAuthID) != "" {
			result, err = m.serverClient.PollOpenAIDevice(session.DeviceAuthID)
		} else {
			result, err = m.service.PollOpenAIDeviceCode(session)
		}
		return openAIDevicePolledMsg{result: result, err: err}
	})
}

func (m *model) probeServerCmd() tea.Cmd {
	return func() tea.Msg {
		if !m.serverClient.Available() {
			return serverStatusMsg{available: false}
		}
		status, err := m.serverClient.OpenAIStatus()
		if err != nil {
			return serverStatusMsg{available: true}
		}
		return serverStatusMsg{available: true, openai: status}
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

func trimForDisplay(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit < 8 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func (m *model) isOpenAIConnected() bool {
	return m.openAIStatus.Connected
}

func (m *model) providerStatusText() string {
	switch m.providerName {
	case "mock":
		return "local"
	case "openai":
		if m.isOpenAIConnected() {
			if strings.TrimSpace(m.openAIStatus.Method) != "" {
				return "conectado (" + m.openAIStatus.Method + ")"
			}
			return "conectado"
		}
		return "sin conectar"
	default:
		return "desconocido"
	}
}

func (m *model) isOpenAIAccountAuth() bool {
	return m.providerName == "openai" && (m.openAIStatus.Method == "device_code" || m.openAIStatus.Method == "oauth")
}

func isOpenAICodexModel(model string) bool {
	switch strings.TrimSpace(model) {
	case "gpt-5.1-codex-max", "gpt-5.1-codex", "gpt-5.1-codex-mini":
		return true
	default:
		return false
	}
}

func defaultOpenAIAccountModel() string {
	return "gpt-5.1-codex-max"
}

func (m *model) normalizeOpenAIModelForCurrentAuth() {
	if m.providerName != "openai" || !m.isOpenAIAccountAuth() {
		return
	}
	if isOpenAICodexModel(m.providerModel) {
		return
	}
	m.providerModel = defaultOpenAIAccountModel()
	_ = m.service.SaveProvider(backend.ProviderConfig{
		Name:  "openai",
		Model: m.providerModel,
	})
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
	menuPanelStyle     = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("#454545")).
				Background(lipgloss.Color("#141414")).
				Padding(1, 2)
	menuTitleStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#f2f2f2")).Bold(true)
	menuHintStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#8f8f8f"))
	menuItemStyle      = lipgloss.NewStyle().
				Background(lipgloss.Color("#1d1d1d")).
				Foreground(lipgloss.Color("#dcdcdc")).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#303030")).
				Padding(0, 1)
	menuItemActiveStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#1d2330")).
				Foreground(lipgloss.Color("#f4f7fb")).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#4d8dff")).
				Padding(0, 1)

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
	statusText := m.providerStatusText()
	logo := logoStyle.Render(strings.Join(logoLines, "\n"))
	statusBar := systemCardStyle.
		Width(46).
		Render(cyanStyle.Render("Provider") + infoStyle.Render(": "+m.providerName+"  ") + cyanStyle.Render("Modelo") + infoStyle.Render(": "+m.providerModel) + "\n" + mutedStyle.Render("Estado: "+statusText))
	meta := lipgloss.JoinHorizontal(
		lipgloss.Top,
		homeMetaAgentStyle.Render("Odrys"),
		homeMetaTextStyle.Render("  "+m.providerModel),
		homeMetaMutedStyle.Render("  "+m.providerName),
	)
	box := homeFrameStyle.Render(m.homeInput.View() + "\n" + meta)
	help := homeHelpStyle.Render("ctrl+o help  ctrl+l modelos")
	if m.serverAvailable {
		help = homeHelpStyle.Render("ctrl+o help  ctrl+l modelos  core on")
	} else {
		help = homeHelpStyle.Render("ctrl+o help  ctrl+l modelos  core off")
	}
	blockWidth := max(lipgloss.Width(logo), lipgloss.Width(box))
	logoRow := lipgloss.PlaceHorizontal(blockWidth, lipgloss.Center, logo)
	statusRow := lipgloss.PlaceHorizontal(blockWidth, lipgloss.Center, statusBar)
	boxRow := lipgloss.PlaceHorizontal(blockWidth, lipgloss.Center, box)
	helpRow := lipgloss.PlaceHorizontal(blockWidth, lipgloss.Center, help)

	extra := ""
	if strings.TrimSpace(m.errText) != "" {
		extra = errorStyle.Render(m.errText)
	} else if strings.TrimSpace(m.infoText) != "" {
		extra = menuHintStyle.Render(m.infoText)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, logoRow, "", statusRow, "", boxRow, helpRow, extra)
	return pageStyle.Width(m.width).Height(m.height).Render(
		lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content),
	)
}

func (m *model) helpView() string {
	items := m.helpMenuItems()
	width := max(34, m.width-28)
	rows := []string{menuTitleStyle.Render("Odrys Menu"), menuHintStyle.Render("Flechas + enter. Esc para volver. Abre con ctrl+o o f1.")}
	for index, item := range items {
		card := lipgloss.NewStyle().Foreground(lipgloss.Color("#dcdcdc"))
		prefix := "  "
		if index == m.helpCursor {
			card = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#f4f7fb")).
				Background(lipgloss.Color("#1d2330"))
			prefix = "› "
		}
		rows = append(rows, card.Width(width-4).Render(prefix+item.title+"  "+menuHintStyle.Render("· "+item.description)))
	}
	panel := menuPanelStyle.
		Padding(0, 1).
		Width(width).
		Render(strings.Join(rows, "\n"))

	return pageStyle.Width(m.width).Height(m.height).Render(
		lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel),
	)
}

func (m *model) sessionsView() string {
	width := max(42, m.width-28)
	rows := []string{
		menuTitleStyle.Render("Sesiones"),
		menuHintStyle.Render("Flechas para navegar, enter para abrir, r para refrescar, esc para volver."),
	}
	if len(m.sessions) == 0 {
		rows = append(rows, menuHintStyle.Render("No hay sesiones guardadas."))
	} else {
		for index, session := range m.sessions {
			card := lipgloss.NewStyle().Foreground(lipgloss.Color("#dcdcdc"))
			prefix := "  "
			if index == m.sessionCursor {
				card = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#f4f7fb")).
					Background(lipgloss.Color("#1d2330"))
				prefix = "› "
			}
			label := fmt.Sprintf("%s%s  %s  %s", prefix, shortSessionID(session.ID), shortSessionDate(session.UpdatedAt), compactSessionTitle(session.Title))
			rows = append(rows, card.Width(width-4).Render(label))
		}
	}
	panel := menuPanelStyle.Width(width).Render(strings.Join(rows, "\n"))
	return pageStyle.Width(m.width).Height(m.height).Render(
		lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel),
	)
}

func (m *model) modelsView() string {
	items := m.modelMenuItems()
	width := max(42, m.width-28)
	rows := []string{
		menuTitleStyle.Render("Modelos"),
		menuHintStyle.Render("Flechas + enter para activar. Esc para volver."),
	}
	currentGroup := ""
	for index, item := range items {
		if item.group != currentGroup {
			currentGroup = item.group
			rows = append(rows, "")
			rows = append(rows, homeMetaAgentStyle.Render(currentGroup))
		}
		card := lipgloss.NewStyle().Foreground(lipgloss.Color("#dcdcdc"))
		prefix := "  "
		if index == m.modelCursor {
			card = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#f4f7fb")).
				Background(lipgloss.Color("#1d2330"))
			prefix = "› "
		}
		status := ""
		if item.provider == m.providerName && item.model == m.providerModel {
			status = "  activo"
		}
		label := fmt.Sprintf("%s%s  %s  %s%s", prefix, item.title, item.model, item.detail, status)
		rows = append(rows, card.Width(width-4).Render(label))
	}
	panel := menuPanelStyle.Width(width).Render(strings.Join(rows, "\n"))
	return pageStyle.Width(m.width).Height(m.height).Render(
		lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel),
	)
}

func (m *model) providersView() string {
	items := m.providerMenuItems()
	width := max(42, m.width-28)
	rows := []string{
		menuTitleStyle.Render("Providers"),
		menuHintStyle.Render("Flechas + enter para activar. Esc para volver."),
	}
	for index, item := range items {
		card := lipgloss.NewStyle().Foreground(lipgloss.Color("#dcdcdc"))
		prefix := "  "
		if index == m.providerCursor {
			card = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#f4f7fb")).
				Background(lipgloss.Color("#1d2330"))
			prefix = "› "
		}
		status := ""
		if item.key == "mock" {
			if m.providerName == "mock" {
				status = "  activo · local"
			} else {
				status = "  local"
			}
		}
		if item.key == "openai" {
			if m.isOpenAIConnected() {
				if m.providerName == "openai" {
					status = "  activo · conectado"
				} else {
					status = "  conectado"
				}
			} else {
				if m.providerName == "openai" {
					status = "  activo · sin conectar"
				} else {
					status = "  sin conectar"
				}
			}
		}
		rows = append(rows, card.Width(width-4).Render(prefix+item.title+"  "+item.description+status))
	}
	panel := menuPanelStyle.Width(width).Render(strings.Join(rows, "\n"))
	return pageStyle.Width(m.width).Height(m.height).Render(
		lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel),
	)
}

func (m *model) openAIAuthView() string {
	items := m.openAIAuthItems()
	width := max(42, m.width-28)
	rows := []string{
		menuTitleStyle.Render("Conectar OpenAI"),
		menuHintStyle.Render("Estado actual: "+m.providerStatusText()+" · Esc para volver."),
	}
	for index, item := range items {
		card := lipgloss.NewStyle().Foreground(lipgloss.Color("#dcdcdc"))
		prefix := "  "
		if index == m.authCursor {
			card = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#f4f7fb")).
				Background(lipgloss.Color("#1d2330"))
			prefix = "› "
		}
		rows = append(rows, card.Width(width-4).Render(prefix+item.title+"  "+item.description))
	}
	if strings.TrimSpace(m.errText) != "" {
		rows = append(rows, "")
		rows = append(rows, errorStyle.Render(m.errText))
	}
	panel := menuPanelStyle.Width(width).Render(strings.Join(rows, "\n"))
	return pageStyle.Width(m.width).Height(m.height).Render(
		lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel),
	)
}

func (m *model) openAIKeyView() string {
	width := max(42, m.width-28)
	body := strings.Join([]string{
		menuTitleStyle.Render("OpenAI API key"),
		menuHintStyle.Render("Pega tu key y pulsa enter. Esc para volver."),
		"",
		composerInputFieldStyle.Width(width-8).Render(m.openAIKeyInput.View()),
		"",
		menuHintStyle.Render("Se guardara en .env local y se activara OpenAI con gpt-4.1-mini."),
	}, "\n")
	if strings.TrimSpace(m.errText) != "" {
		body += "\n\n" + errorStyle.Render(m.errText)
	}
	panel := menuPanelStyle.Width(width).Render(body)
	return pageStyle.Width(m.width).Height(m.height).Render(
		lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel),
	)
}

func (m *model) openAIDeviceView() string {
	width := max(42, m.width-28)
	lines := []string{
		menuTitleStyle.Render("Conectar OpenAI por codigo"),
		menuHintStyle.Render("Selecciona una accion rapida o usa las teclas. Esc para volver."),
	}
	if m.openAIDevice != nil {
		lines = append(lines, "")
		lines = append(lines, systemCardStyle.Width(width-6).Render(
			cyanStyle.Render("Acciones") + "\n" +
				infoStyle.Render("o  abrir navegador") + "\n" +
				infoStyle.Render("c  copiar enlace corto") + "\n" +
				infoStyle.Render("esc  volver"),
		))
		lines = append(lines, "")
		if strings.TrimSpace(m.openAIDevice.LaunchURL) != "" {
			lines = append(lines, cyanStyle.Render("Manual: ")+infoStyle.Render(m.openAIDevice.LaunchURL))
		}
		authURL := fallback(m.openAIDevice.AuthorizationURL, m.openAIDevice.VerificationURL)
		lines = append(lines, mutedStyle.Render("Directa: ")+menuHintStyle.Render(trimForDisplay(authURL, max(32, width-10))))
		if strings.TrimSpace(m.openAIDevice.UserCode) != "" {
			lines = append(lines, cyanStyle.Render("Codigo: ")+infoStyle.Render(m.openAIDevice.UserCode))
		}
		lines = append(lines, mutedStyle.Render("Modelo: "+m.openAIDevice.Model))
		lines = append(lines, mutedStyle.Render("Caduca aprox: "+m.openAIDevice.ExpiresAt))
		lines = append(lines, "")
		lines = append(lines, menuHintStyle.Render("La URL manual corta abre la autorizacion de OpenAI sin tener que copiar el enlace largo."))
	}
	if strings.TrimSpace(m.infoText) != "" {
		lines = append(lines, "")
		lines = append(lines, menuHintStyle.Render(m.infoText))
	}
	if strings.TrimSpace(m.errText) != "" {
		lines = append(lines, "")
		lines = append(lines, errorStyle.Render(m.errText))
	}
	panel := menuPanelStyle.Width(width).Render(strings.Join(lines, "\n"))
	return pageStyle.Width(m.width).Height(m.height).Render(
		lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel),
	)
}

func (m *model) sessionView() string {
	headerWidth := max(20, m.width-6)
	headerText := "# New session - " + fallback(m.sessionAt, time.Now().UTC().Format(time.RFC3339))
	headerMeta := mutedStyle.Render("  [" + m.providerName + " · " + m.providerModel + " · " + m.providerStatusText() + "]")
	header := sessionHeaderStyle.Width(headerWidth).Render(headerText + headerMeta)

	mainViewportInner := sessionViewportStyle.Width(max(20, m.width-10)).Render(m.viewport.View())
	mainViewport := sessionViewportShellStyle.Width(max(20, m.width-6)).Render(mainViewportInner)
	footerLeft := "esc interrupt"
	if m.busy {
		footerLeft = "ejecutando..."
	}
	footer := footerStyle.Width(max(20, m.width-6)).Render(footerLeft)

	composerMeta := lipgloss.JoinHorizontal(
		lipgloss.Top,
		homeMetaAgentStyle.Render(m.currentAgentName),
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

func (m *model) assistantBubble(name, text string) string {
	bubble := eventBlockStyle.Width(m.bubbleWidth(76)).Render(cyanStyle.Render(name) + "\n" + infoStyle.Render(text))
	return lipgloss.NewStyle().Width(max(20, m.width-10)).Align(lipgloss.Left).Render(bubble)
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

func (m *model) sessionsBubble() string {
	if len(m.sessions) == 0 {
		return m.systemBubble("Sesiones", "No hay sesiones guardadas.")
	}
	lines := []string{}
	for _, session := range m.sessions {
		lines = append(lines, fmt.Sprintf("- %s  %s", session.ID, session.Title))
	}
	return m.systemBubble("Sesiones", strings.Join(lines, "\n"))
}

func (m *model) helpMenuItems() []helpMenuItem {
	return []helpMenuItem{
		{key: "openai", title: "Conectar OpenAI", description: "Guia para activar OpenAI y usarlo como provider principal."},
		{key: "worker_hint", title: "Usar worker", description: "Escribe /worker <tarea> para usar el flujo estructurado."},
		{key: "doctor", title: "Ver estado del sistema", description: "Muestra provider, modelo y workspace actual."},
		{key: "scan", title: "Escanear workspace", description: "Lanza un snapshot rapido del repositorio activo."},
		{key: "sessions", title: "Abrir sesiones", description: "Entra en el navegador de sesiones persistentes."},
		{key: "resume_latest", title: "Reanudar ultima sesion", description: "Carga directamente el ultimo historial guardado."},
		{key: "new", title: "Nueva sesion", description: "Vuelve a la portada para iniciar una conversacion nueva."},
		{key: "exit", title: "Salir", description: "Cierra Odrys."},
	}
}

func (m *model) modelMenuItems() []modelMenuItem {
	items := []modelMenuItem{
		{provider: "__providers__", model: "", title: "Provider", detail: "elige y conecta provider", group: "Config"},
		{provider: "mock", model: "odrys-mock-1", title: "Mock local", detail: "sin red", group: "Mock"},
	}
	if m.isOpenAIAccountAuth() {
		items = append(items,
			modelMenuItem{provider: "openai", model: "gpt-5.1-codex-max", title: "GPT-5.1 Codex Max", detail: "default recomendado", group: "OpenAI Cuenta"},
			modelMenuItem{provider: "openai", model: "gpt-5.1-codex", title: "GPT-5.1 Codex", detail: "equilibrado", group: "OpenAI Cuenta"},
			modelMenuItem{provider: "openai", model: "gpt-5.1-codex-mini", title: "GPT-5.1 Codex Mini", detail: "rapido", group: "OpenAI Cuenta"},
		)
		return items
	}
	items = append(items,
		modelMenuItem{provider: "openai", model: "gpt-4.1-mini", title: "GPT-4.1 mini", detail: "rapido", group: "OpenAI API"},
		modelMenuItem{provider: "openai", model: "gpt-4.1", title: "GPT-4.1", detail: "equilibrado", group: "OpenAI API"},
		modelMenuItem{provider: "openai", model: "gpt-5-mini", title: "GPT-5 mini", detail: "razonamiento ligero", group: "OpenAI API"},
	)
	return items
}

func (m *model) providerMenuItems() []providerMenuItem {
	return []providerMenuItem{
		{key: "mock", title: "Mock local", description: "sin red"},
		{key: "openai", title: "OpenAI", description: "cuenta o API key"},
	}
}

func (m *model) openAIAuthItems() []authMenuItem {
	return []authMenuItem{
		{key: "api_key", title: "API key", description: "conectar pegando OPENAI_API_KEY"},
		{key: "browser_code", title: "ChatGPT Plus/Pro", description: "conectar con codigo usando tu cuenta de ChatGPT"},
	}
}

func (m *model) approvalBubble(prompt backend.PermissionPrompt) string {
	return m.systemBubble("Permiso requerido", prompt.Message+"\n\nPulsa y para permitir o n para rechazar.")
}

func (m *model) allowPending() {
	if m.pendingApproval == nil {
		return
	}
	if m.permissionOverrides[m.pendingApproval.Action] == nil {
		m.permissionOverrides[m.pendingApproval.Action] = map[string]string{}
	}
	m.permissionOverrides[m.pendingApproval.Action][m.pendingApproval.Target] = "allow"
}

func (m *model) loadSessionIntoView(sessionID string) error {
	var (
		session backend.Session
		err error
	)
	if m.serverAvailable {
		session, err = m.serverClient.LoadSession(sessionID)
	} else {
		session, err = m.service.LoadSession(sessionID)
	}
	if err != nil {
		return err
	}
	m.sessionID = session.ID
	m.sessionAt = session.CreatedAt
	m.transcript = nil
	m.viewport.SetContent("")
	for _, message := range session.Messages {
		switch message.Role {
		case "user":
			m.appendBlock(m.userBubble(message.Content))
		case "assistant":
			m.appendBlock(m.assistantBubble("Odrys", message.Content))
		default:
			m.appendBlock(m.systemBubble("Sesion", message.Content))
		}
	}
	return nil
}

func (m *model) closeOverlay() (tea.Model, tea.Cmd) {
	if m.previousMode == modeSession {
		m.mode = modeSession
		m.sessionInput.Focus()
		return m, nil
	}
	m.mode = modeHome
	m.homeInput.Focus()
	return m, nil
}

func (m *model) executeHelpSelection() (tea.Model, tea.Cmd) {
	items := m.helpMenuItems()
	if len(items) == 0 {
		return m, nil
	}
	if m.helpCursor < 0 {
		m.helpCursor = 0
	}
	if m.helpCursor >= len(items) {
		m.helpCursor = len(items) - 1
	}
	switch items[m.helpCursor].key {
	case "doctor":
		m.mode = modeSession
		m.previousMode = modeSession
		m.sessionInput.Focus()
		m.appendBlock(m.systemBubble("Doctor", fmt.Sprintf("provider: %s\nmodel: %s\nworkspace: %s", m.providerName, m.providerModel, m.workspace)))
		return m, nil
	case "openai":
		m.previousMode = m.previousModeFallback()
		m.mode = modeProviders
		return m, nil
	case "worker_hint":
		m.mode = modeSession
		m.previousMode = modeSession
		m.currentAgentName = "Odrys"
		m.sessionInput.Focus()
		m.appendBlock(m.systemBubble("Worker", "Usa /worker <tarea> cuando quieras pasar por Cocinero, Auditor y Caja."))
		return m, nil
	case "scan":
		m.mode = modeSession
		m.previousMode = modeSession
		m.sessionInput.Focus()
		m.busy = true
		m.appendBlock(m.systemBubble("Workspace", "Escaneando workspace..."))
		return m, m.scanCmd()
	case "sessions":
		m.previousMode = m.previousModeFallback()
		m.mode = modeSessions
		return m, m.loadSessionsCmd()
	case "resume_latest":
		if len(m.sessions) == 0 {
			m.previousMode = modeSession
			m.mode = modeSession
			m.sessionInput.Focus()
			m.appendBlock(m.errorBubble("No hay sesiones para reanudar."))
			return m, nil
		}
		if err := m.loadSessionIntoView(m.sessions[0].ID); err != nil {
			m.mode = modeSession
			m.sessionInput.Focus()
			m.appendBlock(m.errorBubble(err.Error()))
			return m, nil
		}
		m.mode = modeSession
		m.sessionInput.Focus()
		return m, nil
	case "new":
		m.mode = modeHome
		m.currentAgentName = "Odrys"
		m.homeInput.Focus()
		return m, nil
	case "exit":
		return m, tea.Quit
	default:
		return m, nil
	}
}

func (m *model) previousModeFallback() screenMode {
	if m.previousMode == modeSession {
		return modeSession
	}
	return modeHome
}

func shortSessionID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func oneLine(text string) string {
	line := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	runes := []rune(line)
	if len(runes) > 72 {
		return string(runes[:72]) + "..."
	}
	return line
}

func shortSessionDate(value string) string {
	if len(value) >= 16 {
		return value[:16]
	}
	return value
}

func compactSessionTitle(value string) string {
	title := oneLine(value)
	runes := []rune(title)
	if len(runes) > 40 {
		return string(runes[:40]) + "..."
	}
	return title
}
