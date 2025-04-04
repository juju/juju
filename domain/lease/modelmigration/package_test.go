// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination migrations_mock_test.go github.com/juju/juju/domain/lease/modelmigration Coordinator,ImportService,ExportService
//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination database_mock_test.go github.com/juju/juju/core/database TxnRunner

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
