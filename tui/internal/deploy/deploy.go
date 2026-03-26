package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/danbiagini/freedb-tui/internal/incus"
	"github.com/danbiagini/freedb-tui/internal/registry"
	"github.com/danbiagini/freedb-tui/internal/traefik"
)

type Result struct {
	Status     string `json:"status"`
	App        string `json:"app"`
	Image      string `json:"image"`
	Container  string `json:"container"`
	IP         string `json:"ip"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

type Options struct {
	AppName string
	Image   string // full image ref (e.g., ecr:myapp:v1.2.3)
	Tag     string // just the tag (uses existing image base)
	DryRun  bool
	JSON    bool
}

func Run(opts Options) int {
	start := time.Now()

	log := func(format string, args ...any) {
		if !opts.JSON {
			fmt.Printf(format+"\n", args...)
		}
	}

	fail := func(code int, format string, args ...any) int {
		msg := fmt.Sprintf(format, args...)
		if opts.JSON {
			r := Result{
				Status:     "error",
				App:        opts.AppName,
				Error:      msg,
				DurationMs: time.Since(start).Milliseconds(),
			}
			json.NewEncoder(os.Stdout).Encode(r)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
		}
		return code
	}

	// Load registry
	reg, err := registry.Load("/etc/freedb/registry.json")
	if err != nil {
		return fail(1, "loading registry: %v", err)
	}

	app, ok := reg.Get(opts.AppName)
	if !ok {
		return fail(2, "app %q not found in registry", opts.AppName)
	}

	// Resolve image
	image := opts.Image
	if image == "" && opts.Tag != "" {
		// Use existing image base with new tag
		img := app.Image
		if idx := strings.LastIndex(img, ":"); idx > 0 {
			after := img[idx+1:]
			if !strings.Contains(after, "/") {
				img = img[:idx]
			}
		}
		image = img + ":" + opts.Tag
	}
	if image == "" {
		return fail(1, "either --image or --tag is required")
	}

	log("Deploying %s with %s...", opts.AppName, image)

	if opts.DryRun {
		log("  (dry run — no changes made)")
		log("  Would update %s from %s to %s", opts.AppName, app.Image, image)
		log("  Container: %s", app.ContainerName)
		log("  Domain: %s", app.Domain)
		if opts.JSON {
			r := Result{
				Status:     "dry_run",
				App:        opts.AppName,
				Image:      image,
				DurationMs: time.Since(start).Milliseconds(),
			}
			json.NewEncoder(os.Stdout).Encode(r)
		}
		return 0
	}

	// Acquire deploy lock
	log("  Acquiring deploy lock...")
	lock, err := AcquireLock()
	if err != nil {
		return fail(3, "%v", err)
	}
	defer lock.Release()

	// Connect to incus
	ic, err := incus.Connect("")
	if err != nil {
		return fail(1, "connecting to incus: %v", err)
	}

	ctx := context.Background()
	timestamp := time.Now().Format("0102-1504")
	newName := opts.AppName + "-" + timestamp

	// Resolve actual container name
	oldContainerName := opts.AppName
	if app.ContainerName != "" {
		oldContainerName = app.ContainerName
	}

	// 1. Save env vars
	log("  Saving environment variables...")
	envVars, err := ic.GetEnvVars(ctx, oldContainerName)
	if err != nil {
		envVars = make(map[string]string)
	}
	log("  Found %d environment variables", len(envVars))

	// 2. Delete cached image
	log("  Clearing image cache...")
	_ = ic.DeleteCachedImage(ctx, image)

	// 3. Create container without starting
	log("  Pulling image and creating container %s...", newName)
	if err := ic.InitOCI(ctx, newName, image); err != nil {
		return fail(1, "creating container: %v", err)
	}

	// 4. Restore env vars before starting
	if len(envVars) > 0 {
		log("  Restoring environment variables...")
		if err := ic.RestoreEnvVars(ctx, newName, envVars); err != nil {
			_ = ic.DeleteContainer(ctx, newName)
			return fail(1, "restoring env vars: %v", err)
		}
	}

	// 5. Start container
	log("  Starting container...")
	if err := ic.StartContainer(ctx, newName); err != nil {
		_ = ic.DeleteContainer(ctx, newName)
		return fail(1, "starting container: %v", err)
	}

	// 6. Wait for IP
	log("  Waiting for IP...")
	newIP, err := ic.WaitForIP(ctx, newName, 30*time.Second)
	if err != nil {
		_ = ic.DeleteContainer(ctx, newName)
		return fail(1, "waiting for IP: %v", err)
	}
	log("  Container running at %s", newIP)

	// 7. Switch Traefik route
	if app.Domain != "" {
		log("  Switching Traefik route...")
		if err := traefik.PushRoute(ic, opts.AppName, app.Domain, newIP, app.Port, app.TLS); err != nil {
			_ = ic.DeleteContainer(ctx, newName)
			return fail(1, "updating route: %v", err)
		}
	}

	// 8. Delete old container
	log("  Removing old container %s...", oldContainerName)
	_ = ic.DeleteContainer(ctx, oldContainerName)

	// 9. Update registry
	_ = reg.UpdateIP(opts.AppName, newIP)
	_ = reg.UpdateImage(opts.AppName, image)
	_ = reg.UpdateContainerName(opts.AppName, newName)

	duration := time.Since(start)
	log("  Done in %s.", duration.Truncate(time.Millisecond))

	if opts.JSON {
		r := Result{
			Status:     "ok",
			App:        opts.AppName,
			Image:      image,
			Container:  newName,
			IP:         newIP,
			DurationMs: duration.Milliseconds(),
		}
		json.NewEncoder(os.Stdout).Encode(r)
	}

	return 0
}
