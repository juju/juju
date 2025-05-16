// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination migrations_mock_test.go github.com/juju/juju/domain/model/modelmigration ExportService,ModelImportService,ModelDetailService,UserService

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
