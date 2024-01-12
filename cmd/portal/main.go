package main

import "go.uber.org/zap"

func main() {
	portal := NewPortal()
	err := portal.Initialize()
	if err != nil {
		portal.Logger().Fatal("Failed to initialize portal", zap.Error(err))
	}
	portal.Run()
}
