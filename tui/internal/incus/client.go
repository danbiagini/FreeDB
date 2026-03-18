package incus

import (
	"context"
	"fmt"

	incusclient "github.com/lxc/incus/v6/client"
	"github.com/lxc/incus/v6/shared/api"
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
