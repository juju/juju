// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package status_test -destination lease_mock_test.go github.com/juju/juju/core/lease Checker

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
