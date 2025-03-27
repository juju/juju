// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package registry -destination worker_mock_test.go github.com/juju/worker/v4 Worker
//go:generate go run go.uber.org/mock/mockgen -typed -package registry -destination clock_mock_test.go github.com/juju/clock Clock

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type ImportTest struct{}

var _ = gc.Suite(&ImportTest{})

func (*ImportTest) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/watcher/registry")

	// This package brings in nothing else from outside juju/juju/core
	c.Assert(found, jc.SameContents, []string{
		"core/credential",
		"core/life",
		"core/logger",
		"core/model",
		"core/permission",
		"core/semversion",
		"core/status",
		"core/trace",
		"core/user",
		"internal/errors",
		"internal/logger",
		"internal/uuid",
	})
}
