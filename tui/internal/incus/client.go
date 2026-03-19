package incus

import (
	"bytes"
	"context"
	"fmt"
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

// LaunchOCI launches a container from an OCI image using incus remotes.
// imageRef examples:
//   - "docker.io/traefik/whoami"
//   - "gcr:project/repo/image:tag"
//   - "us-central1-docker.pkg.dev/project/repo/image:tag"
func (c *Client) LaunchOCI(ctx context.Context, name, imageRef string) error {
	remote, alias := parseImageRef(imageRef)

	// Load incus client config to get remote server address
	conf, err := cliconfig.LoadConfig("")
	if err != nil {
		return fmt.Errorf("loading incus config: %w", err)
	}

	remoteConfig, ok := conf.Remotes[remote]
	if !ok {
		return fmt.Errorf("remote %q not found in incus config — add it via [R] Registries", remote)
	}

	req := api.InstancesPost{
		Name:  name,
		Type:  api.InstanceTypeContainer,
		Start: true,
		Source: api.InstanceSource{
			Type:     "image",
			Alias:    alias,
			Server:   remoteConfig.Addr,
			Protocol: "oci",
			Mode:     "pull",
		},
	}

	op, err := c.conn.CreateInstance(req)
	if err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}
	if err := op.Wait(); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
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
