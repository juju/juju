// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/package_mock.go github.com/juju/juju/environs Environ,NetworkingEnviron,CloudEnvironProvider,InstanceTypesFetcher

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
