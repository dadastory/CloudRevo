package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/dadastory/CloudRevo/application"
	"github.com/dadastory/CloudRevo/application/constants"
	"github.com/dadastory/CloudRevo/application/dependency"
	"github.com/dadastory/CloudRevo/pkg/logging"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(serverCmd)
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start a CloudRevo server with the given config file",
	Run: func(cmd *cobra.Command, args []string) {
		dep := dependency.NewDependency(
			dependency.WithConfigPath(confPath),
			dependency.WithRequiredDbVersion(constants.BackendVersion),
		)
		server := application.NewServer(dep)
		logger := dep.Logger()

		server.PrintBanner()

		// Graceful shutdown after received signal.
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
		go shutdown(sigChan, logger, server)

		if err := server.Start(); err != nil {
			logger.Error("Failed to start server: %s", err)
			os.Exit(1)
		}

		defer func() {
			<-sigChan
		}()
	},
}

func shutdown(sigChan chan os.Signal, logger logging.Logger, server application.Server) {
	sig := <-sigChan
	logger.Info("Signal %s received, shutting down server...", sig)
	server.Close()
	close(sigChan)
}
