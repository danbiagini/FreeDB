package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/danbiagini/FreeDB/tui/internal/incus"
	"github.com/danbiagini/FreeDB/tui/internal/registry"
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
	Image   string
	Tag     string
	DryRun  bool
	JSON    bool
}

// ResolveImage determines the full image ref from Options and the existing app config
func ResolveImage(opts Options, app *registry.App) (string, error) {
	image := opts.Image
	if image == "" && opts.Tag != "" {
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
		return "", fmt.Errorf("either --image or --tag is required")
	}
	return image, nil
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
	image, err := ResolveImage(opts, app)
	if err != nil {
		return fail(1, "%v", err)
	}

	log("Deploying %s with %s...", opts.AppName, image)

	if opts.DryRun {
		log("  (dry run — no changes made)")
		log("  Would update %s from %s to %s", opts.AppName, app.Image, image)
		log("  Container: %s", app.ContainerName)
		log("  Domains: %s", strings.Join(app.GetDomains(), ", "))
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

	// Run the update using the shared engine
	result, err := Update(context.Background(), UpdateParams{
		AppName:     opts.AppName,
		Image:       image,
		App:         app,
		IncusClient: ic,
		Registry:    reg,
		OnProgress: func(msg string) {
			log("  %s", msg)
		},
	})
	if err != nil {
		return fail(1, "%v", err)
	}

	duration := time.Since(start)
	log("  Done in %s.", duration.Truncate(time.Millisecond))

	if opts.JSON {
		r := Result{
			Status:     "ok",
			App:        opts.AppName,
			Image:      image,
			Container:  result.NewContainer,
			IP:         result.NewIP,
			DurationMs: duration.Milliseconds(),
		}
		json.NewEncoder(os.Stdout).Encode(r)
	}

	return 0
}
