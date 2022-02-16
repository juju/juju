// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/names/v4"
	"gopkg.in/macaroon.v2"
)

// A Connector is able to provide a Connection.  This connection can be used to
// make API calls via the various packages in github.com/juju/juju/api.
type Connector interface {
	Connect(...DialOption) (Connection, error)
}

// SimpleConnectorConfig aims to provide the same API surface as pilot juju for
// obtaining an api connection.
type SimpleConnectorConfig struct {

	// Address of the controller (required)
	ControllerAddress string

	// I don't know if that's required...
	CACert string

	// UUID of model to connect to (optional)
	ModelUUID string

	// Either Username/Password or Macaroons is required to get authentication.
	Username  string
	Password  string
	Macaroons []macaroon.Slice
}

// A SimpleConnector can provide connections from a simple set of options.
type SimpleConnector struct {
	info            Info
	defaultDialOpts DialOpts
}

var _ Connector = (*SimpleConnector)(nil)

// NewSimpleConnector returns an instance of *SimpleConnector configured to
// connect according to the specified options.  If some options are invalid an
// error is returned.
func NewSimpleConnector(opts SimpleConnectorConfig, dialOptions ...DialOption) (*SimpleConnector, error) {
	info := Info{
		Addrs:    []string{opts.ControllerAddress},
		CACert:   opts.CACert,
		ModelTag: names.NewModelTag(opts.ModelUUID),

		Tag:       names.NewUserTag(opts.Username),
		Password:  opts.Password,
		Macaroons: opts.Macaroons,
	}
	if err := info.Validate(); err != nil {
		return nil, err
	}
	conn := &SimpleConnector{
		info:            info,
		defaultDialOpts: DefaultDialOpts(),
	}
	for _, f := range dialOptions {
		f(&conn.defaultDialOpts)
	}
	return conn, nil
}

// Connect returns a Connection according to c's configuration.
func (c *SimpleConnector) Connect(dialOptions ...DialOption) (Connection, error) {
	opts := c.defaultDialOpts
	for _, f := range dialOptions {
		f(&opts)
	}
	return Open(&c.info, c.defaultDialOpts)
}
