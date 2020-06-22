// Copyright 2012-2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"github.com/juju/cmd"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/worker/logsender"
)

type unitCommand struct {
	cmd.CommandBase
}

func New(ctx *cmd.Context, bufferedLogger *logsender.BufferedLogWriter) cmd.Command {
	return &unitCommand{}
}

// Info returns a description of the command.
func (c *unitCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "unit",
		Purpose: "starting a k8s agent",
	})
}

func (c *unitCommand) Run(ctx *cmd.Context) error {
	ctx.Infof("starting k8sagent unit command")
	return nil
}
