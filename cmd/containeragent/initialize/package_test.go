// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initialize_test

import (
	"testing"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type importSuite struct{}

var _ = gc.Suite(&importSuite{})

func (*importSuite) TestImports(c *gc.C) {
	found := set.NewStrings(
		coretesting.FindJujuCoreImports(c, "github.com/juju/juju/cmd/containeragent/initialize")...)

	expected := set.NewStrings(
		"agent",
		"agent/constants",
		"api",
		"api/agent/agent",
		"api/agent/caasapplication",
		"api/agent/keyupdater",
		"api/base",
		"api/common",
		"api/common/cloudspec",
		"api/watcher",
		"apiserver/errors",
		"caas/kubernetes/provider/constants",
		"charmhub",
		"charmhub/path",
		"charmhub/transport",
		"cloud",
		"cmd",
		"cmd/constants",
		"cmd/containeragent/utils",
		"controller",
		"core/arch",
		"core/backups",
		"core/charm/metrics",
		"core/constraints",
		"core/devices",
		"core/instance",
		"core/leadership",
		"core/lease",
		"core/life",
		"core/logger",
		"core/macaroon",
		"core/machinelock",
		"core/migration",
		"core/model",
		"core/network",
		"core/os",
		"core/output",
		"core/paths",
		"core/permission",
		"core/presence",
		"core/relation",
		"core/resources",
		"core/secrets",
		"core/base",
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
		"pubsub/agent",
		"rpc",
		"rpc/jsoncodec",
		"rpc/params",
		"service/common",
		"service/pebble/plan",
		"service/snap",
		"service/systemd",
		"state/errors",
		"storage",
		"tools",
		"utils/proxy",
		"utils/scriptrunner",
		"version",
		"worker/apicaller",
		"worker/introspection",
		"worker/introspection/pprof",
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
