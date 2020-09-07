// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/apiserver/facades/client/charms BackendState,BackendModel
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/repositories_mock.go github.com/juju/juju/apiserver/facades/client/charms CSRepository,CharmHubClient,Strategy,StoreCharm
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/corecharm_mock.go github.com/juju/juju/core/charm StateCharm
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/storage_mock.go github.com/juju/juju/state/storage Storage
