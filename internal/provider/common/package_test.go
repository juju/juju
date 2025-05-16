// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/zoned_environ.go github.com/juju/juju/internal/provider/common ZonedEnviron
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/instance_configurator.go github.com/juju/juju/internal/provider/common InstanceConfigurator
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/availability_zone.go github.com/juju/juju/core/network AvailabilityZone
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/environs.go github.com/juju/juju/environs CredentialInvalidator

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
