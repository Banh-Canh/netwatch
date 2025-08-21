package cmd

import (
	"os"
	"strconv"

	vtkiov1alpha1 "github.com/Banh-Canh/maxtac/api/v1alpha1"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	k8sScheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap" // Use the default zap logger for the manager

	"github.com/Banh-Canh/netwatch/internal/controller"
	"github.com/Banh-Canh/netwatch/internal/utils/logger" // custom slog logger for the rest, it is preferred for me
)

var managerCmd = &cobra.Command{
	Use:   "manager",
	Short: "Run the Netwatch controller manager.",
	Long: `Starts the Kubernetes controller that watches for Access and ExternalAccess
resources and manages the lifecycle of associated service clones.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := godotenv.Load(); err != nil {
			logger.Logger.Info("No .env file found for manager, using environment variables.")
		}

		// Configure controller-runtime's internal logger (zap).
		// The log level from the root command flag controls its verbosity. So it matches and align with the slog log level.
		useDevMode := false //nolint:all
		if logLevelFlag == "debug" {
			useDevMode = true
		}
		ctrl.SetLogger(zap.New(zap.UseDevMode(useDevMode)))

		scheme := runtime.NewScheme()
		k8sScheme.AddToScheme(scheme)     //nolint:all
		vtkiov1alpha1.AddToScheme(scheme) //nolint:all

		enableLeaderElection := true
		if v, ok := os.LookupEnv("ENABLE_LEADER_ELECTION"); ok {
			if b, err := strconv.ParseBool(v); err == nil {
				enableLeaderElection = b
			}
		}

		if !enableLeaderElection {
			logger.Logger.Warn("Leader election is DISABLED. This should only be used for local development.")
		}

		mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme:                 scheme,
			HealthProbeBindAddress: ":8081",
			LeaderElection:         enableLeaderElection,
			LeaderElectionID:       "netwatch-controller-leader-lock",
		})
		if err != nil {
			logger.Logger.Error("Unable to start controller manager", "error", err)
			os.Exit(1)
		}

		if err = (&controller.NetwatchCleanupReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			logger.Logger.Error("Unable to create cleanup controller", "error", err)
			os.Exit(1)
		}

		if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
			logger.Logger.Error("Unable to set up health check", "error", err)
			os.Exit(1)
		}
		if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
			logger.Logger.Error("Unable to set up ready check", "error", err)
			os.Exit(1)
		}

		logger.Logger.Info("Starting Netwatch controller manager")

		if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
			logger.Logger.Error("Problem running controller manager", "error", err)
			os.Exit(1)
		}
	},
}
