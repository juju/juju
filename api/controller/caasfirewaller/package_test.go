// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

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
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/api/controller/caasfirewaller")

	c.Assert(found, jc.SameContents, []string{
		"api/base",
		"api/common",
		"api/common/charms",
		"api/watcher",
		"charmhub",
		"charmhub/path",
		"charmhub/transport",
		"controller",
		"core/charm/metrics",
		"core/config",
		"core/constraints",
		"core/devices",
		"core/instance",
		"core/life",
		"core/logger",
		"core/migration",
		"core/model",
		"core/network",
		"core/os",
		"core/relation",
		"core/resources",
		"core/secrets",
		"core/series",
		"core/status",
		"core/watcher",
		"docker",
		"docker/registry",
		"docker/registry/image",
		"docker/registry/internal",
		"environs/config",
		"environs/context",
		"environs/tags",
		"feature",
		"juju/osenv",
		"logfwd",
		"logfwd/syslog",
		"pki",
		"proxy",
		"rpc",
		"rpc/params",
		"storage",
		"tools",
		"version",
	})
}
