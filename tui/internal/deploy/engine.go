package deploy

import (
	"context"
	"fmt"
	"time"

	"github.com/danbiagini/FreeDB/tui/internal/incus"
	"github.com/danbiagini/FreeDB/tui/internal/registry"
	"github.com/danbiagini/FreeDB/tui/internal/traefik"
)

// UpdateParams contains everything needed to perform a zero-downtime update
type UpdateParams struct {
	AppName          string
	Image            string
	App              *registry.App
	IncusClient      *incus.Client
	Registry         *registry.AppRegistry
	OnProgress       func(msg string) // optional progress callback
}

// UpdateResult contains the outcome of an update
type UpdateResult struct {
	NewContainer string
	NewIP        string
}

// Update performs a zero-downtime blue-green deployment.
// This is the shared core logic used by both the TUI and CLI.
func Update(ctx context.Context, params UpdateParams) (*UpdateResult, error) {
	ic := params.IncusClient
	app := params.App
	reg := params.Registry
	name := params.AppName
	image := params.Image

	progress := func(msg string) {
		if params.OnProgress != nil {
			params.OnProgress(msg)
		}
	}

	timestamp := time.Now().Format("0102-1504")
	newName := name + "-" + timestamp

	// Resolve actual container name
	oldContainerName := name
	if app.ContainerName != "" {
		oldContainerName = app.ContainerName
	}

	// 1. Save env vars
	progress("Saving environment variables...")
	envVars, err := ic.GetEnvVars(ctx, oldContainerName)
	if err != nil {
		envVars = make(map[string]string)
	}
	progress(fmt.Sprintf("Found %d environment variables", len(envVars)))

	// 2. Delete cached image
	progress("Clearing image cache...")
	_ = ic.DeleteCachedImage(ctx, image)

	// 3. Create container without starting
	progress(fmt.Sprintf("Pulling image and creating container %s...", newName))
	if err := ic.InitOCI(ctx, newName, image); err != nil {
		return nil, fmt.Errorf("creating container: %w", err)
	}

	// 4. Restore env vars before starting
	if len(envVars) > 0 {
		progress("Restoring environment variables...")
		if err := ic.RestoreEnvVars(ctx, newName, envVars); err != nil {
			_ = ic.DeleteContainer(ctx, newName)
			return nil, fmt.Errorf("restoring env vars: %w", err)
		}
	}

	// 5. Start container
	progress("Starting container...")
	if err := ic.StartContainer(ctx, newName); err != nil {
		_ = ic.DeleteContainer(ctx, newName)
		return nil, fmt.Errorf("starting container: %w", err)
	}

	// 6. Wait for IP
	progress("Waiting for IP...")
	newIP, err := ic.WaitForIP(ctx, newName, 30*time.Second)
	if err != nil {
		_ = ic.DeleteContainer(ctx, newName)
		return nil, fmt.Errorf("waiting for IP: %w", err)
	}
	progress(fmt.Sprintf("Container running at %s", newIP))

	// 7. Switch Traefik route
	if app.HasDomains() {
		progress("Switching Traefik route...")
		if err := traefik.PushRoute(ic, name, app.GetDomains(), newIP, app.Port, app.TLS); err != nil {
			_ = ic.DeleteContainer(ctx, newName)
			return nil, fmt.Errorf("updating route: %w", err)
		}
	}

	// 8. Delete old container
	progress(fmt.Sprintf("Removing old container %s...", oldContainerName))
	_ = ic.DeleteContainer(ctx, oldContainerName)

	// 9. Rename new container back to the app name for stable .incus DNS
	//    Incus requires the container to be stopped for rename
	progress(fmt.Sprintf("Renaming %s -> %s...", newName, name))
	_ = ic.StopContainer(ctx, newName)
	if err := ic.RenameContainer(ctx, newName, name); err != nil {
		// Rename failed — start it back up with the old name
		progress(fmt.Sprintf("Warning: rename failed (%v), keeping %s", err, newName))
		_ = ic.StartContainer(ctx, newName)
		_ = reg.UpdateIP(name, newIP)
		_ = reg.UpdateImage(name, image)
		_ = reg.UpdateContainerName(name, newName)
		if removed, err := ic.CleanupCachedImages(ctx); err == nil && removed > 0 {
			progress(fmt.Sprintf("Cleaned up %d cached image(s)", removed))
		}
		return &UpdateResult{
			NewContainer: newName,
			NewIP:        newIP,
		}, nil
	}
	_ = ic.StartContainer(ctx, name)

	// Wait for IP after restart (may change)
	if ip, err := ic.WaitForIP(ctx, name, 15*time.Second); err == nil {
		newIP = ip
	}

	// Update Traefik route with new IP
	if app.HasDomains() {
		_ = traefik.PushRoute(ic, name, app.GetDomains(), newIP, app.Port, app.TLS)
	}

	// 10. Update registry — container name is back to the app name
	_ = reg.UpdateIP(name, newIP)
	_ = reg.UpdateImage(name, image)
	_ = reg.UpdateContainerName(name, name)

	// 11. Clean up cached images to prevent disk exhaustion
	if removed, err := ic.CleanupCachedImages(ctx); err == nil && removed > 0 {
		progress(fmt.Sprintf("Cleaned up %d cached image(s)", removed))
	}

	return &UpdateResult{
		NewContainer: name,
		NewIP:        newIP,
	}, nil
}
