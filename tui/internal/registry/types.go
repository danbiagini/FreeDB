package registry

import "time"

type AppType string

const (
	AppTypeContainer AppType = "container"
	AppTypeVM        AppType = "vm"
)

type App struct {
	Name          string            `json:"name"`
	ContainerName string            `json:"container_name,omitempty"` // actual incus container name (may differ after updates)
	Type          AppType           `json:"type"`
	Image         string            `json:"image,omitempty"`
	Domains       []string          `json:"domains,omitempty"`
	Port          int               `json:"port"`
	TLS           bool              `json:"tls"`
	HasDB         bool              `json:"has_db"`
	DBName        string            `json:"db_name,omitempty"`
	DBUser        string            `json:"db_user,omitempty"`
	DBEnvVar      string            `json:"db_env_var,omitempty"`
	EnvVars       map[string]string `json:"env_vars,omitempty"`
	LastIP        string            `json:"last_ip"`
	CloudID       string            `json:"cloud_id,omitempty"`
	VMSpec        *VMSpec           `json:"vm_spec,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
}

// GetDomains returns the app's domains slice.
func (a *App) GetDomains() []string {
	return a.Domains
}

// SetDomains sets the app's domains.
func (a *App) SetDomains(domains []string) {
	a.Domains = domains
}

// HasDomains returns true if the app has at least one domain configured.
func (a *App) HasDomains() bool {
	return len(a.Domains) > 0
}

type VMSpec struct {
	MachineType string `json:"machine_type"`
	DiskSizeGB  int    `json:"disk_size_gb"`
	Image       string `json:"image"`
	Zone        string `json:"zone"`
}
