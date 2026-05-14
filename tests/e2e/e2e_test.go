package e2e_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

var airgapctlBin string

func TestMain(m *testing.M) {
	// go test runs with the package directory as the working directory.
	// From tests/e2e/, the repo root is ../.. and the binary should be
	// produced inside this directory.
	build := exec.Command("go", "build", "-o", "airgapctl", "../../cmd/airgapctl")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		// Fail fast so that no tests run when the binary cannot be built.
		os.Exit(1)
	}
	airgapctlBin = "./airgapctl"
	os.Exit(m.Run())
}

// runAirgapctl executes the compiled airgapctl binary with the given args and
// returns stdout, stderr, and the exit code. Output is logged via t.Log.
func runAirgapctl(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(airgapctlBin, args...)
	outBuf := new(strings.Builder)
	errBuf := new(strings.Builder)
	cmd.Stdout = outBuf
	cmd.Stderr = errBuf

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	stdout = outBuf.String()
	stderr = errBuf.String()

	if stdout != "" {
		t.Logf("stdout:\n%s", stdout)
	}
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}

	return stdout, stderr, exitCode
}

// fixturePath returns the value of the given environment variable, or defaultPath
// if the variable is not set.
func fixturePath(envVar, defaultPath string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return defaultPath
}

func TestE2E_Help(t *testing.T) {
	stdout, stderr, exitCode := runAirgapctl(t, "--help")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", exitCode, stderr)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("expected stdout to contain 'Usage:', got:\n%s", stdout)
	}
}

func TestE2E_InvalidFlag(t *testing.T) {
	stdout, stderr, exitCode := runAirgapctl(t, "--nonexistent-flag")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code, got %d", exitCode)
	}
	if !strings.Contains(stderr, "error") && !strings.Contains(stderr, "unknown") && !strings.Contains(stderr, "flag") {
		t.Errorf("expected stderr to contain error text about unknown flag, got stdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
}
