// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/mocks.go github.com/juju/juju/apiserver/common/charms State,Application,Charm,Model

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
