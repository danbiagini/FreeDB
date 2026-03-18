package dashboard

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/danbiagini/freedb-tui/internal/config"
	"github.com/danbiagini/freedb-tui/internal/incus"
	"github.com/danbiagini/freedb-tui/internal/registry"
	"github.com/danbiagini/freedb-tui/internal/traefik"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12")).
			MarginBottom(1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9"))
)

type refreshMsg struct {
	rows []table.Row
	err  error
}

type Model struct {
	table       table.Model
	incusClient *incus.Client
	registry    *registry.AppRegistry
	cfg         *config.Config
	lastRefresh time.Time
	err         error
}

func NewModel(ic *incus.Client, reg *registry.AppRegistry, cfg *config.Config) Model {
	columns := []table.Column{
		{Title: "Name", Width: 16},
		{Title: "Status", Width: 10},
		{Title: "Domain", Width: 24},
		{Title: "IP", Width: 16},
		{Title: "Mem", Width: 8},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(12),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	return Model{
		table:       t,
		incusClient: ic,
		registry:    reg,
		cfg:         cfg,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refresh(), m.tick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshMsg:
		m.lastRefresh = time.Now()
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.table.SetRows(msg.rows)
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.refresh(), m.tick())

	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			return m, m.refresh()
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("FreeDB"))
	b.WriteString("\n")

	b.WriteString(m.table.View())
	b.WriteString("\n")

	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n")
	}

	ago := time.Since(m.lastRefresh).Truncate(time.Second)
	help := fmt.Sprintf("[a] Add App  [r] Refresh  [q] Quit                 Refreshed %s ago", ago)
	b.WriteString(helpStyle.Render(help))

	return b.String()
}

type tickMsg time.Time

func (m Model) tick() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) refresh() tea.Cmd {
	return func() tea.Msg {
		containers, err := m.incusClient.ListContainers(context.Background())
		if err != nil {
			return refreshMsg{err: err}
		}

		systemContainers := map[string]bool{
			m.cfg.ProxyContainer: true,
			m.cfg.DBContainer:    true,
		}

		registeredApps := make(map[string]*registry.App)
		for _, app := range m.registry.List() {
			registeredApps[app.Name] = app
		}

		var rows []table.Row

		// Sort: system containers first, then apps alphabetically
		sort.Slice(containers, func(i, j int) bool {
			iSys := systemContainers[containers[i].Name]
			jSys := systemContainers[containers[j].Name]
			if iSys != jSys {
				return iSys
			}
			return containers[i].Name < containers[j].Name
		})

		for _, c := range containers {
			domain := "—"
			if app, ok := registeredApps[c.Name]; ok {
				domain = app.Domain

				// IP drift detection: if IP changed, update route and registry
				if c.IP != "" && c.IP != app.LastIP && app.Domain != "" {
					_ = traefik.PushRoute(m.incusClient, app.Name, app.Domain, c.IP, app.Port)
					_ = m.registry.UpdateIP(app.Name, c.IP)
				}
			}

			mem := "—"
			if c.MemUsageMB > 0 {
				mem = fmt.Sprintf("%dMB", c.MemUsageMB)
			}

			ip := c.IP
			if ip == "" {
				ip = "—"
			}

			rows = append(rows, table.Row{
				c.Name,
				c.Status,
				domain,
				ip,
				mem,
			})
		}

		return refreshMsg{rows: rows}
	}
}
