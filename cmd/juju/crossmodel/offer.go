// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
)

const (
	offerCommandDoc = `
A vendor offers a service endpoint for use by consumers.

We could offer the same service under different URLs to allow different consumer metering rules.

Example:
$ juju offer db2:db 
$ juju offer db2:db local:db2
$ juju offer -e prod db2:db,log jaas:/u/ibm/hosted-db2
$ juju offer hosted-db2:db,log jaas:/u/ibm/hosted-db2 --to public
`
	offerCommandAgs = `
<service-name>:<endpoint-name>[,...] [<endpoint-url>] [--to <user-ident>,...]
where 

endpoint-url    For local endpoints:
                local:/u/<username>/<envname>/<servicename>

                    $ juju offer db2:db 
                    
                endpoint “db” available at local:/u/user-name/env-name/hosted-db2
                    
                For JAAS endpoints:
                jaas:/u/<username>/<servicename>
                    
                    $ juju offer db2:db jaas:/u/ibm/hosted-db2

                endpoint “db” available at jaas:/u/ibm/hosted-db2     
`
)

// NewOfferCommand constructs comands that enables endpoints for export.
func NewOfferCommand() cmd.Command {
	return envcmd.Wrap(&offerCommand{})
}

type offerCommand struct {
	CrossModelCommandBase
	api OfferAPI

	// Service stores service name.
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
		Purpose: "offer service endpoints for consumption",
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
		if err := c.parseOfferedURL(args[1]); err != nil {
			return err
		}
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

	return c.api.Offer(c.Service, c.Endpoints, c.URL, c.Users)
}

// OfferAPI defines the API methods that the offer command uses.
type OfferAPI interface {
	Close() error
	Offer(service string, endpoints []string, url string, users []string) error
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
	c.Service = parts[0]
	endpoints := strings.SplitN(parts[1], ",", -1)
	if len(endpoints) < 1 || endpoints[0] == "" {
		return errors.Errorf(`specify endpoints for %v" `, c.Service)
	}

	c.Endpoints = endpoints
	return nil
}

func (c *offerCommand) parseOfferedURL(arg string) error {
	// TODO(anastasiamac 2015-11-02) validate url
	c.URL = arg
	return nil
}
