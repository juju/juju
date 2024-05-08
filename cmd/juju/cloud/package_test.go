// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/remove_mocks.go github.com/juju/juju/cmd/juju/cloud RemoveCloudAPI

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
