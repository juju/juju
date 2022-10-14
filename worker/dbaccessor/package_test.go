// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package dbaccessor -destination package_mock_test.go github.com/juju/juju/worker/dbaccessor Logger,DBApp
//go:generate go run github.com/golang/mock/mockgen -package dbaccessor -destination clock_mock_test.go github.com/juju/clock Clock

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
