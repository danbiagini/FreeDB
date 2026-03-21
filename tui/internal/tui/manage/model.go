package manage

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
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
	subviewConfirmRestart
	subviewConfirmDelete
	subviewUpdateTag
	subviewConfirmUpdate
	subviewEnvVars
	subviewEnvVarAdd
	subviewEnvVarConfirmDelete
	subviewEnvVarRestartPrompt
)

type actionResult struct {
	msg string
	err error
}

type logsResult struct {
	content string
	err     error
}

type detailResult struct {
	detail *incus.ContainerDetail
	err    error
}

type envVarsResult struct {
	envVars map[string]string
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
	detail      *incus.ContainerDetail
	message     string
	err         error
	done        bool
	busy        bool
	// Update
	updateInput textinput.Model
	updateImage string // resolved image with new tag
	// Env var editor
	envModified bool
	envVars     map[string]string
	envKeys     []string // sorted keys for display
	envSelected int
	envInput    textinput.Model
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

// containerName returns the actual incus container name (may differ from app name after updates)
func (m Model) containerName() string {
	if m.app != nil && m.app.ContainerName != "" {
		return m.app.ContainerName
	}
	return m.appName
}

func (m Model) Init() tea.Cmd {
	return m.fetchDetail()
}

func (m Model) fetchDetail() tea.Cmd {
	name := m.containerName()
	ic := m.incusClient
	return func() tea.Msg {
		detail, err := ic.GetContainerDetail(context.Background(), name)
		if err != nil {
			return detailResult{err: err}
		}
		return detailResult{detail: detail}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case detailResult:
		if msg.err == nil {
			m.detail = msg.detail
		}
		return m, nil

	case envVarsResult:
		m.busy = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.envVars = msg.envVars
			m.envKeys = make([]string, 0, len(msg.envVars))
			for k := range msg.envVars {
				m.envKeys = append(m.envKeys, k)
			}
			sort.Strings(m.envKeys)
			m.envSelected = 0
			m.subview = subviewEnvVars
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
		// Refresh detail after action
		return m, m.fetchDetail()

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

	case subviewConfirmRestart:
		switch key {
		case "c":
			m.busy = true
			m.subview = subviewMenu
			return m, m.restartApp()
		case "s":
			m.busy = true
			m.subview = subviewMenu
			return m, m.restartService()
		case "esc", "n":
			m.subview = subviewMenu
			return m, nil
		}
		return m, nil

	case subviewUpdateTag:
		switch key {
		case "esc":
			m.subview = subviewMenu
			return m, nil
		case "enter":
			tag := strings.TrimSpace(m.updateInput.Value())
			if tag == "" {
				tag = "latest"
			}
			// Build the new image ref with the chosen tag
			img := m.app.Image
			// Strip old tag
			if idx := strings.LastIndex(img, ":"); idx > 0 {
				after := img[idx+1:]
				if !strings.Contains(after, "/") {
					img = img[:idx]
				}
			}
			m.updateImage = img + ":" + tag
			m.subview = subviewConfirmUpdate
			return m, nil
		}
		var cmd tea.Cmd
		m.updateInput, cmd = m.updateInput.Update(msg)
		return m, cmd

	case subviewConfirmUpdate:
		switch key {
		case "y":
			m.busy = true
			m.subview = subviewMenu
			return m, m.updateApp(m.updateImage)
		case "n", "esc":
			m.subview = subviewMenu
			return m, nil
		}
		return m, nil

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
			m.busy = true
			m.message = ""
			return m, m.stopApp()

		case "t":
			m.busy = true
			m.message = ""
			return m, m.startApp()

		case "r":
			m.subview = subviewConfirmRestart
			return m, nil

		case "l":
			m.busy = true
			m.message = ""
			return m, m.fetchLogs()

		case "e":
			m.subview = subviewEnvVars
			m.message = ""
			m.err = nil
			return m, m.fetchEnvVars()

		case "u":
			if m.isSystem || m.app == nil {
				return m, nil
			}
			// Extract current tag from image ref
			currentTag := "latest"
			img := m.app.Image
			if idx := strings.LastIndex(img, ":"); idx > 0 {
				// Make sure : is a tag separator not a remote separator
				after := img[idx+1:]
				if !strings.Contains(after, "/") {
					currentTag = after
				}
			}
			m.updateInput = textinput.New()
			m.updateInput.SetValue(currentTag)
			m.updateInput.Focus()
			m.updateInput.CharLimit = 50
			m.subview = subviewUpdateTag
			m.err = nil
			return m, textinput.Blink

		case "d":
			if m.isSystem {
				return m, nil
			}
			m.subview = subviewConfirmDelete
			return m, nil
		}

	case subviewEnvVarRestartPrompt:
		switch key {
		case "y":
			m.envModified = false
			m.busy = true
			m.subview = subviewMenu
			return m, m.restartApp()
		case "n", "esc":
			m.envModified = false
			m.subview = subviewMenu
			return m, nil
		}
		return m, nil

	case subviewEnvVars:
		switch key {
		case "esc":
			if m.envModified {
				m.subview = subviewEnvVarRestartPrompt
				return m, nil
			}
			m.subview = subviewMenu
			return m, nil
		case "a":
			m.subview = subviewEnvVarAdd
			m.envInput = textinput.New()
			m.envInput.Placeholder = "KEY=VALUE"
			m.envInput.CharLimit = 200
			m.envInput.Focus()
			m.err = nil
			return m, textinput.Blink
		case "d":
			if len(m.envKeys) > 0 && m.envSelected < len(m.envKeys) {
				m.subview = subviewEnvVarConfirmDelete
			}
			return m, nil
		case "up", "k":
			if m.envSelected > 0 {
				m.envSelected--
			}
			return m, nil
		case "down", "j":
			if m.envSelected < len(m.envKeys)-1 {
				m.envSelected++
			}
			return m, nil
		}

	case subviewEnvVarAdd:
		switch key {
		case "esc":
			m.subview = subviewEnvVars
			return m, nil
		case "enter":
			val := strings.TrimSpace(m.envInput.Value())
			if val == "" {
				m.subview = subviewEnvVars
				return m, nil
			}
			if !strings.Contains(val, "=") {
				m.err = fmt.Errorf("format: KEY=VALUE")
				return m, nil
			}
			parts := strings.SplitN(val, "=", 2)
			m.busy = true
			m.err = nil
			m.envModified = true
			return m, m.setEnvVar(parts[0], parts[1])
		}
		var cmd tea.Cmd
		m.envInput, cmd = m.envInput.Update(msg)
		return m, cmd

	case subviewEnvVarConfirmDelete:
		switch key {
		case "y":
			if m.envSelected < len(m.envKeys) {
				m.busy = true
				m.envModified = true
				return m, m.deleteEnvVar(m.envKeys[m.envSelected])
			}
			m.subview = subviewEnvVars
			return m, nil
		case "n", "esc":
			m.subview = subviewEnvVars
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

	if m.subview == subviewEnvVarRestartPrompt {
		b.WriteString(titleStyle.Render(fmt.Sprintf("Environment: %s", m.appName)))
		b.WriteString("\n\n")
		b.WriteString("  Environment variables were modified.\n")
		b.WriteString("  Restart container for changes to take effect?\n\n")
		b.WriteString("  [y] Yes  [n] No\n")
		return b.String()
	}

	if m.subview == subviewEnvVars || m.subview == subviewEnvVarAdd || m.subview == subviewEnvVarConfirmDelete {
		b.WriteString(titleStyle.Render(fmt.Sprintf("Environment: %s", m.appName)))
		b.WriteString("\n\n")

		if len(m.envKeys) == 0 {
			b.WriteString(dimStyle.Render("  No environment variables set"))
			b.WriteString("\n")
		} else {
			selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))
			for i, k := range m.envKeys {
				v := m.envVars[k]
				line := fmt.Sprintf("  %-24s = %s", k, v)
				if i == m.envSelected {
					b.WriteString(selectedStyle.Render(line))
				} else {
					b.WriteString(line)
				}
				b.WriteString("\n")
			}
		}

		b.WriteString("\n")

		if m.subview == subviewEnvVarAdd {
			b.WriteString(fmt.Sprintf("  %s\n", m.envInput.View()))
		}

		if m.subview == subviewEnvVarConfirmDelete && m.envSelected < len(m.envKeys) {
			b.WriteString(warnStyle.Render(fmt.Sprintf("  Delete %s? [y/n]", m.envKeys[m.envSelected])))
			b.WriteString("\n")
		}

		if m.busy {
			b.WriteString("  Working...\n")
		}
		if m.err != nil {
			b.WriteString(errStyle.Render(fmt.Sprintf("  %v", m.err)) + "\n")
		}
		if m.message != "" {
			b.WriteString(successStyle.Render("  "+m.message) + "\n")
		}

		b.WriteString("\n")
		b.WriteString(dimStyle.Render("  [a] Add  [d] Delete  [esc] Back"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("  Note: container may need restart for changes to take effect"))
		return b.String()
	}

	b.WriteString(titleStyle.Render(m.appName))
	b.WriteString("\n\n")

	// Show live container details
	if m.detail != nil {
		d := m.detail
		b.WriteString(fmt.Sprintf("  Status:  %s\n", d.Status))
		b.WriteString(fmt.Sprintf("  IP:      %s\n", d.IP))
		if d.MemLimitMB > 0 {
			b.WriteString(fmt.Sprintf("  Memory:  %d / %d MB\n", d.MemUsageMB, d.MemLimitMB))
		} else if d.MemUsageMB > 0 {
			b.WriteString(fmt.Sprintf("  Memory:  %d MB\n", d.MemUsageMB))
		}
		if d.DiskUsageMB > 0 {
			b.WriteString(fmt.Sprintf("  Disk:    %d MB\n", d.DiskUsageMB))
		}
		if d.Uptime > 0 {
			b.WriteString(fmt.Sprintf("  Uptime:  %s\n", formatDuration(d.Uptime)))
		}
		if d.Processes > 0 {
			b.WriteString(fmt.Sprintf("  Procs:   %d\n", d.Processes))
		}
		if d.BytesIn > 0 || d.BytesOut > 0 {
			b.WriteString(fmt.Sprintf("  Net I/O: %s in / %s out\n", formatBytes(d.BytesIn), formatBytes(d.BytesOut)))
		}
		b.WriteString(fmt.Sprintf("  Created: %s\n", d.Created))
	}

	// Show app-specific info from registry
	if m.app != nil {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  Image:   %s\n", m.app.Image))
		b.WriteString(fmt.Sprintf("  Domain:  %s\n", m.app.Domain))
		b.WriteString(fmt.Sprintf("  Port:    %d\n", m.app.Port))
		if m.app.HasDB {
			b.WriteString(fmt.Sprintf("  DB:      %s\n", m.app.DBName))
		}
	}

	b.WriteString("\n")

	if m.subview == subviewConfirmRestart {
		b.WriteString("  Restart type:\n")
		b.WriteString("    [c] Container restart (stop and start the entire container)\n")
		b.WriteString("    [s] Service restart (restart services inside the container)\n")
		b.WriteString("    [esc] Cancel\n")
		return b.String()
	}

	if m.subview == subviewUpdateTag {
		b.WriteString(fmt.Sprintf("  Update %s\n\n", m.appName))
		b.WriteString(dimStyle.Render(fmt.Sprintf("  Current image: %s\n", m.app.Image)))
		b.WriteString(fmt.Sprintf("  Tag: %s\n", m.updateInput.View()))
		b.WriteString(dimStyle.Render("\n  [enter] Continue  [esc] Cancel"))
		return b.String()
	}

	if m.subview == subviewConfirmUpdate {
		b.WriteString(fmt.Sprintf("  Update %s?\n\n", m.appName))
		b.WriteString(fmt.Sprintf("  Image: %s\n", m.updateImage))
		b.WriteString("\n")
		b.WriteString("  This will:\n")
		b.WriteString("    1. Pull fresh image from registry\n")
		b.WriteString("    2. Launch new container alongside the current one\n")
		b.WriteString("    3. Restore environment variables\n")
		b.WriteString("    4. Switch Traefik route to new container (zero downtime)\n")
		b.WriteString("    5. Remove old container\n")
		b.WriteString("\n")
		b.WriteString("  [y] Yes  [n] No\n")
		return b.String()
	}

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
		b.WriteString(dimStyle.Render("  [s] Stop  [t] Start  [r] Restart  [l] Logs  [e] Env  [esc] Back"))
	} else {
		b.WriteString(dimStyle.Render("  [s] Stop  [t] Start  [r] Restart  [u] Update  [l] Logs  [e] Env  [d] Delete  [esc] Back"))
	}

	return b.String()
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func (m Model) stopApp() tea.Cmd {
	name := m.containerName()
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
	name := m.containerName()
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
	name := m.containerName()
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

func (m Model) restartService() tea.Cmd {
	name := m.containerName()
	ic := m.incusClient
	return func() tea.Msg {
		ctx := context.Background()

		// Determine which services to restart
		// For known system containers, restart their specific service
		// For app containers, restart all non-system services
		var services []string
		switch name {
		case "proxy1":
			services = []string{"traefik"}
		case "db1":
			services = []string{"postgresql"}
		default:
			// For app containers, try to restart all active services
			// List failed or active services and restart them
			output, err := ic.Exec(ctx, name, []string{
				"systemctl", "list-units", "--type=service", "--state=active",
				"--no-pager", "--no-legend", "--plain",
			})
			if err == nil {
				for _, line := range strings.Split(output, "\n") {
					fields := strings.Fields(line)
					if len(fields) > 0 && strings.HasSuffix(fields[0], ".service") {
						svc := strings.TrimSuffix(fields[0], ".service")
						// Skip system services
						if svc != "systemd-journald" && svc != "systemd-logind" &&
							svc != "dbus" && svc != "cron" && svc != "ssh" &&
							!strings.HasPrefix(svc, "systemd-") {
							services = append(services, svc)
						}
					}
				}
			}
		}

		if len(services) == 0 {
			return actionResult{msg: "No services found to restart"}
		}

		var restarted []string
		for _, svc := range services {
			_, err := ic.Exec(ctx, name, []string{"systemctl", "restart", svc})
			if err != nil {
				return actionResult{err: fmt.Errorf("restarting %s: %w", svc, err)}
			}
			restarted = append(restarted, svc)
		}

		return actionResult{msg: fmt.Sprintf("Restarted services: %s", strings.Join(restarted, ", "))}
	}
}

func (m Model) fetchLogs() tea.Cmd {
	name := m.containerName()
	ic := m.incusClient
	return func() tea.Msg {
		ctx := context.Background()

		// Try journalctl first (system containers with systemd)
		if output, err := ic.Exec(ctx, name, []string{
			"journalctl", "-n", "200", "--no-pager", "--no-hostname",
		}); err == nil && strings.TrimSpace(output) != "" {
			return logsResult{content: output}
		}

		// For OCI containers: read the host-side console log
		// This is where Incus captures stdout/stderr from the container's init process
		consolePath := fmt.Sprintf("/var/log/incus/%s/console.log", name)
		if data, err := os.ReadFile(consolePath); err == nil && len(data) > 0 {
			return logsResult{content: string(data)}
		}

		// Try syslog as last resort
		if output, err := ic.Exec(ctx, name, []string{
			"tail", "-n", "200", "/var/log/syslog",
		}); err == nil && strings.TrimSpace(output) != "" {
			return logsResult{content: output}
		}

		return logsResult{content: "(no logs available — OCI containers log to /var/log/incus/<name>/console.log on the host)"}
	}
}

func (m Model) deleteApp() tea.Cmd {
	name := m.appName
	cName := m.containerName()
	app := m.app
	ic := m.incusClient
	reg := m.registry
	return func() tea.Msg {
		ctx := context.Background()

		// Delete container
		if err := ic.DeleteContainer(ctx, cName); err != nil {
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

func (m Model) updateApp(image string) tea.Cmd {
	name := m.appName
	app := m.app
	ic := m.incusClient
	reg := m.registry
	return func() tea.Msg {
		if app == nil {
			return actionResult{err: fmt.Errorf("no app config found")}
		}

		ctx := context.Background()
		timestamp := time.Now().Format("0102-1504") // MMDD-HHMM
		newName := name + "-" + timestamp

		// Resolve the actual container name (may differ from app name after previous updates)
		oldContainerName := name
		if app.ContainerName != "" {
			oldContainerName = app.ContainerName
		}

		// 1. Save env vars from the old container
		envVars, err := ic.GetEnvVars(ctx, oldContainerName)
		if err != nil {
			envVars = make(map[string]string) // continue without env vars
		}

		// 2. Delete cached image to force a fresh pull
		_ = ic.DeleteCachedImage(ctx, image)

		// 3. Launch new container from the fresh image
		if err := ic.LaunchOCI(ctx, newName, image); err != nil {
			return actionResult{err: fmt.Errorf("launching new container: %w", err)}
		}

		// 3. Wait for IP on the new container
		newIP, err := ic.WaitForIP(ctx, newName, 30*time.Second)
		if err != nil {
			// Cleanup: delete the new container
			_ = ic.DeleteContainer(ctx, newName)
			return actionResult{err: fmt.Errorf("new container failed to get IP: %w", err)}
		}

		// 4. Restore env vars on the new container
		if len(envVars) > 0 {
			if err := ic.RestoreEnvVars(ctx, newName, envVars); err != nil {
				_ = ic.DeleteContainer(ctx, newName)
				return actionResult{err: fmt.Errorf("restoring env vars: %w", err)}
			}
		}

		// 5. Switch Traefik route to the new container (zero-downtime cutover)
		if app.Domain != "" {
			if err := traefik.PushRoute(ic, name, app.Domain, newIP, app.Port, app.TLS); err != nil {
				_ = ic.DeleteContainer(ctx, newName)
				return actionResult{err: fmt.Errorf("updating route: %w", err)}
			}
		}

		// 6. Delete the old container (no longer receiving traffic)
		_ = ic.DeleteContainer(ctx, oldContainerName)

		// 7. Update registry — keep app name, track new container name
		_ = reg.UpdateIP(name, newIP)
		_ = reg.UpdateImage(name, image)
		_ = reg.UpdateContainerName(name, newName)

		return actionResult{msg: fmt.Sprintf("Updated to %s", image)}
	}
}

func (m Model) fetchEnvVars() tea.Cmd {
	name := m.containerName()
	ic := m.incusClient
	return func() tea.Msg {
		envs, err := ic.GetEnvVars(context.Background(), name)
		return envVarsResult{envVars: envs, err: err}
	}
}

func (m Model) setEnvVar(key, value string) tea.Cmd {
	name := m.containerName()
	ic := m.incusClient
	return func() tea.Msg {
		if err := ic.SetEnvVar(context.Background(), name, key, value); err != nil {
			return actionResult{err: err}
		}
		// Refresh the env var list
		envs, _ := ic.GetEnvVars(context.Background(), name)
		return envVarsResult{envVars: envs}
	}
}

func (m Model) deleteEnvVar(key string) tea.Cmd {
	name := m.containerName()
	ic := m.incusClient
	return func() tea.Msg {
		if err := ic.DeleteEnvVar(context.Background(), name, key); err != nil {
			return actionResult{err: err}
		}
		envs, _ := ic.GetEnvVars(context.Background(), name)
		return envVarsResult{envVars: envs}
	}
}
