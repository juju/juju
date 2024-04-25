// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/package_mock.go github.com/juju/juju/internal/worker/upgradeseries Facade,UnitDiscovery,Upgrader

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
