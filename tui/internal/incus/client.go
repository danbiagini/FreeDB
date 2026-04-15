package incus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	incusclient "github.com/lxc/incus/v6/client"
	"github.com/lxc/incus/v6/shared/api"
	cliconfig "github.com/lxc/incus/v6/shared/cliconfig"
)

type Client struct {
	conn incusclient.InstanceServer
}

type ContainerInfo struct {
	Name       string
	Status     string
	IP         string
	MemUsageMB int64
	CPUSeconds float64
}

type ContainerDetail struct {
	Name        string
	Status      string
	IP          string
	MemUsageMB  int64
	MemLimitMB  int64
	DiskUsageMB int64
	CPUSeconds  float64
	AvgCPUPct   float64
	Uptime      time.Duration
	Pid         int64
	Processes   int64
	Created     string
	BytesIn     int64
	BytesOut    int64
}

func Connect(socketPath string) (*Client, error) {
	var conn incusclient.InstanceServer
	var err error

	if socketPath != "" {
		conn, err = incusclient.ConnectIncusUnix(socketPath, nil)
	} else {
		conn, err = incusclient.ConnectIncusUnix("", nil)
	}
	if err != nil {
		return nil, fmt.Errorf("connecting to incus: %w", err)
	}

	return &Client{conn: conn}, nil
}

func (c *Client) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	instances, err := c.conn.GetInstancesFull(api.InstanceTypeContainer)
	if err != nil {
		return nil, fmt.Errorf("listing instances: %w", err)
	}

	var containers []ContainerInfo
	for _, inst := range instances {
		info := ContainerInfo{
			Name:   inst.Name,
			Status: inst.Status,
		}

		if inst.State != nil {
			// Extract IPv4 address
			if eth0, ok := inst.State.Network["eth0"]; ok {
				for _, addr := range eth0.Addresses {
					if addr.Family == "inet" {
						info.IP = addr.Address
						break
					}
				}
			}

			// Memory usage in MB
			info.MemUsageMB = inst.State.Memory.Usage / (1024 * 1024)

			// CPU usage in seconds
			info.CPUSeconds = float64(inst.State.CPU.Usage) / 1e9
		}

		containers = append(containers, info)
	}

	return containers, nil
}

func (c *Client) GetContainerIP(ctx context.Context, name string) (string, error) {
	state, _, err := c.conn.GetInstanceState(name)
	if err != nil {
		return "", fmt.Errorf("getting state for %s: %w", name, err)
	}

	if eth0, ok := state.Network["eth0"]; ok {
		for _, addr := range eth0.Addresses {
			if addr.Family == "inet" {
				return addr.Address, nil
			}
		}
	}

	return "", fmt.Errorf("no IPv4 address found for %s", name)
}

func (c *Client) GetContainerDetail(ctx context.Context, name string) (*ContainerDetail, error) {
	inst, _, err := c.conn.GetInstanceFull(name)
	if err != nil {
		return nil, fmt.Errorf("getting instance %s: %w", name, err)
	}

	detail := &ContainerDetail{
		Name:    inst.Name,
		Status:  inst.Status,
		Created: inst.CreatedAt.Format("2006-01-02 15:04"),
	}

	// Compute uptime from last start
	if inst.State != nil && !inst.State.StartedAt.IsZero() {
		detail.Uptime = time.Since(inst.State.StartedAt)
	}

	if inst.State != nil {
		detail.Pid = inst.State.Pid
		detail.Processes = inst.State.Processes
		detail.MemUsageMB = inst.State.Memory.Usage / (1024 * 1024)
		if inst.State.Memory.Total > 0 {
			detail.MemLimitMB = inst.State.Memory.Total / (1024 * 1024)
		}
		detail.CPUSeconds = float64(inst.State.CPU.Usage) / 1e9
		if detail.Uptime.Seconds() > 0 {
			detail.AvgCPUPct = (detail.CPUSeconds / detail.Uptime.Seconds()) * 100
		}

		if eth0, ok := inst.State.Network["eth0"]; ok {
			for _, addr := range eth0.Addresses {
				if addr.Family == "inet" {
					detail.IP = addr.Address
					break
				}
			}
			detail.BytesIn = eth0.Counters.BytesReceived
			detail.BytesOut = eth0.Counters.BytesSent
		}

		if root, ok := inst.State.Disk["root"]; ok {
			detail.DiskUsageMB = root.Usage / (1024 * 1024)
		}
	}

	return detail, nil
}

