package distribution

import "testing"

func TestImageName(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"library path", "library/nginx", "nginx"},
		{"full ref with tag", "registry.com/ns/app:1.0", "app"},
		{"plain name", "nginx", "nginx"},
		{"empty", "", ""},
		{"digest ref", "docker.io/library/nginx@sha256:abc123", "nginx"},
		{"tag and digest", "registry.com/ns/app:1.0@sha256:abc123", "app"},
		{"port registry digest", "localhost:5000/nginx@sha256:abc123", "nginx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ImageName(tt.ref)
			if got != tt.want {
				t.Errorf("ImageName(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}

func TestImagePath(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"with registry host and tag", "registry.com/ns/app:1.0", "ns/app"},
		{"with library namespace and tag", "library/nginx:latest", "library/nginx"},
		{"no registry no tag", "nginx", "nginx"},
		{"plain tag", "nginx:latest", "nginx"},
		{"with namespace", "library/nginx:latest", "library/nginx"},
		{"with registry host", "docker.io/library/nginx:latest", "library/nginx"},
		{"with registry port", "localhost:5000/library/nginx:latest", "library/nginx"},
		{"registry no tag", "docker.io/library/nginx", "library/nginx"},
		{"localhost no tag", "localhost:5000/nginx", "nginx"},
		{"digest ref", "docker.io/library/nginx@sha256:abc123", "library/nginx"},
		{"registry digest", "registry.com/nginx@sha256:abc123", "nginx"},
		{"port registry digest", "localhost:5000/nginx@sha256:abc123", "nginx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ImagePath(tt.ref)
			if got != tt.want {
				t.Errorf("ImagePath(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}

func TestStripTag(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"tag", "nginx:latest", "nginx"},
		{"no tag", "nginx", "nginx"},
		{"digest", "nginx@sha256:abc", "nginx@sha256:abc"},
		{"port registry tag", "localhost:5000/nginx:latest", "localhost:5000/nginx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripTag(tt.ref)
			if got != tt.want {
				t.Errorf("StripTag(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}
