package registry

import "time"

type AppType string

const (
	AppTypeContainer AppType = "container"
	AppTypeVM        AppType = "vm"
)

type App struct {
	Name      string            `json:"name"`
	Type      AppType           `json:"type"`
	Image     string            `json:"image,omitempty"`
	Domain    string            `json:"domain"`
	Port      int               `json:"port"`
	TLS       bool              `json:"tls"`
	HasDB     bool              `json:"has_db"`
	DBName    string            `json:"db_name,omitempty"`
	DBUser    string            `json:"db_user,omitempty"`
	EnvVars   map[string]string `json:"env_vars,omitempty"`
	LastIP    string            `json:"last_ip"`
	CloudID   string            `json:"cloud_id,omitempty"`
	VMSpec    *VMSpec           `json:"vm_spec,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

type VMSpec struct {
	MachineType string `json:"machine_type"`
	DiskSizeGB  int    `json:"disk_size_gb"`
	Image       string `json:"image"`
	Zone        string `json:"zone"`
}
