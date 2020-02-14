// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate mockgen -package mocks -destination mocks/package_mock.go github.com/juju/juju/apiserver/facades/client/subnets Backing

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
