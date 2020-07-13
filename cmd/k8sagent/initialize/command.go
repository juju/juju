// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initialize

import (
	"github.com/juju/cmd"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/k8sagent/config"
)

var (
	// TODO(ycliuhw): ensure below symlinks with hooktool symlinks(->jujuc) together in init subcommand of k8sagent.
	k8sAgentSymlinks = []string{config.JujuRun, config.JujuDumpLogs, config.JujuIntrospect}
	// TODO(ycliuhw): prepare paths, agent.conf etc(what caas operator has been done).
)

type initCommand struct {
	cmd.CommandBase
}

func New() cmd.Command {
	return &initCommand{}
}

// Info returns a description of the command.
func (c *initCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "init",
		Purpose: "initialize k8sagent state",
	})
}

func (c *initCommand) Run(ctx *cmd.Context) error {
	ctx.Infof("starting k8sagent init command")
	return nil
}
