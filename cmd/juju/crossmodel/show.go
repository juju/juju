// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
)

const showCommandDoc = `
Show extended information about an exported service.

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
// allows to show details of offered application's endpoint.
func NewShowOfferedEndpointCommand() cmd.Command {
	showCmd := &showCommand{}
	showCmd.newAPIFunc = func() (ShowAPI, error) {
		return showCmd.NewCrossModelAPI()
	}
	return modelcmd.WrapController(showCmd)
}

// Init implements Command.Init.
func (c *showCommand) Init(args []string) (err error) {
	if len(args) != 1 {
		return errors.New("must specify endpoint URL")
	}

	url := args[0]
	if _, err := crossmodel.ParseApplicationURL(url); err != nil {
		return err
	}
	c.url = url
	return nil
}

// Info implements Command.Info.
func (c *showCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show-endpoints",
		Purpose: "Shows offered applications' endpoints details",
		Doc:     showCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *showCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CrossModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatShowTabular,
	})
}

// Run implements Command.Run.
func (c *showCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	found, err := api.ApplicationOffer(c.url)
	if err != nil {
		return err
	}

	output, err := convertRemoteApplications(found)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, output)
}

// ShowAPI defines the API methods that cross model show command uses.
type ShowAPI interface {
	Close() error
	ApplicationOffer(url string) (params.ApplicationOffer, error)
}

// ShowRemoteApplication defines the serialization behaviour of remote application.
// This is used in map-style yaml output where remote application name is the key.
type ShowRemoteApplication struct {
	// Endpoints list of offered application endpoints.
	Endpoints map[string]RemoteEndpoint `yaml:"endpoints" json:"endpoints"`

	// Description is the user entered description.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// convertRemoteApplications takes any number of api-formatted remote applications and
// creates a collection of ui-formatted services.
func convertRemoteApplications(services ...params.ApplicationOffer) (map[string]ShowRemoteApplication, error) {
	if len(services) == 0 {
		return nil, nil
	}
	output := make(map[string]ShowRemoteApplication, len(services))
	for _, one := range services {
		service := ShowRemoteApplication{Endpoints: convertRemoteEndpoints(one.Endpoints...)}
		if one.ApplicationDescription != "" {
			service.Description = one.ApplicationDescription
		}
		output[one.ApplicationName] = service
	}
	return output, nil
}
