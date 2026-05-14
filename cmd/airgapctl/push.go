package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(home, ".docker", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var cfg dockerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	// If we found auths and don't have credentials yet, try to use them.
	// In the stub we just load the file; full credential matching happens in U4.
	return nil
}
