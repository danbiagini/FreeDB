package remotes

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/danbiagini/FreeDB/tui/internal/incus"
)

type subview int

const (
	subviewList subview = iota
	subviewAddName
	subviewAddAddr
	subviewAddAuthType
	subviewAddUsername
	subviewAddPassword
	subviewAddConfirm
	subviewConfirmDelete
)

type authType int

const (
	authNone authType = iota
	authUserPass
)

type refreshResult struct {
	remotes []incus.RemoteInfo
	err     error
}

type actionResult struct {
	msg string
	err error
}

type Model struct {
	subview     subview
	incusClient *incus.Client
	remotes     []incus.RemoteInfo
	selected    int
	inputs      []textinput.Model
	authChoice  authType
	message     string
	err         error
	done        bool
	busy        bool
}

func NewModel(ic *incus.Client) Model {
	inputs := make([]textinput.Model, 4)

	inputs[0] = textinput.New()
	inputs[0].Placeholder = "myregistry"
	inputs[0].CharLimit = 30

	inputs[1] = textinput.New()
	inputs[1].Placeholder = "us-central1-docker.pkg.dev"
	inputs[1].CharLimit = 100

	inputs[2] = textinput.New()
	inputs[2].Placeholder = "username"
	inputs[2].CharLimit = 100

	inputs[3] = textinput.New()
	inputs[3].Placeholder = "password or token"
	inputs[3].CharLimit = 200
	inputs[3].EchoMode = textinput.EchoPassword

	return Model{
		subview:     subviewList,
		incusClient: ic,
		inputs:      inputs,
	}
}

func (m Model) Done() bool { return m.done }

