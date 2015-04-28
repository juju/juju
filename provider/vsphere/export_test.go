// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo
// +build !go1.2 go1.3

package vsphere

import (
	"github.com/juju/juju/environs"
)

var (
	Provider environs.EnvironProvider = providerInstance
)

func init() {
	environs.RegisterProvider(providerType, providerInstance)
}

func ExposeEnvFakeClient(env *environ) *fakeClient {
	return env.client.connection.RoundTripper.(*fakeClient)
}

var _ environs.Environ = (*environ)(nil)
