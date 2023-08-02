// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	coretesting.MgoTestPackage(t)
}

type ImportSuite struct{}

var _ = gc.Suite(&ImportSuite{})

func (*ImportSuite) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/api")

	c.Assert(found, jc.SameContents, []string{
		"api/agent/keyupdater",
		"api/base",
		"api/watcher",
		"core/arch",
		"core/constraints",
		"core/devices",
		"core/instance",
		"core/life",
		"core/macaroon",
		"core/migration",
		"core/model",
		"core/network",
		"core/os",
		"core/paths",
		"core/relation",
		"core/resources",
		"core/secrets",
		"core/base",
		"core/status",
		"core/watcher",
		"docker",
		"environs/context",
		"feature",
		"proxy",
		"rpc",
		"rpc/jsoncodec",
		"rpc/params",
		"storage",
		"tools",
		"utils/proxy",
		"version",
	})
}
