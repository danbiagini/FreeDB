package config

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

type HostInfo struct {
	Hostname string
	Cloud    string
	IP       string
	OS       string
	Arch     string
	CPUs     int
}

func GetHostInfo() *HostInfo {
	info := &HostInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
		CPUs: runtime.NumCPU(),
	}

	info.Hostname, _ = os.Hostname()
	info.Cloud = detectCloud()
	info.IP = getIP(info.Cloud)

	return info
}

func (h *HostInfo) String() string {
	parts := []string{h.Hostname}
	if h.Cloud != "unknown" {
		parts = append(parts, h.Cloud)
	}
	if h.IP != "" {
		parts = append(parts, h.IP)
	}
	parts = append(parts, fmt.Sprintf("%d CPUs", h.CPUs))
	return strings.Join(parts, " | ")
}

func detectCloud() string {
	client := &http.Client{Timeout: 1 * time.Second}

	// GCP
	req, _ := http.NewRequest("GET", "http://metadata.google.internal/computeMetadata/v1/", nil)
	if req != nil {
		req.Header.Set("Metadata-Flavor", "Google")
		if resp, err := client.Do(req); err == nil {
			resp.Body.Close()
			return "GCP"
		}
	}

	// AWS IMDSv2
	req, _ = http.NewRequest("PUT", "http://169.254.169.254/latest/api/token", nil)
	if req != nil {
		req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "10")
		if resp, err := client.Do(req); err == nil {
			resp.Body.Close()
			return "AWS"
		}
	}

	return "unknown"
}

func getIP(cloud string) string {
	client := &http.Client{Timeout: 1 * time.Second}

	switch cloud {
	case "GCP":
		req, _ := http.NewRequest("GET",
			"http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/access-configs/0/external-ip", nil)
		if req != nil {
			req.Header.Set("Metadata-Flavor", "Google")
			if resp, err := client.Do(req); err == nil {
				defer resp.Body.Close()
				buf := make([]byte, 64)
				n, _ := resp.Body.Read(buf)
				return strings.TrimSpace(string(buf[:n]))
			}
		}
	case "AWS":
		// Get token first
		req, _ := http.NewRequest("PUT", "http://169.254.169.254/latest/api/token", nil)
		if req != nil {
			req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "10")
			if resp, err := client.Do(req); err == nil {
				buf := make([]byte, 256)
				n, _ := resp.Body.Read(buf)
				resp.Body.Close()
				token := strings.TrimSpace(string(buf[:n]))

				req2, _ := http.NewRequest("GET", "http://169.254.169.254/latest/meta-data/public-ipv4", nil)
				if req2 != nil {
					req2.Header.Set("X-aws-ec2-metadata-token", token)
					if resp2, err := client.Do(req2); err == nil {
						defer resp2.Body.Close()
						buf2 := make([]byte, 64)
						n2, _ := resp2.Body.Read(buf2)
						return strings.TrimSpace(string(buf2[:n2]))
					}
				}
			}
		}
	}

	return ""
}
