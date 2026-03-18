package manage

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/danbiagini/freedb-tui/internal/db"
	"github.com/danbiagini/freedb-tui/internal/incus"
	"github.com/danbiagini/freedb-tui/internal/registry"
	"github.com/danbiagini/freedb-tui/internal/traefik"
)

type subview int

const (
	subviewMenu subview = iota
	subviewLogs
	subviewConfirmDelete
)

type actionResult struct {
	msg string
	err error
}

type logsResult struct {
	content string
	err     error
}

type Model struct {
	appName     string
	app         *registry.App
	isSystem    bool
	incusClient *incus.Client
	registry    *registry.AppRegistry
	subview     subview
	viewport    viewport.Model
	message     string
	err         error
	done        bool
	busy        bool
}

func NewModel(appName string, app *registry.App, isSystem bool, ic *incus.Client, reg *registry.AppRegistry, width, height int) Model {
	if width < 40 {
		width = 120
	}
	if height < 10 {
		height = 30
	}
	vp := viewport.New(width-4, height-6)

	return Model{
		appName:     appName,
		app:         app,
		isSystem:    isSystem,
		incusClient: ic,
		registry:    reg,
		subview:     subviewMenu,
		viewport:    vp,
	}
}

func (m Model) Done() bool { return m.done }

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case actionResult:
		m.busy = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.message = msg.msg
			m.err = nil
		}
		return m, nil

	case logsResult:
		m.busy = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.subview = subviewLogs
		m.viewport.SetContent(msg.content)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = msg.Height - 6
	}

	if m.subview == subviewLogs {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch m.subview {
	case subviewLogs:
		if key == "esc" || key == "q" {
			m.subview = subviewMenu
			return m, nil
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case subviewConfirmDelete:
		switch key {
		case "y":
			m.busy = true
			m.subview = subviewMenu
			return m, m.deleteApp()
		case "n", "esc":
			m.subview = subviewMenu
			return m, nil
		}
		return m, nil

	case subviewMenu:
		if m.busy {
			return m, nil
		}

		switch key {
		case "esc":
			m.done = true
			return m, nil

		case "s":
			if m.isSystem {
				return m, nil
			}
			m.busy = true
			m.message = ""
			return m, m.stopApp()

		case "t":
			if m.isSystem {
				return m, nil
			}
			m.busy = true
			m.message = ""
			return m, m.startApp()

		case "r":
			if m.isSystem {
				return m, nil
			}
			m.busy = true
			m.message = ""
			return m, m.restartApp()

		case "l":
			m.busy = true
			m.message = ""
			return m, m.fetchLogs()

		case "d":
			if m.isSystem {
				return m, nil
			}
			m.subview = subviewConfirmDelete
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
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))

	var b strings.Builder

	if m.subview == subviewLogs {
		b.WriteString(titleStyle.Render(fmt.Sprintf("Logs: %s", m.appName)))
		b.WriteString("\n")
		b.WriteString(m.viewport.View())
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("[esc] Back  [↑↓] Scroll"))
		return b.String()
	}

	b.WriteString(titleStyle.Render(m.appName))
	b.WriteString("\n\n")

	if m.app != nil {
		b.WriteString(fmt.Sprintf("  Type:   %s\n", m.app.Type))
		b.WriteString(fmt.Sprintf("  Image:  %s\n", m.app.Image))
		b.WriteString(fmt.Sprintf("  Domain: %s\n", m.app.Domain))
		b.WriteString(fmt.Sprintf("  Port:   %d\n", m.app.Port))
		b.WriteString(fmt.Sprintf("  IP:     %s\n", m.app.LastIP))
		if m.app.HasDB {
			b.WriteString(fmt.Sprintf("  DB:     %s\n", m.app.DBName))
		}
	} else {
		b.WriteString("  (system container)\n")
	}

	b.WriteString("\n")

	if m.subview == subviewConfirmDelete {
		b.WriteString(warnStyle.Render("  Delete this app? This removes the container, Traefik route,"))
		b.WriteString("\n")
		if m.app != nil && m.app.HasDB {
			b.WriteString(warnStyle.Render("  and drops the database."))
			b.WriteString("\n")
		}
		b.WriteString(warnStyle.Render("  [y] Yes  [n] No"))
		return b.String()
	}

	if m.busy {
		b.WriteString("  Working...\n")
	}

	if m.err != nil {
		b.WriteString(errStyle.Render(fmt.Sprintf("  Error: %v", m.err)))
		b.WriteString("\n")
	}
	if m.message != "" {
		b.WriteString(successStyle.Render("  "+m.message) + "\n")
	}

	b.WriteString("\n")
	if m.isSystem {
		b.WriteString(dimStyle.Render("  [l] Logs  [esc] Back"))
	} else {
		b.WriteString(dimStyle.Render("  [s] Stop  [t] Start  [r] Restart  [l] Logs  [d] Delete  [esc] Back"))
	}

	return b.String()
}

