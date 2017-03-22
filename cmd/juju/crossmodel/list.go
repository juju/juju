// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
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
       - charm name
   <filter-scope> is optional and is used to limit the scope of the search using the search term, one of:
       - url
       - interface
       - charm

Examples:
    $ juju offers
    $ juju offers user/model
    $ juju offers --url user/model
    $ juju offers --interface db2

    $ juju offers --interface db2
    mycontroller
    Application  Charm  Connected  Store         URL                     Endpoint  Interface  Role
    db2          db2    123        mycontroller  admin/controller.mysql  db        db2        provider

`

// listCommand returns storage instances.
type listCommand struct {
	ApplicationOffersCommandBase

	out cmd.Output

	newAPIFunc func() (ListAPI, error)

	filters []crossmodel.ApplicationOfferFilter
}

// NewListEndpointsCommand constructs new list endpoint command.
func NewListEndpointsCommand() cmd.Command {
	listCmd := &listCommand{}
	listCmd.newAPIFunc = func() (ListAPI, error) {
		return listCmd.NewApplicationOffersAPI()
	}
	return modelcmd.Wrap(listCmd)
}

// Init implements Command.Init.
func (c *listCommand) Init(args []string) (err error) {
	// TODO (anastasiamac 2015-11-17)  need to get filters from user input
	return cmd.CheckEmpty(args)
}

// Info implements Command.Info.
func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "offers",
		Aliases: []string{"list-offers"},
		Purpose: "Lists shared endpoints",
		Doc:     listCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *listCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ApplicationOffersCommandBase.SetFlags(f)

	// TODO (anastasiamac 2015-11-17)  need to get filters from user input
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatListTabular,
	})
}

// Run implements Command.Run.
func (c *listCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	// TODO (anastasiamac 2015-11-17) add input filters
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
	controllerName := c.ControllerName()
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
	// CharmName is the charm name of this application.
	CharmName string `yaml:"charm,omitempty" json:"charm,omitempty"`

	// UsersCount is the count of how many users are connected to this shared application.
	UsersCount int `yaml:"connected,omitempty" json:"connected,omitempty"`

	// Location is part of Juju location where this application is shared relative to the store.
	Location string `yaml:"url" json:"url"`

	// Endpoints is a list of application endpoints.
	Endpoints map[string]RemoteEndpoint `yaml:"endpoints" json:"endpoints"`
}

type directoryApplications map[string]ListOfferItem

func formatApplicationOfferDetails(store string, all []crossmodel.ApplicationOfferDetails) (map[string]directoryApplications, error) {
	controllerOffers := make(map[string]directoryApplications)
	for _, one := range all {
		url, err := crossmodel.ParseApplicationURL(one.OfferURL)
		if err != nil {
			return nil, errors.Annotatef(err, "%v", one.OfferURL)
		}
		if url.Source == "" {
			url.Source = store
		}

		// Get offers for this controller.
		offersMap, ok := controllerOffers[url.Source]
		if !ok {
			offersMap = make(directoryApplications)
			controllerOffers[url.Source] = offersMap
		}

		// Store offers by name.
		offersMap[one.OfferName] = convertOfferToListItem(url, one)
	}
	return controllerOffers, nil
}

func convertOfferToListItem(url *crossmodel.ApplicationURL, offer crossmodel.ApplicationOfferDetails) ListOfferItem {
	item := ListOfferItem{
		CharmName:  offer.CharmName,
		Location:   offer.OfferURL,
		UsersCount: offer.ConnectedCount,
		Endpoints:  convertCharmEndpoints(offer.Endpoints...),
	}
	return item
}

// convertCharmEndpoints takes any number of charm relations and
// creates a collection of ui-formatted endpoints.
func convertCharmEndpoints(relations ...charm.Relation) map[string]RemoteEndpoint {
	if len(relations) == 0 {
		return nil
	}
	output := make(map[string]RemoteEndpoint, len(relations))
	for _, one := range relations {
		output[one.Name] = RemoteEndpoint{one.Interface, string(one.Role)}
	}
	return output
}
