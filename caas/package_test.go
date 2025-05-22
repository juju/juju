// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/broker_mock.go github.com/juju/juju/caas Broker
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/application_mock.go github.com/juju/juju/caas Application

