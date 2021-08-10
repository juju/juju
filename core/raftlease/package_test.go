// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package raftlease -destination writer_mock_test.go io Writer
//go:generate go run github.com/golang/mock/mockgen -package raftlease -destination logger_mock_test.go github.com/juju/juju/core/raftlease ErrorLogger

func TestPackage(t *testing.T) {
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
