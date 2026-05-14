package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func executeCommand(cmd *cobra.Command, args ...string) (string, string, *cobra.Command, error) {
	bufOut := new(bytes.Buffer)
	bufErr := new(bytes.Buffer)
	cmd.SetOut(bufOut)
	cmd.SetErr(bufErr)
	cmd.SetArgs(args)
	c, err := cmd.ExecuteC()
	return bufOut.String(), bufErr.String(), c, err
}

func TestRootCommandHasSubcommands(t *testing.T) {
	cmd := newRootCmd()
	children := cmd.Commands()
	if len(children) == 0 {
		t.Fatal("expected root command to have subcommands")
	}

	names := make(map[string]bool)
	for _, c := range children {
		names[c.Name()] = true
	}

	expected := []string{"list", "push", "values"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected subcommand %q to exist", name)
		}
	}
}

func TestRootCommandPersistentFlags(t *testing.T) {
	cmd := newRootCmd()
	bundleFlag := cmd.Flag("bundle")
	if bundleFlag == nil {
		t.Fatal("expected --bundle persistent flag")
	}
	verboseFlag := cmd.Flag("verbose")
	if verboseFlag == nil {
		t.Fatal("expected --verbose persistent flag")
	}
}

func TestListWithBundleFlag(t *testing.T) {
	cmd := newRootCmd()
	_, _, _, err := executeCommand(cmd, "list", "--bundle", "/tmp/fake-bundle.airgap")
	if err == nil {
		t.Fatal("expected error for nonexistent bundle")
	}
	if !strings.Contains(err.Error(), "/tmp/fake-bundle.airgap") {
		t.Errorf("expected error to contain bundle path, got: %v", err)
	}
}

func TestListWithPositionalArg(t *testing.T) {
	cmd := newRootCmd()
	_, _, _, err := executeCommand(cmd, "list", "/tmp/fake-bundle.airgap")
	if err == nil {
		t.Fatal("expected error for nonexistent bundle")
	}
	if !strings.Contains(err.Error(), "/tmp/fake-bundle.airgap") {
		t.Errorf("expected error to contain bundle path, got: %v", err)
	}
}

func TestListFlagWinsOverPositional(t *testing.T) {
	cmd := newRootCmd()
	_, _, _, err := executeCommand(cmd, "list", "--bundle", "/flag/path", "/positional/path")
	if err == nil {
		t.Fatal("expected error for nonexistent bundle")
	}
	if !strings.Contains(err.Error(), "/flag/path") {
		t.Errorf("expected error to use --bundle flag value, got: %v", err)
	}
	if strings.Contains(err.Error(), "/positional/path") {
		t.Errorf("expected error not to use positional arg when --bundle is set")
	}
}

func TestPushRequiresRegistry(t *testing.T) {
	// Create a valid mock bundle so bundle validation passes
	tmpDir := t.TempDir()
	bundlePath := createMockAirgapBundle(t, tmpDir, []string{}, []mockImageLayout{})

	cmd := newRootCmd()
	_, _, _, err := executeCommand(cmd, "push", "--bundle", bundlePath)
	if err == nil {
		t.Fatal("expected error when --registry is missing")
	}
	if !strings.Contains(err.Error(), "registry") {
		t.Errorf("expected error to mention registry, got: %v", err)
	}
}

func TestValuesRequiresChart(t *testing.T) {
	// Create a valid mock bundle so bundle validation passes
	tmpDir := t.TempDir()
	bundlePath := createMockAirgapBundle(t, tmpDir, []string{}, []mockImageLayout{})

	cmd := newRootCmd()
	_, _, _, err := executeCommand(cmd, "values", "--bundle", bundlePath)
	if err == nil {
		t.Fatal("expected error when --chart is missing")
	}
	if !strings.Contains(err.Error(), "chart") {
		t.Errorf("expected error to mention chart, got: %v", err)
	}
}

