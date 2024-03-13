package cobra_util

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/ydb-platform/ydbops/pkg/options"
)

type PersistentPreRunEFunc func(cmd *cobra.Command, args []string) error

// Right now, Cobra does not support chaining PersistentPreRun commands.
// https://github.com/spf13/cobra/issues/216
//
// If we want to declare PersistentPreRun and also want parent's
// PersistentPreRun command called, we need to manually call it.
// This function is a wrapper that can be specified in PersistentPreRun
// commands of children, look at `ydbops restart storage` implementation.
func makePersistentPreRunE(original PersistentPreRunEFunc) PersistentPreRunEFunc {
	wrapped := func(cmd *cobra.Command, args []string) error {
		if parent := cmd.Parent(); parent != nil {
			if parent.PersistentPreRunE != nil {
				if err := parent.PersistentPreRunE(parent, args); err != nil {
					return err
				}
			}
		}

		return original(cmd, args)
	}

	return wrapped
}

func determinePadding(curCommand, subCommandLineNumber, totalCommands int) string {
	if curCommand == totalCommands - 1 {
		if subCommandLineNumber == 0 {
			return "└─ "
		} else {
			return "   "
		}
	} else {
		if subCommandLineNumber == 0 {
			return "├─ "
		} else {
			return "│  "
		}
	}
}

func generateCommandTree(cmd *cobra.Command) []string {
		result := []string{cmd.Name()}
		if cmd.HasAvailableSubCommands() {
			subCommandLen := len(cmd.Commands())
			for i := 0; i < len(cmd.Commands()); i++ {
				subCmd := cmd.Commands()[i]
				if !subCmd.Hidden {
					subCmdTree := generateCommandTree(subCmd)
					for j, line := range subCmdTree {
						result = append(result, determinePadding(i, j, subCommandLen) + line)
					}
				}
			}
		}
		return result
}

func SetDefaultsOn(cmd *cobra.Command, opts options.Options) *cobra.Command {
	cmd.Flags().SortFlags = false
	cmd.PersistentFlags().SortFlags = false

	cobra.AddTemplateFunc("drawNiceTree", func(cmd *cobra.Command) string {
		if cmd.HasAvailableSubCommands() {
			var builder strings.Builder
			builder.WriteString("Subcommands:")
			for _, line := range generateCommandTree(cmd) {
				builder.WriteString("\n")
				builder.WriteString(line)
			}
			builder.WriteString("\n")
			return builder.String()
		}
		return ""
	})

	cmd.SetUsageTemplate(UsageTemplate)

	if cmd.PersistentPreRunE != nil {
		cmd.PersistentPreRunE = makePersistentPreRunE(
			func(cmd *cobra.Command, args []string) error {
				if opts != nil {
					return opts.Validate()
				}
        return nil
			},
		)
	}

	return cmd
}
