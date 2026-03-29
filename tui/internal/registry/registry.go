package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type AppRegistry struct {
	Apps     map[string]*App `json:"apps"`
	FilePath string          `json:"-"`
	mu       sync.Mutex
}

func Load(path string) (*AppRegistry, error) {
	r := &AppRegistry{
		Apps:     make(map[string]*App),
		FilePath: path,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, r); err != nil {
		return nil, err
	}
	if r.Apps == nil {
		r.Apps = make(map[string]*App)
	}

	return r, nil
}

func (r *AppRegistry) Save() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	dir := filepath.Dir(r.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(r.FilePath, data, 0644)
}

func (r *AppRegistry) Add(app *App) error {
	r.mu.Lock()
	r.Apps[app.Name] = app
	r.mu.Unlock()
	return r.Save()
}

func (r *AppRegistry) Remove(name string) error {
	r.mu.Lock()
	delete(r.Apps, name)
	r.mu.Unlock()
	return r.Save()
}

func (r *AppRegistry) Get(name string) (*App, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	app, ok := r.Apps[name]
	return app, ok
}

// Reload re-reads the registry from disk, picking up changes made by other processes
func (r *AppRegistry) Reload() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := os.ReadFile(r.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var fresh AppRegistry
	if err := json.Unmarshal(data, &fresh); err != nil {
		return err
	}
	if fresh.Apps != nil {
		r.Apps = fresh.Apps
	}
	return nil
}

func (r *AppRegistry) List() []*App {
	r.mu.Lock()
	defer r.mu.Unlock()
	apps := make([]*App, 0, len(r.Apps))
	for _, app := range r.Apps {
		apps = append(apps, app)
	}
	return apps
}

func (r *AppRegistry) UpdateIP(name, ip string) error {
	r.mu.Lock()
	if app, ok := r.Apps[name]; ok {
		app.LastIP = ip
	}
	r.mu.Unlock()
	return r.Save()
}

func (r *AppRegistry) UpdateContainerName(name, containerName string) error {
	r.mu.Lock()
	if app, ok := r.Apps[name]; ok {
		app.ContainerName = containerName
	}
	r.mu.Unlock()
	return r.Save()
}

func (r *AppRegistry) UpdateImage(name, image string) error {
	r.mu.Lock()
	if app, ok := r.Apps[name]; ok {
		app.Image = image
	}
	r.mu.Unlock()
	return r.Save()
}
