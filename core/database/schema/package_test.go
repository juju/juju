// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package schema -destination tx_mock_test.go github.com/juju/juju/core/database/schema Tx

type ImportTest struct{}

func TestImportTest(t *stdtesting.T) { tc.Run(t, &ImportTest{}) }
func (*ImportTest) TestImports(c *tc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/database/schema")

	// This package should only depend on other core packages.
	// If this test fails with a non-core package, please check the dependencies.
	c.Assert(found, tc.SameContents, []string{
		"core/database",
		"internal/errors",
	})
}
