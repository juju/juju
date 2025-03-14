// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/state_mock.go github.com/juju/juju/apiserver/facades/client/charms/interfaces BackendState,Application,Machine,Unit,Downloader
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/repository.go github.com/juju/juju/core/charm Repository,CharmArchive
//go:generate go run go.uber.org/mock/mockgen -typed -package charms -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/charms ModelConfigService,ApplicationService,MachineService