func TestPushCredentialsFromEnvVars(t *testing.T) {
	os.Setenv("AIRGAPCTL_REGISTRY_USERNAME", "envuser")
	os.Setenv("AIRGAPCTL_REGISTRY_PASSWORD", "envpass")
	defer os.Unsetenv("AIRGAPCTL_REGISTRY_USERNAME")
	defer os.Unsetenv("AIRGAPCTL_REGISTRY_PASSWORD")

	// Create a valid mock bundle
	tmpDir := t.TempDir()
	manifestJSON := makeSingleArchManifest(t)
	bundlePath := createMockAirgapBundle(t, tmpDir, []string{"nginx:latest"}, []mockImageLayout{
		{repo: "library/nginx", tag: "latest", manifestDigest: "nginx123", manifestJSON: manifestJSON},
	})

	// Create a mock registry server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/blobs/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/blobs/uploads/"):
			w.Header().Set("Location", "/upload")
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/upload"):
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/manifests/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/manifests/"):
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cmd := newRootCmd()
	out, outErr, _, err := executeCommand(cmd, "push", "--registry", server.URL, "--bundle", bundlePath)
	if err != nil {
		t.Fatalf("unexpected error: %v (stderr: %s)", err, outErr)
	}
	if !strings.Contains(out, "Pushing") {
		t.Errorf("expected push to proceed to RunE, got stdout: %s, stderr: %s", out, outErr)
	}
}

func TestBundlePathValidation(t *testing.T) {
	cmd := newRootCmd()
	// Create a temporary directory to simulate a bundle path that is a directory, not a file
	tmpDir := t.TempDir()
	_, _, _, err := executeCommand(cmd, "list", "--bundle", tmpDir)
	if err == nil {
		t.Fatal("expected error when bundle path is a directory")
	}
}

func TestValuesOutputFlag(t *testing.T) {
	cmd := newRootCmd()
	// Just verify the flag exists and defaults correctly
	cmd.SetArgs([]string{"values", "--help"})
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.ExecuteC()
	if !strings.Contains(out.String(), "--output") {
		t.Error("expected --output flag in values help")
	}
}

func TestPushRegistryFlags(t *testing.T) {
	cmd := newRootCmd()
	// Verify flags exist in help
	cmd.SetArgs([]string{"push", "--help"})
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.ExecuteC()

	expectedFlags := []string{"--registry", "--username", "--password", "--token", "--tls-skip-verify", "--tls-ca-cert"}
	for _, flag := range expectedFlags {
		if !strings.Contains(out.String(), flag) {
			t.Errorf("expected %s flag in push help", flag)
		}
	}
}

func TestHelpListsSubcommands(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--help"})
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.ExecuteC()

	help := out.String()
	for _, sub := range []string{"list", "push", "values"} {
		if !strings.Contains(help, sub) {
			t.Errorf("expected help to list %q subcommand", sub)
		}
	}
}

func TestDockerConfigFallback(t *testing.T) {
	// Create a fake docker config dir
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".docker")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create a mock registry server first so we know its URL
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/blobs/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/blobs/uploads/"):
			w.Header().Set("Location", "/upload")
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/upload"):
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/manifests/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/manifests/"):
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	configContent := fmt.Sprintf(`{"auths":{"%s":{"auth":"ZmFrZTp0b2tlbg=="}}}`, server.URL)
	configPath := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a valid mock bundle
	manifestJSON := makeSingleArchManifest(t)
	bundleFile := createMockAirgapBundle(t, tmpDir, []string{"nginx:latest"}, []mockImageLayout{
		{repo: "library/nginx", tag: "latest", manifestDigest: "nginx123", manifestJSON: manifestJSON},
	})

	oldDockerConfig := os.Getenv("DOCKER_CONFIG")
	os.Setenv("DOCKER_CONFIG", configDir)
	defer func() {
		if oldDockerConfig == "" {
			os.Unsetenv("DOCKER_CONFIG")
		} else {
			os.Setenv("DOCKER_CONFIG", oldDockerConfig)
		}
	}()

	cmd := newRootCmd()
	out, outErr, _, err := executeCommand(cmd, "push", "--registry", server.URL, "--bundle", bundleFile)
	if err != nil {
		t.Fatalf("unexpected error: %v (stderr: %s)", err, outErr)
	}
	if !strings.Contains(out, "Pushing") {
		t.Errorf("expected push to proceed to RunE with docker config fallback, got stdout: %s, stderr: %s", out, outErr)
	}
	// Verify credentials were actually loaded from docker config
	if pushOpts.username != "fake" || pushOpts.password != "token" {
		t.Errorf("expected credentials from docker config (fake:token), got username=%q password=%q", pushOpts.username, pushOpts.password)
	}
}
