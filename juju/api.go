// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"fmt"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state/api"
)

// APIConn holds a connection to a juju environment and its
// associated state through its API interface.
type APIConn struct {
	Environ environs.Environ
	State   *api.State
}

// NewAPIConn returns a new Conn that uses the
// given environment. The environment must have already
// been bootstrapped.
func NewAPIConn(environ environs.Environ, dialOpts api.DialOpts) (*APIConn, error) {
	_, info, err := environ.StateInfo()
	if err != nil {
		return nil, err
	}
	info.Tag = "user-admin"
	password := environ.Config().AdminSecret()
	if password == "" {
		return nil, fmt.Errorf("cannot connect without admin-secret")
	}
	info.Password = password

	st, err := api.Open(info, dialOpts)
	// TODO(rog): handle errUnauthorized when the API handles passwords.
	if err != nil {
		return nil, err
	}
	// TODO(rog): implement updateSecrets (see Conn.updateSecrets)
	return &APIConn{
		Environ: environ,
		State:   st,
	}, nil
}

// Close terminates the connection to the environment and releases
// any associated resources.
func (c *APIConn) Close() error {
	return c.State.Close()
}

// NewAPIClientFromName returns an APIConn pointing at the environName
// environment, or the default environment if environName is "".
func NewAPIClientFromName(environName string) (*api.Client, error) {
	environ, err := environs.NewFromName(environName)
	if err != nil {
		return nil, err
	}
	apiconn, err := NewAPIConn(environ, api.DefaultDialOpts())
	if err != nil {
		return nil, err
	}
	return apiconn.State.Client(), nil
}
