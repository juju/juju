// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package status_test -destination lease_mock_test.go github.com/juju/juju/core/lease Checker

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
