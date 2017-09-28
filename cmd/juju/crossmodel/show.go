// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

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
		Purpose: "Shows offered applications' endpoints details.",
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
	url, err := crossmodel.ParseOfferURL(c.url)
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
	accountDetails, err := c.CurrentAccountDetails()
	if err != nil {
		return err
	}
	loggedInUser := accountDetails.User

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

	output, err := convertOffers(controllerName, names.NewUserTag(loggedInUser), found)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, output)
}

// ShowAPI defines the API methods that cross model show command uses.
type ShowAPI interface {
	Close() error
	ApplicationOffer(url string) (*crossmodel.ApplicationOfferDetails, error)
}

type OfferUser struct {
	UserName    string `yaml:"-" json:"-"`
	DisplayName string `yaml:"display-name,omitempty" json:"display-name,omitempty"`
	Access      string `yaml:"access" json:"access"`
}

// ShowOfferedApplication defines the serialization behaviour of an application offer.
// This is used in map-style yaml output where remote application name is the key.
type ShowOfferedApplication struct {
	// Description is the user entered description.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Access is the level of access the user has on the offer.
	Access string `yaml:"access" json:"access"`

	// Endpoints list of offered application endpoints.
	Endpoints map[string]RemoteEndpoint `yaml:"endpoints" json:"endpoints"`

	// Users are the users who can access the offer.
	Users map[string]OfferUser `yaml:"users,omitempty" json:"users,omitempty"`
}

// convertOffers takes any number of api-formatted remote applications and
// creates a collection of ui-formatted offers.
func convertOffers(
	store string, loggedInUser names.UserTag, offers ...*crossmodel.ApplicationOfferDetails,
) (map[string]ShowOfferedApplication, error) {
	if len(offers) == 0 {
		return nil, nil
	}
	output := make(map[string]ShowOfferedApplication, len(offers))
	for _, one := range offers {
		access := accessForUser(loggedInUser, one.Users)
		app := ShowOfferedApplication{
			Access:    access,
			Endpoints: convertRemoteEndpoints(one.Endpoints...),
			Users:     convertUsers(one.Users...),
		}
		if one.ApplicationDescription != "" {
			app.Description = one.ApplicationDescription
		}
		url, err := crossmodel.ParseOfferURL(one.OfferURL)
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

func convertUsers(users ...crossmodel.OfferUserDetails) map[string]OfferUser {
	if len(users) == 0 {
		return nil
	}
	output := make(map[string]OfferUser, len(users))
	for _, one := range users {
		output[one.UserName] = OfferUser{one.UserName, one.DisplayName, string(one.Access)}
	}
	return output
}
