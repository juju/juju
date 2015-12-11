// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/resource"
)

// CharmStore has the charm store API methods needed by ShowCommand.
type CharmStore interface {
	// ListResources lists the resources for each of the given charms.
	ListResources(charms []string) ([][]resource.Info, error)

	// Close closes the client.
	Close() error
}

// ShowCommand implements the show-resources command.
type ShowCommand struct {
	envcmd.EnvCommandBase
	out     cmd.Output
	charmID string

	newAPIClient func(c *ShowCommand) (CharmStore, error)
}

// NewShowCommand returns a new command that lists resources defined
// by a charm.
func NewShowCommand(newAPIClient func(c *ShowCommand) (CharmStore, error)) *ShowCommand {
	cmd := &ShowCommand{
		newAPIClient: newAPIClient,
	}
	return cmd
}

var showDoc = `
This command will report the resources for a charm in the charm store.
`

// Info implements cmd.Command.
func (c *ShowCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show-resources",
		Args:    "charm-id",
		Purpose: "display the resources for a charm in the charm store",
		Doc:     showDoc,
	}
}

// SetFlags implements cmd.Command.
func (c *ShowCommand) SetFlags(f *gnuflag.FlagSet) {
	defaultFormat := "tabular"
	c.out.AddFlags(f, defaultFormat, map[string]cmd.Formatter{
		"tabular": FormatTabular,
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
	})
}

// Init implements cmd.Command.
func (c *ShowCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("missing charm ID")
	}
	c.charmID = args[0]

	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// Run implements cmd.Command.
func (c *ShowCommand) Run(ctx *cmd.Context) error {
	// TODO(ericsnow) Adjust this to the charm store.

	apiclient, err := c.newAPIClient(c)
	if err != nil {
		// TODO(ericsnow) Return a more user-friendly error?
		return errors.Trace(err)
	}
	defer apiclient.Close()

	charms := []string{c.charmID}
	infos, err := apiclient.ListResources(charms)
	if err != nil {
		return errors.Trace(err)
	}
	if len(infos) != 1 {
		return errors.New("got bad data from charm store")
	}

	// Note that we do not worry about c.CompatVersion for show-resources...
	formatter := newInfoListFormatter(infos[0])
	formatted := formatter.format()
	return c.out.Write(ctx, formatted)
}
