// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	stdtesting "testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type ImportSuite struct{}

func TestImportSuite(t *stdtesting.T) { tc.Run(t, &ImportSuite{}) }
func (*ImportSuite) TestImports(c *tc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/api")

	c.Assert(found, tc.SameContents, []string{
		"api/agent/keyupdater",
		"api/base",
		"api/watcher",
		"core/arch",
		"core/backups",
		"core/base",
		"core/constraints",
		"core/credential",
		"core/devices",
		"core/errors",
		"core/facades",
		"core/http",
		"core/instance",
		"core/life",
		"core/logger",
		"core/migration",
		"core/model",
		"core/network",
		"core/unit",
		"core/os/ostype",
		"core/paths",
		"core/permission",
		"core/relation",
		"core/resource",
		"core/secrets",
		"core/semversion",
		"core/status",
		"core/trace",
		"core/user",
		"core/version",
		"core/watcher",
		"domain/model/errors",
		"domain/secret/errors",
		"domain/secretbackend/errors",
		"internal/charm",
		"internal/charm/assumes",
		"internal/charm/hooks",
		"internal/charm/resource",
		"internal/errors",
		"internal/featureflag",
		"internal/http",
		"internal/logger",
		"internal/macaroon",
		"internal/proxy",
		"internal/proxy/config",
		"internal/rpcreflect",
		"internal/storage",
		"internal/stringcompare",
		"internal/tools",
		"internal/uuid",
		"rpc",
		"rpc/jsoncodec",
		"rpc/params",
	})
}
