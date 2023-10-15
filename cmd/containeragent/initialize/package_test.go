// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !windows
// +build !windows

package initialize_test

import (
	"testing"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type importSuite struct{}

var _ = gc.Suite(&importSuite{})

func (*importSuite) TestImports(c *gc.C) {
	found := set.NewStrings(
		coretesting.FindJujuCoreImports(c, "github.com/juju/juju/cmd/containeragent/initialize")...)

	expected := set.NewStrings(
		"agent",
		"agent/tools",
		"api",
		"api/agent/agent",
		"api/base",
		"api/agent/caasapplication",
		"api/common",
		"api/common/cloudspec",
		"api/agent/keyupdater",
		"api/watcher",
		"apiserver/errors",
		"rpc/params",
		"cmd/containeragent/utils",
		"caas/kubernetes",
		"caas/kubernetes/pod",
		"caas/kubernetes/provider/constants",
		"caas/kubernetes/provider/proxy",
		"charmhub",
		"charmhub/path",
		"charmhub/transport",
		"charmstore",
		"cloud",
		"cmd",
		"cmd/constants",
		"controller",
		"core/charm/metrics",
		"core/constraints",
		"core/devices",
		"core/instance",
		"core/leadership",
		"core/lease",
		"core/life",
		"core/logger",
		"core/machinelock",
		"core/migration",
		"core/model",
		"core/network",
		"core/os",
		"core/paths",
		"core/relation",
		"core/resources",
		"core/series",
		"core/status",
		"core/watcher",
		"docker",
		"docker/registry",
		"docker/registry/image",
		"docker/registry/internal",
		"environs/cloudspec",
		"environs/config",
		"environs/context",
		"environs/tags",
		"feature",
		"juju/names",
		"juju/osenv",
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
		"rpc",
		"rpc/jsoncodec",
		"service",
		"service/common",
		"service/pebble/plan",
		"service/snap",
		"service/systemd",
		"service/upstart",
		"service/windows",
		"state/errors",
		"storage",
		"tools",
		"utils/proxy",
		"utils/scriptrunner",
		"version",
		"worker",
		"worker/apicaller",
	)

	unexpected := found.Difference(expected)
	// TODO: review if there are any un-expected imports!
	// Show the values rather than just checking the length so a failing
	// test shows them.
	c.Check(unexpected.SortedValues(), jc.DeepEquals, []string{})
	// If unneeded show any values this is good as we've reduced
	// dependencies, and they should be removed from expected above.
	unneeded := expected.Difference(found)
	c.Check(unneeded.SortedValues(), jc.DeepEquals, []string{})
}
