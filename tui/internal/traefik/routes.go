package traefik

import (
	"fmt"

	"github.com/danbiagini/freedb-tui/internal/incus"
)

const proxyContainer = "proxy1"
const routeDir = "/etc/traefik/manual"

func PushRoute(ic *incus.Client, name, domain, ip string, port int, tls bool) error {
	data := RouteData{
		Name:   name,
		Domain: domain,
		IP:     ip,
		Port:   port,
		TLS:    tls,
	}

	content, err := RenderRoute(data)
	if err != nil {
		return fmt.Errorf("rendering route template: %w", err)
	}

	path := fmt.Sprintf("%s/%s.yaml", routeDir, name)
	if err := ic.PushFile(proxyContainer, path, content); err != nil {
		return fmt.Errorf("pushing route to %s: %w", proxyContainer, err)
	}

	return nil
}

func DeleteRoute(ic *incus.Client, name string) error {
	path := fmt.Sprintf("%s/%s.yaml", routeDir, name)
	return ic.DeleteFile(proxyContainer, path)
}
