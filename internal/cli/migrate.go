package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	yamlv3 "gopkg.in/yaml.v3"

	workflowyaml "github.com/diasYuri/agentflow/internal/adapters/yaml"
	"github.com/diasYuri/agentflow/internal/core/workflow"
)

func newMigrateCommand() *cobra.Command {
	var toVersion string
	var outPath string
	cmd := &cobra.Command{
		Use:   "migrate <workflow>",
		Short: "Migrate a workflow to a newer version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if toVersion != "2" {
				return fmt.Errorf("unsupported target version %q (only \"2\" is supported)", toVersion)
			}
			spec, err := workflowyaml.DecodeWorkflow(args[0])
			if err != nil {
				return err
			}
			if spec.Version != workflow.WorkflowVersion1 {
				return fmt.Errorf("workflow version is %q; migration only supports source version %q", spec.Version, workflow.WorkflowVersion1)
			}
			migrated := migrateSpecToV2(spec)
			data, err := yamlMarshalWorkflow(migrated)
			if err != nil {
				return fmt.Errorf("marshal migrated workflow: %w", err)
			}
			if outPath != "" {
				if err := os.WriteFile(outPath, data, 0o644); err != nil {
					return fmt.Errorf("write output file: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "migrated workflow written to %s\n", outPath)
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
	cmd.Flags().StringVar(&toVersion, "to", "2", "target workflow version")
	cmd.Flags().StringVar(&outPath, "out", "", "output file path (default: stdout)")
	return cmd
}

func migrateSpecToV2(spec *workflow.WorkflowSpec) workflow.WorkflowSpec {
	migrated := *spec
	migrated.Version = workflow.WorkflowVersion2
	return migrated
}

func yamlMarshalWorkflow(spec workflow.WorkflowSpec) ([]byte, error) {
	return yamlv3.Marshal(spec)
}
