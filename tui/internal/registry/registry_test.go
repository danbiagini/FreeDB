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
		Domain:    "myapp.example.com",
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
	if got.Domain != "myapp.example.com" {
		t.Fatalf("expected domain myapp.example.com, got %s", got.Domain)
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
