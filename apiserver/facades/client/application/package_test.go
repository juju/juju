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
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/repository_mock.go github.com/juju/juju/apiserver/facades/client/application Repository
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/charm_mock.go github.com/juju/juju/apiserver/facades/client/application StateCharm
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/model_mock.go github.com/juju/juju/apiserver/facades/client/application StateModel
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/charmstore_mock.go github.com/juju/juju/apiserver/facades/client/application State
//go:generate go run github.com/golang/mock/mockgen -package application -destination update_series_mocks_test.go github.com/juju/juju/apiserver/facades/client/application Application,UpdateSeriesState,UpdateSeriesValidator
