// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/model"
)

const findCommandDoc = `
Search for offers you can see. Without a URL term, the search covers all
models on the selected controller, not just the current model. Use a
controller name followed by a colon to search another controller, or give
a model to limit the search to that model.

Use ` + "`--offer`" + ` without a URL term to search by part of an offer name.
With a positional URL term, its offer-name component replaces ` + "`--offer`" + `,
even when that component is empty. To search by name in a particular
model, include the name in the URL term. If nothing matches, the command
returns an error.

The table lists interfaces as interface:endpoint pairs. Finding an offer
does not by itself mean you have permission to consume it.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/offer/
`

const findCommandExamples = `
Find offers visible to you on the current controller:

    juju find-offers

Find offers visible to you on the controller named ` + "`mycontroller`" + `:

    juju find-offers mycontroller:

Find visible offers in model ` + "`staging/mymodel`" + `:

    juju find-offers staging/mymodel

Find visible offers with an endpoint that uses the ` + "`mysql`" + ` interface:

    juju find-offers --interface mysql

Find visible offers whose names contain ` + "`db2`" + ` in model ` + "`staging/mymodel`" + ` on ` + "`mycontroller`" + `:

    juju find-offers mycontroller:staging/mymodel.db2

Find visible offers whose names contain ` + "`db2`" + ` on the current controller:

    juju find-offers --offer db2

`

type findCommand struct {
	RemoteEndpointsCommandBase

	url            string
	source         string
	modelQualifier string
	modelName      string
	offerName      string
	interfaceName  string

	out        cmd.Output
	newAPIFunc func(context.Context, string) (FindAPI, error)
}

// NewFindEndpointsCommand constructs command that
// allows to find offered application endpoints.
func NewFindEndpointsCommand() cmd.Command {
	findCmd := &findCommand{}
	findCmd.newAPIFunc = func(ctx context.Context, controllerName string) (FindAPI, error) {
		return findCmd.NewRemoteEndpointsAPI(ctx, controllerName)
	}
	return modelcmd.Wrap(findCmd)
}

// Init implements Command.Init.
func (c *findCommand) Init(args []string) (err error) {
	if c.offerName != "" && c.url != "" {
		return errors.New("cannot specify both a URL term and offer term")
	}
	url, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return errors.Trace(err)
	}
	if url != "" {
		if c.url != "" {
			return errors.New("URL term cannot be specified twice")
		}
		c.url = url
	}
	return nil
}

// Info implements Command.Info.
func (c *findCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "find-offers",
		Args:     "[<offer-url-term>]",
		Purpose:  "Find offers visible to you on a controller.",
		Doc:      findCommandDoc,
		Examples: findCommandExamples,
		SeeAlso: []string{
			"show-offer",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *findCommand) SetFlags(f *gnuflag.FlagSet) {
	c.RemoteEndpointsCommandBase.SetFlags(f)
	f.StringVar(&c.url, "url", "", "Specify the URL term instead of a positional argument; cannot be used with --offer")
	f.StringVar(&c.interfaceName, "interface", "", "Return results matching the interface name")
	f.StringVar(&c.offerName, "offer", "", "Match offer names containing this text")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatFindTabular,
	})
}

// Run implements Command.Run.
func (c *findCommand) Run(ctx *cmd.Context) (err error) {
	if err := c.validateOrSetURL(); err != nil {
		return errors.Trace(err)
	}
	accountDetails, err := c.CurrentAccountDetails()
	if err != nil {
		return err
	}
	loggedInUser := accountDetails.User

	api, err := c.newAPIFunc(ctx, c.source)
	if err != nil {
		return err
	}
	defer api.Close()

	filter := crossmodel.ApplicationOfferFilter{
		ModelQualifier: model.Qualifier(c.modelQualifier),
		ModelName:      c.modelName,
		OfferName:      c.offerName,
	}
	if c.interfaceName != "" {
		filter.Endpoints = []crossmodel.EndpointFilterTerm{{
			Interface: c.interfaceName,
		}}
	}
	found, err := api.FindApplicationOffers(ctx, filter)
	if err != nil {
		return err
	}

	output, err := convertFoundOffers(c.source, names.NewUserTag(loggedInUser), found...)
	if err != nil {
		return err
	}
	if len(output) == 0 {
		return errors.New("no matching application offers found")
	}
	return c.out.Write(ctx, output)
}

func (c *findCommand) validateOrSetURL() error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	if c.url == "" {
		c.url = controllerName + ":"
		c.source = controllerName
		return nil
	}
	urlParts, err := crossmodel.ParseOfferURLParts(c.url)
	if err != nil {
		return errors.Trace(err)
	}
	if urlParts.Source != "" {
		c.source = urlParts.Source
	} else {
		c.source = controllerName
	}
	qualifier := urlParts.ModelQualifier
	if qualifier == "" {
		accountDetails, err := c.CurrentAccountDetails()
		if err != nil {
			return errors.Trace(err)
		}
		qualifier = accountDetails.User
	}
	c.modelQualifier = qualifier
	c.modelName = urlParts.ModelName
	c.offerName = urlParts.Name
	return nil
}

// FindAPI defines the API methods that cross model find command uses.
type FindAPI interface {
	Close() error
	FindApplicationOffers(ctx context.Context, filters ...crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error)
}

// ApplicationOfferResult defines the serialization behaviour of an application offer.
// This is used in map-style yaml output where offer URL is the key.
type ApplicationOfferResult struct {
	// Access is the level of access the user has on the offer.
	Access string `yaml:"access" json:"access"`

	// Endpoints is the list of offered application endpoints.
	Endpoints map[string]RemoteEndpoint `yaml:"endpoints" json:"endpoints"`

	// Users are the users who can access the offer.
	Users map[string]OfferUser `yaml:"users,omitempty" json:"users,omitempty"`
}

func accessForUser(user names.UserTag, users []crossmodel.OfferUserDetails) string {
	for _, u := range users {
		if u.UserName == user.Id() {
			return string(u.Access)
		}
	}
	return "-"
}

// convertFoundOffers takes any number of api-formatted remote applications and
// creates a collection of ui-formatted applications.
func convertFoundOffers(
	store string, loggedInUser names.UserTag, offers ...*crossmodel.ApplicationOfferDetails,
) (map[string]ApplicationOfferResult, error) {
	if len(offers) == 0 {
		return nil, nil
	}
	output := make(map[string]ApplicationOfferResult, len(offers))
	for _, one := range offers {
		access := accessForUser(loggedInUser, one.Users)
		app := ApplicationOfferResult{
			Access:    access,
			Endpoints: convertRemoteEndpoints(one.Endpoints...),
			Users:     convertUsers(one.Users...),
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
