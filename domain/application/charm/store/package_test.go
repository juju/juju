// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package store -destination store_mock_test.go github.com/juju/juju/domain/application/charm/store ObjectStore

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
