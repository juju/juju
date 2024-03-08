// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/context_mock.go github.com/juju/juju/internal/upgrades Context

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

var UpgradeOperations = &upgradeOperations
