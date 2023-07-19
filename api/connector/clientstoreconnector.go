// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package connector

import (
	"errors"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"

	"github.com/juju/juju/api"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
)

var ErrEmptyControllerName = errors.New("empty controller name")

// A Connector can provie api.Connection instances based on a ClientStore
type ClientStoreConnector struct {
	config          ClientStoreConfig
	defaultDialOpts api.DialOpts
}

var _ Connector = (*ClientStoreConnector)(nil)

// ClientStoreConfig provides configuration for a Connector.
type ClientStoreConfig struct {
	// Controller to connect to.  Required
	ControllerName string

	// Model to connect to.  Optional
	ModelUUID string

	// Defaults to the file client store
	ClientStore jujuclient.ClientStore

	// Defaults to the user for the controller
	AccountDetails *jujuclient.AccountDetails
}

// NewClientStore returns a new *ClientStoreConnector instance for the given config, or an error if
// there was a problem setting up the connector.
func NewClientStore(config ClientStoreConfig, dialOptions ...api.DialOption) (*ClientStoreConnector, error) {
	if config.ControllerName == "" {
		return nil, ErrEmptyControllerName
	}
	if config.ClientStore == nil {
		config.ClientStore = jujuclient.NewFileClientStore()
	}
	if config.AccountDetails == nil {
		d, err := config.ClientStore.AccountDetails(config.ControllerName)
		if err != nil {
			return nil, err
		}
		config.AccountDetails = d
	}
	conn := &ClientStoreConnector{
		config:          config,
		defaultDialOpts: api.DefaultDialOpts(),
	}
	for _, opt := range dialOptions {
		opt(&conn.defaultDialOpts)
	}
	return conn, nil
}

// Connect returns an api.Connection to the controller / model specified in c's
// config, or an error if there was a problem opening the connection.
func (c *ClientStoreConnector) Connect(dialOptions ...api.DialOption) (api.Connection, error) {
	opts := c.defaultDialOpts
	for _, f := range dialOptions {
		f(&opts)
	}

	// By default there is no bakery client in the dial options, so we reproduce
	// behaviour that is scattered around the code to obtain a bakery client
	// with a cookie jar from the client store.
	jar, err := c.config.ClientStore.CookieJar(c.config.ControllerName)
	if err != nil {
		return nil, err
	}

	bakeryClient := httpbakery.NewClient()
	bakeryClient.Jar = jar
	opts.BakeryClient = bakeryClient

	return juju.NewAPIConnection(juju.NewAPIConnectionParams{
		ControllerName: c.config.ControllerName,
		Store:          c.config.ClientStore,
		OpenAPI:        api.Open,
		DialOpts:       opts,
		AccountDetails: c.config.AccountDetails,
		ModelUUID:      c.config.ModelUUID,
	})
}
