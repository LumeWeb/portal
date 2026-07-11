package main

import (
	"context"
	"fmt"
	"go.lumeweb.com/portal/cmd/internal/cli"
	_ "go.lumeweb.com/portal/service"
	"os"
)

func main() {
	command := cli.NewPortalCLI()
	ctx := context.Background()
	if err := command.Run(ctx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
