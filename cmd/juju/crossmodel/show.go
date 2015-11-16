// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/model/crossmodel"
)

const showCommandDoc = `
Show extended information about service's endpoints previously exported through "juju offer".

This command is aimed for a user who wants to see more detail about what’s offered behind a particular URL.

options:
-o, --output (= "")
   specify an output file
--format (= tabular)
   specify output format (tabular|json|yaml)

Examples:
   $ juju show-endpoints local:/u/fred/prod/db2
   $ juju show-endpoints vendor:/u/ibm/hosted-db2
`

type showCommand struct {
	CrossModelCommandBase

	url        string
	out        cmd.Output
	newAPIFunc func() (ShowAPI, error)
}

// NewShowOfferedEndpointCommand constructs command that
// allows to show details of offered service's endpoint.
func NewShowOfferedEndpointCommand() cmd.Command {
	showCmd := &showCommand{}
	showCmd.newAPIFunc = func() (ShowAPI, error) {
		return showCmd.NewCrossModelAPI()
	}
	return envcmd.Wrap(showCmd)
}

// Init implements Command.Init.
func (c *showCommand) Init(args []string) (err error) {
	if len(args) != 1 {
		return errors.New("must specify endpoint URL")
	}

	url := args[0]
	if _, err := crossmodel.ParseServiceURL(url); err != nil {
		return err
	}
	c.url = url
	return nil
}

// Info implements Command.Info.
func (c *showCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show-endpoints",
		Purpose: "shows offered services' endpoints details",
		Doc:     showCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *showCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CrossModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatTabular,
	})
}

// Run implements Command.Run.
func (c *showCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	found, err := api.Show(c.url)
	if err != nil {
		return err
	}

	output, err := convertRemoteServices(found)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, output)
}

// ShowAPI defines the API methods that cross model show command uses.
type ShowAPI interface {
	Close() error
	Show(url string) (params.ServiceOffer, error)
}
