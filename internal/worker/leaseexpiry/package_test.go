// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leaseexpiry

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package leaseexpiry_test -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -package leaseexpiry_test -destination store_mock_test.go github.com/juju/juju/core/lease ExpiryStore

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
