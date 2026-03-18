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

// LaunchOCI launches a container from an OCI image using incus remotes.
// imageRef examples: "docker.io/traefik/whoami", "traefik/whoami", "docker:traefik/whoami"
func (c *Client) LaunchOCI(ctx context.Context, name, imageRef string) error {
	remote := "docker"
	alias := imageRef

	if strings.Contains(imageRef, ":") {
		parts := strings.SplitN(imageRef, ":", 2)
		if !strings.Contains(parts[0], ".") && !strings.Contains(parts[0], "/") {
			remote = parts[0]
			alias = parts[1]
		}
	}

	// Strip docker.io/ prefix — use the "docker" remote instead
	alias = strings.TrimPrefix(alias, "docker.io/")

	// Load incus client config to access configured remotes
	conf, err := cliconfig.LoadConfig("")
	if err != nil {
		return fmt.Errorf("loading incus config: %w", err)
	}

	// Connect to the OCI remote image server
	imgServer, err := conf.GetImageServer(remote)
	if err != nil {
		return fmt.Errorf("connecting to remote %q: %w", remote, err)
	}

	// Resolve the image alias
	imgAlias, _, err := imgServer.GetImageAlias(alias)
	if err != nil {
		return fmt.Errorf("resolving image %q on remote %q: %w", alias, remote, err)
	}

	// Get the full image info
	image, _, err := imgServer.GetImage(imgAlias.Target)
	if err != nil {
		return fmt.Errorf("getting image info for %q: %w", alias, err)
	}

	// Create instance from the remote image
	req := api.InstancesPost{
		Name: name,
		Type: api.InstanceTypeContainer,
		Source: api.InstanceSource{
			Type: "image",
		},
	}

	op, err := c.conn.CreateInstanceFromImage(imgServer, *image, req)
	if err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}
	if err := op.Wait(); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}

	return c.StartContainer(ctx, name)
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
