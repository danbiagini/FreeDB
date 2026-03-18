package traefik

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/danbiagini/freedb-tui/internal/incus"
)

// ServiceMetrics holds per-service traffic stats from Traefik Prometheus metrics
type ServiceMetrics struct {
	Requests     map[string]float64 // requests by status code (e.g., "200" -> 150)
	TotalReqs    float64
	ErrorReqs    float64 // 4xx + 5xx
	BytesIn      float64
	BytesOut     float64
	Connections  float64
}

// metricsLineRegex matches Prometheus metric lines like:
// traefik_service_requests_total{code="200",method="GET",protocol="http",service="myapp@file"} 42
var metricsLineRegex = regexp.MustCompile(`^(\w+)\{([^}]*)\}\s+(\S+)`)
var labelRegex = regexp.MustCompile(`(\w+)="([^"]*)"`)

// FetchMetrics scrapes the Traefik Prometheus endpoint via the proxy1 container
// and returns per-service metrics keyed by service name (without @file suffix)
func FetchMetrics(ic *incus.Client, proxyContainer string) (map[string]*ServiceMetrics, error) {
	// Get proxy1 IP
	ip, err := ic.GetContainerIP(context.Background(), proxyContainer)
	if err != nil {
		return nil, fmt.Errorf("getting proxy IP: %w", err)
	}

	// Fetch metrics endpoint
	resp, err := http.Get(fmt.Sprintf("http://%s:8080/metrics", ip))
	if err != nil {
		return nil, fmt.Errorf("fetching metrics: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading metrics: %w", err)
	}

	return parseMetrics(string(body)), nil
}

func parseMetrics(body string) map[string]*ServiceMetrics {
	result := make(map[string]*ServiceMetrics)

	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		matches := metricsLineRegex.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		metricName := matches[1]
		labelsStr := matches[2]
		valueStr := matches[3]

		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}

		// Parse labels
		labels := make(map[string]string)
		for _, lm := range labelRegex.FindAllStringSubmatch(labelsStr, -1) {
			labels[lm[1]] = lm[2]
		}

		// We only care about service-level metrics
		serviceName, ok := labels["service"]
		if !ok {
			continue
		}

		// Strip @file or @internal suffix
		if idx := strings.Index(serviceName, "@"); idx >= 0 {
			serviceName = serviceName[:idx]
		}

		sm, ok := result[serviceName]
		if !ok {
			sm = &ServiceMetrics{
				Requests: make(map[string]float64),
			}
			result[serviceName] = sm
		}

		switch metricName {
		case "traefik_service_requests_total":
			code := labels["code"]
			sm.Requests[code] = value
			sm.TotalReqs += value
			if len(code) > 0 && (code[0] == '4' || code[0] == '5') {
				sm.ErrorReqs += value
			}

		case "traefik_service_requests_bytes_total":
			sm.BytesIn += value

		case "traefik_service_responses_bytes_total":
			sm.BytesOut += value

		case "traefik_service_open_connections":
			sm.Connections += value
		}
	}

	return result
}
