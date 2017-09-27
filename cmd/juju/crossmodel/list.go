// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api/applicationoffers"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/jujuclient"
)

const listCommandDoc = `
List information about applications' endpoints that have been shared.

options:
-o, --output (= "")
   specify an output file
--format (= tabular)
   specify output format (json|tabular|yaml)
[[--<filter-scope> ]<filter-term>],...
   <filter-term> is free text and will be matched against any of:
       - endpoint URL prefix
       - interface name
       - application name
   <filter-scope> is optional and is used to limit the scope of the search using the search term, one of:
       - interface
       - application

Examples:
    $ juju offers
    $ juju offers -m model
    $ juju offers --interface db2
    $ juju offers --application mysql

    $ juju offers --interface db2
    mycontroller
    Application  Charm  Connected  Store         URL                     Endpoint  Interface  Role
    db2          db2    123        mycontroller  admin/controller.mysql  db        db2        provider

`

// listCommand returns storage instances.
type listCommand struct {
	modelcmd.ModelCommandBase

	out cmd.Output

	newAPIFunc    func() (ListAPI, error)
	refreshModels func(jujuclient.ClientStore, string) error

	interfaceName   string
	applicationName string
	filters         []crossmodel.ApplicationOfferFilter
}

// NewListEndpointsCommand constructs new list endpoint command.
func NewListEndpointsCommand() cmd.Command {
	listCmd := &listCommand{}
	listCmd.newAPIFunc = func() (ListAPI, error) {
		return listCmd.NewApplicationOffersAPI()
	}
	listCmd.refreshModels = listCmd.ModelCommandBase.RefreshModels
	return modelcmd.Wrap(listCmd)
}

// NewApplicationOffersAPI returns an application offers api for the root api endpoint
// that the command returns.
func (c *listCommand) NewApplicationOffersAPI() (*applicationoffers.Client, error) {
	root, err := c.NewControllerAPIRoot()
	if err != nil {
		return nil, err
	}
	return applicationoffers.NewClient(root), nil
}

// Init implements Command.Init.
func (c *listCommand) Init(args []string) (err error) {
	return cmd.CheckEmpty(args)
}

// Info implements Command.Info.
func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "offers",
		Aliases: []string{"list-offers"},
		Purpose: "Lists shared endpoints.",
		Doc:     listCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *listCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.applicationName, "application", "", "return results matching the application")
	f.StringVar(&c.interfaceName, "interface", "", "return results matching the interface name")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatListTabular,
		"summary": formatListSummary,
	})
}

// Run implements Command.Run.
func (c *listCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}

	modelName, _, err := c.ModelDetails()
	if err != nil {
		return errors.Trace(err)
	}
	if !jujuclient.IsQualifiedModelName(modelName) {
		store := modelcmd.QualifyingClientStore{c.ClientStore()}
		var err error
		modelName, err = store.QualifiedModelName(controllerName, modelName)
		if err != nil {
			return errors.Trace(err)
		}
	}

	unqualifiedModelName, ownerTag, err := jujuclient.SplitModelName(modelName)
	if err != nil {
		return errors.Trace(err)
	}
	c.filters = []crossmodel.ApplicationOfferFilter{{
		OwnerName:       ownerTag.Name(),
		ModelName:       unqualifiedModelName,
		ApplicationName: c.applicationName,
	}}
	if c.interfaceName != "" {
		c.filters[0].Endpoints = []crossmodel.EndpointFilterTerm{{
			Interface: c.interfaceName,
		}}
	}

	offeredApplications, err := api.ListOffers(c.filters...)
	if err != nil {
		return err
	}

	// Filter out valid output, if any...
	valid := []crossmodel.ApplicationOfferDetails{}
	for _, application := range offeredApplications {
		if application.Error != nil {
			fmt.Fprintf(ctx.Stderr, "%v\n", application.Error)
			continue
		}
		if application.Result != nil {
			valid = append(valid, *application.Result)
		}
	}
	if len(valid) == 0 {
		return nil
	}

	// For now, all offers come from the one controller.
	data, err := formatApplicationOfferDetails(controllerName, valid)
	if err != nil {
		return errors.Annotate(err, "failed to format found applications")
	}

	return c.out.Write(ctx, data)
}

