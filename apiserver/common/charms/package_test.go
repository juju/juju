// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/mocks.go github.com/juju/juju/apiserver/common/charms State,Application,Charm,Model

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}
