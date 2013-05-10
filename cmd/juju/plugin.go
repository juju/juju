package main

import (
	"fmt"

	"launchpad.net/juju-core/cmd"
)

func RunPlugin(ctx *cmd.Context, subcommand string, args []string) error {
	return fmt.Errorf("unrecognized command: juju %s", subcommand)
}
