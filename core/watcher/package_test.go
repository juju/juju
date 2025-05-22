// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type ImportTest struct{}

func TestImportTest(t *stdtesting.T) { tc.Run(t, &ImportTest{}) }
func (s *ImportTest) TestImports(c *tc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/watcher")

	// This package brings in nothing else from outside juju/juju/core
	c.Assert(found, tc.SameContents, []string{
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
