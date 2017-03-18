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
    $ juju list-offers
    $ juju list-offers vendor:
    $ juju list-offers --url vendor:/u/ibm
    $ juju list-offers --interface db2

    $ juju list-offers --interface db2
    LOCAL
    APPLICATION           CHARM  INTERFACES   CONNECTED  STORE  URL 
    fred/prod/hosted-db2  db2    db2, log     23         local  /u/fred/prod/hosted-db2 
    mary/test/hosted-db2  db2    db2          0          local  /u/mary/test/hosted-db2

    VENDOR
    APPLICATION             CHARM  INTERFACES   CONNECTED  STORE  URL
    ibm/us-east/hosted-db2  db2    db2          3212       vendor   /u/ibm/hosted-db2

`

// listCommand returns storage instances.
type listCommand struct {
	CrossModelCommandBase

	out cmd.Output

	newAPIFunc func() (ListAPI, error)

	filters []crossmodel.ApplicationOfferFilter
}

// NewListEndpointsCommand constructs new list endpoint command.
func NewListEndpointsCommand() cmd.Command {
	listCmd := &listCommand{}
	listCmd.newAPIFunc = func() (ListAPI, error) {
		return listCmd.NewCrossModelAPI()
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
	c.CrossModelCommandBase.SetFlags(f)

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

	if len(c.filters) == 0 {
		c.filters = []crossmodel.ApplicationOfferFilter{{ApplicationURL: "local:"}}
	}
	// TODO (anastasiamac 2015-11-17) add input filters
	offeredApplications, err := api.ListOffers(c.filters...)
	if err != nil {
		return err
	}

	// Filter out valid output, if any...
	valid := []crossmodel.OfferedApplicationDetails{}
	for _, service := range offeredApplications {
		if service.Error != nil {
			fmt.Fprintf(ctx.Stderr, "%v\n", service.Error)
			continue
		}
		if service.Result != nil {
			valid = append(valid, *service.Result)
		}
	}
	if len(valid) == 0 {
		return nil
	}

	data, err := formatOfferedApplicationDetails(valid)
	if err != nil {
		return errors.Annotate(err, "failed to format found applications")
	}

	return c.out.Write(ctx, data)
}

// ListAPI defines the API methods that list endpoints command use.
type ListAPI interface {
	Close() error
	ListOffers(filters ...crossmodel.ApplicationOfferFilter) ([]crossmodel.OfferedApplicationDetailsResult, error)
}

// ListServiceItem defines the serialization behaviour of a service item in endpoints list.
type ListServiceItem struct {
	// CharmName is the charm name of this service.
	CharmName string `yaml:"charm,omitempty" json:"charm,omitempty"`

	// UsersCount is the count of how many users are connected to this shared service.
	UsersCount int `yaml:"connected,omitempty" json:"connected,omitempty"`

	// Store is the name of the store which offers this shared service.
	Store string `yaml:"store" json:"store"`

	// Location is part of Juju location where this service is shared relative to the store.
	Location string `yaml:"url" json:"url"`

	// Endpoints is a list of service endpoints.
	Endpoints map[string]RemoteEndpoint `yaml:"endpoints" json:"endpoints"`
}

type directoryApplications map[string]ListServiceItem

func formatOfferedApplicationDetails(all []crossmodel.OfferedApplicationDetails) (map[string]directoryApplications, error) {
	directories := make(map[string]directoryApplications)
	for _, one := range all {
		url, err := crossmodel.ParseApplicationURL(one.ApplicationURL)
		if err != nil {
			return nil, err
		}

		// Get services for this directory.
		servicesMap, ok := directories[url.Directory]
		if !ok {
			servicesMap = make(directoryApplications)
			directories[url.Directory] = servicesMap
		}

		// Store services by name.
		servicesMap[url.ApplicationName] = convertServiceToListItem(url, one)
	}
	return directories, nil
}

func convertServiceToListItem(url *crossmodel.ApplicationURL, service crossmodel.OfferedApplicationDetails) ListServiceItem {
	item := ListServiceItem{
		CharmName: service.CharmName,
		// TODO (anastasiamac 2-15-11-20) what is the difference between store and directory.
		// At this stage, the distinction is unclear apart from strong desire to call "directory" "store".
		Store: url.Directory,
		// Location is the suffix of the service's url, the part after "<directory name>:".
		Location:   service.ApplicationURL[len(url.Directory)+1:],
		UsersCount: service.ConnectedCount,
		Endpoints:  convertCharmEndpoints(service.Endpoints...),
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
