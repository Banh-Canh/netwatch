package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/Banh-Canh/netwatch/internal/utils/logger"
)

var (
	versionFlag  bool
	logLevelFlag string
	version      = "dev"
)

var RootCmd = &cobra.Command{
	Use:   "netwatch",
	Short: "A tool to manage temporary Kubernetes network access via a web UI and a controller.",
	Long: `Netwatch provides both a web UI (the 'server' subcommand) and a Kubernetes
controller (the 'manager' subcommand) to handle temporary network policies in a cluster.`,
	Run: func(cmd *cobra.Command, args []string) {
		if versionFlag {
			fmt.Printf("Netwatch version: %s\n", version)
			os.Exit(0)
		}
		cmd.Help() //nolint:all
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// This runs before any subcommand, ensuring the logger is always configured.
		initConfig()
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func initConfig() {
	logLevel := slog.LevelInfo
	if logLevelFlag == "debug" {
		logLevel = slog.LevelDebug
	}
	logger.InitializeLogger(logLevel)
}

func init() {
	RootCmd.AddCommand(serverCmd)
	RootCmd.AddCommand(managerCmd)
	RootCmd.Flags().BoolVarP(&versionFlag, "version", "v", false, "Display version information")
	RootCmd.PersistentFlags().StringVarP(&logLevelFlag, "log-level", "l", "", "Override log level (e.g., 'debug')")
}
