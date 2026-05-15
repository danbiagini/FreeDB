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

	"github.com/danbiagini/FreeDB/tui/internal/config"
	"github.com/danbiagini/FreeDB/tui/internal/incus"
	"github.com/danbiagini/FreeDB/tui/internal/registry"
	"github.com/danbiagini/FreeDB/tui/internal/traefik"
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

type containerData struct {
	info       incus.ContainerInfo
	appName    string // display name (may differ from container name after updates)
	domain     string
	image      string
	isApp      bool
}

type refreshMsg struct {
	containers  []containerData
	cpuReadings map[string]float64
	metrics     map[string]*traefik.ServiceMetrics
	diskUsed    uint64
	diskTotal   uint64
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
	hostInfo    *config.HostInfo
	showVersion bool
	totalMemMB  int64
	totalCPU    float64
	diskUsed    uint64
	diskTotal   uint64
	err         error
}

func NewModel(ic *incus.Client, reg *registry.AppRegistry, cfg *config.Config) Model {
	columns := []table.Column{
		{Title: "Name", Width: 16},
		{Title: "Status", Width: 10},
		{Title: "Image", Width: 18},
		{Title: "Domain", Width: 20},
		{Title: "Mem", Width: 7},
		{Title: "CPU", Width: 6},
		{Title: "Reqs", Width: 7},
		{Title: "Err%", Width: 5},
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
	hostInfo := config.GetHostInfo()

	return Model{
		table:       t,
		incusClient: ic,
		registry:    reg,
		cfg:         cfg,
		history:     history,
		hostInfo:    hostInfo,
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

			// Update disk usage
			m.diskUsed = msg.diskUsed
			m.diskTotal = msg.diskTotal

			// Build rows with latest cpuPercent
			m.totalMemMB = 0
			m.totalCPU = 0
			var rows []table.Row
			for _, cd := range msg.containers {
				mem := "—"
				if cd.info.MemUsageMB > 0 {
					mem = fmt.Sprintf("%dMB", cd.info.MemUsageMB)
					m.totalMemMB += cd.info.MemUsageMB
				}

				cpu := "—"
				if pct, ok := m.cpuPercent[cd.info.Name]; ok && strings.EqualFold(cd.info.Status, "running") {
					m.totalCPU += pct
					if pct < 0.1 {
						cpu = "<0.1%"
					} else {
						cpu = fmt.Sprintf("%.1f%%", pct)
					}
				}

				reqs := "—"
				errPct := "—"
				if msg.metrics != nil {
					if sm, ok := msg.metrics[cd.appName]; ok && sm.TotalReqs > 0 {
						reqs = formatReqs(sm.TotalReqs)
						pct := (sm.ErrorReqs / sm.TotalReqs) * 100
						if pct > 0 {
							errPct = fmt.Sprintf("%.1f", pct)
						} else {
							errPct = "0"
						}
					}
				}

				rows = append(rows, table.Row{
					cd.appName,
					cd.info.Status,
					cd.image,
					cd.domain,
					mem,
					cpu,
					reqs,
					errPct,
				})
			}
			m.table.SetRows(rows)
		}
		m.lastRefresh = now
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.refresh(), m.tick())

	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			return m, m.refresh()
		case "v":
			m.showVersion = !m.showVersion
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("FreeDB"))
	if m.hostInfo != nil {
		b.WriteString("  ")
		b.WriteString(helpStyle.Render(m.hostInfo.String()))
	}
	b.WriteString("\n")

	b.WriteString(m.table.View())
	b.WriteString("\n")

	// Resource summary
	var parts []string
	if m.totalMemMB > 0 {
		if m.totalMemMB >= 1024 {
			parts = append(parts, fmt.Sprintf("Mem: %.1f GB", float64(m.totalMemMB)/1024))
		} else {
			parts = append(parts, fmt.Sprintf("Mem: %d MB", m.totalMemMB))
		}
	}
	if m.totalCPU > 0 {
		parts = append(parts, fmt.Sprintf("CPU: %.1f%%", m.totalCPU))
	}
	if m.diskTotal > 0 {
		usedGB := float64(m.diskUsed) / (1024 * 1024 * 1024)
		totalGB := float64(m.diskTotal) / (1024 * 1024 * 1024)
		pct := float64(m.diskUsed) / float64(m.diskTotal) * 100
		parts = append(parts, fmt.Sprintf("Disk: %.1f/%.0f GB (%.0f%%)", usedGB, totalGB, pct))
	}
	if len(parts) > 0 {
		b.WriteString(helpStyle.Render("  " + strings.Join(parts, "  |  ")))
		b.WriteString("\n")
	}

	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n")
	}

	if m.showVersion {
		b.WriteString(helpStyle.Render(fmt.Sprintf("  Version: %s", m.cfg.Version)))
		b.WriteString("\n")
	}

	ago := time.Since(m.lastRefresh).Truncate(time.Second)
	help := fmt.Sprintf("[a] Add App  [enter] Manage  [D] Databases  [R] Registries  [v] Version  [q] Quit  %s ago", ago)
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
	return func() tea.Msg {
		containers, err := m.incusClient.ListContainers(context.Background())
		if err != nil {
			return refreshMsg{err: err}
		}

		// Fetch Traefik metrics (best effort)
		metrics, _ := traefik.FetchMetrics(m.incusClient, m.cfg.ProxyContainer)

		// Reload registry from disk to pick up changes from CLI deploys
		_ = m.registry.Reload()

		systemContainers := map[string]bool{
			m.cfg.ProxyContainer: true,
			m.cfg.DBContainer:    true,
		}

		registeredApps := make(map[string]*registry.App)
		for _, app := range m.registry.List() {
			registeredApps[app.Name] = app
			if app.ContainerName != "" {
				registeredApps[app.ContainerName] = app
			}
		}

		cpuReadings := make(map[string]float64)
		var data []containerData

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
			displayName := c.Name
			domain := "—"
			image := "—"
			isApp := false
			if app, ok := registeredApps[c.Name]; ok {
				isApp = true
				displayName = app.Name // use app name, not container name
				domains := app.GetDomains()
				if len(domains) > 0 {
					domain = domains[0]
					if len(domains) > 1 {
						domain = fmt.Sprintf("%s +%d", domain, len(domains)-1)
					}
				}
				img := app.Image
				if parts := strings.Split(img, "/"); len(parts) > 1 {
					img = parts[len(parts)-1]
				}
				if img != "" {
					image = img
				}

				// IP drift detection
				if c.IP != "" && c.IP != app.LastIP && app.HasDomains() {
					_ = traefik.PushRoute(m.incusClient, app.Name, app.GetDomains(), c.IP, app.Port, app.TLS)
					_ = m.registry.UpdateIP(app.Name, c.IP)
				}
			}

			cpuReadings[c.Name] = c.CPUSeconds
			data = append(data, containerData{
				info:    c,
				appName: displayName,
				domain:  domain,
				image:  image,
				isApp:  isApp,
			})
		}

		// Fetch storage pool usage (best effort)
		var diskUsed, diskTotal uint64
		if m.cfg.StoragePool != "" {
			if usage, err := m.incusClient.GetStoragePoolUsage(m.cfg.StoragePool); err == nil {
				diskUsed = usage.UsedBytes
				diskTotal = usage.TotalBytes
			}
		}

		return refreshMsg{containers: data, cpuReadings: cpuReadings, metrics: metrics, diskUsed: diskUsed, diskTotal: diskTotal}
	}
}
