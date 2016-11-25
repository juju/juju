// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"github.com/juju/juju/environs"
)

var (
	Provider environs.EnvironProvider = providerInstance
)

func ExposeEnvFakeClient(env *environ) (*fakeClient, func() error, error) {
	conn, closer, err := env.client.connection()
	closer = func() error { return nil }
	return conn.RoundTripper.(*fakeClient), closer, err
}

var _ environs.Environ = (*environ)(nil)
