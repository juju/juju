// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The support package contains the implementation of the juju sla
// command.
package support

import (
	"encoding/json"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	api "github.com/juju/romulus/api/support"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/cmd/modelcmd"
)

// authorizationClient defines the interface of an api client that
// the command uses to create an sla authorization macaroon.
type authorizationClient interface {
	// Authorize returns the sla authorization macaroon for the specified model,
	Authorize(modelUUID, supportLevel string) (*macaroon.Macaroon, error)
}

var newAuthorizationClient = func(options ...api.ClientOption) (authorizationClient, error) {
	return api.NewSupportAuthClient(options...)
}

// NewSupportCommand returns a new command that is used to set sla credentials for a
// deployed application.
func NewSupportCommand() cmd.Command {
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
    juju sla essential # Set the support level to essential
    juju sla standard --budget 1000 set the support level to essential witha maximum budget of $1000
`,
	}
}

// Init implements cmd.Command.
func (c *supportCommand) Init(args []string) error {
	// TODO if 0 we could just show the current level.
	if len(args) < 1 {
		return errors.New("need to specify support level")
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
	m, err := authClient.Authorize(modelUUID, c.Level)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ms := macaroon.Slice{m}
	return json.Marshal(ms)
}

// Run implements cmd.Command.
func (c *supportCommand) Run(ctx *cmd.Context) error {
	root, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	modelTag, ok := root.ModelTag()
	if !ok {
		return errors.Errorf("failed to obtain model uuid")
	}
	client := modelconfig.NewClient(root)
	credentials, err := c.requestSupportCredentials(modelTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	// TODO Needs to be set model credentials
	err = client.SetSupport(c.Level, credentials)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
