package traefik

import (
	"bytes"
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
      rule: "Host(` + "`" + `{{.Domain}}` + "`" + `)"
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
	Name   string
	Domain string
	IP     string
	Port   int
	TLS    bool
}

func RenderRoute(data RouteData) ([]byte, error) {
	tmpl, err := template.New("route").Parse(routeTemplate)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
