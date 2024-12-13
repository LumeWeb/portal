package main

import (
	"context"
	"go.lumeweb.com/portal/cmd/portal/cli"
	_ "go.lumeweb.com/portal/service"
	"os"
)

func main() {
	command := cli.NewPortalCLI()
	ctx := context.Background()
	if err := command.Run(ctx, os.Args); err != nil {
		os.Exit(1)
	}
}
