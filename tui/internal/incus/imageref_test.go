package incus

import (
	"testing"
)

func TestParseImageRef(t *testing.T) {
	tests := []struct {
		input      string
		wantRemote string
		wantAlias  string
	}{
		// remote:alias format
		{"gcr:project/repo/image:tag", "gcr", "project/repo/image:tag"},
		{"docker:traefik/whoami", "docker", "traefik/whoami"},
		{"ecr:my-repo:latest", "ecr", "my-repo:latest"},

		// docker.io prefix stripped
		{"docker.io/traefik/whoami", "docker", "traefik/whoami"},
		{"docker.io/library/nginx:latest", "docker", "library/nginx:latest"},

		// bare image (defaults to docker)
		{"traefik/whoami", "docker", "traefik/whoami"},
		{"nginx", "docker", "nginx"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			remote, alias := parseImageRef(tt.input)
			if remote != tt.wantRemote {
				t.Errorf("parseImageRef(%q) remote = %q, want %q", tt.input, remote, tt.wantRemote)
			}
			if alias != tt.wantAlias {
				t.Errorf("parseImageRef(%q) alias = %q, want %q", tt.input, alias, tt.wantAlias)
			}
		})
	}
}
