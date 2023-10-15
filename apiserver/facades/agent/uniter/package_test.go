// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/newlxdprofile.go github.com/juju/juju/apiserver/facades/agent/uniter LXDProfileBackendV2,LXDProfileMachineV2,LXDProfileUnitV2,LXDProfileCharmV2,LXDProfileModelV2
