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
		"cloud",
		"cmd",
		"cmd/constants",
		"cmd/containeragent/utils",
		"controller",
		"core/arch",
		"core/backups",
		"core/base",
		"core/charm/metrics",
		"core/constraints",
		"core/devices",
		"core/facades",
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
		"core/objectstore",
		"core/os",
		"core/output",
		"core/paths",
		"core/permission",
		"core/presence",
		"core/relation",
		"core/resources",
		"core/secrets",
		"core/status",
		"core/trace",
		"core/upgrade",
		"core/watcher",
		"domain/credential",
		"environs/cloudspec",
		"environs/config",
		"environs/envcontext",
		"environs/tags",
		"internal/charmhub",
		"internal/charmhub/path",
		"internal/charmhub/transport",
		"internal/feature",
		"internal/logfwd",
		"internal/logfwd/syslog",
		"internal/mongo",
		"internal/network",
		"internal/network/debinterfaces",
		"internal/network/netplan",
		"internal/packaging",
		"internal/packaging/dependency",
		"internal/pki",
		"internal/proxy",
		"internal/proxy/config",
		"internal/pubsub/agent",
		"internal/rpcreflect",
		"internal/scriptrunner",
		"internal/service/common",
		"internal/service/pebble/plan",
		"internal/service/snap",
		"internal/service/systemd",
		"internal/storage",
		"internal/tools",
		"internal/worker/apicaller",
		"internal/worker/introspection",
		"internal/worker/introspection/pprof",
		"juju/osenv",
		"provider/lxd/lxdnames",
		"rpc",
		"rpc/jsoncodec",
		"rpc/params",
		"state/errors",
		"version",
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
