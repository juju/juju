// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/newlxdprofile.go github.com/juju/juju/apiserver/facades/agent/uniter LXDProfileBackendV2,LXDProfileMachineV2,LXDProfileUnitV2,LXDProfileCharmV2,LXDProfileModelV2
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/domain_mock.go github.com/juju/juju/apiserver/facades/agent/uniter ControllerConfigGetter

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}
