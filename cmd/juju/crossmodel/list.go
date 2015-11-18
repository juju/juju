// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
)

const listCommandDoc = `
List information about remote services' endpoints that have been shared.

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
    $ juju list-endpoints
    $ juju list-endpoints vendor:
    $ juju list-endpoints --url vendor:/u/ibm
    $ juju list-endpoints --interface db2

    $ juju list-endpoints --interface db2
    LOCAL
    APPLICATION           CHARM  INTERFACES   CONNECTED  STORE  URL 
    fred/prod/hosted-db2  db2    db2, log     23         local  /u/fred/prod/hosted-db2 
    mary/test/hosted-db2  db2    db2          0          local  /u/mary/test/hosted-db2

    JAAS
    APPLICATION             CHARM  INTERFACES   CONNECTED  STORE  URL
    ibm/us-east/hosted-db2  db2    db2          3212       jaas   /u/ibm/hosted-db2

`

// listCommand returns storage instances.
type listCommand struct {
	CrossModelCommandBase

	out cmd.Output

	newAPIFunc func() (ListAPI, error)

	filters map[string][]string
}

// NewListEndpointsCommand constructs new list endpoint command.
func NewListEndpointsCommand() cmd.Command {
	listCmd := &listCommand{}
	listCmd.newAPIFunc = func() (ListAPI, error) {
		return listCmd.NewCrossModelAPI()
	}
	return envcmd.Wrap(listCmd)
}

// Init implements Command.Init.
func (c *listCommand) Init(args []string) (err error) {
	// TODO (anastasiamac 2015-11-17)  need to get filters from user input
	// filter scope -key- can only be a few values
	// , including "" for "no filter scope specified as it is optional"
	return cmd.CheckEmpty(args)
}

// Info implements Command.Info.
func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-endpoints",
		Purpose: "lists shared endpoints",
		Doc:     listCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *listCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CrossModelCommandBase.SetFlags(f)

	// TODO (anastasiamac 2015-11-17)  need to get filters from user input
	// filter scope -key- can only be a few values
	//, including "" for "no filter scope specified as it is optional"
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
	// Expecting back a map grouped by directory.
	found, err := api.List(c.filters)
	if err != nil {
		return err
	}

	// Filter out valid output, if any...
	valid := make(map[string][]params.ListEndpointsServiceItem)
	for directory, items := range found {
		for _, one := range items {
			if one.Error != nil {
				fmt.Fprintf(ctx.Stderr, "%v\n", one.Error)
				continue
			}
			if one.Result != nil {
				valid[directory] = append(valid[directory], *one.Result)
			}
		}
	}
	if len(valid) == 0 {
		return nil
	}
	return c.out.Write(ctx, formatServiceItems(valid))
}

// ListAPI defines the API methods that list endpoints command use.
type ListAPI interface {
	Close() error
	List(filters map[string][]string) (map[string][]params.ListEndpointsServiceItemResult, error)
}

// ListServiceItem defines the serialization behaviour of a service item in endpoints list.
type ListServiceItem struct {
	// CharmName is the charm name of this service.
	CharmName string `yaml:"charm" json:"charm"`

	// UsersCount is the count of how many users are connected to this shared service.
	UsersCount int `yaml:"connected" json:"connected"`

	// Store is the name of the store which offers this shared service.
	Store string `yaml:"store" json:"store"`

	// Location is part of Juju location where this service is shared relative to the store.
	Location string `yaml:"url" json:"url"`

	// Endpoints is a list of service endpoints.
	Endpoints map[string]RemoteEndpoint `yaml:"endpoints" json:"endpoints"`
}

func formatServiceItems(all map[string][]params.ListEndpointsServiceItem) map[string]map[string]ListServiceItem {
	items := make(map[string]map[string]ListServiceItem)
	for directory, services := range all {
		servicesMap := make(map[string]ListServiceItem)
		for _, service := range services {
			servicesMap[service.ApplicationName] = convertServiceToListItem(service)
		}
		items[directory] = servicesMap
	}
	return items
}

func convertServiceToListItem(p params.ListEndpointsServiceItem) ListServiceItem {
	item := ListServiceItem{
		CharmName:  p.CharmName,
		Store:      p.Store,
		Location:   p.Location,
		UsersCount: p.UsersCount,
		Endpoints:  convertRemoteEndpoints(p.Endpoints...),
	}
	return item
}
