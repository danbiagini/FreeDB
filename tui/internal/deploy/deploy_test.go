package deploy

import (
	"testing"

	"github.com/danbiagini/freedb-tui/internal/registry"
)

func TestResolveImage(t *testing.T) {
	tests := []struct {
		name      string
		opts      Options
		app       *registry.App
		want      string
		wantError bool
	}{
		{
			name: "explicit image",
			opts: Options{Image: "ecr:myapp:v1.2.3"},
			app:  &registry.App{Image: "ecr:myapp:latest"},
			want: "ecr:myapp:v1.2.3",
		},
		{
			name: "tag only with colon in existing",
			opts: Options{Tag: "v2.0"},
			app:  &registry.App{Image: "ecr:myapp:latest"},
			want: "ecr:myapp:v2.0",
		},
		{
			name: "tag only without existing tag",
			opts: Options{Tag: "v1.0"},
			app:  &registry.App{Image: "docker.io/myapp"},
			want: "docker.io/myapp:v1.0",
		},
		{
			name: "tag with docker.io image",
			opts: Options{Tag: "v3"},
			app:  &registry.App{Image: "docker.io/traefik/whoami:latest"},
			want: "docker.io/traefik/whoami:v3",
		},
		{
			name:      "neither image nor tag",
			opts:      Options{},
			app:       &registry.App{Image: "ecr:myapp:latest"},
			wantError: true,
		},
		{
			name: "image takes precedence over tag",
			opts: Options{Image: "ecr:myapp:v1", Tag: "v2"},
			app:  &registry.App{Image: "ecr:myapp:latest"},
			want: "ecr:myapp:v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveImage(tt.opts, tt.app)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ResolveImage() = %q, want %q", got, tt.want)
			}
		})
	}
}
