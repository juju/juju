// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/testing"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
)

type mockEnviron struct {
	environs.NetworkingEnviron

	stub      testing.Stub
	spaceInfo *environs.ProviderSpaceInfo
}

func (e *mockEnviron) ProviderSpaceInfo(space *network.SpaceInfo) (*environs.ProviderSpaceInfo, error) {
	e.stub.MethodCall(e, "ProviderSpaceInfo", space)
	return e.spaceInfo, e.stub.NextErr()
}

type mockNoNetworkEnviron struct {
	environs.Environ
}
