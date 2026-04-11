package addapp

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/danbiagini/FreeDB/tui/internal/registry"
)

// key sends a KeyMsg to the model and returns the updated model.
func key(m Model, k string) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	return updated.(Model)
}

func enter(m Model) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return updated.(Model)
}

func esc(m Model) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	return updated.(Model)
}

// typeText sends each character as a key event, simulating user typing.
func typeText(m Model, text string) Model {
	for _, ch := range text {
		m = key(m, string(ch))
	}
	return m
}

// newTestModel creates a Model suitable for step-transition testing.
func newTestModel(t *testing.T) Model {
	t.Helper()
	reg, _ := registry.Load(filepath.Join(t.TempDir(), "reg.json"))
	return NewModel(nil, reg, nil)
}

// advancePastName types a valid name and presses enter.
func advancePastName(m Model) Model {
	m = typeText(m, "myapp")
	return enter(m)
}

// advancePastImage types an image and presses enter.
func advancePastImage(m Model, image string) Model {
	m = typeText(m, image)
	return enter(m)
}

func TestInitialStep(t *testing.T) {
	m := newTestModel(t)
	if m.step != stepName {
		t.Fatalf("expected stepName, got %d", m.step)
	}
}

func TestNameValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "myapp", false},
		{"with hyphens", "my-app", false},
		{"starts with number", "1app", true},
		{"uppercase", "MyApp", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m = typeText(m, tt.input)
			m = enter(m)

			if tt.wantErr {
				if m.err == nil {
					t.Error("expected validation error")
				}
				if m.step != stepName {
					t.Errorf("expected to stay on stepName, got %d", m.step)
				}
			} else {
				if m.err != nil {
					t.Errorf("unexpected error: %v", m.err)
				}
				if m.step != stepImage {
					t.Errorf("expected stepImage, got %d", m.step)
				}
			}
		})
	}
}

func TestImageRequired(t *testing.T) {
	m := newTestModel(t)
	m = advancePastName(m)
	m = enter(m) // empty image

	if m.err == nil {
		t.Error("expected error for empty image")
	}
	if m.step != stepImage {
		t.Errorf("expected stepImage, got %d", m.step)
	}
}

func TestImageAdvancesToExpose(t *testing.T) {
	m := newTestModel(t)
	m = advancePastName(m)
	m = advancePastImage(m, "docker.io/traefik/whoami")

	if m.step != stepExpose {
		t.Fatalf("expected stepExpose, got %d", m.step)
	}
}

// Full flow: OCI image, with Traefik, with TLS, with DB
func TestFlow_OCI_Traefik_TLS_DB(t *testing.T) {
	m := newTestModel(t)
	m = advancePastName(m)
	m = advancePastImage(m, "docker.io/traefik/whoami")

	// Expose via Traefik: yes
	if m.step != stepExpose {
		t.Fatalf("expected stepExpose, got %d", m.step)
	}
	m = key(m, "y")

	// Domain
	if m.step != stepDomain {
		t.Fatalf("expected stepDomain, got %d", m.step)
	}
	m = typeText(m, "myapp.example.com")
	m = enter(m)

	// Port
	if m.step != stepPort {
		t.Fatalf("expected stepPort, got %d", m.step)
	}
	m = enter(m) // accept default 8080

	// TLS
	if m.step != stepTLS {
		t.Fatalf("expected stepTLS, got %d", m.step)
	}
	m = key(m, "y")

	// DB
	if m.step != stepDB {
		t.Fatalf("expected stepDB, got %d", m.step)
	}
	m = key(m, "y")

	// DB env var
	if m.step != stepDBEnvVar {
		t.Fatalf("expected stepDBEnvVar, got %d", m.step)
	}
	m = enter(m) // accept default DATABASE_URL

	// Env vars — empty to finish
	if m.step != stepEnvVars {
		t.Fatalf("expected stepEnvVars, got %d", m.step)
	}
	m = enter(m)

	// Confirm
	if m.step != stepConfirm {
		t.Fatalf("expected stepConfirm, got %d", m.step)
	}

	if !m.needsRoute {
		t.Error("expected needsRoute=true")
	}
	if !m.tls {
		t.Error("expected tls=true")
	}
	if !m.needsDB {
		t.Error("expected needsDB=true")
	}
}