func (c *Client) StartContainer(ctx context.Context, name string) error {
	reqState := api.InstanceStatePut{
		Action:  "start",
		Timeout: -1,
	}
	op, err := c.conn.UpdateInstanceState(name, reqState, "")
	if err != nil {
		return fmt.Errorf("starting %s: %w", name, err)
	}
	return op.Wait()
}

func (c *Client) StopContainer(ctx context.Context, name string) error {
	reqState := api.InstanceStatePut{
		Action:  "stop",
		Timeout: 30,
	}
	op, err := c.conn.UpdateInstanceState(name, reqState, "")
	if err != nil {
		return fmt.Errorf("stopping %s: %w", name, err)
	}
	return op.Wait()
}

func (c *Client) DeleteContainer(ctx context.Context, name string) error {
	// Stop first if running
	_ = c.StopContainer(ctx, name)

	op, err := c.conn.DeleteInstance(name)
	if err != nil {
		return fmt.Errorf("deleting %s: %w", name, err)
	}
	return op.Wait()
}

// DeleteCachedImage removes a locally cached OCI image to force a fresh pull.
// Matches images by alias containing the image ref (best effort).
func (c *Client) DeleteCachedImage(ctx context.Context, imageRef string) error {
	_, alias := parseImageRef(imageRef)

	images, err := c.conn.GetImages()
	if err != nil {
		return err
	}

	for _, img := range images {
		// Match by alias or description containing the image ref
		for _, a := range img.Aliases {
			if strings.Contains(a.Name, alias) || strings.Contains(a.Description, alias) {
				op, err := c.conn.DeleteImage(img.Fingerprint)
				if err != nil {
					continue
				}
				_ = op.Wait()
				return nil
			}
		}
		// Also check the update source
		if img.UpdateSource != nil && strings.Contains(img.UpdateSource.Alias, alias) {
			op, err := c.conn.DeleteImage(img.Fingerprint)
			if err != nil {
				continue
			}
			_ = op.Wait()
			return nil
		}
	}

	return nil // no cached image found, that's fine
}

// CleanupCachedImages removes all cached images from the local image store.
// Running containers don't need cached images — the image is unpacked into
// the container's storage at creation time. Returns the number of images removed.
func (c *Client) CleanupCachedImages(ctx context.Context) (int, error) {
	images, err := c.conn.GetImages()
	if err != nil {
		return 0, err
	}

	removed := 0
	for _, img := range images {
		if !img.Cached {
			continue
		}
		op, err := c.conn.DeleteImage(img.Fingerprint)
		if err != nil {
			continue // in use or already deleted
		}
		_ = op.Wait()
		removed++
	}

	return removed, nil
}

// GetInstanceConfig returns the full config map for a container

func (c *Client) RenameContainer(ctx context.Context, oldName, newName string) error {
	op, err := c.conn.RenameInstance(oldName, api.InstancePost{Name: newName})
	if err != nil {
		return fmt.Errorf("renaming %s to %s: %w", oldName, newName, err)
	}
	return op.Wait()
}

func (c *Client) GetInstanceConfig(ctx context.Context, name string) (map[string]string, error) {
	inst, _, err := c.conn.GetInstance(name)
	if err != nil {
		return nil, fmt.Errorf("getting instance %s: %w", name, err)
	}
	return inst.Config, nil
}

