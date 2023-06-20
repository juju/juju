// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddressupdater_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/facade_mock.go github.com/juju/juju/worker/apiaddressupdater APIAddresser

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
