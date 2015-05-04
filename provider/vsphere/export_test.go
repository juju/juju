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

func ExposeEnvFakeClient(env *environ) *fakeClient {
	return env.client.connection.RoundTripper.(*fakeClient)
}

var _ environs.Environ = (*environ)(nil)
