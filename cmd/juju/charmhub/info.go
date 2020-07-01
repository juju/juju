// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/api/charmhub"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

const (
	infoSummary = "Displays detailed information about charm hub charms."
	infoDoc     = `
The charm can be specified by name or by path. Names are looked for both in the
store and in the deployed charms.

Examples:
    juju info postgresql

See also:
    find
`
)

// NewInfoCommand wraps infoCommand with sane model settings.
func NewInfoCommand() cmd.Command {
	return modelcmd.Wrap(&infoCommand{})
}

// infoCommand supplies the "info" CLI command used to display info
// about charm snaps.
type infoCommand struct {
	modelcmd.ModelCommandBase

	api InfoCommandAPI

	verbose       bool
	charmOrBundle string
}

// Info returns help related info about the command, it implements
// part of the cmd.Command interface.
func (c *infoCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "info",
		Args:    "[options] <charm>",
		Purpose: infoSummary,
		Doc:     infoDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags defines flags which can be used with the info command.
// It implements part of the cmd.Command interface.
func (c *infoCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	// TODO (hml)
	// add --config
}

// Init initializes the info command, including validating the provided
// flags. It implements part of the cmd.Command interface.
func (c *infoCommand) Init(args []string) error {
	if len(args) != 1 {
		return errors.Errorf("expected a charm or bundle name")
	}
	if err := c.validateCharmOrBundle(args[0]); err != nil {
		return err
	}
	c.charmOrBundle = args[0]
	return nil
}

// Run is the business logic of the info command.  It implements the meaty
// part of the cmd.Command interface.
func (c *infoCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	info, err := client.Info(c.charmOrBundle)
	if err != nil {
		return err
	}

	return makeInfoWriter(ctx, &info).Print()
}

func (c *infoCommand) validateCharmOrBundle(_ string) error {
	// TODO:
	// Implement validation of charm hub charm names.  Exit for
	// charmstore and local charms.
	return nil
}

// getAPI returns the API that supplies methods
// required to execute this command.
func (c *infoCommand) getAPI() (InfoCommandAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	client := charmhub.NewClient(api)
	return client, nil
}