// Full flow: OCI image, with Traefik, no TLS, no DB
func TestFlow_OCI_Traefik_NoTLS_NoDB(t *testing.T) {
	m := newTestModel(t)
	m = advancePastName(m)
	m = advancePastImage(m, "docker.io/traefik/whoami")

	m = key(m, "y") // expose: yes
	m = typeText(m, "myapp.example.com")
	m = enter(m)    // domain
	m = enter(m)    // port (default)
	m = key(m, "n") // TLS: no
	m = key(m, "n") // DB: no
	m = enter(m)    // env vars: done

	if m.step != stepConfirm {
		t.Fatalf("expected stepConfirm, got %d", m.step)
	}
	if !m.needsRoute {
		t.Error("expected needsRoute=true")
	}
	if m.tls {
		t.Error("expected tls=false")
	}
	if m.needsDB {
		t.Error("expected needsDB=false")
	}
}

// Full flow: OCI image, no Traefik, with DB
func TestFlow_OCI_NoTraefik_DB(t *testing.T) {
	m := newTestModel(t)
	m = advancePastName(m)
	m = advancePastImage(m, "docker.io/library/redis")

	// Expose: no — should skip domain/port/TLS
	m = key(m, "n")

	if m.step != stepDB {
		t.Fatalf("expected stepDB after declining expose, got %d", m.step)
	}
	m = key(m, "y") // DB: yes

	if m.step != stepDBEnvVar {
		t.Fatalf("expected stepDBEnvVar, got %d", m.step)
	}
	m = enter(m) // accept default

	if m.step != stepEnvVars {
		t.Fatalf("expected stepEnvVars, got %d", m.step)
	}
	m = enter(m) // done

	if m.step != stepConfirm {
		t.Fatalf("expected stepConfirm, got %d", m.step)
	}
	if m.needsRoute {
		t.Error("expected needsRoute=false")
	}
	if !m.needsDB {
		t.Error("expected needsDB=true")
	}
}

// Full flow: OCI image, no Traefik, no DB
func TestFlow_OCI_NoTraefik_NoDB(t *testing.T) {
	m := newTestModel(t)
	m = advancePastName(m)
	m = advancePastImage(m, "docker.io/library/redis")

	m = key(m, "n") // expose: no
	m = key(m, "n") // DB: no
	m = enter(m)    // env vars: done

	if m.step != stepConfirm {
		t.Fatalf("expected stepConfirm, got %d", m.step)
	}
	if m.needsRoute {
		t.Error("expected needsRoute=false")
	}
	if m.needsDB {
		t.Error("expected needsDB=false")
	}
}

// Full flow: System container, with Traefik, with DB
func TestFlow_System_Traefik_DB(t *testing.T) {
	m := newTestModel(t)
	m = advancePastName(m)
	m = advancePastImage(m, "ubuntu/24.04/cloud")

	m = key(m, "y") // expose: yes

	if m.step != stepDomain {
		t.Fatalf("expected stepDomain, got %d", m.step)
	}
	m = typeText(m, "sys.example.com")
	m = enter(m)    // domain
	m = enter(m)    // port (default)
	m = key(m, "y") // TLS: yes
	m = key(m, "y") // DB: yes
	m = enter(m)    // DB env var (default)
	m = enter(m)    // env vars: done

	if m.step != stepConfirm {
		t.Fatalf("expected stepConfirm, got %d", m.step)
	}
	if !m.needsRoute {
		t.Error("expected needsRoute=true")
	}
	if !m.needsDB {
		t.Error("expected needsDB=true")
	}
}