// RestoreEnvVars sets all environment.* config keys on a container
func (c *Client) RestoreEnvVars(ctx context.Context, name string, envVars map[string]string) error {
	for k, v := range envVars {
		if err := c.SetEnvVar(ctx, name, k, v); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) SetEnvVar(ctx context.Context, name, key, value string) error {
	inst, etag, err := c.conn.GetInstance(name)
	if err != nil {
		return fmt.Errorf("getting instance %s: %w", name, err)
	}

	if inst.Config == nil {
		inst.Config = make(map[string]string)
	}
	inst.Config["environment."+key] = value

	op, err := c.conn.UpdateInstance(name, inst.Writable(), etag)
	if err != nil {
		return fmt.Errorf("updating instance %s: %w", name, err)
	}
	return op.Wait()
}

func (c *Client) GetEnvVars(ctx context.Context, name string) (map[string]string, error) {
	inst, _, err := c.conn.GetInstance(name)
	if err != nil {
		return nil, fmt.Errorf("getting instance %s: %w", name, err)
	}

	envs := make(map[string]string)
	for k, v := range inst.Config {
		if strings.HasPrefix(k, "environment.") {
			envs[strings.TrimPrefix(k, "environment.")] = v
		}
	}
	return envs, nil
}

func (c *Client) DeleteEnvVar(ctx context.Context, name, key string) error {
	inst, etag, err := c.conn.GetInstance(name)
	if err != nil {
		return fmt.Errorf("getting instance %s: %w", name, err)
	}

	delete(inst.Config, "environment."+key)

	op, err := c.conn.UpdateInstance(name, inst.Writable(), etag)
	if err != nil {
		return fmt.Errorf("updating instance %s: %w", name, err)
	}
	return op.Wait()
}

func (c *Client) PushFile(instance, path string, content []byte) error {
	return c.conn.CreateInstanceFile(instance, path, incusclient.InstanceFileArgs{
		Content:   bytes.NewReader(content),
		UID:       0,
		GID:       0,
		Mode:      0644,
		Type:      "file",
		WriteMode: "overwrite",
	})
}

func (c *Client) DeleteFile(instance, path string) error {
	return c.conn.DeleteInstanceFile(instance, path)
}

func (c *Client) Exec(ctx context.Context, instance string, cmd []string) (string, error) {
	var stdout, stderr bytes.Buffer

	args := incusclient.InstanceExecArgs{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	op, err := c.conn.ExecInstance(instance, api.InstanceExecPost{
		Command:     cmd,
		WaitForWS:   true,
		Interactive: false,
	}, &args)
	if err != nil {
		return "", fmt.Errorf("exec in %s: %w", instance, err)
	}

	if err := op.Wait(); err != nil {
		return "", fmt.Errorf("exec in %s: %w (stderr: %s)", instance, err, stderr.String())
	}

	return stdout.String(), nil
}

func (c *Client) LaunchContainer(ctx context.Context, name, image string) error {
	req := api.InstancesPost{
		Name: name,
		Source: api.InstanceSource{
			Type:     "image",
			Protocol: "simplestreams",
			Server:   "https://images.linuxcontainers.org",
			Alias:    image,
		},
		Type: api.InstanceTypeContainer,
	}

	op, err := c.conn.CreateInstance(req)
	if err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}
	if err := op.Wait(); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}

	return c.StartContainer(ctx, name)
}

// parseImageRef resolves an image reference to a remote name and alias.
// Supports formats:
//   - "gcr:project/repo/image:tag"                            → remote=gcr, alias=project/repo/image:tag
//   - "docker.io/traefik/whoami"                              → remote=docker, alias=traefik/whoami
//   - "us-central1-docker.pkg.dev/project/repo/image:tag"     → remote matching that addr, alias=project/repo/image:tag
//   - "traefik/whoami"                                        → remote=docker, alias=traefik/whoami
func parseImageRef(imageRef string) (string, string) {
	// Format: "remote:image" (e.g., "gcr:project/repo/image:tag")
	if strings.Contains(imageRef, ":") {
		parts := strings.SplitN(imageRef, ":", 2)
		if !strings.Contains(parts[0], ".") && !strings.Contains(parts[0], "/") {
			return parts[0], parts[1]
		}
	}

	// Format: "registry.host/path" — match against configured remotes
	conf, err := cliconfig.LoadConfig("")
	if err == nil {
		for name, r := range conf.Remotes {
			if r.Protocol != "oci" {
				continue
			}
			host := strings.TrimPrefix(r.Addr, "https://")
			host = strings.TrimPrefix(host, "http://")
			if strings.HasPrefix(imageRef, host+"/") {
				alias := strings.TrimPrefix(imageRef, host+"/")
				return name, alias
			}
		}
	}

	// Default: strip docker.io/ prefix, use "docker" remote
	alias := strings.TrimPrefix(imageRef, "docker.io/")
	return "docker", alias
}

