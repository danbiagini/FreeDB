package traefik

import (
	"bytes"
	"strings"
	"text/template"
)

const routeTemplate = `http:
  routers:
    {{.Name}}-router:
      entryPoints:
{{- if .TLS}}
        - "websecure"
{{- else}}
        - "web"
{{- end}}
      rule: "{{hostList .Domains}}"
      service: {{.Name}}
{{- if .TLS}}
      tls:
        certResolver: myresolver
{{- end}}
  services:
    {{.Name}}:
      loadBalancer:
        servers:
          - url: "http://{{.IP}}:{{.Port}}/"
`

type RouteData struct {
	Name    string
	Domains []string
	IP      string
	Port    int
	TLS     bool
}

// hostList renders domains as a Traefik v3 rule expression.
// Single domain:  Host(`a.com`)
// Multiple:       Host(`a.com`) || Host(`b.com`)
func hostList(domains []string) string {
	parts := make([]string, len(domains))
	for i, d := range domains {
		parts[i] = "Host(`" + d + "`)"
	}
	return strings.Join(parts, " || ")
}

func RenderRoute(data RouteData) ([]byte, error) {
	funcMap := template.FuncMap{
		"hostList": hostList,
	}
	tmpl, err := template.New("route").Funcs(funcMap).Parse(routeTemplate)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
