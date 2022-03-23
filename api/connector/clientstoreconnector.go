// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package connector

import (
	"errors"

	"github.com/juju/juju/api"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
)

var ErrEmptyControllerName = errors.New("empty controller name")

// A Connector can provie api.Connection instances based on a ClientStore
type ClientStoreConnector struct {
	config          ClientStoreConnectorConfig
	defaultDialOpts api.DialOpts
}

// ClientStoreConnectorConfig provides configuration for a Connector.
type ClientStoreConnectorConfig struct {
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
func NewClientStore(config ClientStoreConnectorConfig, dialOptions ...api.DialOption) (*ClientStoreConnector, error) {
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
	return juju.NewAPIConnection(juju.NewAPIConnectionParams{
		ControllerName: c.config.ControllerName,
		Store:          c.config.ClientStore,
		OpenAPI:        api.Open,
		DialOpts:       opts,
		AccountDetails: c.config.AccountDetails,
		ModelUUID:      c.config.ModelUUID,
	})
}