func (m Model) stopApp() tea.Cmd {
	name := m.appName
	ic := m.incusClient
	return func() tea.Msg {
		err := ic.StopContainer(context.Background(), name)
		if err != nil {
			return actionResult{err: err}
		}
		return actionResult{msg: "Stopped"}
	}
}

func (m Model) startApp() tea.Cmd {
	name := m.appName
	ic := m.incusClient
	app := m.app
	reg := m.registry
	return func() tea.Msg {
		ctx := context.Background()
		if err := ic.StartContainer(ctx, name); err != nil {
			return actionResult{err: err}
		}

		// Wait for IP and update route if app is registered
		if app != nil && app.Domain != "" {
			ip, err := ic.WaitForIP(ctx, name, 15_000_000_000) // 15s
			if err == nil && ip != app.LastIP {
				_ = traefik.PushRoute(ic, app.Name, app.Domain, ip, app.Port, app.TLS)
				_ = reg.UpdateIP(app.Name, ip)
			}
		}

		return actionResult{msg: "Started"}
	}
}

func (m Model) restartApp() tea.Cmd {
	name := m.appName
	ic := m.incusClient
	app := m.app
	reg := m.registry
	return func() tea.Msg {
		ctx := context.Background()
		_ = ic.StopContainer(ctx, name)
		if err := ic.StartContainer(ctx, name); err != nil {
			return actionResult{err: err}
		}

		if app != nil && app.Domain != "" {
			ip, err := ic.WaitForIP(ctx, name, 15_000_000_000)
			if err == nil && ip != app.LastIP {
				_ = traefik.PushRoute(ic, app.Name, app.Domain, ip, app.Port, app.TLS)
				_ = reg.UpdateIP(app.Name, ip)
			}
		}

		return actionResult{msg: "Restarted"}
	}
}

func (m Model) fetchLogs() tea.Cmd {
	name := m.appName
	ic := m.incusClient
	return func() tea.Msg {
		output, err := ic.Exec(context.Background(), name, []string{
			"journalctl", "-n", "200", "--no-pager",
		})
		if err != nil {
			// Fallback: try syslog
			output, err = ic.Exec(context.Background(), name, []string{
				"tail", "-n", "200", "/var/log/syslog",
			})
			if err != nil {
				return logsResult{err: fmt.Errorf("could not fetch logs: %w", err)}
			}
		}
		return logsResult{content: output}
	}
}

func (m Model) deleteApp() tea.Cmd {
	name := m.appName
	app := m.app
	ic := m.incusClient
	reg := m.registry
	return func() tea.Msg {
		ctx := context.Background()

		// Delete container
		if err := ic.DeleteContainer(ctx, name); err != nil {
			return actionResult{err: fmt.Errorf("deleting container: %w", err)}
		}

		// Delete Traefik route
		_ = traefik.DeleteRoute(ic, name)

		// Drop database if applicable
		if app != nil && app.HasDB {
			_ = db.DropDatabase(ctx, ic, app.DBName)
		}

		// Remove from registry
		if err := reg.Remove(name); err != nil {
			return actionResult{err: fmt.Errorf("removing from registry: %w", err)}
		}

		return actionResult{msg: "Deleted"}
	}
}
