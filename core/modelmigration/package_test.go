// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination getter_mock_test.go github.com/juju/juju/core/database DBGetter,TxnRunner
//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination op_mock_test.go github.com/juju/juju/core/modelmigration Operation
//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination description_mock_test.go github.com/juju/description/v6 Model

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type ImportTest struct{}

var _ = gc.Suite(&ImportTest{})

func (*ImportTest) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/modelmigration")

	// This package should only depend on other core packages.
	// If this test fails with a non-core package, please check the dependencies.
	c.Assert(found, jc.SameContents, []string{
		"core/database",
	})
}
