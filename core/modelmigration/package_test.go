// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination getter_mock_test.go github.com/juju/juju/core/database DBGetter,TxnRunner
//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination op_mock_test.go github.com/juju/juju/core/modelmigration Operation
//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination description_mock_test.go github.com/juju/description/v9 Model

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
		"core/credential",
		"core/database",
		"core/errors",
		"core/life",
		"core/logger",
		"core/model",
		"core/permission",
		"core/semversion",
		"core/status",
		"core/user",
		"internal/errors",
		"internal/uuid",
	})
}
