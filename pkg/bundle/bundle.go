// Package bundle provides functionality for extracting and parsing Replicated .airgap bundles.
package bundle

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Bundle represents a parsed .airgap bundle.
type Bundle struct {
	Version     string   `yaml:"Version"`
	Type        string   `yaml:"Type"`
	SavedImages []string `yaml:"SavedImages"`

	bundlePath string
}

// OpenBundle opens a .airgap file, extracts and parses airgap.yaml, and returns a Bundle.
func OpenBundle(path string) (*Bundle, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening bundle: %w", err)
	}
	defer f.Close()

	tr := tar.NewReader(f)
	var airgapYamlData []byte
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading bundle tar: %w", err)
		}

		if header.Name == "airgap.yaml" {
			airgapYamlData, err = io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("reading airgap.yaml: %w", err)
			}
			break
		}
	}

	if airgapYamlData == nil {
		return nil, fmt.Errorf("airgap.yaml not found in bundle")
	}

	var b Bundle
	if err := yaml.Unmarshal(airgapYamlData, &b); err != nil {
		return nil, fmt.Errorf("parsing airgap.yaml: %w", err)
	}
	b.bundlePath = path

	if b.SavedImages == nil {
		return nil, fmt.Errorf("airgap.yaml missing SavedImages field")
	}

	return &b, nil
}

// ExtractTo extracts all contents of the .airgap bundle to the specified directory.
func (b *Bundle) ExtractTo(dest string) error {
	if err := os.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("creating extraction directory: %w", err)
	}

	f, err := os.Open(b.bundlePath)
	if err != nil {
		return fmt.Errorf("opening bundle: %w", err)
	}
	defer f.Close()

	tr := tar.NewReader(f)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading bundle tar: %w", err)
		}

		target := filepath.Join(dest, header.Name)
		// Prevent directory traversal
		if !isWithinDest(dest, target) {
			return fmt.Errorf("invalid tar entry: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("creating directory %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("creating parent directory for %s: %w", target, err)
			}
			out, err := os.Create(target)
			if err != nil {
				return fmt.Errorf("creating file %s: %w", target, err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return fmt.Errorf("writing file %s: %w", target, err)
			}
			out.Close()
			if err := os.Chmod(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("setting permissions on %s: %w", target, err)
			}
		}
	}

	return nil
}

// isWithinDest checks if target is within the destination directory to prevent directory traversal.
func isWithinDest(dest, target string) bool {
	rel, err := filepath.Rel(dest, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