// ListAPI defines the API methods that list endpoints command use.
type ListAPI interface {
	Close() error
	ListOffers(filters ...crossmodel.ApplicationOfferFilter) ([]crossmodel.ApplicationOfferDetailsResult, error)
}

// ListOfferItem defines the serialization behaviour of an offer item in endpoints list.
type ListOfferItem struct {
	// OfferName is the name of the offer.
	OfferName string `yaml:"-" json:"-"`

	// ApplicationName is the application backing this offer.
	ApplicationName string `yaml:"application" json:"application"`

	// Store is the controller hosting this offer.
	Source string `yaml:"store,omitempty" json:"store,omitempty"`

	// CharmURL is the charm URL of this application.
	CharmURL string `yaml:"charm,omitempty" json:"charm,omitempty"`

	// OfferURL is part of Juju location where this offer is shared relative to the store.
	OfferURL string `yaml:"offer-url" json:"offer-url"`

	// Endpoints is a list of application endpoints.
	Endpoints map[string]RemoteEndpoint `yaml:"endpoints" json:"endpoints"`

	// Connections holds details of connections to the offer.
	Connections []offerConnectionDetails `yaml:"connections,omitempty" json:"connections,omitempty"`
}

type offeredApplications map[string]ListOfferItem

type offerConnectionStatus struct {
	Current string `json:"current" yaml:"current"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
	Since   string `json:"since,omitempty" yaml:"since,omitempty"`
}

type offerConnectionDetails struct {
	SourceModelUUID string                `json:"source-model-uuid" yaml:"source-model-uuid"`
	Username        string                `json:"username" yaml:"username"`
	RelationId      int                   `json:"relation-id" yaml:"relation-id"`
	Endpoint        string                `json:"endpoint" yaml:"endpoint"`
	Status          offerConnectionStatus `json:"status" yaml:"status"`
	IngressSubnets  []string              `json:"ingress-subnets,omitempty" yaml:"ingress-subnets,omitempty"`
}

func formatApplicationOfferDetails(store string, all []crossmodel.ApplicationOfferDetails) (offeredApplications, error) {
	result := make(offeredApplications)
	for _, one := range all {
		url, err := crossmodel.ParseOfferURL(one.OfferURL)
		if err != nil {
			return nil, errors.Annotatef(err, "%v", one.OfferURL)
		}
		if url.Source == "" {
			url.Source = store
		}

		// Store offers by name.
		result[one.OfferName] = convertOfferToListItem(url, one)
	}
	return result, nil
}

func convertOfferToListItem(url *crossmodel.OfferURL, offer crossmodel.ApplicationOfferDetails) ListOfferItem {
	item := ListOfferItem{
		OfferName:       offer.OfferName,
		ApplicationName: offer.ApplicationName,
		Source:          url.Source,
		CharmURL:        offer.CharmURL,
		OfferURL:        offer.OfferURL,
		Endpoints:       convertCharmEndpoints(offer.Endpoints...),
	}
	for _, conn := range offer.Connections {
		item.Connections = append(item.Connections, offerConnectionDetails{
			SourceModelUUID: conn.SourceModelUUID,
			Username:        conn.Username,
			RelationId:      conn.RelationId,
			Endpoint:        conn.Endpoint,
			Status: offerConnectionStatus{
				Current: conn.Status.String(),
				Message: conn.Message,
				Since:   friendlyDuration(conn.Since),
			},
			IngressSubnets: conn.IngressSubnets,
		})
	}
	return item
}

func friendlyDuration(when *time.Time) string {
	if when == nil {
		return ""
	}
	return common.UserFriendlyDuration(*when, time.Now())
}

// convertCharmEndpoints takes any number of charm relations and
// creates a collection of ui-formatted endpoints.
func convertCharmEndpoints(relations ...charm.Relation) map[string]RemoteEndpoint {
	if len(relations) == 0 {
		return nil
	}
	output := make(map[string]RemoteEndpoint, len(relations))
	for _, one := range relations {
		output[one.Name] = RemoteEndpoint{one.Name, one.Interface, string(one.Role)}
	}
	return output
}
