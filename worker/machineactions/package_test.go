// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/mock_facade.go github.com/juju/juju/worker/machineactions Facade
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/mock_lock.go github.com/juju/juju/core/machinelock Lock
