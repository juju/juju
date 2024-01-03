// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"io"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"

	"github.com/juju/juju/agent"
	agenterrors "github.com/juju/juju/agent/errors"
	"github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/internal/worker/apicaller"
)

// ConnectFunc connects to the API as the given agent.
type ConnectFunc func(agent.Agent) (io.Closer, error)

// ConnectAsAgent really connects to the API specified in the agent
// config. It's extracted so tests can pass something else in.
func ConnectAsAgent(a agent.Agent) (io.Closer, error) {
	return apicaller.ScaryConnect(a, api.Open, loggo.GetLogger("juju.agent"))
}

type checkConnectionCommand struct {
	cmd.CommandBase
	agentName string
	config    agentconf.AgentConf
	connect   ConnectFunc
}

// NewCheckConnectionCommand returns a command that will test
// connecting to the API with details from the agent's config.
func NewCheckConnectionCommand(config agentconf.AgentConf, connect ConnectFunc) cmd.Command {
	return &checkConnectionCommand{
		config:  config,
		connect: connect,
	}
}

// Info is part of cmd.Command.
func (c *checkConnectionCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "check-connection",
		Args:    "<agent-name>",
		Purpose: "check connection to the API server for the specified agent",
	})
}

// Init is part of cmd.Command.
func (c *checkConnectionCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("agent-name argument is required%w", errors.Hide(agenterrors.FatalError))
	}
	agentName, args := args[0], args[1:]
	if err := cmd.CheckEmpty(args); err != nil {
		return err
	}
	tag, err := names.ParseTag(agentName)
	if err != nil {
		return errors.Annotatef(err, "agent-name")
	}
	if tag.Kind() != "machine" && tag.Kind() != "unit" {
		return fmt.Errorf("agent-name must be a machine or unit tag%w", errors.Hide(agenterrors.FatalError))
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
