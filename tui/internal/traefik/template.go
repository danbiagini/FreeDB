package traefik

import (
	"bytes"
	"text/template"
)

const routeTemplate = `http:
  routers:
    {{.Name}}-router:
      entryPoints:
        - "websecure"
      rule: "Host(` + "`" + `{{.Domain}}` + "`" + `)"
      service: {{.Name}}
      tls:
        certResolver: myresolver
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
