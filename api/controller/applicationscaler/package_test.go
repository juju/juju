// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationscaler_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type ImportSuite struct{}

var _ = gc.Suite(&ImportSuite{})

func (*ImportSuite) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/api/controller/applicationscaler")

	c.Assert(found, jc.SameContents, []string{
		"api/base",
		"core/constraints",
		"core/devices",
		"core/instance",
		"core/life",
		"core/migration",
		"core/model",
		"core/network",
		"core/relation",
		"core/resources",
		"core/secrets",
		"core/status",
		"core/watcher",
		"docker",
		"environs/context",
		"proxy",
		"rpc/params",
		"storage",
		"tools",
	})
}
