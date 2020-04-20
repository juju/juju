// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/package_mock.go github.com/juju/juju/environs NetworkingEnviron,CloudEnvironProvider

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
