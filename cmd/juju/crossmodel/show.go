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
Show extended information about an offered application.

This command is aimed for a user who wants to see more detail about what’s offered behind a particular URL.

options:
-o, --output (= "")
   specify an output file
--format (= tabular)
   specify output format (tabular|json|yaml)

Examples:
   $ juju show-endpoints fred/prod.db2
   $ juju show-endpoints anothercontroller:fred/prod.db2

See also:
   find-endpoints
`

type showCommand struct {
	RemoteEndpointsCommandBase

	url        string
	out        cmd.Output
	newAPIFunc func(string) (ShowAPI, error)
}

// NewShowOfferedEndpointCommand constructs command that
// allows to show details of offered application's endpoint.
func NewShowOfferedEndpointCommand() cmd.Command {
	showCmd := &showCommand{}
	showCmd.newAPIFunc = func(controllerName string) (ShowAPI, error) {
		return showCmd.NewRemoteEndpointsAPI(controllerName)
	}
	return modelcmd.WrapController(showCmd)
}

// Init implements Command.Init.
func (c *showCommand) Init(args []string) (err error) {
	if len(args) != 1 {
		return errors.New("must specify endpoint URL")
	}
	c.url = args[0]
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
	c.RemoteEndpointsCommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatShowTabular,
	})
}

// Run implements Command.Run.
func (c *showCommand) Run(ctx *cmd.Context) (err error) {
	url, err := crossmodel.ParseApplicationURL(c.url)
	if err != nil {
		return err
	}
	controllerName := url.Source
	if controllerName == "" {
		controllerName, err = c.ControllerName()
		if err != nil {
			return err
		}
	}
	api, err := c.newAPIFunc(controllerName)
	if err != nil {
		return err
	}
	defer api.Close()

	url.Source = ""
	found, err := api.ApplicationOffer(url.String())
	if err != nil {
		return err
	}

	output, err := convertOffers(controllerName, found)
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

// ShowOfferedApplication defines the serialization behaviour of an application offer.
// This is used in map-style yaml output where remote application name is the key.
type ShowOfferedApplication struct {
	// Access is the level of access the user has on the offer.
	Access string `yaml:"access" json:"access"`

	// Endpoints list of offered application endpoints.
	Endpoints map[string]RemoteEndpoint `yaml:"endpoints" json:"endpoints"`

	// Description is the user entered description.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// convertOffers takes any number of api-formatted remote applications and
// creates a collection of ui-formatted offers.
func convertOffers(store string, offers ...params.ApplicationOffer) (map[string]ShowOfferedApplication, error) {
	if len(offers) == 0 {
		return nil, nil
	}
	output := make(map[string]ShowOfferedApplication, len(offers))
	for _, one := range offers {
		app := ShowOfferedApplication{
			Access:    one.Access,
			Endpoints: convertRemoteEndpoints(one.Endpoints...),
		}
		if one.ApplicationDescription != "" {
			app.Description = one.ApplicationDescription
		}
		url, err := crossmodel.ParseApplicationURL(one.OfferURL)
		if err != nil {
			return nil, err
		}
		if url.Source == "" {
			url.Source = store
		}
		output[url.String()] = app
	}
	return output, nil
}
