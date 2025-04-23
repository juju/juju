// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination package_mock_test.go github.com/juju/juju/domain/relation/modelmigration ImportService

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
