// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd"
)

const showCommandDoc = `
This command is intended to enable users to learn more about the
application offered from a particular URL. In addition to the URL of
the offer, extra information is provided from the readme file of the
charm being offered.
`

const showCommandExamples = `
To show the extended information for the application 'prod' offered
from the model 'default' on the same Juju controller:

     juju show-offer default.prod

The supplied URL can also include a model qualifier where offers require them. 
This will be given as part of the URL retrieved from the
'juju find-offers' command. To show information for the application
'prod' from the model 'staging/default':

    juju show-offer staging/default.prod

To show the information regarding the application 'prod' offered from
the model 'default' on an accessible controller named 'controller':

    juju show-offer controller:default.prod

`

type showCommand struct {
	RemoteEndpointsCommandBase

	url        string
	out        cmd.Output
	newAPIFunc func(context.Context, string) (ShowAPI, error)
}

// NewShowOfferedEndpointCommand constructs command that
// allows to show details of offered application's endpoint.
func NewShowOfferedEndpointCommand() cmd.Command {
	showCmd := &showCommand{}
	showCmd.newAPIFunc = func(ctx context.Context, controllerName string) (ShowAPI, error) {
		return showCmd.NewRemoteEndpointsAPI(ctx, controllerName)
	}
	return modelcmd.Wrap(showCmd)
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
	return jujucmd.Info(&cmd.Info{
		Name:     "show-offer",
		Args:     "[<controller>:]<offer url>",
		Purpose:  "Shows extended information about the offered application.",
		Doc:      showCommandDoc,
		Examples: showCommandExamples,
		SeeAlso: []string{
			"find-offers",
		},
	})
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
	controllerName, err := c.ControllerName()
	if err != nil {
		return err
	}
	url, err := crossmodel.ParseOfferURL(c.url)
	if err != nil {
		if !names.IsValidApplication(c.url) {
			return errors.Trace(err)
		}
		url = &crossmodel.OfferURL{
			ApplicationName: c.url,
		}
	}
	accountDetails, err := c.CurrentAccountDetails()
	if err != nil {
		return err
	}
	loggedInUser := accountDetails.User

	if url.ModelName == "" {
		modelName, qualifier, err := c.ModelNameWithQualifier()
		if err != nil {
			return errors.Trace(err)
		}
		url.ModelQualifier = qualifier
		url.ModelName = modelName
	}

	if url.ModelQualifier == "" {
		qualifier := model.QualifierFromUserTag(names.NewUserTag(accountDetails.User))
		url.ModelQualifier = qualifier.String()
	}
	if url.Source != "" {
		controllerName = url.Source
	}

	api, err := c.newAPIFunc(ctx, controllerName)
	if err != nil {
		return err
	}
	defer api.Close()

	url.Source = ""
	found, err := api.ApplicationOffer(ctx, url.String())
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
	ApplicationOffer(ctx context.Context, url string) (*crossmodel.ApplicationOfferDetails, error)
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
