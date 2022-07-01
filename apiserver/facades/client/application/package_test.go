// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/storage_mock.go github.com/juju/juju/state/storage Storage
//go:generate go run github.com/golang/mock/mockgen -package application -destination updateseries_mocks_test.go github.com/juju/juju/apiserver/facades/client/application Application,Charm,UpdateSeriesState,UpdateSeriesValidator,CharmhubClient
//go:generate go run github.com/golang/mock/mockgen -package application -destination updateseries_mocks_test.go github.com/juju/juju/apiserver/facades/client/application Application,Charm,UpdateSeriesState,UpdateSeriesValidator,CharmhubClient
