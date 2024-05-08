// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package lease -destination lease_mock_test.go github.com/juju/juju/core/lease Secretary

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
