// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/updater_mocks.go github.com/juju/juju/internal/worker/authenticationworker Client

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
