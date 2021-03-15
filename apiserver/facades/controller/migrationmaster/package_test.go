// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/backend.go github.com/juju/juju/apiserver/facades/controller/migrationmaster Backend,OfferConnection
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/precheckbackend.go github.com/juju/juju/migration PrecheckBackend
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/state.go github.com/juju/juju/state ModelMigration,NotifyWatcher,ExternalController

package migrationmaster_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}
