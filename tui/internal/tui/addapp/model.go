package addapp

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/danbiagini/freedb-tui/internal/config"
	"github.com/danbiagini/freedb-tui/internal/db"
	"github.com/danbiagini/freedb-tui/internal/incus"
	"github.com/danbiagini/freedb-tui/internal/registry"
	"github.com/danbiagini/freedb-tui/internal/traefik"
)

var nameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

type step int

const (
	stepName step = iota
	stepImage
	stepDomain
	stepPort
	stepDB
	stepConfirm
	stepDeploying
	stepDone
)

type deployResult struct {
	err error
}

type Model struct {
	step        step
	inputs      []textinput.Model
	needsDB     bool
	incusClient *incus.Client
	registry    *registry.AppRegistry
	cfg         *config.Config
	err         error
	deployMsg   string
	done        bool
	cancelled   bool
}

func NewModel(ic *incus.Client, reg *registry.AppRegistry, cfg *config.Config) Model {
	inputs := make([]textinput.Model, 4)

	inputs[0] = textinput.New()
	inputs[0].Placeholder = "myapp"
	inputs[0].Focus()
	inputs[0].CharLimit = 30

	inputs[1] = textinput.New()
	inputs[1].Placeholder = "images:debian/12/cloud"
	inputs[1].CharLimit = 100

	inputs[2] = textinput.New()
	inputs[2].Placeholder = "myapp.example.com"
	inputs[2].CharLimit = 100

	inputs[3] = textinput.New()
	inputs[3].Placeholder = "8080"
	inputs[3].CharLimit = 5

	return Model{
		step:        stepName,
		inputs:      inputs,
		incusClient: ic,
		registry:    reg,
		cfg:         cfg,
	}
}

func (m Model) Done() bool      { return m.done }
func (m Model) Cancelled() bool { return m.cancelled }

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.cancelled = true
			m.done = true
			return m, nil

		case "y", "n":
			if m.step == stepDB {
				m.handleDBStep(msg.String())
				return m, nil
			}

		case "enter":
			return m.handleEnter()
		}

	case deployResult:
		if msg.err != nil {
			m.err = msg.err
			m.step = stepDone
			m.done = true
		} else {
			m.deployMsg = "App deployed successfully!"
			m.step = stepDone
			m.done = true
		}
		return m, nil
	}

	if m.step < stepDB {
		var cmd tea.Cmd
		idx := int(m.step)
		m.inputs[idx], cmd = m.inputs[idx].Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepName:
		name := strings.TrimSpace(m.inputs[0].Value())
		if !nameRegex.MatchString(name) {
			m.err = fmt.Errorf("name must be lowercase alphanumeric with hyphens, starting with a letter")
			return m, nil
		}
		if _, exists := m.registry.Get(name); exists {
			m.err = fmt.Errorf("app %q already exists", name)
			return m, nil
		}
		m.err = nil
		m.step = stepImage
		m.inputs[1].Focus()
		return m, nil

	case stepImage:
		if strings.TrimSpace(m.inputs[1].Value()) == "" {
			m.err = fmt.Errorf("image is required")
			return m, nil
		}
		m.err = nil
		m.step = stepDomain
		m.inputs[2].Focus()
		return m, nil

	case stepDomain:
		if strings.TrimSpace(m.inputs[2].Value()) == "" {
			m.err = fmt.Errorf("domain is required")
			return m, nil
		}
		m.err = nil
		m.step = stepPort
		m.inputs[3].Focus()
		m.inputs[3].SetValue("8080")
		return m, nil

	case stepPort:
		m.err = nil
		m.step = stepDB
		return m, nil

	case stepDB:
		// Handled by y/n keys below
		return m, nil

	case stepConfirm:
		m.step = stepDeploying
		return m, m.deploy()
	}

	return m, nil
}

func (m Model) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).MarginBottom(1)
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("Add App"))
	b.WriteString("\n\n")

	labels := []string{"Name:", "Image:", "Domain:", "Port:"}

	for i, label := range labels {
		if m.step > step(i) {
			b.WriteString(fmt.Sprintf("  %s %s\n", label, m.inputs[i].Value()))
		} else if m.step == step(i) {
			b.WriteString(fmt.Sprintf("  %s %s\n", label, m.inputs[i].View()))
		}
	}

	if m.step >= stepDB && m.step < stepDeploying {
		if m.step == stepDB {
			b.WriteString("\n  Needs database? [y/n] ")
		} else {
			dbStr := "no"
			if m.needsDB {
				dbStr = "yes"
			}
			b.WriteString(fmt.Sprintf("  Database: %s\n", dbStr))
		}
	}

	if m.step == stepConfirm {
		b.WriteString("\n  Press [enter] to deploy, [esc] to cancel\n")
	}

	if m.step == stepDeploying {
		b.WriteString("\n  Deploying...\n")
	}

	if m.step == stepDone {
		if m.err != nil {
			b.WriteString("\n" + errStyle.Render(fmt.Sprintf("  Deploy failed: %v", m.err)) + "\n")
		} else {
			b.WriteString("\n" + successStyle.Render("  "+m.deployMsg) + "\n")
		}
		b.WriteString(dimStyle.Render("\n  Press any key to return to dashboard"))
	}

	if m.err != nil && m.step < stepDone {
		b.WriteString("\n" + errStyle.Render(fmt.Sprintf("  %v", m.err)))
	}

	b.WriteString(dimStyle.Render("\n\n  [esc] Cancel"))

	return b.String()
}

func (m *Model) handleDBStep(key string) {
	switch key {
	case "y":
		m.needsDB = true
		m.step = stepConfirm
	case "n":
		m.needsDB = false
		m.step = stepConfirm
	}
}

func (m Model) deploy() tea.Cmd {
	name := strings.TrimSpace(m.inputs[0].Value())
	image := strings.TrimSpace(m.inputs[1].Value())
	domain := strings.TrimSpace(m.inputs[2].Value())
	portStr := strings.TrimSpace(m.inputs[3].Value())
	needsDB := m.needsDB

	port := 8080
	if portStr != "" {
		fmt.Sscanf(portStr, "%d", &port)
	}

	ic := m.incusClient
	reg := m.registry

	return func() tea.Msg {
		ctx := context.Background()

		// Launch container
		if err := ic.LaunchContainer(ctx, name, image); err != nil {
			return deployResult{err: fmt.Errorf("launching container: %w", err)}
		}

		// Wait for IP
		ip, err := ic.WaitForIP(ctx, name, 30*time.Second)
		if err != nil {
			return deployResult{err: fmt.Errorf("waiting for IP: %w", err)}
		}

		// Create Traefik route
		if err := traefik.PushRoute(ic, name, domain, ip, port); err != nil {
			return deployResult{err: fmt.Errorf("creating route: %w", err)}
		}

		// Create database if requested
		dbName := ""
		if needsDB {
			dbName = name
			if err := db.CreateDatabase(ctx, ic, name); err != nil {
				return deployResult{err: fmt.Errorf("creating database: %w", err)}
			}
		}

		// Save to registry
		app := &registry.App{
			Name:      name,
			Type:      registry.AppTypeContainer,
			Image:     image,
			Domain:    domain,
			Port:      port,
			HasDB:     needsDB,
			DBName:    dbName,
			DBUser:    dbName,
			LastIP:    ip,
			CreatedAt: time.Now(),
		}
		if err := reg.Add(app); err != nil {
			return deployResult{err: fmt.Errorf("saving to registry: %w", err)}
		}

		return deployResult{}
	}
}