// checkImageArch inspects an OCI image and verifies it matches the host architecture.
// Returns nil if the architecture matches or if inspection fails (best-effort check).
func checkImageArch(ctx context.Context, imageRef string) error {
	remote, alias := parseImageRef(imageRef)

	// Build the full registry reference for skopeo
	// skopeo needs docker:// style refs, not incus remote:alias format
	var skopeoRef string
	switch remote {
	case "docker":
		skopeoRef = "docker://docker.io/" + alias
	default:
		// For custom remotes, try to resolve the address
		conf, err := cliconfig.LoadConfig("")
		if err != nil {
			return nil // best-effort
		}
		r, ok := conf.Remotes[remote]
		if !ok {
			return nil
		}
		host := strings.TrimPrefix(r.Addr, "https://")
		host = strings.TrimPrefix(host, "http://")
		skopeoRef = "docker://" + host + "/" + alias
	}

	cmd := exec.CommandContext(ctx, "skopeo", "inspect", "--raw", skopeoRef)
	output, err := cmd.Output()
	if err != nil {
		return nil // best-effort: if skopeo fails, let incus handle it
	}

	// Check if it's a manifest list (multi-arch) or single manifest
	var manifest struct {
		MediaType string `json:"mediaType"`
		// Single-arch fields
		Config struct {
			MediaType string `json:"mediaType"`
		} `json:"config"`
		// Multi-arch fields
		Manifests []struct {
			Platform struct {
				Architecture string `json:"architecture"`
				OS           string `json:"os"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(output, &manifest); err != nil {
		return nil
	}

	hostArch := runtime.GOARCH // amd64, arm64, etc.

	// Multi-arch image: check if our arch is in the list
	if len(manifest.Manifests) > 0 {
		for _, m := range manifest.Manifests {
			if m.Platform.Architecture == hostArch {
				return nil
			}
		}
		var available []string
		for _, m := range manifest.Manifests {
			if m.Platform.OS != "" {
				available = append(available, m.Platform.OS+"/"+m.Platform.Architecture)
			} else {
				available = append(available, m.Platform.Architecture)
			}
		}
		return fmt.Errorf("image %s is not available for %s (available: %s). Rebuild with: docker buildx build --platform linux/%s",
			imageRef, hostArch, strings.Join(available, ", "), hostArch)
	}

	// Single-arch image: inspect the config to get architecture
	cmd = exec.CommandContext(ctx, "skopeo", "inspect", skopeoRef)
	output, err = cmd.Output()
	if err != nil {
		return nil
	}

	var inspectResult struct {
		Architecture string `json:"Architecture"`
	}
	if err := json.Unmarshal(output, &inspectResult); err != nil {
		return nil
	}

	if inspectResult.Architecture != "" && inspectResult.Architecture != hostArch {
		return fmt.Errorf("image %s is built for %s but this host is %s. Rebuild with: docker buildx build --platform linux/%s",
			imageRef, inspectResult.Architecture, hostArch, hostArch)
	}

	return nil
}

// LaunchOCI launches a container from an OCI image using the incus CLI.
// The Go API's server-side pull doesn't handle authenticated registries correctly
// (the daemon's skopeo context differs from the CLI's client-side pull).
// Using the CLI ensures auth.json and XDG_RUNTIME_DIR are resolved properly.
//
// imageRef examples:
//   - "docker.io/traefik/whoami"
//   - "gcr:project/repo/image:tag"
//   - "us-central1-docker.pkg.dev/project/repo/image:tag"
func (c *Client) LaunchOCI(ctx context.Context, name, imageRef string) error {
	if err := checkImageArch(ctx, imageRef); err != nil {
		return err
	}

	remote, alias := parseImageRef(imageRef)
	ref := fmt.Sprintf("%s:%s", remote, alias)

	cmd := exec.CommandContext(ctx, "incus", "launch", ref, name, "--profile", "default")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("creating %s: %s", name, strings.TrimSpace(string(output)))
	}

	return nil
}

// InitOCI creates an OCI container without starting it.
// Use this when you need to configure env vars before the app starts.
func (c *Client) InitOCI(ctx context.Context, name, imageRef string) error {
	if err := checkImageArch(ctx, imageRef); err != nil {
		return err
	}

	remote, alias := parseImageRef(imageRef)
	ref := fmt.Sprintf("%s:%s", remote, alias)

	cmd := exec.CommandContext(ctx, "incus", "init", ref, name, "--profile", "default")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("creating %s: %s", name, strings.TrimSpace(string(output)))
	}

	return nil
}

func (c *Client) WaitForIP(ctx context.Context, name string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ip, err := c.GetContainerIP(ctx, name)
		if err == nil && ip != "" {
			return ip, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return "", fmt.Errorf("timeout waiting for IP on %s", name)
}

// StoragePoolUsage holds disk usage for a storage pool.
type StoragePoolUsage struct {
	UsedBytes  uint64
	TotalBytes uint64
}

// GetStoragePoolUsage returns disk usage for the named storage pool.
func (c *Client) GetStoragePoolUsage(poolName string) (*StoragePoolUsage, error) {
	resources, err := c.conn.GetStoragePoolResources(poolName)
	if err != nil {
		return nil, fmt.Errorf("getting storage pool resources: %w", err)
	}
	return &StoragePoolUsage{
		UsedBytes:  resources.Space.Used,
		TotalBytes: resources.Space.Total,
	}, nil
}
