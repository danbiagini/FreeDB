package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/danbiagini/FreeDB/tui/internal/config"
	"github.com/danbiagini/FreeDB/tui/internal/incus"
	"github.com/danbiagini/FreeDB/tui/internal/registry"
	"github.com/danbiagini/FreeDB/tui/internal/tui/addapp"
	"github.com/danbiagini/FreeDB/tui/internal/tui/dashboard"
	"github.com/danbiagini/FreeDB/tui/internal/tui/databases"
	"github.com/danbiagini/FreeDB/tui/internal/tui/manage"
	"github.com/danbiagini/FreeDB/tui/internal/tui/remotes"
)

type view int

const (
	viewDashboard view = iota
	viewAddApp
	viewManageApp
	viewRemotes
	viewDatabases
)

type Model struct {
	cfg       *config.Config
	incus     *incus.Client
	registry  *registry.AppRegistry
	current   view
	dashboard dashboard.Model
	addApp    *addapp.Model
	manage    *manage.Model
	remotes   *remotes.Model
	dbs       *databases.Model
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
	case viewRemotes:
		return m.updateRemotes(msg)
	case viewDatabases:
		return m.updateDatabases(msg)
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
		case "R":
			rm := remotes.NewModel(m.incus)
			m.remotes = &rm
			m.current = viewRemotes
			return m, rm.Init()
		case "D":
			dbm := databases.NewModel(m.incus, m.registry)
			m.dbs = &dbm
			m.current = viewDatabases
			return m, dbm.Init()
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

func (m Model) updateRemotes(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.remotes == nil {
		m.current = viewDashboard
		return m, nil
	}

	updated, cmd := m.remotes.Update(msg)
	rm := updated.(remotes.Model)
	m.remotes = &rm

	if rm.Done() {
		m.current = viewDashboard
		m.remotes = nil
		return m, m.dashboard.Init()
	}

	return m, cmd
}

func (m Model) updateDatabases(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.dbs == nil {
		m.current = viewDashboard
		return m, nil
	}

	updated, cmd := m.dbs.Update(msg)
	dbm := updated.(databases.Model)
	m.dbs = &dbm

	if dbm.Done() {
		m.current = viewDashboard
		m.dbs = nil
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
	case viewRemotes:
		if m.remotes != nil {
			return m.remotes.View()
		}
	case viewDatabases:
		if m.dbs != nil {
			return m.dbs.View()
		}
	}
	return m.dashboard.View()
}
