package check

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/danbiagini/freedb-tui/internal/config"
	"github.com/danbiagini/freedb-tui/internal/incus"
)

type Result struct {
	Name   string
	OK     bool
	Detail string
}

func RunChecks(cfg *config.Config) []Result {
	var results []Result

	// 1. Incus socket
	ic, err := incus.Connect(cfg.IncusSocket)
	if err != nil {
		results = append(results, Result{"Incus connection", false, err.Error()})
		return results // Can't check further without incus
	}
	results = append(results, Result{"Incus connection", true, "connected"})

	// 2. List containers
	containers, err := ic.ListContainers(context.Background())
	if err != nil {
		results = append(results, Result{"Container listing", false, err.Error()})
		return results
	}
	results = append(results, Result{"Container listing", true, fmt.Sprintf("%d containers found", len(containers))})

	// 3. proxy1 running
	proxy1Running := false
	proxy1IP := ""
	for _, c := range containers {
		if c.Name == cfg.ProxyContainer {
			if strings.EqualFold(c.Status, "running") {
				proxy1Running = true
				proxy1IP = c.IP
			}
			break
		}
	}
	if proxy1Running {
		results = append(results, Result{"Proxy (proxy1)", true, fmt.Sprintf("running at %s", proxy1IP)})
	} else {
		results = append(results, Result{"Proxy (proxy1)", false, "not running"})
	}

	// 4. db1 running
	db1Running := false
	db1IP := ""
	for _, c := range containers {
		if c.Name == cfg.DBContainer {
			if strings.EqualFold(c.Status, "running") {
				db1Running = true
				db1IP = c.IP
			}
			break
		}
	}
	if db1Running {
		results = append(results, Result{"Database (db1)", true, fmt.Sprintf("running at %s", db1IP)})
	} else {
		results = append(results, Result{"Database (db1)", false, "not running"})
	}

	// 5. Traefik metrics reachable
	if proxy1Running && proxy1IP != "" {
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(fmt.Sprintf("http://%s:8080/metrics", proxy1IP))
		if err != nil {
			results = append(results, Result{"Traefik metrics", false, err.Error()})
		} else {
			resp.Body.Close()
			results = append(results, Result{"Traefik metrics", true, fmt.Sprintf("reachable at %s:8080", proxy1IP)})
		}
	}

	// 6. Registry file
	if _, err := os.Stat(cfg.RegistryPath); err == nil {
		results = append(results, Result{"App registry", true, cfg.RegistryPath})
	} else if os.IsNotExist(err) {
		results = append(results, Result{"App registry", true, "not yet created (will be created on first app deploy)"})
	} else {
		results = append(results, Result{"App registry", false, err.Error()})
	}

	// 7. OCI remotes
	remotes, err := ic.ListRemotes()
	if err != nil {
		results = append(results, Result{"OCI remotes", false, err.Error()})
	} else {
		var names []string
		for _, r := range remotes {
			names = append(names, r.Name)
		}
		if len(names) > 0 {
			results = append(results, Result{"OCI remotes", true, strings.Join(names, ", ")})
		} else {
			results = append(results, Result{"OCI remotes", false, "none configured"})
		}
	}

	// 8. Auth.json
	authPath := "/home/incus/.config/containers/auth.json"
	if data, err := os.ReadFile(authPath); err == nil {
		if len(data) > 20 { // more than just empty {"auths":{}}
			results = append(results, Result{"Registry auth", true, "auth.json present with credentials"})
		} else {
			results = append(results, Result{"Registry auth", true, "auth.json present (no private registry credentials)"})
		}
	} else {
		results = append(results, Result{"Registry auth", false, "auth.json missing — OCI pulls may fail"})
	}

	return results
}

func PrintResults(results []Result) {
	fmt.Println("FreeDB Health Check")
	fmt.Println("===================")
	fmt.Println()

	allOK := true
	for _, r := range results {
		icon := "✓"
		if !r.OK {
			icon = "✗"
			allOK = false
		}
		fmt.Printf("  %s  %-20s %s\n", icon, r.Name, r.Detail)
	}

	fmt.Println()
	if allOK {
		fmt.Println("All checks passed.")
	} else {
		fmt.Println("Some checks failed. See above for details.")
	}
}
