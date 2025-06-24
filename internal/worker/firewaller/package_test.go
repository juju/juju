// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/facade_mocks.go github.com/juju/juju/internal/worker/firewaller FirewallerAPI,RemoteRelationsAPI,CrossModelFirewallerFacadeCloser,EnvironFirewaller,EnvironModelFirewaller,EnvironInstances,EnvironInstance
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/entity_mocks.go github.com/juju/juju/internal/worker/firewaller Machine,Unit,Application
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/credential_mocks.go github.com/juju/juju/internal/worker/common CredentialAPI

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}
