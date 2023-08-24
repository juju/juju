// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package migration_test -destination migration_mock_test.go github.com/juju/juju/internal/migration ControllerConfigGetter

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
