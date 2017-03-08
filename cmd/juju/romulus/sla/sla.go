// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The sla package contains the implementation of the juju sla
// command.
package sla

import (
	"encoding/json"
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	api "github.com/juju/romulus/api/sla"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/cmd/modelcmd"
)

// authorizationClient defines the interface of an api client that
// the command uses to create an sla authorization macaroon.
type authorizationClient interface {
	// Authorize returns the sla authorization macaroon for the specified model,
	Authorize(modelUUID, supportLevel, budget string) (*macaroon.Macaroon, error)
}

var newAuthorizationClient = func(options ...api.ClientOption) (authorizationClient, error) {
	return api.NewClient(options...)
}

// NewSLACommand returns a new command that is used to set sla credentials for a
// deployed application.
func NewSLACommand() cmd.Command {
	return modelcmd.Wrap(&supportCommand{})
}

// supportCommand is a command-line tool for setting
// Model.SLACredential for development & demonstration purposes.
type supportCommand struct {
	modelcmd.ModelCommandBase

	Level  string
	Budget string
}

// SetFlags sets additional flags for the support command.
func (c *supportCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.Budget, "budget", "", "the maximum spend for the model")
	// TODO set the budget
}

// Info implements cmd.Command.
func (c *supportCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "sla",
		Aliases: []string{"support"},
		Args:    "<level>",
		Purpose: "Set the support level for a model.",
		Doc: `
Set the support level for the model, effective immediately.
Examples:
    juju sla essential              # set the support level to essential
    juju sla standard --budget 1000 # set the support level to essential witha maximum budget of $1000
    juju sla                        # display the current support level for the model.
`,
	}
}

// Init implements cmd.Command.
func (c *supportCommand) Init(args []string) error {
	if len(args) < 1 {
		return nil
	}
	c.Level = args[0]
	return cmd.CheckEmpty(args[1:])
}

func (c *supportCommand) requestSupportCredentials(modelUUID string) ([]byte, error) {
	hc, err := c.BakeryClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	authClient, err := newAuthorizationClient(api.HTTPClient(hc))
	if err != nil {
		return nil, errors.Trace(err)
	}
	m, err := authClient.Authorize(modelUUID, c.Level, c.Budget)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ms := macaroon.Slice{m}
	return json.Marshal(ms)
}

func displayCurrentLevel(client *modelconfig.Client, ctx *cmd.Context) error {
	level, err := client.SLALevel()
	if err != nil {
		return errors.Trace(err)
	}
	fmt.Fprintln(ctx.Stdout, level)
	return nil
}

// Run implements cmd.Command.
func (c *supportCommand) Run(ctx *cmd.Context) error {
	root, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	client := modelconfig.NewClient(root)

	if c.Level == "" {
		return displayCurrentLevel(client, ctx)
	}
	modelTag, ok := root.ModelTag()
	if !ok {
		return errors.Errorf("failed to obtain model uuid")
	}
	credentials, err := c.requestSupportCredentials(modelTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	err = client.SetSLALevel(c.Level, credentials)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
