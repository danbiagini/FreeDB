# Multi-Domain Support for FreeDB Apps

Tracking issue: [#66](https://github.com/danbiagini/FreeDB/issues/66)

## Context

Apps currently support a single domain. Users need multiple domains per app (e.g., `api.example.com` and `api.staging.example.com`, or `example.com` and `www.example.com`). This should be configurable during app creation and editable from the manage view.

## Approach

### Storage: Dual fields for backward compatibility

- Add `Domains []string` to `registry.App` alongside existing `Domain string`
- Helper methods: `GetDomains()`, `SetDomains()`, `HasDomains()`
- On registry load, normalize: if `Domains` is empty but `Domain` is set, populate `Domains` from `Domain`
- On save, keep both in sync (`Domain` = first entry of `Domains`)
- Zero migration needed — old registry files just work

### Traefik: Multi-host rule

- Change `RouteData.Domain` to `Domains []string`
- Use Traefik's `Host(`a`, `b`)` syntax via a `hostList` template function
- Change `PushRoute(ic, name, domain, ip, port, tls)` to `PushRoute(ic, name, domains, ip, port, tls)`

### TUI Add App: Comma-separated input

- Change placeholder to `myapp.example.com, www.myapp.example.com`
- Parse comma-separated input into `[]string`
- Increase char limit to 200

### TUI Manage View: Domain editor

- Follow the env vars pattern: `[o] Domains` opens list view
- `[a]` add, `[d]` delete, arrow keys to navigate, `[esc]` back
- After changes, push updated Traefik route and save registry

### Dashboard: Primary domain + count

- Show `myapp.example.com +1` when multiple domains configured

## Files to Modify

| File | Change |
|------|--------|
| `tui/internal/registry/types.go` | Add `Domains` field, helper methods |
| `tui/internal/registry/registry.go` | Normalize on load/reload, add `UpdateDomains()` |
| `tui/internal/traefik/template.go` | Multi-host template, `Domains []string` in RouteData, `hostList` func |
| `tui/internal/traefik/routes.go` | Change `PushRoute` signature to `domains []string` |
| `tui/internal/deploy/engine.go` | Use `HasDomains()` / `GetDomains()` |
| `tui/internal/tui/dashboard/model.go` | Primary domain + count display |
| `tui/internal/tui/addapp/model.go` | Comma-separated input, parse to slice |
| `tui/internal/tui/manage/model.go` | Add domain editor subview (add/delete) |
| `tui/main.go` | Update CLI list/status/destroy output |

## Implementation Order

1. Registry types + helpers (foundation)
2. Registry load normalization + `UpdateDomains`
3. Traefik template + routes (multi-host rendering)
4. Deploy engine (update callers)
5. Dashboard (display)
6. Add app flow (comma-separated input)
7. Manage view (domain editor)
8. CLI output
9. Tests: template_test.go (multi-domain), registry_test.go (backward compat), addapp model_test.go

## Verification

- Build and run all tests: `cd tui && go build ./... && go test ./...`
- Test on instance:
  - Create app with multiple domains via TUI
  - Verify Traefik route has multi-host rule
  - Add/remove domain from manage view
  - Verify existing single-domain apps still work (backward compat)
  - Check dashboard shows `domain +N` format
  - `freedb status myapp` shows all domains
