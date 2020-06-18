// Copyright 2012-2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package init

import (
	"github.com/juju/cmd"
	"github.com/juju/loggo"
	"github.com/juju/os/series"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/paths"
)

var (
	logger         = loggo.GetLogger("juju.cmd.k8sagent.init")
	jujuRun        = paths.MustSucceed(paths.JujuRun(series.MustHostSeries()))
	jujuDumpLogs   = paths.MustSucceed(paths.JujuDumpLogs(series.MustHostSeries()))
	jujuIntrospect = paths.MustSucceed(paths.JujuIntrospect(series.MustHostSeries()))

	// TODO(ycliuhw): ensure below symlinks with hooktool symlinks(->jujuc) together in init subcommand of k8sagent.
	k8sAgentSymlinks = []string{jujuRun, jujuDumpLogs, jujuIntrospect}
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

func (c *initCommand) Run(_ *cmd.Context) error {
	logger.Infof("starting k8sagent init command")
	return nil
}
