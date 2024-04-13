// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/broker_mock.go github.com/juju/juju/caas Broker
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/application_mock.go github.com/juju/juju/caas Application

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
