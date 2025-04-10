// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/state_mock.go github.com/juju/juju/internal/upgrades/upgradevalidation State,ModelAgentService
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/lxd_mock.go github.com/juju/juju/internal/provider/lxd ServerFactory,Server

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

var (
	CheckForDeprecatedUbuntuSeriesForModel = checkForDeprecatedUbuntuSeriesForModel
	GetCheckTargetVersionForModel          = getCheckTargetVersionForModel
	GetCheckForLXDVersion                  = getCheckForLXDVersion
)
