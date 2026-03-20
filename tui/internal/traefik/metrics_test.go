package traefik

import (
	"testing"
)

const sampleMetrics = `# HELP traefik_service_requests_total How many HTTP requests processed, partitioned by status code, protocol, and method.
# TYPE traefik_service_requests_total counter
traefik_service_requests_total{code="200",method="GET",protocol="http",service="myapp@file"} 150
traefik_service_requests_total{code="404",method="GET",protocol="http",service="myapp@file"} 3
traefik_service_requests_total{code="200",method="GET",protocol="http",service="whoami@file"} 42
traefik_service_requests_total{code="500",method="POST",protocol="http",service="myapp@file"} 1
# HELP traefik_service_requests_bytes_total The total volume of requests in bytes.
# TYPE traefik_service_requests_bytes_total counter
traefik_service_requests_bytes_total{code="200",method="GET",protocol="http",service="myapp@file"} 15000
# HELP traefik_service_responses_bytes_total The total volume of responses in bytes.
# TYPE traefik_service_responses_bytes_total counter
traefik_service_responses_bytes_total{code="200",method="GET",protocol="http",service="myapp@file"} 450000
# HELP traefik_service_open_connections How many open connections exist on a service.
# TYPE traefik_service_open_connections gauge
traefik_service_open_connections{method="GET",protocol="http",service="myapp@file"} 2
`

func TestParseMetrics(t *testing.T) {
	result := parseMetrics(sampleMetrics)

	myapp, ok := result["myapp"]
	if !ok {
		t.Fatal("myapp not found in results")
	}

	if myapp.TotalReqs != 154 { // 150 + 3 + 1
		t.Errorf("expected 154 total requests, got %.0f", myapp.TotalReqs)
	}

	if myapp.ErrorReqs != 4 { // 3 (404) + 1 (500)
		t.Errorf("expected 4 error requests, got %.0f", myapp.ErrorReqs)
	}

	if myapp.BytesIn != 15000 {
		t.Errorf("expected 15000 bytes in, got %.0f", myapp.BytesIn)
	}

	if myapp.BytesOut != 450000 {
		t.Errorf("expected 450000 bytes out, got %.0f", myapp.BytesOut)
	}

	if myapp.Connections != 2 {
		t.Errorf("expected 2 connections, got %.0f", myapp.Connections)
	}

	whoami, ok := result["whoami"]
	if !ok {
		t.Fatal("whoami not found in results")
	}

	if whoami.TotalReqs != 42 {
		t.Errorf("expected 42 requests for whoami, got %.0f", whoami.TotalReqs)
	}

	if whoami.ErrorReqs != 0 {
		t.Errorf("expected 0 errors for whoami, got %.0f", whoami.ErrorReqs)
	}
}

func TestParseMetricsStripsServiceSuffix(t *testing.T) {
	input := `traefik_service_requests_total{code="200",method="GET",protocol="http",service="test@file"} 10
traefik_service_requests_total{code="200",method="GET",protocol="http",service="test@internal"} 5
`
	result := parseMetrics(input)

	test, ok := result["test"]
	if !ok {
		t.Fatal("test not found")
	}
	// Both @file and @internal map to "test"
	if test.TotalReqs != 15 {
		t.Errorf("expected 15 total, got %.0f", test.TotalReqs)
	}
}

func TestParseMetricsEmpty(t *testing.T) {
	result := parseMetrics("")
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d entries", len(result))
	}
}
