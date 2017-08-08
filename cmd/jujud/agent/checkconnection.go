// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/worker/apicaller"
)

// ConnectFunc describes what we need to check whether the connection
// details are set up correctly for the specified agent.
type ConnectFunc func(agent.Agent) (io.Closer, error)

// ReallyConnect really connects to the API specified in the agent
// config. It's extracted so tests can pass something else in.
func ReallyConnect(a agent.Agent) (io.Closer, error) {
	return apicaller.ScaryConnect(a, api.Open)
}

type checkConnectionCommand struct {
	cmd.CommandBase
	agentName string
	config    AgentConf
	connect   ConnectFunc
}

// NewCheckConnectionCommand returns a command that will test
// connecting to the API with details from the agent's config.
func NewCheckConnectionCommand(config AgentConf, connect ConnectFunc) cmd.Command {
	return &checkConnectionCommand{
		config:  config,
		connect: connect,
	}
}

// Info is part of cmd.Command.
func (c *checkConnectionCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "check-connection",
		Args:    "<agent-name>",
		Purpose: "check connection to the API server",
	}
}

// Init is part of cmd.Command.
func (c *checkConnectionCommand) Init(args []string) error {
	if len(args) == 0 {
		return &util.FatalError{"agent-name argument is required"}
	}
	agentName, args := args[0], args[1:]
	tag, err := names.ParseTag(agentName)
	if err != nil {
		return errors.Annotatef(err, "agent-name")
	}
	if tag.Kind() != "machine" && tag.Kind() != "unit" {
		return &util.FatalError{"agent-name must be a machine or unit tag"}
	}
	if err := cmd.CheckEmpty(args); err != nil {
		return err
	}
	err = c.config.ReadConfig(agentName)
	if err != nil {
		return errors.Trace(err)
	}
	c.agentName = agentName
	return nil
}

// Run is part of cmd.Command.
func (c *checkConnectionCommand) Run(ctx *cmd.Context) error {
	conn, err := c.connect(c.config)
	if err != nil {
		return errors.Annotatef(err, "checking connection for %s", c.agentName)
	}
	err = conn.Close()
	if err != nil {
		return errors.Annotatef(err, "closing connection for %s", c.agentName)
	}
	return nil
}
