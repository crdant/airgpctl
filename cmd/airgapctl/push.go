package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/replicatedhq/airgapctl/pkg/bundle"
	"github.com/replicatedhq/airgapctl/pkg/distribution"
	"github.com/replicatedhq/airgapctl/pkg/registry"
	"github.com/spf13/cobra"
)

var pushOpts struct {
	registry      string
	namespace     string
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
				if err := loadDockerConfig(); err != nil {
					return err
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := bundle.OpenBundle(globalOpts.bundle)
			if err != nil {
				return fmt.Errorf("opening bundle: %w", err)
			}

			extractDir, err := os.MkdirTemp("", "airgapctl-push-*")
			if err != nil {
				return fmt.Errorf("creating temp directory: %w", err)
			}
			defer os.RemoveAll(extractDir)

			if err := b.ExtractTo(extractDir); err != nil {
				return fmt.Errorf("extracting bundle: %w", err)
			}

			walker := distribution.NewWalker(extractDir)
			images, err := walker.ResolveImages(b.Spec.SavedImages)
			if err != nil {
				return fmt.Errorf("resolving images: %w", err)
			}

			var pushRequests []registry.ImagePush
			for _, img := range images {
				// Extract full repository path from the original source ref, stripping the source registry host
				destRepo := distribution.ImagePath(img.SourceRef)
				if pushOpts.namespace != "" {
					// Prepend namespace while preserving the original image path
					destRepo = pushOpts.namespace + "/" + destRepo
				}
				pushRequests = append(pushRequests, registry.ImagePush{
					Source:   img,
					DestRepo: destRepo,
					Tag:      img.Tag,
				})
			}

			cfg := registry.Config{
				Registry: pushOpts.registry,
				Username: pushOpts.username,
				Password: pushOpts.password,
				Token:    pushOpts.token,
				Insecure: pushOpts.tlsSkipVerify,
			}
			pusher := registry.NewPusher(cfg)

			fmt.Fprintf(cmd.OutOrStdout(), "Pushing %d images to %s\n", len(pushRequests), pushOpts.registry)
			start := time.Now()

			reports := pusher.Push(context.Background(), pushRequests, walker, func(p registry.Progress) {
				fmt.Fprintf(cmd.OutOrStdout(), "[%d/%d] %s\n", p.Current, p.Total, p.Image)
			})

			var pushed, skipped, failed int
			for _, r := range reports {
				if !r.Success {
					failed++
					if r.Error != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "Failed: %s: %v\n", r.Image.SourceRef, r.Error)
					}
				} else if r.Skipped {
					skipped++
				} else {
					pushed++
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Done in %s — %d pushed, %d skipped, %d failed\n",
				time.Since(start).Round(time.Second), pushed, skipped, failed)

			if failed > 0 {
				return fmt.Errorf("%d image(s) failed to push", failed)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&pushOpts.registry, "registry", "", "destination registry URL (required)")
	cmd.Flags().StringVar(&pushOpts.namespace, "namespace", "", "destination namespace/prefix for image repositories")
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
			return fmt.Errorf("loading docker config: resolving home directory: %w", err)
		}
		configPath = filepath.Join(home, ".docker", "config.json")
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("loading docker config %s: %w", configPath, err)
	}
	var cfg dockerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parsing docker config %s: %w", configPath, err)
	}

	// Look for an auth entry matching the destination registry.
	// Try exact match first, then strip scheme/port prefixes.
	registry := pushOpts.registry
	candidates := []string{registry}
	if strings.HasPrefix(registry, "https://") {
		candidates = append(candidates, strings.TrimPrefix(registry, "https://"))
	} else if strings.HasPrefix(registry, "http://") {
		candidates = append(candidates, strings.TrimPrefix(registry, "http://"))
	} else {
		// Registry has no scheme; also try common scheme prefixes.
		candidates = append(candidates, "https://"+registry, "http://"+registry)
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
			if len(parts) != 2 {
				return fmt.Errorf("invalid auth format for %s: expected username:password", key)
			}
			pushOpts.username = parts[0]
			pushOpts.password = parts[1]
			return nil
		}
	}
	return nil
}
