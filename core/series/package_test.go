// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/v2/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package series -destination distrosource_mock_test.go github.com/juju/juju/core/series DistroSource

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type ImportTest struct{}

var _ = gc.Suite(&ImportTest{})

func (*ImportTest) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/v2/core/series")
	c.Assert(found, jc.SameContents, []string{
		"core/os",
	})
}
