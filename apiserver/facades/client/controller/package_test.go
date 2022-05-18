// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/controller_mock.go github.com/juju/juju/apiserver/facades/client/controller ControllerState,ControllerNode

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
