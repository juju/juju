// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/zoned_environ.go github.com/juju/juju/internal/provider/common ZonedEnviron
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/instance_configurator.go github.com/juju/juju/internal/provider/common InstanceConfigurator
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/availability_zone.go github.com/juju/juju/core/network AvailabilityZone

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
