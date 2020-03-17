// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series_test

import (
	"testing"

	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

//go:generate mockgen -package series -destination distrosource_mock_test.go github.com/juju/juju/core/series DistroSource

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type ImportTest struct{}

var _ = gc.Suite(&ImportTest{})

func (*ImportTest) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/series")

	// This package brings in nothing else from juju/juju
	c.Assert(found, gc.HasLen, 0)
}