func (m Model) Init() tea.Cmd {
	return m.fetchRemotes()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshResult:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.remotes = msg.remotes
			m.err = nil
		}
		return m, nil

	case actionResult:
		m.busy = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.message = msg.msg
			m.err = nil
		}
		return m, m.fetchRemotes()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Update active text input
	if m.subview >= subviewAddName && m.subview <= subviewAddPassword {
		idx := int(m.subview) - int(subviewAddName)
		if idx >= 0 && idx < len(m.inputs) {
			var cmd tea.Cmd
			m.inputs[idx], cmd = m.inputs[idx].Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch m.subview {
	case subviewList:
		switch key {
		case "esc":
			m.done = true
			return m, nil
		case "a":
			m.subview = subviewAddName
			m.inputs[0].SetValue("")
			m.inputs[0].Focus()
			m.message = ""
			m.err = nil
			return m, textinput.Blink
		case "d":
			if len(m.remotes) > 0 && m.selected < len(m.remotes) {
				m.subview = subviewConfirmDelete
			}
			return m, nil
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
			return m, nil
		case "down", "j":
			if m.selected < len(m.remotes)-1 {
				m.selected++
			}
			return m, nil
		}

	case subviewAddName:
		switch key {
		case "esc":
			m.subview = subviewList
			return m, nil
		case "enter":
			if strings.TrimSpace(m.inputs[0].Value()) == "" {
				m.err = fmt.Errorf("name is required")
				return m, nil
			}
			m.err = nil
			m.subview = subviewAddAddr
			m.inputs[1].SetValue("")
			m.inputs[1].Focus()
			return m, nil
		}

	case subviewAddAddr:
		switch key {
		case "esc":
			m.subview = subviewList
			return m, nil
		case "enter":
			if strings.TrimSpace(m.inputs[1].Value()) == "" {
				m.err = fmt.Errorf("address is required")
				return m, nil
			}
			m.err = nil
			m.subview = subviewAddAuthType
			return m, nil
		}

	case subviewAddAuthType:
		switch key {
		case "esc":
			m.subview = subviewList
			return m, nil
		case "1":
			m.authChoice = authNone
			m.subview = subviewAddConfirm
			return m, nil
		case "2":
			m.authChoice = authUserPass
			m.subview = subviewAddUsername
			m.inputs[2].SetValue("")
			m.inputs[2].Focus()
			return m, nil
		}

	case subviewAddUsername:
		switch key {
		case "esc":
			m.subview = subviewList
			return m, nil
		case "enter":
			m.subview = subviewAddPassword
			m.inputs[3].SetValue("")
			m.inputs[3].Focus()
			return m, nil
		}

	case subviewAddPassword:
		switch key {
		case "esc":
			m.subview = subviewList
			return m, nil
		case "enter":
			m.subview = subviewAddConfirm
			return m, nil
		}

	case subviewAddConfirm:
		switch key {
		case "esc":
			m.subview = subviewList
			return m, nil
		case "enter":
			m.busy = true
			m.subview = subviewList
			return m, m.addRemote()
		}

	case subviewConfirmDelete:
		switch key {
		case "y":
			m.busy = true
			m.subview = subviewList
			return m, m.deleteRemote()
		case "n", "esc":
			m.subview = subviewList
			return m, nil
		}
	}

	return m, nil
}

func (m Model) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).MarginBottom(1)
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("Container Registries"))
	b.WriteString("\n\n")

	switch m.subview {
	case subviewAddName, subviewAddAddr, subviewAddAuthType, subviewAddUsername, subviewAddPassword, subviewAddConfirm:
		return m.viewAdd(titleStyle, labelStyle, dimStyle, errStyle)
	}

	if len(m.remotes) == 0 {
		b.WriteString(dimStyle.Render("  No OCI remotes configured"))
		b.WriteString("\n")
	} else {
		for i, r := range m.remotes {
			auth := "public"
			if r.HasAuth {
				auth = "authenticated"
			}
			line := fmt.Sprintf("  %-16s %-40s %s", r.Name, r.Addr, auth)
			if i == m.selected {
				b.WriteString(selectedStyle.Render(line))
			} else {
				b.WriteString(line)
			}
			b.WriteString("\n")
		}
	}

	if m.subview == subviewConfirmDelete && m.selected < len(m.remotes) {
		b.WriteString("\n")
		b.WriteString(warnStyle.Render(fmt.Sprintf("  Delete remote %q? [y/n]", m.remotes[m.selected].Name)))
		return b.String()
	}

	b.WriteString("\n")

	if m.busy {
		b.WriteString("  Working...\n")
	}
	if m.err != nil {
		b.WriteString(errStyle.Render(fmt.Sprintf("  Error: %v", m.err)) + "\n")
	}
	if m.message != "" {
		b.WriteString(successStyle.Render("  "+m.message) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  [a] Add remote  [d] Delete  [esc] Back"))

	return b.String()
}

func (m Model) viewAdd(titleStyle, labelStyle, dimStyle, errStyle lipgloss.Style) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Add Registry"))
	b.WriteString("\n\n")

	labels := []string{"Name:", "Address:"}
	for i, label := range labels {
		sv := subviewAddName + subview(i)
		if m.subview > sv {
			b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render(label), m.inputs[i].Value()))
		} else if m.subview == sv {
			b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render(label), m.inputs[i].View()))
		}
	}

	if m.subview == subviewAddAuthType {
		b.WriteString("\n  Authentication:\n")
		b.WriteString("    [1] None (public registry)\n")
		b.WriteString("    [2] Username / token\n")
	}

	if m.subview > subviewAddAuthType {
		if m.authChoice == authNone {
			b.WriteString(fmt.Sprintf("  %s none\n", labelStyle.Render("Auth:")))
		} else {
			if m.subview > subviewAddUsername {
				b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Username:"), m.inputs[2].Value()))
			} else if m.subview == subviewAddUsername {
				b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Username:"), m.inputs[2].View()))
			}
			if m.subview > subviewAddPassword {
				b.WriteString(fmt.Sprintf("  %s ****\n", labelStyle.Render("Password:")))
			} else if m.subview == subviewAddPassword {
				b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Password:"), m.inputs[3].View()))
			}
		}
	}

	if m.subview == subviewAddConfirm {
		b.WriteString("\n  Press [enter] to add, [esc] to cancel\n")
	}

	if m.err != nil {
		b.WriteString("\n" + errStyle.Render(fmt.Sprintf("  %v", m.err)))
	}

	b.WriteString(dimStyle.Render("\n\n  [esc] Cancel"))
	return b.String()
}

func (m Model) fetchRemotes() tea.Cmd {
	ic := m.incusClient
	return func() tea.Msg {
		remotes, err := ic.ListRemotes()
		return refreshResult{remotes: remotes, err: err}
	}
}

func (m Model) addRemote() tea.Cmd {
	name := strings.TrimSpace(m.inputs[0].Value())
	addr := strings.TrimSpace(m.inputs[1].Value())
	username := strings.TrimSpace(m.inputs[2].Value())
	password := strings.TrimSpace(m.inputs[3].Value())
	if m.authChoice == authNone {
		username = ""
		password = ""
	}
	ic := m.incusClient

	return func() tea.Msg {
		err := ic.AddRemote(name, addr, username, password)
		if err != nil {
			return actionResult{err: err}
		}
		return actionResult{msg: fmt.Sprintf("Remote %q added", name)}
	}
}

func (m Model) deleteRemote() tea.Cmd {
	if m.selected >= len(m.remotes) {
		return nil
	}
	name := m.remotes[m.selected].Name
	ic := m.incusClient

	return func() tea.Msg {
		err := ic.RemoveRemote(name)
		if err != nil {
			return actionResult{err: err}
		}
		return actionResult{msg: fmt.Sprintf("Remote %q removed", name)}
	}
}
