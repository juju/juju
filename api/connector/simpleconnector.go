// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package connector

import (
	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
)

// SimpleConfig aims to provide the same API surface as pilot juju for
// obtaining an api connection.
type SimpleConfig struct {

	// Addresses of controllers (at least one required, more than one for HA).
	ControllerAddresses []string

	// I don't know if that's required
	CACert string

	// UUID of model to connect to (optional)
	ModelUUID string

	// Either Username/Password or Macaroons is required to get authentication.
	Username  string
	Password  string
	Macaroons []macaroon.Slice

	// ClientID holds the client id part of client credentials used for authentication.
	ClientID string
	// ClientSecret holds the client secret part of client
	// credentials used for authentication.
	ClientSecret string
}

// A SimpleConnector can provide connections from a simple set of options.
type SimpleConnector struct {
	info            api.Info
	defaultDialOpts api.DialOpts
}

var _ Connector = (*SimpleConnector)(nil)

// NewSimple returns an instance of *SimpleConnector configured to
// connect according to the specified options.  If some options are invalid an
// error is returned.
func NewSimple(opts SimpleConfig, dialOptions ...api.DialOption) (*SimpleConnector, error) {
	info := api.Info{
		Addrs:    opts.ControllerAddresses,
		CACert:   opts.CACert,
		ModelTag: names.NewModelTag(opts.ModelUUID),
	}

	// When the client intends to login via client credentials (like a service
	// account) they leave `SimpleConfig.Username` empty and assign the client
	// credentials to `SimpleConfig.ClientID` and `SimpleConfig.ClientSecret`.
	// In such cases, assigning a user tag to `info.Tag` will result in panic.
	if opts.Username != "" {
		info.Tag = names.NewUserTag(opts.Username)
		info.Password = opts.Password
		info.Macaroons = opts.Macaroons
	}

	if err := info.Validate(); err != nil {
		return nil, err
	}

	dialOpts := api.DefaultDialOpts()
	if opts.Username == "" {
		dialOpts.LoginProvider = api.NewClientCredentialsLoginProvider(
			opts.ClientID,
			opts.ClientSecret,
		)
	}

	conn := &SimpleConnector{
		info:            info,
		defaultDialOpts: dialOpts,
	}

	for _, f := range dialOptions {
		f(&conn.defaultDialOpts)
	}
	return conn, nil
}

// Connect returns a Connection according to c's configuration.
func (c *SimpleConnector) Connect(dialOptions ...api.DialOption) (api.Connection, error) {
	opts := c.defaultDialOpts
	for _, f := range dialOptions {
		f(&opts)
	}
	return api.Open(&c.info, c.defaultDialOpts)
}
