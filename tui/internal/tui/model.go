package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/danbiagini/freedb-tui/internal/config"
	"github.com/danbiagini/freedb-tui/internal/incus"
	"github.com/danbiagini/freedb-tui/internal/registry"
	"github.com/danbiagini/freedb-tui/internal/tui/addapp"
	"github.com/danbiagini/freedb-tui/internal/tui/dashboard"
	"github.com/danbiagini/freedb-tui/internal/tui/manage"
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
	manage    *manage.Model
	width     int
	height    int
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
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	switch m.current {
	case viewDashboard:
		return m.updateDashboard(msg)
	case viewAddApp:
		return m.updateAddApp(msg)
	case viewManageApp:
		return m.updateManage(msg)
	}

	return m, nil
}

func (m Model) updateDashboard(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "a":
			aa := addapp.NewModel(m.incus, m.registry, m.cfg)
			m.addApp = &aa
			m.current = viewAddApp
			return m, aa.Init()
		case "enter":
			selected := m.dashboard.SelectedApp()
			if selected != "" {
				app, _ := m.registry.Get(selected)
				isSystem := selected == m.cfg.ProxyContainer || selected == m.cfg.DBContainer
				mg := manage.NewModel(selected, app, isSystem, m.incus, m.registry, m.width, m.height)
				m.manage = &mg
				m.current = viewManageApp
				return m, mg.Init()
			}
		}
	}

	updated, cmd := m.dashboard.Update(msg)
	m.dashboard = updated.(dashboard.Model)
	return m, cmd
}

func (m Model) updateAddApp(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.addApp == nil {
		m.current = viewDashboard
		return m, nil
	}

	updated, cmd := m.addApp.Update(msg)
	aa := updated.(addapp.Model)
	m.addApp = &aa

	if aa.Done() {
		m.current = viewDashboard
		m.addApp = nil
		return m, m.dashboard.Init()
	}

	return m, cmd
}

func (m Model) updateManage(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.manage == nil {
		m.current = viewDashboard
		return m, nil
	}

	updated, cmd := m.manage.Update(msg)
	mg := updated.(manage.Model)
	m.manage = &mg

	if mg.Done() {
		m.current = viewDashboard
		m.manage = nil
		return m, m.dashboard.Init()
	}

	return m, cmd
}

func (m Model) View() string {
	switch m.current {
	case viewAddApp:
		if m.addApp != nil {
			return m.addApp.View()
		}
	case viewManageApp:
		if m.manage != nil {
			return m.manage.View()
		}
	}
	return m.dashboard.View()
}