// Full flow: System container, no Traefik, no DB
func TestFlow_System_NoTraefik_NoDB(t *testing.T) {
	m := newTestModel(t)
	m = advancePastName(m)
	m = advancePastImage(m, "debian/12/cloud")

	m = key(m, "n") // expose: no
	m = key(m, "n") // DB: no
	m = enter(m)    // env vars: done

	if m.step != stepConfirm {
		t.Fatalf("expected stepConfirm, got %d", m.step)
	}
	if m.needsRoute {
		t.Error("expected needsRoute=false")
	}
	if m.needsDB {
		t.Error("expected needsDB=false")
	}
}

// Full flow: System container, no Traefik, with DB
func TestFlow_System_NoTraefik_DB(t *testing.T) {
	m := newTestModel(t)
	m = advancePastName(m)
	m = advancePastImage(m, "ubuntu/24.04/cloud")

	m = key(m, "n") // expose: no
	m = key(m, "y") // DB: yes
	m = enter(m)    // DB env var (default)
	m = enter(m)    // env vars: done

	if m.step != stepConfirm {
		t.Fatalf("expected stepConfirm, got %d", m.step)
	}
	if m.needsRoute {
		t.Error("expected needsRoute=false")
	}
	if !m.needsDB {
		t.Error("expected needsDB=true")
	}
}

// Full flow: System container, with Traefik, no DB
func TestFlow_System_Traefik_NoDB(t *testing.T) {
	m := newTestModel(t)
	m = advancePastName(m)
	m = advancePastImage(m, "debian/12/cloud")

	m = key(m, "y") // expose: yes
	m = typeText(m, "sys.example.com")
	m = enter(m)    // domain
	m = enter(m)    // port (default)
	m = key(m, "n") // TLS: no
	m = key(m, "n") // DB: no
	m = enter(m)    // env vars: done

	if m.step != stepConfirm {
		t.Fatalf("expected stepConfirm, got %d", m.step)
	}
	if !m.needsRoute {
		t.Error("expected needsRoute=true")
	}
	if m.tls {
		t.Error("expected tls=false")
	}
	if m.needsDB {
		t.Error("expected needsDB=false")
	}
}

func TestEnvVarsAccumulate(t *testing.T) {
	m := newTestModel(t)
	m = advancePastName(m)
	m = advancePastImage(m, "docker.io/library/nginx")
	m = key(m, "n") // expose: no
	m = key(m, "n") // DB: no

	// Add two env vars
	m = typeText(m, "FOO=bar")
	m = enter(m)
	m = typeText(m, "BAZ=qux")
	m = enter(m)
	m = enter(m) // empty to finish

	if m.step != stepConfirm {
		t.Fatalf("expected stepConfirm, got %d", m.step)
	}
	if len(m.envVars) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(m.envVars))
	}
	if m.envVars[0] != "FOO=bar" {
		t.Errorf("expected FOO=bar, got %s", m.envVars[0])
	}
	if m.envVars[1] != "BAZ=qux" {
		t.Errorf("expected BAZ=qux, got %s", m.envVars[1])
	}
}

func TestEnvVarValidation(t *testing.T) {
	m := newTestModel(t)
	m = advancePastName(m)
	m = advancePastImage(m, "docker.io/library/nginx")
	m = key(m, "n") // expose: no
	m = key(m, "n") // DB: no

	// Invalid format (no =)
	m = typeText(m, "INVALID")
	m = enter(m)

	if m.err == nil {
		t.Error("expected error for invalid env var format")
	}
	if m.step != stepEnvVars {
		t.Errorf("expected to stay on stepEnvVars, got %d", m.step)
	}
}

func TestEscCancels(t *testing.T) {
	m := newTestModel(t)
	m = esc(m)

	if !m.cancelled {
		t.Error("expected cancelled=true")
	}
	if !m.done {
		t.Error("expected done=true")
	}
}

func TestDomainRequired(t *testing.T) {
	m := newTestModel(t)
	m = advancePastName(m)
	m = advancePastImage(m, "docker.io/traefik/whoami")
	m = key(m, "y") // expose: yes
	m = enter(m)    // empty domain

	if m.err == nil {
		t.Error("expected error for empty domain")
	}
	if m.step != stepDomain {
		t.Errorf("expected to stay on stepDomain, got %d", m.step)
	}
}
