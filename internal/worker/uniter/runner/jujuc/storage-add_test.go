// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type storageAddSuite struct {
	storageSuite
}

var _ = gc.Suite(&storageAddSuite{})

func (s *storageAddSuite) getStorageUnitAddCommand(c *gc.C) cmd.Command {
	hctx, _ := s.ContextSuite.NewHookContext()
	com, err := jujuc.NewCommand(hctx, "storage-add")
	c.Assert(err, jc.ErrorIsNil)
	return jujuc.NewJujucCommandWrappedForTest(com)
}

func (s *storageAddSuite) assertOutput(c *gc.C, ctx *cmd.Context, o, e string) {
	c.Assert(bufferString(ctx.Stdout), gc.Equals, o)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, e)
}

type tstData struct {
	args []string
	code int
	err  string
}

func (s *storageAddSuite) TestStorageAddInit(c *gc.C) {
	tests := []tstData{
		{[]string{}, 1, "storage add requires a storage directive"},
		{[]string{"data=-676"}, 1, `.*cannot parse count: count must be gre.*`},
		{[]string{"data="}, 1, ".*storage directives require at least one.*"},
		{[]string{"data=pool"}, 1, `.*only count can be specified for "data".*`},
		{[]string{"data=pool,1M"}, 1, `.*only count can be specified for "data".*`},
		{[]string{"data=1M"}, 1, `.*only count can be specified for "data".*`},
		{[]string{"data=2,1M"}, 1, `.*only count can be specified for "data".*`},
		{[]string{"cache", "data=2,1M"}, 1, `.*only count can be specified for "data".*`},
	}
	for i, t := range tests {
		c.Logf("test %d: %#v", i, t.args)
		com := s.getStorageUnitAddCommand(c)
		cmdtesting.TestInit(c, com, t.args, t.err)
	}
}

func (s *storageAddSuite) TestAddUnitStorage(c *gc.C) {
	tests := []tstData{
		{[]string{"data=676"}, 0, ""},
		{[]string{"data"}, 0, ``},
	}

	for i, t := range tests {
		c.Logf("test %d: %#v", i, t.args)
		com := s.getStorageUnitAddCommand(c)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, gc.Equals, t.code)
		s.assertOutput(c, ctx, "", t.err)
	}
}
