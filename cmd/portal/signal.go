package main

import (
	"github.com/LumeWeb/portal"
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
	exitCode := exitCodeSuccess

	if err := portal.Stop(); err != nil {
		logger.Error("failed to stop portal", zap.Error(err))
		exitCode = exitCodeFailedQuit
	}

	ctx := portal.Context()
	for _, exitFunc := range ctx.ExitFuncs() {
		err := exitFunc(ctx)
		if err != nil {
			logger.Error("error during exit", zap.Error(err))
			exitCode = exitCodeFailedQuit
		}
	}

	go func() {
		defer func() {
			logger = logger.With(zap.Int("exit_code", exitCode))
			if exitCode == exitCodeSuccess {
				logger.Info("shutdown complete")
			} else {
				logger.Error("unclean shutdown")
			}
			os.Exit(exitCode)
		}()
	}()
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
				os.Exit(exitCodeForceQuit)

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

// Exit codes. Generally, you should NOT
// automatically restart the process if the
// exit code is ExitCodeFailedStartup (1).
const (
	exitCodeSuccess = iota
	exitCodeFailedStartup
	exitCodeForceQuit
	exitCodeFailedQuit
)
