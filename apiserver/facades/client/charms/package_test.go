// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/apiserver/facades/client/charms/interfaces BackendState,BackendModel,Application,Machine,Unit,Downloader
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/repository.go github.com/juju/juju/core/charm Repository,RepositoryFactory,CharmArchive
