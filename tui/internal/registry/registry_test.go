package registry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	reg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(reg.Apps) != 0 {
		t.Fatalf("expected 0 apps, got %d", len(reg.Apps))
	}
}

func TestAddAndGet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	reg, _ := Load(path)

	app := &App{
		Name:      "myapp",
		Type:      AppTypeContainer,
		Domains:   []string{"myapp.example.com"},
		Port:      8080,
		CreatedAt: time.Now(),
	}
	if err := reg.Add(app); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	got, ok := reg.Get("myapp")
	if !ok {
		t.Fatal("Get returned false")
	}
	domains := got.GetDomains()
	if len(domains) == 0 || domains[0] != "myapp.example.com" {
		t.Fatalf("expected domain myapp.example.com, got %v", domains)
	}
}

func TestRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	reg, _ := Load(path)

	reg.Add(&App{Name: "myapp", Type: AppTypeContainer})
	reg.Remove("myapp")

	_, ok := reg.Get("myapp")
	if ok {
		t.Fatal("expected app to be removed")
	}
}

func TestPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	reg, _ := Load(path)

	reg.Add(&App{Name: "myapp", Type: AppTypeContainer, Port: 3000})

	// Reload from disk
	reg2, err := Load(path)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	got, ok := reg2.Get("myapp")
	if !ok {
		t.Fatal("app not found after reload")
	}
	if got.Port != 3000 {
		t.Fatalf("expected port 3000, got %d", got.Port)
	}
}

func TestUpdateIP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	reg, _ := Load(path)

	reg.Add(&App{Name: "myapp", LastIP: "10.0.0.1"})
	reg.UpdateIP("myapp", "10.0.0.2")

	got, _ := reg.Get("myapp")
	if got.LastIP != "10.0.0.2" {
		t.Fatalf("expected IP 10.0.0.2, got %s", got.LastIP)
	}
}

func TestList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	reg, _ := Load(path)

	reg.Add(&App{Name: "app1"})
	reg.Add(&App{Name: "app2"})

	apps := reg.List()
	if len(apps) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(apps))
	}
}

func TestLoadCorruptedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	os.WriteFile(path, []byte("not json"), 0644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error loading corrupted file")
	}
}

func TestPostMigrationLoad(t *testing.T) {
	// Simulate a registry that has already been through the v1.0 migration:
	// domains array is populated, legacy domain field may still be present in JSON
	// but the Go struct ignores it.
	path := filepath.Join(t.TempDir(), "registry.json")
	migrated := `{"apps":{"myapp":{"name":"myapp","type":"container","domains":["myapp.example.com"],"port":8080,"last_ip":"","created_at":"0001-01-01T00:00:00Z"}}}`
	os.WriteFile(path, []byte(migrated), 0644)

	reg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	app, ok := reg.Get("myapp")
	if !ok {
		t.Fatal("app not found")
	}
	got := app.GetDomains()
	if len(got) != 1 || got[0] != "myapp.example.com" {
		t.Fatalf("expected [myapp.example.com], got %v", got)
	}
}

func TestMultiDomainSetAndGet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	reg, _ := Load(path)

	app := &App{Name: "myapp", Type: AppTypeContainer}
	app.SetDomains([]string{"myapp.example.com", "www.myapp.example.com"})
	reg.Add(app)

	got, ok := reg.Get("myapp")
	if !ok {
		t.Fatal("app not found")
	}
	domains := got.GetDomains()
	if len(domains) == 0 || domains[0] != "myapp.example.com" {
		t.Fatalf("expected primary domain myapp.example.com, got %v", domains)
	}
	if len(domains) != 2 {
		t.Fatalf("expected 2 domains, got %v", domains)
	}
}

func TestUpdateDomains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	reg, _ := Load(path)

	reg.Add(&App{Name: "myapp", Type: AppTypeContainer, Domains: []string{"myapp.example.com"}})
	reg.UpdateDomains("myapp", []string{"myapp.example.com", "api.myapp.example.com"})

	// Reload from disk and verify persistence
	reg2, _ := Load(path)
	app, ok := reg2.Get("myapp")
	if !ok {
		t.Fatal("app not found after reload")
	}
	domains := app.GetDomains()
	if len(domains) != 2 {
		t.Fatalf("expected 2 domains after reload, got %v", domains)
	}
	if domains[1] != "api.myapp.example.com" {
		t.Fatalf("expected api.myapp.example.com, got %s", domains[1])
	}
}
