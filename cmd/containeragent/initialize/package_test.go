// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initialize_test

import (
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type importSuite struct{}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (*importSuite) TestImports(c *tc.C) {
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
		"api/watcher",
		"apiserver/errors",
		"cmd",
		"cmd/constants",
		"cmd/containeragent/utils",
		"controller",
		"core/arch",
		"core/backups",
		"core/base",
		"core/charm/metrics",
		"core/constraints",
		"core/credential",
		"core/devices",
		"core/errors",
		"core/facades",
		"core/http",
		"core/instance",
		"core/leadership",
		"core/lease",
		"core/life",
		"core/logger",
		"core/machinelock",
		"core/migration",
		"core/model",
		"core/modelconfig",
		"core/network",
		"core/objectstore",
		"core/os/ostype",
		"core/paths",
		"core/permission",
		"core/relation",
		"core/resource",
		"core/secrets",
		"core/semversion",
		"core/status",
		"core/trace",
		"core/unit",
		"core/upgrade",
		"core/user",
		"core/version",
		"core/watcher",
		"domain/model/errors",
		"domain/secret/errors",
		"domain/secretbackend/errors",
		"environs/config",
		"environs/tags",
		"internal/charm",
		"internal/charm/assumes",
		"internal/charm/hooks",
		"internal/charm/resource",
		"internal/charmhub",
		"internal/charmhub/path",
		"internal/charmhub/transport",
		"internal/cmd",
		"internal/configschema",
		"internal/errors",
		"internal/featureflag",
		"internal/http",
		"internal/logger",
		"internal/macaroon",
		"internal/mongo",
		"internal/network",
		"internal/network/netplan",
		"internal/packaging/commands",
		"internal/packaging/config",
		"internal/packaging/dependency",
		"internal/packaging/manager",
		"internal/packaging/source",
		"internal/password",
		"internal/pki",
		"internal/proxy",
		"internal/proxy/config",
		"internal/provider/kubernetes/constants",
		"internal/rpcreflect",
		"internal/scriptrunner",
		"internal/service/common",
		"internal/service/pebble/identity",
		"internal/service/pebble/plan",
		"internal/service/snap",
		"internal/service/systemd",
		"internal/storage",
		"internal/stringcompare",
		"internal/tools",
		"internal/uuid",
		"internal/worker/apicaller",
		"internal/worker/introspection",
		"internal/worker/introspection/pprof",
		"juju/osenv",
		"juju/sockets",
		"rpc",
		"rpc/jsoncodec",
		"rpc/params",
		"state/errors",
	)

	unexpected := found.Difference(expected)
	// TODO: review if there are any un-expected imports!
	// Show the values rather than just checking the length so a failing
	// test shows them.
	c.Check(unexpected.SortedValues(), tc.DeepEquals, []string{})
	// If unneeded show any values this is good as we've reduced
	// dependencies, and they should be removed from expected above.
	unneeded := expected.Difference(found)
	c.Check(unneeded.SortedValues(), tc.DeepEquals, []string{})
}
