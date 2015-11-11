// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/crossmodel"
)

const showCommandDoc = `
Show extended information about service's endpoints previously exported through "juju offer".

This command is aimed for a user who wants to see more detail about what’s offered behind a particular URL.
The details are shown if the user has read permission to the directory containing the endpoint.

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

	url string
	out cmd.Output
	api ShowAPI
}

// NewShowOfferedEndpointCommand constructs command that
// allows to show details of offered service's endpoint.
func NewShowOfferedEndpointCommand() cmd.Command {
	return envcmd.Wrap(&showCommand{})
}

// Init implements Command.Init.
func (c *showCommand) Init(args []string) (err error) {
	if len(args) != 1 {
		return errors.New("must specify endpoint URL")
	}

	url := args[0]
	if !crossmodel.IsValidURL(url) {
		return errors.NotValidf("endpoint url %q", url)
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
	if c.api == nil {
		api, err := getShowAPI(c)
		if err != nil {
			return err
		}
		defer api.Close()
		c.api = api
	}

	found, err := c.api.Show(c.url)
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
	Show(url string) (params.RemoteServiceInfo, error)
}

var getShowAPI = (*showCommand).getShowAPI

func (c *showCommand) getShowAPI() (ShowAPI, error) {
	return c.NewCrossModelAPI()
}
