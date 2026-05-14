package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var valuesOpts struct {
	chart     string
	registry  string
	namespace string
	output    string
	template  string
}

func newValuesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "values",
		Short: "Generate Helm values file with remapped image references",
		Long:  `Read a Helm chart's values.yaml, detect image references, and generate an output YAML with images remapped to the destination registry.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if valuesOpts.chart == "" {
				return fmt.Errorf("--chart is required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "Generating values from chart %s\n", valuesOpts.chart)
			fmt.Fprintf(cmd.OutOrStdout(), "Bundle: %s\n", globalOpts.bundle)
			fmt.Fprintf(cmd.OutOrStdout(), "Output: %s\n", valuesOpts.output)
			fmt.Fprintln(cmd.OutOrStdout(), "  (values implementation pending)")
			return nil
		},
	}

	cmd.Flags().StringVar(&valuesOpts.chart, "chart", "", "path to Helm chart directory (required)")
	cmd.Flags().StringVar(&valuesOpts.registry, "registry", "", "destination registry URL")
	cmd.Flags().StringVar(&valuesOpts.namespace, "namespace", "", "destination namespace/prefix")
	cmd.Flags().StringVar(&valuesOpts.output, "output", "values-airgap.yaml", "output values file path")
	cmd.Flags().StringVar(&valuesOpts.template, "template", "", "Go template for destination image path")

	_ = cmd.MarkFlagRequired("chart")

	return cmd
}
