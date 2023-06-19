// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

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
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/api/controller/caasunitprovisioner")

	c.Assert(found, jc.SameContents, []string{
		"agent",
		"agent/constants",
		"agent/tools",
		"api",
		"api/agent/keyupdater",
		"api/base",
		"api/common",
		"api/common/charms",
		"api/watcher",
		"caas",
		"caas/kubernetes",
		"caas/kubernetes/pod",
		"caas/kubernetes/provider/constants",
		"caas/kubernetes/provider/proxy",
		"caas/kubernetes/provider/utils",
		"caas/specs",
		"charmhub",
		"charmhub/path",
		"charmhub/transport",
		"cloud",
		"cloudconfig/instancecfg",
		"cloudconfig/podcfg",
		"controller",
		"core/annotations",
		"core/assumes",
		"core/charm/metrics",
		"core/config",
		"core/constraints",
		"core/devices",
		"core/instance",
		"core/life",
		"core/logger",
		"core/lxdprofile",
		"core/macaroon",
		"core/machinelock",
		"core/migration",
		"core/model",
		"core/network",
		"core/network/firewall",
		"core/os",
		"core/paths",
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
		"environs",
		"environs/cloudspec",
		"environs/config",
		"environs/context",
		"environs/imagemetadata",
		"environs/instances",
		"environs/simplestreams",
		"environs/storage",
		"environs/tags",
		"environs/utils",
		"feature",
		"juju/names",
		"juju/osenv",
		"jujuclient",
		"logfwd",
		"logfwd/syslog",
		"mongo",
		"network",
		"network/debinterfaces",
		"network/netplan",
		"packaging",
		"packaging/dependency",
		"pki",
		"provider/lxd/lxdnames",
		"proxy",
		"proxy/errors",
		"proxy/factory",
		"rpc",
		"rpc/jsoncodec",
		"rpc/params",
		"service",
		"service/common",
		"service/snap",
		"service/systemd",
		"storage",
		"tools",
		"utils/proxy",
		"utils/scriptrunner",
		"version",
	})
}
