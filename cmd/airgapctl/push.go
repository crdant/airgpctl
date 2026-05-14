package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var pushOpts struct {
	registry      string
	username      string
	password      string
	token         string
	tlsSkipVerify bool
	tlsCaCert     string
}

func newPushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push bundle images to a registry",
		Long:  `Push all container images from the bundle to the destination OCI-compatible registry.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if pushOpts.registry == "" {
				return fmt.Errorf("--registry is required")
			}
			// Resolve credentials priority: flags > env vars > docker config
			if pushOpts.username == "" {
				pushOpts.username = os.Getenv("AIRGAPCTL_REGISTRY_USERNAME")
			}
			if pushOpts.password == "" {
				pushOpts.password = os.Getenv("AIRGAPCTL_REGISTRY_PASSWORD")
			}
			if pushOpts.token == "" {
				pushOpts.token = os.Getenv("AIRGAPCTL_REGISTRY_TOKEN")
			}
			if pushOpts.username == "" && pushOpts.token == "" {
				_ = loadDockerConfig()
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "Pushing images from %s to registry %s\n", globalOpts.bundle, pushOpts.registry)
			fmt.Fprintln(cmd.OutOrStdout(), "  (push implementation pending)")
			return nil
		},
	}

	cmd.Flags().StringVar(&pushOpts.registry, "registry", "", "destination registry URL (required)")
	cmd.Flags().StringVar(&pushOpts.username, "username", "", "registry username")
	cmd.Flags().StringVar(&pushOpts.password, "password", "", "registry password")
	cmd.Flags().StringVar(&pushOpts.token, "token", "", "registry bearer token")
	cmd.Flags().BoolVar(&pushOpts.tlsSkipVerify, "tls-skip-verify", false, "skip TLS certificate verification")
	cmd.Flags().StringVar(&pushOpts.tlsCaCert, "tls-ca-cert", "", "path to custom CA certificate file")

	_ = cmd.MarkFlagRequired("registry")

	return cmd
}

// dockerConfig mirrors the minimal structure of ~/.docker/config.json we need.
type dockerConfig struct {
	Auths map[string]struct {
		Username string `json:"username,omitempty"`
		Password string `json:"password,omitempty"`
		Auth     string `json:"auth,omitempty"`
	} `json:"auths,omitempty"`
}

func loadDockerConfig() error {
	var configPath string
	if envDir := os.Getenv("DOCKER_CONFIG"); envDir != "" {
		configPath = filepath.Join(envDir, "config.json")
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		configPath = filepath.Join(home, ".docker", "config.json")
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var cfg dockerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}

	// Look for an auth entry matching the destination registry.
	// Try exact match first, then strip scheme/port prefixes.
	registry := pushOpts.registry
	candidates := []string{registry}
	if strings.HasPrefix(registry, "https://") {
		candidates = append(candidates, strings.TrimPrefix(registry, "https://"))
	} else if strings.HasPrefix(registry, "http://") {
		candidates = append(candidates, strings.TrimPrefix(registry, "http://"))
	}
	if idx := strings.Index(registry, ":"); idx > 0 {
		candidates = append(candidates, registry[:idx])
	}

	for _, key := range candidates {
		auth, ok := cfg.Auths[key]
		if !ok {
			continue
		}
		if auth.Username != "" && auth.Password != "" {
			pushOpts.username = auth.Username
			pushOpts.password = auth.Password
			return nil
		}
		if auth.Auth != "" {
			decoded, err := base64.StdEncoding.DecodeString(auth.Auth)
			if err != nil {
				return fmt.Errorf("invalid base64 auth for %s: %w", key, err)
			}
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 {
				pushOpts.username = parts[0]
				pushOpts.password = parts[1]
				return nil
			}
		}
	}
	return nil
}
