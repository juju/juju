// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate mockgen -package mocks -destination mocks/package_mock.go github.com/juju/juju/apiserver/facades/client/spaces Backing,BlockChecker,Machine

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
