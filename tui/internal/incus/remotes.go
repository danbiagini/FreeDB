package incus

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cliconfig "github.com/lxc/incus/v6/shared/cliconfig"
)

type RemoteInfo struct {
	Name     string
	Addr     string
	Protocol string
	Public   bool
	HasAuth  bool
}

type AuthJSON struct {
	Auths map[string]AuthEntry `json:"auths"`
}

type AuthEntry struct {
	Auth string `json:"auth"`
}

const authJSONPath = "/home/incus/.config/containers/auth.json"

// ListRemotes returns all configured OCI remotes
func (c *Client) ListRemotes() ([]RemoteInfo, error) {
	conf, err := cliconfig.LoadConfig("")
	if err != nil {
		return nil, fmt.Errorf("loading incus config: %w", err)
	}

	authJSON, _ := loadAuthJSON()

	var remotes []RemoteInfo
	for name, remote := range conf.Remotes {
		if remote.Protocol != "oci" {
			continue
		}

		hasAuth := false
		if authJSON != nil {
			// Check if auth.json has credentials for this registry
			host := strings.TrimPrefix(remote.Addr, "https://")
			host = strings.TrimPrefix(host, "http://")
			if _, ok := authJSON.Auths[host]; ok {
				hasAuth = true
			}
			// Also check with https:// prefix
			if _, ok := authJSON.Auths[remote.Addr]; ok {
				hasAuth = true
			}
		}

		remotes = append(remotes, RemoteInfo{
			Name:     name,
			Addr:     remote.Addr,
			Protocol: remote.Protocol,
			Public:   remote.Public,
			HasAuth:  hasAuth,
		})
	}

	return remotes, nil
}

// AddRemote adds an OCI remote to incus and optionally sets up auth
func (c *Client) AddRemote(name, addr, username, password string) error {
	conf, err := cliconfig.LoadConfig("")
	if err != nil {
		return fmt.Errorf("loading incus config: %w", err)
	}

	// Ensure https:// prefix
	if !strings.HasPrefix(addr, "https://") && !strings.HasPrefix(addr, "http://") {
		addr = "https://" + addr
	}

	// Add remote to incus config
	conf.Remotes[name] = cliconfig.Remote{
		Addr:     addr,
		Protocol: "oci",
		Public:   username == "",
	}

	if err := conf.SaveConfig(conf.ConfigPath("config.yml")); err != nil {
		return fmt.Errorf("saving incus config: %w", err)
	}

	// Set up auth if credentials provided
	if username != "" && password != "" {
		if err := setRegistryAuth(addr, username, password); err != nil {
			return fmt.Errorf("setting auth: %w", err)
		}
	}

	return nil
}

// RemoveRemote removes an OCI remote from incus and its auth entry
func (c *Client) RemoveRemote(name string) error {
	conf, err := cliconfig.LoadConfig("")
	if err != nil {
		return fmt.Errorf("loading incus config: %w", err)
	}

	remote, ok := conf.Remotes[name]
	if !ok {
		return fmt.Errorf("remote %q not found", name)
	}

	// Remove auth entry
	_ = removeRegistryAuth(remote.Addr)

	// Remove remote from config
	delete(conf.Remotes, name)
	if err := conf.SaveConfig(conf.ConfigPath("config.yml")); err != nil {
		return fmt.Errorf("saving incus config: %w", err)
	}

	return nil
}

func loadAuthJSON() (*AuthJSON, error) {
	data, err := os.ReadFile(authJSONPath)
	if err != nil {
		return nil, err
	}

	var auth AuthJSON
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, err
	}
	if auth.Auths == nil {
		auth.Auths = make(map[string]AuthEntry)
	}

	return &auth, nil
}

func saveAuthJSON(auth *AuthJSON) error {
	dir := filepath.Dir(authJSONPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(authJSONPath, data, 0644)
}

func setRegistryAuth(addr, username, password string) error {
	auth, err := loadAuthJSON()
	if err != nil {
		auth = &AuthJSON{Auths: make(map[string]AuthEntry)}
	}

	// Use the hostname as the key (matching Docker convention)
	host := strings.TrimPrefix(addr, "https://")
	host = strings.TrimPrefix(host, "http://")

	encoded := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	auth.Auths[host] = AuthEntry{Auth: encoded}

	return saveAuthJSON(auth)
}

func removeRegistryAuth(addr string) error {
	auth, err := loadAuthJSON()
	if err != nil {
		return nil // nothing to remove
	}

	host := strings.TrimPrefix(addr, "https://")
	host = strings.TrimPrefix(host, "http://")
	delete(auth.Auths, host)

	return saveAuthJSON(auth)
}
