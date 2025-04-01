// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package removal

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package removal -destination package_mocks_test.go github.com/juju/juju/internal/worker/removal RemovalService,Clock

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
