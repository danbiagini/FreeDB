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

	"github.com/danbiagini/FreeDB/tui/internal/config"
	"github.com/danbiagini/FreeDB/tui/internal/db"
	"github.com/danbiagini/FreeDB/tui/internal/incus"
	"github.com/danbiagini/FreeDB/tui/internal/registry"
	"github.com/danbiagini/FreeDB/tui/internal/traefik"
)

var nameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

type step int

const (
	stepName step = iota
	stepImage
	stepExpose
	stepDomain
	stepPort
	stepTLS
	stepDB
	stepDBEnvVar
	stepEnvVars
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
	needsRoute  bool
	needsDB     bool
	tls         bool
	envVars     []string // accumulated KEY=VALUE entries
	envInput    textinput.Model
	incusClient *incus.Client
	registry    *registry.AppRegistry
	cfg         *config.Config
	err         error
	deployMsg   string
	done        bool
	cancelled   bool
}

func NewModel(ic *incus.Client, reg *registry.AppRegistry, cfg *config.Config) Model {
	inputs := make([]textinput.Model, 5)

	inputs[0] = textinput.New()
	inputs[0].Placeholder = "myapp"
	inputs[0].Focus()
	inputs[0].CharLimit = 30

	inputs[1] = textinput.New()
	inputs[1].Placeholder = "docker.io/traefik/whoami or debian/12/cloud"
	inputs[1].CharLimit = 100

	inputs[2] = textinput.New()
	inputs[2].Placeholder = "myapp.example.com, www.myapp.example.com"
	inputs[2].CharLimit = 200

	inputs[3] = textinput.New()
	inputs[3].Placeholder = "8080"
	inputs[3].CharLimit = 5

	inputs[4] = textinput.New()
	inputs[4].Placeholder = "DATABASE_URL"
	inputs[4].CharLimit = 50

	envInput := textinput.New()
	envInput.Placeholder = "KEY=VALUE (enter to add, empty to finish)"
	envInput.CharLimit = 200

	return Model{
		step:        stepName,
		inputs:      inputs,
		envInput:    envInput,
		tls:         true, // default: TLS enabled
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
		if m.step == stepDone {
			m.done = true
			return m, nil
		}

		switch msg.String() {
		case "esc":
			m.cancelled = true
			m.done = true
			return m, nil

		case "y", "n":
			if m.step == stepExpose {
				m.needsRoute = msg.String() == "y"
				if m.needsRoute {
					m.step = stepDomain
					m.inputs[2].Focus()
				} else {
					m.step = stepDB
				}
				return m, nil
			}
			if m.step == stepTLS {
				m.tls = msg.String() == "y"
				m.step = stepDB
				return m, nil
			}
			if m.step == stepDB {
				m.needsDB = msg.String() == "y"
				if m.needsDB {
					m.step = stepDBEnvVar
					m.inputs[4].SetValue("DATABASE_URL")
					m.inputs[4].Focus()
					return m, nil
				}
				m.step = stepEnvVars
				m.envInput.SetValue("")
				m.envInput.Focus()
				return m, nil
			}

		case "enter":
			return m.handleEnter()
		}

	case deployResult:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.deployMsg = "App deployed successfully!"
		}
		m.step = stepDone
		return m, nil
	}

	if m.step == stepName || m.step == stepImage || m.step == stepDomain || m.step == stepPort || m.step == stepDBEnvVar {
		var cmd tea.Cmd
		idx := map[step]int{
			stepName: 0, stepImage: 1, stepDomain: 2, stepPort: 3, stepDBEnvVar: 4,
		}[m.step]
		m.inputs[idx], cmd = m.inputs[idx].Update(msg)
		return m, cmd
	}
	if m.step == stepEnvVars {
		var cmd tea.Cmd
		m.envInput, cmd = m.envInput.Update(msg)
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
		m.step = stepExpose
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
		m.step = stepTLS
		return m, nil

	case stepTLS:
		return m, nil

	case stepDB:
		return m, nil

	case stepDBEnvVar:
		m.err = nil
		m.step = stepEnvVars
		m.envInput.SetValue("")
		m.envInput.Focus()
		return m, nil

	case stepEnvVars:
		// Enter with empty value = done adding env vars
		val := strings.TrimSpace(m.envInput.Value())
		if val == "" {
			m.step = stepConfirm
			return m, nil
		}
		if !strings.Contains(val, "=") {
			m.err = fmt.Errorf("format: KEY=VALUE")
			return m, nil
		}
		m.err = nil
		m.envVars = append(m.envVars, val)
		m.envInput.SetValue("")
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
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("Add App"))
	b.WriteString("\n\n")

	// Name
	if m.step > stepName {
		b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("App name:"), m.inputs[0].Value()))
	} else if m.step == stepName {
		b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("App name:"), m.inputs[0].View()))
	}

	// Image
	if m.step > stepImage {
		b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Image:"), m.inputs[1].Value()))
	} else if m.step == stepImage {
		b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Image:"), m.inputs[1].View()))
	}

	// Expose via Traefik
	if m.step >= stepExpose && m.step < stepDeploying {
		if m.step == stepExpose {
			b.WriteString(fmt.Sprintf("\n  %s [y/n] ", labelStyle.Render("Expose via Traefik?")))
		} else {
			expStr := "no"
			if m.needsRoute {
				expStr = "yes"
			}
			b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Expose:"), expStr))
		}
	}

	// Domain, Port, TLS — only if exposed
	if m.needsRoute {
		if m.step > stepDomain {
			b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Domain:"), m.inputs[2].Value()))
		} else if m.step == stepDomain {
			b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Domain:"), m.inputs[2].View()))
		}

		portHint := "  " + dimStyle.Render("(port your app listens on inside the container)")
		if m.step > stepPort {
			b.WriteString(fmt.Sprintf("  %s %s%s\n", labelStyle.Render("App port:"), m.inputs[3].Value(), portHint))
		} else if m.step == stepPort {
			b.WriteString(fmt.Sprintf("  %s %s%s\n", labelStyle.Render("App port:"), m.inputs[3].View(), portHint))
		}

		// TLS option
		if m.step >= stepTLS && m.step < stepDeploying {
			if m.step == stepTLS {
				b.WriteString(fmt.Sprintf("\n  %s [y/n] ", labelStyle.Render("TLS (Let's Encrypt):")))
			} else {
				tlsStr := "yes (Let's Encrypt)"
				if !m.tls {
					tlsStr = "no (HTTP only)"
				}
				b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("TLS:"), tlsStr))
			}
		}
	}

	// Database option
	if m.step >= stepDB && m.step < stepDeploying {
		if m.step == stepDB {
			b.WriteString(fmt.Sprintf("\n  %s [y/n] ", labelStyle.Render("Needs database?")))
		} else if m.step > stepDB {
			dbStr := "no"
			if m.needsDB {
				dbStr = "yes"
			}
			b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Database:"), dbStr))
		}
	}

	// DB env var name
	if m.needsDB && m.step >= stepDBEnvVar && m.step < stepDeploying {
		if m.step == stepDBEnvVar {
			b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("DB env var:"), m.inputs[4].View()))
		} else {
			b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("DB env var:"), m.inputs[4].Value()))
		}
	}

	// Environment variables
	if m.step >= stepEnvVars && m.step < stepDeploying {
		if len(m.envVars) > 0 {
			for _, ev := range m.envVars {
				parts := strings.SplitN(ev, "=", 2)
				b.WriteString(fmt.Sprintf("  %s %s\n", dimStyle.Render(parts[0]+"="), parts[1]))
			}
		}
		if m.step == stepEnvVars {
			b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("Env var:"), m.envInput.View()))
			b.WriteString(dimStyle.Render("    Enter KEY=VALUE, empty line to finish"))
			b.WriteString("\n")
		}
	}

	// External access summary
	if m.step == stepConfirm {
		if m.needsRoute {
			primaryDomain := strings.SplitN(strings.TrimSpace(m.inputs[2].Value()), ",", 2)[0]
			if m.tls {
				b.WriteString(dimStyle.Render(fmt.Sprintf("\n  External: https://%s (port 443, TLS via Let's Encrypt)", primaryDomain)))
			} else {
				b.WriteString(dimStyle.Render(fmt.Sprintf("\n  External: http://%s (port 80, no TLS)", primaryDomain)))
			}
		} else {
			b.WriteString(dimStyle.Render("\n  Internal only — no external routing"))
		}
		b.WriteString("\n\n  Press [enter] to deploy, [esc] to cancel\n")
	}

	if m.step == stepDeploying {
		b.WriteString("\n  Deploying...\n")
	}

	if m.step == stepDone {
		if m.err != nil {
			b.WriteString("\n" + errStyle.Render(fmt.Sprintf("  Deploy failed: %v", m.err)) + "\n")
		} else {
			b.WriteString("\n" + successStyle.Render("  "+m.deployMsg) + "\n")
			if m.needsRoute {
				primaryDomain := strings.SplitN(strings.TrimSpace(m.inputs[2].Value()), ",", 2)[0]
				if m.tls {
					b.WriteString(dimStyle.Render(fmt.Sprintf("  Access at: https://%s", primaryDomain)) + "\n")
				} else {
					b.WriteString(dimStyle.Render(fmt.Sprintf("  Access at: http://%s", primaryDomain)) + "\n")
				}
			}
		}
		b.WriteString(dimStyle.Render("\n  Press any key to return to dashboard"))
		return b.String()
	}

	if m.err != nil && m.step < stepDone {
		b.WriteString("\n" + errStyle.Render(fmt.Sprintf("  %v", m.err)))
	}

	if m.step < stepDeploying {
		b.WriteString(dimStyle.Render("\n\n  [esc] Cancel"))
	}

	return b.String()
}

