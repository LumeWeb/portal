package portalcmd

import (
	"go.lumeweb.com/portal"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"syscall"
)

// exitProcessFromSignal exits the process from a system signal.
func exitProcessFromSignal(sigName string) {
	ctx := portal.Context()
	logger := ctx.Logger().With(zap.String("signal", sigName))
	exitProcess(logger)
}

func exitProcess(logger *zap.Logger) {
	portal.Shutdown(portal.ActivePortal(), logger)
}

func trapSignals() {
	ctx := portal.Context()
	logger := ctx.Logger()
	go func() {
		sigchan := make(chan os.Signal, 1)
		signal.Notify(sigchan, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGUSR1, syscall.SIGUSR2)

		for sig := range sigchan {
			switch sig {
			case syscall.SIGQUIT:
				logger.Info("quitting process immediately", zap.String("signal", "SIGQUIT"))
				os.Exit(core.ExitCodeForceQuit)

			case syscall.SIGTERM:
				logger.Info("shutting down apps, then terminating", zap.String("signal", "SIGTERM"))
				exitProcessFromSignal("SIGTERM")

			case syscall.SIGUSR1:
				logger.Info("not implemented", zap.String("signal", "SIGUSR1"))

			case syscall.SIGUSR2:
				logger.Info("not implemented", zap.String("signal", "SIGUSR2"))

			case syscall.SIGHUP:
				// ignore; this signal is sometimes sent outside of the user's control
				logger.Info("not implemented", zap.String("signal", "SIGHUP"))
			}
		}

	}()
}
