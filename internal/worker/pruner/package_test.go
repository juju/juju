// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pruner_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/mocks_facade.go github.com/juju/juju/internal/worker/pruner Facade

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
