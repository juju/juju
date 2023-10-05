// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/caller_mock.go github.com/juju/juju/api/base APICaller
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/machine_mock.go github.com/juju/juju/api/agent/provisioner MachineProvisioner

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}
