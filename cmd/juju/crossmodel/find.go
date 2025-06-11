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

const findCommandDoc = `
Find which offered application endpoints are available to the current user.

This command is aimed for a user who wants to discover what endpoints are available to them.
`

const findCommandExamples = `
    juju find-offers
    juju find-offers mycontroller:
    juju find-offers staging/mymodel
    juju find-offers --interface mysql
    juju find-offers --url staging/mymodel.db2
    juju find-offers --offer db2
   
`

type findCommand struct {
	RemoteEndpointsCommandBase

	url            string
	source         string
	modelQualifier model.Qualifier
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
		Purpose:  "Find offered application endpoints.",
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
	f.StringVar(&c.url, "url", "", "return results matching the offer URL")
	f.StringVar(&c.interfaceName, "interface", "", "return results matching the interface name")
	f.StringVar(&c.offerName, "offer", "", "return results matching the offer name")
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
		ModelQualifier: c.modelQualifier,
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
	qualifier := model.Qualifier(urlParts.ModelQualifier)
	if qualifier == "" {
		accountDetails, err := c.CurrentAccountDetails()
		if err != nil {
			return errors.Trace(err)
		}
		qualifier = model.QualifierFromUserTag(names.NewUserTag(accountDetails.User))
	}
	c.modelQualifier = qualifier
	c.modelName = urlParts.ModelName
	c.offerName = urlParts.ApplicationName
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
