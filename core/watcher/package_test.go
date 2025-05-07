// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	coretesting "github.com/juju/juju/internal/testing"
)

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}

type ImportTest struct{}

var _ = tc.Suite(&ImportTest{})

func (s *ImportTest) TestImports(c *tc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/watcher")

	// This package brings in nothing else from outside juju/juju/core
	c.Assert(found, jc.SameContents, []string{
		"core/credential",
		"core/errors",
		"core/life",
		"core/logger",
		"core/migration",
		"core/model",
		"core/network",
		"core/permission",
		"core/resource",
		"core/secrets",
		"core/semversion",
		"core/status",
		"core/trace",
		"core/unit",
		"core/user",
		"internal/charm/resource",
		"internal/errors",
		"internal/logger",
		"internal/uuid",
	})

}
