// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/facade_mock.go github.com/juju/juju/internal/worker/caasmodelconfigmanager Facade
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/broker_mock.go github.com/juju/juju/internal/worker/caasmodelconfigmanager CAASBroker

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
