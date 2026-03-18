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
	rows        []table.Row
	cpuReadings map[string]float64
	metrics     map[string]*traefik.ServiceMetrics
	err         error
}

type Model struct {
	table       table.Model
	incusClient *incus.Client
	registry    *registry.AppRegistry
	cfg         *config.Config
	lastRefresh time.Time
	prevCPU     map[string]float64 // previous CPU seconds per container
	prevTime    time.Time          // time of previous CPU reading
	cpuPercent  map[string]float64 // computed CPU % per container
	history     *traefik.MetricsHistory
	curMetrics  map[string]*traefik.ServiceMetrics
	err         error
}

func NewModel(ic *incus.Client, reg *registry.AppRegistry, cfg *config.Config) Model {
	columns := []table.Column{
		{Title: "Name", Width: 16},
		{Title: "Status", Width: 10},
		{Title: "Image", Width: 20},
		{Title: "Domain", Width: 22},
		{Title: "Mem", Width: 7},
		{Title: "CPU", Width: 6},
		{Title: "Today", Width: 8},
		{Title: "7d avg", Width: 8},
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

	histPath := "/etc/freedb/metrics-history.json"
	history, _ := traefik.LoadHistory(histPath)

	return Model{
		table:       t,
		incusClient: ic,
		registry:    reg,
		cfg:         cfg,
		history:     history,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refresh(), m.tick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshMsg:
		now := time.Now()
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			// Compute CPU % from delta
			if m.prevCPU != nil && !m.prevTime.IsZero() {
				elapsed := now.Sub(m.prevTime).Seconds()
				if elapsed > 0 {
					if m.cpuPercent == nil {
						m.cpuPercent = make(map[string]float64)
					}
					for name, curr := range msg.cpuReadings {
						if prev, ok := m.prevCPU[name]; ok {
							delta := curr - prev
							m.cpuPercent[name] = (delta / elapsed) * 100
						}
					}
				}
			}
			m.prevCPU = msg.cpuReadings
			m.prevTime = now

			// Update traffic metrics
			if msg.metrics != nil {
				m.curMetrics = msg.metrics
				if m.history != nil {
					if len(m.history.Baseline) == 0 {
						m.history.UpdateBaseline(msg.metrics)
					}
					m.history.RecordSnapshot(msg.metrics)
					_ = m.history.Save()
				}
			}

			m.table.SetRows(msg.rows)
		}
		m.lastRefresh = now
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
	help := fmt.Sprintf("[a] Add App  [enter] Manage  [r] Refresh  [q] Quit  Refreshed %s ago", ago)
	b.WriteString(helpStyle.Render(help))

	return b.String()
}

func formatReqs(n float64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", n/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", n/1000)
	}
	return fmt.Sprintf("%.0f", n)
}

func (m Model) SelectedApp() string {
	row := m.table.SelectedRow()
	if row == nil {
		return ""
	}
	return row[0] // Name column
}

type tickMsg time.Time

func (m Model) tick() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) refresh() tea.Cmd {
	cpuPercent := m.cpuPercent
	history := m.history
	return func() tea.Msg {
		containers, err := m.incusClient.ListContainers(context.Background())
		if err != nil {
			return refreshMsg{err: err}
		}

		// Fetch Traefik metrics (best effort — don't fail if unavailable)
		metrics, _ := traefik.FetchMetrics(m.incusClient, m.cfg.ProxyContainer)

		systemContainers := map[string]bool{
			m.cfg.ProxyContainer: true,
			m.cfg.DBContainer:    true,
		}

		registeredApps := make(map[string]*registry.App)
		for _, app := range m.registry.List() {
			registeredApps[app.Name] = app
		}

		var rows []table.Row
		cpuReadings := make(map[string]float64)

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
			image := "—"
			if app, ok := registeredApps[c.Name]; ok {
				domain = app.Domain
				// Show short image name (strip registry prefix, keep tag)
				img := app.Image
				if parts := strings.Split(img, "/"); len(parts) > 1 {
					img = parts[len(parts)-1]
				}
				if img != "" {
					image = img
				}

				// IP drift detection: if IP changed, update route and registry
				if c.IP != "" && c.IP != app.LastIP && app.Domain != "" {
					_ = traefik.PushRoute(m.incusClient, app.Name, app.Domain, c.IP, app.Port, app.TLS)
					_ = m.registry.UpdateIP(app.Name, c.IP)
				}
			}

			mem := "—"
			if c.MemUsageMB > 0 {
				mem = fmt.Sprintf("%dMB", c.MemUsageMB)
			}

			cpuReadings[c.Name] = c.CPUSeconds
			cpu := "—"
			if _, ok := cpuPercent[c.Name]; ok && c.Status == "RUNNING" {
				pct := cpuPercent[c.Name]
				if pct < 0.1 {
					cpu = "<0.1%"
				} else {
					cpu = fmt.Sprintf("%.1f%%", pct)
				}
			}

			today := "—"
			avg7d := "—"
			if metrics != nil && history != nil {
				todayReqs := history.TodayRequests(c.Name, metrics)
				if todayReqs > 0 {
					today = formatReqs(todayReqs)
				}
				avgReqs := history.AvgDailyRequests(c.Name, 7)
				if avgReqs > 0 {
					avg7d = formatReqs(avgReqs)
				}
			}

			rows = append(rows, table.Row{
				c.Name,
				c.Status,
				image,
				domain,
				mem,
				cpu,
				today,
				avg7d,
			})
		}

		return refreshMsg{rows: rows, cpuReadings: cpuReadings, metrics: metrics}
	}
}