func (m Model) deploy() tea.Cmd {
	name := strings.TrimSpace(m.inputs[0].Value())
	image := strings.TrimSpace(m.inputs[1].Value())
	var domains []string
	for _, d := range strings.Split(m.inputs[2].Value(), ",") {
		d = strings.TrimSpace(d)
		if d != "" {
			domains = append(domains, d)
		}
	}
	portStr := strings.TrimSpace(m.inputs[3].Value())
	needsRoute := m.needsRoute
	needsDB := m.needsDB
	dbEnvVar := strings.TrimSpace(m.inputs[4].Value())
	if dbEnvVar == "" {
		dbEnvVar = "DATABASE_URL"
	}
	tls := m.tls
	envVars := make([]string, len(m.envVars))
	copy(envVars, m.envVars)

	port := 8080
	if portStr != "" {
		fmt.Sscanf(portStr, "%d", &port)
	}

	ic := m.incusClient
	reg := m.registry

	return func() tea.Msg {
		ctx := context.Background()

		// Launch container — detect OCI vs Linux container image
		// OCI if: has a registry domain (.io, .com, .dev), or uses remote:image format
		// where the remote is a configured OCI remote
		isOCI := strings.Contains(image, "docker.io") ||
			strings.Contains(image, ".io/") ||
			strings.Contains(image, ".com/") ||
			strings.Contains(image, ".dev/")
		if !isOCI && strings.Contains(image, ":") {
			parts := strings.SplitN(image, ":", 2)
			if !strings.Contains(parts[0], ".") && !strings.Contains(parts[0], "/") {
				// Looks like "remote:alias" — check if it's a known OCI remote
				if remotes, err := ic.ListRemotes(); err == nil {
					for _, r := range remotes {
						if r.Name == parts[0] {
							isOCI = true
							break
						}
					}
				}
			}
		}
		// 1. Create container WITHOUT starting (need to set env vars first)
		if isOCI {
			if err := ic.InitOCI(ctx, name, image); err != nil {
				return deployResult{err: fmt.Errorf("creating OCI container: %w", err)}
			}
		} else {
			if err := ic.LaunchContainer(ctx, name, image); err != nil {
				return deployResult{err: fmt.Errorf("launching container: %w", err)}
			}
		}

		// 2. Create database if requested
		dbName := ""
		if needsDB {
			dbName = name
			dbPassword, err := db.CreateDatabase(ctx, ic, name)
			if err != nil {
				return deployResult{err: fmt.Errorf("creating database: %w", err)}
			}

			// Use stable DNS name for db connection string
			connStr := db.GetDBConnectionString("db1.incus", name, dbPassword)

			// Inject DATABASE_URL env var BEFORE starting
			if err := ic.SetEnvVar(ctx, name, dbEnvVar, connStr); err != nil {
				return deployResult{err: fmt.Errorf("setting %s: %w", dbEnvVar, err)}
			}
		}

		// 3. Set additional env vars BEFORE starting
		for _, ev := range envVars {
			parts := strings.SplitN(ev, "=", 2)
			if len(parts) == 2 {
				if err := ic.SetEnvVar(ctx, name, parts[0], parts[1]); err != nil {
					return deployResult{err: fmt.Errorf("setting env %s: %w", parts[0], err)}
				}
			}
		}

		// 4. Start the container now that env vars are configured
		if isOCI {
			if err := ic.StartContainer(ctx, name); err != nil {
				return deployResult{err: fmt.Errorf("starting container: %w", err)}
			}
		}

		// 5. Wait for IP
		ip, err := ic.WaitForIP(ctx, name, 30*time.Second)
		if err != nil {
			return deployResult{err: fmt.Errorf("waiting for IP: %w", err)}
		}

		// 6. Create Traefik route (skip for internal-only containers)
		if needsRoute && len(domains) > 0 {
			if err := traefik.PushRoute(ic, name, domains, ip, port, tls); err != nil {
				return deployResult{err: fmt.Errorf("creating route: %w", err)}
			}
		}

		// Save to registry
		envMap := make(map[string]string)
		for _, ev := range envVars {
			parts := strings.SplitN(ev, "=", 2)
			if len(parts) == 2 {
				envMap[parts[0]] = parts[1]
			}
		}
		app := &registry.App{
			Name:      name,
			Type:      registry.AppTypeContainer,
			Image:     image,
			Port:      port,
			TLS:       tls,
			HasDB:     needsDB,
			DBName:    dbName,
			DBUser:    dbName,
			DBEnvVar:  dbEnvVar,
			EnvVars:   envMap,
			LastIP:    ip,
			CreatedAt: time.Now(),
		}
		app.SetDomains(domains)
		if err := reg.Add(app); err != nil {
			return deployResult{err: fmt.Errorf("saving to registry: %w", err)}
		}

		return deployResult{}
	}
}
