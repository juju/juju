// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner_test

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
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/api/controller/caasoperatorprovisioner")

	c.Assert(found, jc.SameContents, []string{
		"api/base",
		"api/common/charms",
		"api/watcher",
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
		"rpc",
		"rpc/params",
		"storage",
		"tools",
	})
}
