// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// The functions below break through the Connection abstraction to access or
// modify part of the underlying state.  They need to be exported because they
// are used in tests for the client facade client.

// SetServerAddressForTesting allows changing the URL to the internal API server that
// AddLocalCharm uses in order to test NotImplementedError.  Hopefully it will
// soon be gone forever.
func SetServerAddressForTesting(c Connection, scheme, addr string) {
	c.(*state).serverScheme = scheme
	c.(*state).addr = addr
}

// EmptyConnectionForTesting exists only to allow api/client/client.BarebonesClient() to
// be implemented.  Hopefully it will soon be gone forever.
func EmptyConnectionForTesting() Connection {
	return &state{}
}
