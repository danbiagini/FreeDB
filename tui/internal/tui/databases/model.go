package databases

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/danbiagini/freedb-tui/internal/db"
	"github.com/danbiagini/freedb-tui/internal/incus"
)

type subview int

const (
	subviewList subview = iota
	subviewCreateName
	subviewCreateDone
	subviewConfirmDrop
)

type listResult struct {
	databases []db.DatabaseInfo
	err       error
}

type createResult struct {
	name     string
	password string
	err      error
}

type actionResult struct {
	msg string
	err error
}

type Model struct {
	incusClient *incus.Client
	subview     subview
	databases   []db.DatabaseInfo
	selected    int
	nameInput   textinput.Model
	lastCreated string
	lastPassword string
	message     string
	err         error
	done        bool
	busy        bool
}

func NewModel(ic *incus.Client) Model {
	input := textinput.New()
	input.Placeholder = "mydb"
	input.CharLimit = 30

	return Model{
		incusClient: ic,
		subview:     subviewList,
		nameInput:   input,
	}
}

func (m Model) Done() bool { return m.done }

func (m Model) Init() tea.Cmd {
	return m.fetchDatabases()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case listResult:
		m.busy = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.databases = msg.databases
			m.err = nil
		}
		return m, nil

	case createResult:
		m.busy = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.lastCreated = msg.name
			m.lastPassword = msg.password
			m.subview = subviewCreateDone
			m.err = nil
		}
		return m, m.fetchDatabases()

	case actionResult:
		m.busy = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.message = msg.msg
			m.err = nil
		}
		return m, m.fetchDatabases()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.subview == subviewCreateName {
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch m.subview {
	case subviewCreateDone:
		// Any key returns to list
		m.subview = subviewList
		return m, nil

	case subviewList:
		switch key {
		case "esc":
			m.done = true
			return m, nil
		case "a":
			m.subview = subviewCreateName
			m.nameInput.SetValue("")
			m.nameInput.Focus()
			m.message = ""
			m.err = nil
			return m, textinput.Blink
		case "d":
			if len(m.databases) > 0 && m.selected < len(m.databases) {
				m.subview = subviewConfirmDrop
			}
			return m, nil
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
			return m, nil
		case "down", "j":
			if m.selected < len(m.databases)-1 {
				m.selected++
			}
			return m, nil
		}

	case subviewCreateName:
		switch key {
		case "esc":
			m.subview = subviewList
			return m, nil
		case "enter":
			name := strings.TrimSpace(m.nameInput.Value())
			if name == "" {
				m.err = fmt.Errorf("database name is required")
				return m, nil
			}
			m.busy = true
			m.err = nil
			m.subview = subviewList
			return m, m.createDatabase(name)
		}

	case subviewConfirmDrop:
		switch key {
		case "y":
			if m.selected < len(m.databases) {
				m.busy = true
				m.subview = subviewList
				return m, m.dropDatabase(m.databases[m.selected].Name)
			}
			m.subview = subviewList
			return m, nil
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

	var b strings.Builder
	b.WriteString(titleStyle.Render("Databases (db1)"))
	b.WriteString("\n\n")

	if m.subview == subviewCreateDone {
		b.WriteString(successStyle.Render(fmt.Sprintf("  Database %q created!", m.lastCreated)))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("  Connection string:\n"))
		connStr := db.GetDBConnectionString("db1.incus", m.lastCreated, m.lastPassword)
		b.WriteString(fmt.Sprintf("  %s\n", connStr))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  SSH tunnel:\n"))
		b.WriteString(fmt.Sprintf("  ssh -L 5432:db1.incus:5432 user@host\n"))
		b.WriteString(fmt.Sprintf("  psql %s\n", db.GetDBConnectionString("localhost", m.lastCreated, m.lastPassword)))
		b.WriteString("\n")
		b.WriteString(warnStyle.Render("  Save this password — it cannot be retrieved later."))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("  Press any key to continue"))
		return b.String()
	}

	if m.subview == subviewCreateName {
		b.WriteString(fmt.Sprintf("  Database name: %s\n", m.nameInput.View()))
		b.WriteString("\n")
		if m.err != nil {
			b.WriteString(errStyle.Render(fmt.Sprintf("  %v", m.err)) + "\n")
		}
		b.WriteString(dimStyle.Render("  [enter] Create  [esc] Cancel"))
		return b.String()
	}

	if len(m.databases) == 0 {
		b.WriteString(dimStyle.Render("  No databases found"))
		b.WriteString("\n")
	} else {
		b.WriteString(fmt.Sprintf("  %-20s %-16s %s\n", "NAME", "OWNER", "SIZE"))
		b.WriteString(fmt.Sprintf("  %-20s %-16s %s\n", "----", "-----", "----"))
		for i, d := range m.databases {
			line := fmt.Sprintf("  %-20s %-16s %s", d.Name, d.Owner, d.Size)
			if i == m.selected {
				b.WriteString(selectedStyle.Render(line))
			} else {
				b.WriteString(line)
			}
			b.WriteString("\n")
		}
	}

	if m.subview == subviewConfirmDrop && m.selected < len(m.databases) {
		b.WriteString("\n")
		b.WriteString(warnStyle.Render(fmt.Sprintf("  Drop database %q and its user? This cannot be undone. [y/n]", m.databases[m.selected].Name)))
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

	// Show last backup status
	b.WriteString("\n")
	backupInfo := getLastBackupInfo()
	if backupInfo != "" {
		b.WriteString(dimStyle.Render("  Last backup: " + backupInfo))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  [a] Create database  [d] Drop database  [esc] Back"))

	return b.String()
}

func getLastBackupInfo() string {
	statusFile := "/var/lib/freedb/backup-status.json"
	data, err := os.ReadFile(statusFile)
	if err == nil {
		var status struct {
			Database    string `json:"database"`
			Status      string `json:"status"`
			Timestamp   string `json:"timestamp"`
			File        string `json:"file"`
			SizeBytes   int64  `json:"size_bytes"`
			CloudUpload string `json:"cloud_upload"`
			Error       string `json:"error"`
		}
		if json.Unmarshal(data, &status) == nil {
			size := formatSize(status.SizeBytes)
			cloud := status.CloudUpload
			if cloud == "uploaded" {
				cloud = "uploaded to cloud"
			} else if cloud == "failed" {
				cloud = "cloud upload FAILED"
			} else if cloud == "skipped" {
				cloud = "local only"
			}

			result := fmt.Sprintf("%s — %s (%s, %s)", status.Status, status.File, size, cloud)
			if status.Timestamp != "" {
				result += " at " + status.Timestamp
			}
			if status.Error != "" {
				result += " [" + status.Error + "]"
			}
			return result
		}
	}

	// Fallback: check backup directory for files
	backupDir := "/var/lib/freedb/backups"
	entries, err := os.ReadDir(backupDir)
	if err != nil || len(entries) == 0 {
		return "no backups found"
	}

	var newest os.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if newest == nil {
			newest = e
			continue
		}
		ni, _ := newest.Info()
		ei, _ := e.Info()
		if ni != nil && ei != nil && ei.ModTime().After(ni.ModTime()) {
			newest = e
		}
	}

	if newest == nil {
		return "no backups found"
	}

	info, _ := newest.Info()
	if info == nil {
		return newest.Name()
	}

	return fmt.Sprintf("%s (%s, %s)", newest.Name(), formatSize(info.Size()), info.ModTime().Format("2006-01-02 15:04"))
}

func formatSize(bytes int64) string {
	if bytes > 1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	} else if bytes > 1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%d B", bytes)
}

func (m Model) fetchDatabases() tea.Cmd {
	ic := m.incusClient
	return func() tea.Msg {
		dbs, err := db.ListDatabases(context.Background(), ic)
		return listResult{databases: dbs, err: err}
	}
}

func (m Model) createDatabase(name string) tea.Cmd {
	ic := m.incusClient
	return func() tea.Msg {
		password, err := db.CreateDatabase(context.Background(), ic, name)
		if err != nil {
			return createResult{err: err}
		}
		return createResult{name: name, password: password}
	}
}

func (m Model) dropDatabase(name string) tea.Cmd {
	ic := m.incusClient
	return func() tea.Msg {
		if err := db.DropDatabase(context.Background(), ic, name); err != nil {
			return actionResult{err: err}
		}
		return actionResult{msg: fmt.Sprintf("Database %q dropped", name)}
	}
}
