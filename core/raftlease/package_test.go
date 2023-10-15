// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package raftlease -destination writer_mock_test.go io Writer

func Test(t *testing.T) {
	gc.TestingT(t)
}

type ImportSuite struct{}

var _ = gc.Suite(&ImportSuite{})

func (*ImportSuite) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/raftlease")

	// This package only brings in other core packages.
	c.Assert(found, jc.SameContents, []string{
		"core/globalclock",
		"core/lease",
	})
}
