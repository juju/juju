// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package upgradedatabase -destination package_mock_test.go github.com/juju/juju/worker/upgradedatabase Logger
//go:generate go run go.uber.org/mock/mockgen -package upgradedatabase -destination lock_mock_test.go github.com/juju/juju/worker/gate Lock

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
