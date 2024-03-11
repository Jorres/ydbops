package cmd

import (
	"github.com/spf13/cobra"
	"github.com/ydb-platform/ydb-ops/cmd/restart"
	"github.com/ydb-platform/ydb-ops/cmd/restart/storage"
	"github.com/ydb-platform/ydb-ops/cmd/restart/tenant"
	"github.com/ydb-platform/ydb-ops/pkg/options"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func addAndReturn(cmd *cobra.Command, rest ...*cobra.Command) *cobra.Command {
	for _, subCmd := range rest {
		cmd.AddCommand(subCmd)
	}
	return cmd
}

func registerAllSubcommands(root *cobra.Command) {
	_ = addAndReturn(root,
		addAndReturn(NewRestartCmd(),
			addAndReturn(restart.NewStorageCmd(),
				storage.NewStorageK8sCmd(),
				storage.NewStorageBaremetalCmd(),
			),
			addAndReturn(restart.NewTenantCmd(),
				tenant.NewTenantBaremetalCmd(),
			),
			restart.NewRunCmd(),
		),
	)
}

var RootCmd *cobra.Command

func InitRootCmd(logLevelSetter zap.AtomicLevel, logger *zap.SugaredLogger) {
	RootCmd = &cobra.Command{
		Use:   "ydb-ops",
		Short: "ydb-ops: a CLI tool with common YDB cluster maintenance operations",
		Long:  "ydb-ops: a CLI tool with common YDB cluster maintenance operations",
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			logLevel := "info"
			if options.RootOptionsInstance.Verbose {
				logLevel = "debug"
			}

			lvc, err := zapcore.ParseLevel(logLevel)
			if err != nil {
				logger.Warn("Failed to set level")
				return err
			}
			logLevelSetter.SetLevel(lvc)

			zap.S().Debugf("Current logging level enabled: %s", logLevel)

			return options.RootOptionsInstance.Validate()
		},
		// TODO decide if we need to hide this, for more compact --help
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}

	defer func() {
		_ = logger.Sync()
	}()

	options.Logger = logger

	options.RootOptionsInstance.DefineFlags(RootCmd.PersistentFlags())

	registerAllSubcommands(RootCmd)
}