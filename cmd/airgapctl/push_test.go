package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestImagePath(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		expected string
	}{
		{"plain tag", "nginx:latest", "nginx"},
		{"with namespace", "library/nginx:latest", "library/nginx"},
		{"with registry host", "docker.io/library/nginx:latest", "library/nginx"},
		{"with registry port", "localhost:5000/library/nginx:latest", "library/nginx"},
		{"registry no tag", "docker.io/library/nginx", "library/nginx"},
		{"localhost no tag", "localhost:5000/nginx", "nginx"},
		{"digest ref", "docker.io/library/nginx@sha256:abc123", "library/nginx"},
		{"registry digest", "registry.com/nginx@sha256:abc123", "nginx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := imagePath(tt.ref)
			if got != tt.expected {
				t.Errorf("imagePath(%q) = %q, want %q", tt.ref, got, tt.expected)
			}
		})
	}
}

func TestLoadDockerConfig_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	origDockerConfig := os.Getenv("DOCKER_CONFIG")
	os.Setenv("DOCKER_CONFIG", tmpDir)
	defer os.Setenv("DOCKER_CONFIG", origDockerConfig)

	pushOpts.registry = "myregistry.example.com"

	err := loadDockerConfig()
	if err == nil {
		t.Fatal("expected error for missing docker config file")
	}
	want := "loading docker config"
	if !contains(err.Error(), want) {
		t.Errorf("expected error to contain %q, got: %v", want, err)
	}
}

func TestLoadDockerConfig_MalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	origDockerConfig := os.Getenv("DOCKER_CONFIG")
	os.Setenv("DOCKER_CONFIG", tmpDir)
	defer os.Setenv("DOCKER_CONFIG", origDockerConfig)

	malformed := `this is not valid json{`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(malformed), 0644); err != nil {
		t.Fatalf("writing malformed config: %v", err)
	}

	pushOpts.registry = "myregistry.example.com"

	err := loadDockerConfig()
	if err == nil {
		t.Fatal("expected error for malformed docker config")
	}
	want := "parsing docker config"
	if !contains(err.Error(), want) {
		t.Errorf("expected error to contain %q, got: %v", want, err)
	}
}

func TestLoadDockerConfig_SchemeMatching(t *testing.T) {
	tmpDir := t.TempDir()
	origDockerConfig := os.Getenv("DOCKER_CONFIG")
	os.Setenv("DOCKER_CONFIG", tmpDir)
	defer os.Setenv("DOCKER_CONFIG", origDockerConfig)

	// Write a docker config keyed by https://myregistry.example.com
	config := fmt.Sprintf(`{"auths":{"https://myregistry.example.com":{"username":"admin","password":"secret"}}}`)
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(config), 0644); err != nil {
		t.Fatalf("writing docker config: %v", err)
	}

	// Registry passed without scheme should still match the https:// key
	pushOpts.registry = "myregistry.example.com"
	pushOpts.username = ""
	pushOpts.password = ""

	if err := loadDockerConfig(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pushOpts.username != "admin" {
		t.Errorf("expected username 'admin', got %q", pushOpts.username)
	}
	if pushOpts.password != "secret" {
		t.Errorf("expected password 'secret', got %q", pushOpts.password)
	}
}

func TestLoadDockerConfig_MalformedAuthField(t *testing.T) {
	tmpDir := t.TempDir()
	origDockerConfig := os.Getenv("DOCKER_CONFIG")
	os.Setenv("DOCKER_CONFIG", tmpDir)
	defer os.Setenv("DOCKER_CONFIG", origDockerConfig)

	// base64 of "user-without-password-separator" (no colon)
	badAuth := base64.StdEncoding.EncodeToString([]byte("userwithoutcolon"))
	config := fmt.Sprintf(`{"auths":{"myregistry.example.com":{"auth":"%s"}}}`, badAuth)
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(config), 0644); err != nil {
		t.Fatalf("writing docker config: %v", err)
	}

	pushOpts.registry = "myregistry.example.com"
	pushOpts.username = ""
	pushOpts.password = ""

	err := loadDockerConfig()
	if err == nil {
		t.Fatal("expected error for malformed auth field")
	}
	want := "invalid auth format"
	if !contains(err.Error(), want) {
		t.Errorf("expected error to contain %q, got: %v", want, err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSub(s, substr))
}

func containsSub(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
