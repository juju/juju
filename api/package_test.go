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
	gc.TestingT(t)
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
		"core/backups",
		"core/base",
		"core/constraints",
		"core/credential",
		"core/devices",
		"core/facades",
		"core/instance",
		"core/life",
		"core/logger",
		"core/macaroon",
		"core/migration",
		"core/model",
		"core/network",
		"core/os",
		"core/os/ostype",
		"core/paths",
		"core/permission",
		"core/relation",
		"core/resources",
		"core/secrets",
		"core/status",
		"core/trace",
		"core/user",
		"core/watcher",
		"domain/secret/errors",
		"domain/secretbackend/errors",
		"environs/envcontext",
		"internal/charm",
		"internal/charm/assumes",
		"internal/charm/hooks",
		"internal/charm/resource",
		"internal/feature",
		"internal/http",
		"internal/logger",
		"internal/proxy",
		"internal/proxy/config",
		"internal/rpcreflect",
		"internal/storage",
		"internal/tools",
		"internal/uuid",
		"rpc",
		"rpc/jsoncodec",
		"rpc/params",
		"version",
	})
}
