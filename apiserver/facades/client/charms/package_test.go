// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	stdtesting "testing"

	"github.com/juju/juju/internal/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/state_mock.go github.com/juju/juju/apiserver/facades/client/charms/interfaces BackendState,Application,Machine,Unit,Downloader
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/repository.go github.com/juju/juju/core/charm Repository,RepositoryFactory,CharmArchive
//go:generate go run go.uber.org/mock/mockgen -typed -package charms_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/charms ModelConfigService
