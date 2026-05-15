package traefik

import (
	"strings"
	"testing"
)

func TestRenderRouteTLS(t *testing.T) {
	data := RouteData{
		Name:    "myapp",
		Domains: []string{"myapp.example.com"},
		IP:      "10.0.0.42",
		Port:    8080,
		TLS:     true,
	}

	out, err := RenderRoute(data)
	if err != nil {
		t.Fatalf("RenderRoute failed: %v", err)
	}

	s := string(out)

	if !strings.Contains(s, "websecure") {
		t.Error("expected websecure entrypoint for TLS")
	}
	if !strings.Contains(s, "certResolver: myresolver") {
		t.Error("expected certResolver for TLS")
	}
	if !strings.Contains(s, `Host(`) {
		t.Error("expected host rule")
	}
	if !strings.Contains(s, "Host(`myapp.example.com`)") {
		t.Error("expected single-domain host rule")
	}
	if !strings.Contains(s, "http://10.0.0.42:8080/") {
		t.Error("expected backend URL")
	}
}

func TestRenderRouteNoTLS(t *testing.T) {
	data := RouteData{
		Name:    "myapp",
		Domains: []string{"myapp.example.com"},
		IP:      "10.0.0.42",
		Port:    80,
		TLS:     false,
	}

	out, err := RenderRoute(data)
	if err != nil {
		t.Fatalf("RenderRoute failed: %v", err)
	}

	s := string(out)

	if !strings.Contains(s, "web") {
		t.Error("expected web entrypoint for non-TLS")
	}
	if strings.Contains(s, "websecure") {
		t.Error("should not have websecure for non-TLS")
	}
	if strings.Contains(s, "certResolver") {
		t.Error("should not have certResolver for non-TLS")
	}
	if !strings.Contains(s, "http://10.0.0.42:80/") {
		t.Error("expected backend URL with port 80")
	}
}

func TestRenderRouteMultiDomain(t *testing.T) {
	data := RouteData{
		Name:    "myapp",
		Domains: []string{"myapp.example.com", "www.myapp.example.com"},
		IP:      "10.0.0.42",
		Port:    8080,
		TLS:     true,
	}

	out, err := RenderRoute(data)
	if err != nil {
		t.Fatalf("RenderRoute failed: %v", err)
	}

	s := string(out)

	if !strings.Contains(s, "Host(`myapp.example.com`) || Host(`www.myapp.example.com`)") {
		t.Errorf("expected multi-host rule, got:\n%s", s)
	}
	if !strings.Contains(s, "websecure") {
		t.Error("expected websecure entrypoint for TLS")
	}
}
