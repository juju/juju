// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/model/crossmodel"
)

const (
	offerCommandDoc = `
A vendor offers deployed service endpoints for use by consumers in their own models.

Examples:
$ juju offer db2:db 
$ juju offer db2:db local:db2
$ juju offer -e prod db2:db,log vendor:/u/ibm/hosted-db2
$ juju offer hosted-db2:db,log vendor:/u/ibm/hosted-db2 --to public
`
	offerCommandAgs = `
<service-name>:<endpoint-name>[,...] [<endpoint-url>] [--to <user-ident>,...]
where 

endpoint-url    For local endpoints:
                local:/u/<username>/<envname>/<servicename>

                    $ juju offer db2:db 
                    
                endpoint “db” available at local:/u/user-name/env-name/hosted-db2
                    
                For vendor endpoints:
                vendor:/u/<username>/<servicename>
                    
                    $ juju offer db2:db vendor:/u/ibm/hosted-db2

                endpoint “db” available at vendor:/u/ibm/hosted-db2     
`
)

// NewOfferCommand constructs commands that enables endpoints for export.
func NewOfferCommand() cmd.Command {
	return envcmd.Wrap(&offerCommand{})
}

type offerCommand struct {
	CrossModelCommandBase
	api OfferAPI

	// Service stores service tag.
	Service string

	// Endpoints stores a list of endpoints that are being offered.
	Endpoints []string

	// URL stores juju location where these endpoints are offered from.
	URL string

	// Users stores a list of users that these endpoints are offered to.
	Users []string
}

// Info implements Command.Info.
func (c *offerCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "offer",
		Purpose: "offer service endpoints for use in other models",
		Args:    offerCommandAgs,
		Doc:     offerCommandDoc,
	}
}

// Init implements Command.Init.
func (c *offerCommand) Init(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("an offer must at least specify service endpoint")
	}
	if len(args) > 2 {
		return fmt.Errorf("an offer can only specify service endpoints and url")
	}

	if err := c.parseEndpoints(args[0]); err != nil {
		return err
	}

	if len(args) == 2 {
		hostedURL := args[1]
		if !crossmodel.IsValidURL(hostedURL) {
			return errors.Errorf(`hosted url %q is not valid" `, hostedURL)
		}
		c.URL = hostedURL
	}

	return nil
}

// SetFlags implements Command.SetFlags.
func (c *offerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CrossModelCommandBase.SetFlags(f)
	f.Var(cmd.NewStringsValue(nil, &c.Users), "to", "users that these endpoints are offered to")
}

// Run implements Command.Run.
func (c *offerCommand) Run(_ *cmd.Context) error {
	if c.api == nil {
		api, err := getOfferAPI(c)
		if err != nil {
			return err
		}
		defer api.Close()
		c.api = api
	}

	userTags := make([]string, len(c.Users))
	for i, user := range c.Users {
		if !names.IsValidUser(user) {
			return errors.NotValidf(`user name %q`, user)
		}
		userTags[i] = names.NewUserTag(user).String()
	}

	results, err := c.api.Offer(c.Service, c.Endpoints, c.URL, userTags)
	if err != nil {
		return err
	}
	return params.ErrorResults{results}.Combine()
}

// OfferAPI defines the API methods that the offer command uses.
type OfferAPI interface {
	Close() error
	Offer(service string, endpoints []string, url string, users []string) ([]params.ErrorResult, error)
}

var getOfferAPI = (*offerCommand).getOfferAPI

func (c *offerCommand) getOfferAPI() (OfferAPI, error) {
	return c.NewCrossModelAPI()
}

func (c *offerCommand) parseEndpoints(arg string) error {
	parts := strings.SplitN(arg, ":", -1)

	if len(parts) != 2 {
		return errors.New(`endpoints must conform to format "<service-name>:<endpoint-name>[,...]" `)
	}

	serviceName := parts[0]
	if !names.IsValidService(serviceName) {
		return errors.NotValidf(`service name %q`, serviceName)
	}
	c.Service = names.NewServiceTag(serviceName).String()

	endpoints := strings.SplitN(parts[1], ",", -1)
	if len(endpoints) < 1 || endpoints[0] == "" {
		return errors.Errorf(`specify endpoints for %v" `, serviceName)
	}

	c.Endpoints = endpoints
	return nil
}
