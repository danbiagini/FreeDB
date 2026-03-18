package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/danbiagini/freedb-tui/internal/config"
	"github.com/danbiagini/freedb-tui/internal/incus"
	"github.com/danbiagini/freedb-tui/internal/registry"
	"github.com/danbiagini/freedb-tui/internal/tui/addapp"
	"github.com/danbiagini/freedb-tui/internal/tui/dashboard"
)

type view int

const (
	viewDashboard view = iota
	viewAddApp
	viewManageApp
)

type Model struct {
	cfg       *config.Config
	incus     *incus.Client
	registry  *registry.AppRegistry
	current   view
	dashboard dashboard.Model
	addApp    *addapp.Model
	width     int
	height    int
	err       error
}

func NewModel(cfg *config.Config, ic *incus.Client, reg *registry.AppRegistry) Model {
	return Model{
		cfg:       cfg,
		incus:     ic,
		registry:  reg,
		current:   viewDashboard,
		dashboard: dashboard.NewModel(ic, reg, cfg),
	}
}

func (m Model) Init() tea.Cmd {
	return m.dashboard.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.current == viewDashboard {
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "a":
				aa := addapp.NewModel(m.incus, m.registry, m.cfg)
				m.addApp = &aa
				m.current = viewAddApp
				return m, aa.Init()
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	switch m.current {
	case viewDashboard:
		updated, cmd := m.dashboard.Update(msg)
		m.dashboard = updated.(dashboard.Model)
		return m, cmd

	case viewAddApp:
		if m.addApp != nil {
			updated, cmd := m.addApp.Update(msg)
			aa := updated.(addapp.Model)
			m.addApp = &aa

			if aa.Done() {
				m.current = viewDashboard
				m.addApp = nil
				// Force a dashboard refresh
				return m, m.dashboard.Init()
			}

			return m, cmd
		}
	}

	return m, nil
}

func (m Model) View() string {
	switch m.current {
	case viewAddApp:
		if m.addApp != nil {
			return m.addApp.View()
		}
	}
	return m.dashboard.View()
}
