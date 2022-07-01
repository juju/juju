// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	stdtesting "testing"

	"github.com/juju/juju/v2/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/firewaller_mocks.go github.com/juju/juju/apiserver/facades/controller/firewaller State,ControllerConfigAPI

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
