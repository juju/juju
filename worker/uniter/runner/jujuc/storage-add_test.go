// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type storageAddSuite struct {
	storageSuite
}

var _ = gc.Suite(&storageAddSuite{})

func (s *storageAddSuite) newHookContext() jujuc.Context {
	hctx, _ := s.NewHookContext()
	return hctx
}

func (s *storageAddSuite) getStorageUnitAddCommand(c *gc.C) cmd.Command {
	hctx := s.newHookContext()
	com, err := jujuc.NewCommand(hctx, cmdString("storage-add"))
	c.Assert(err, jc.ErrorIsNil)
	return com
}

func (s *storageAddSuite) TestHelp(c *gc.C) {
	com := s.getStorageUnitAddCommand(c)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)
	help := `
usage: storage-add <charm storage name>[=count] ...
purpose: add storage instances
`[1:] +
		jujuc.StorageAddDoc
	s.assertOutput(c, ctx, help, "")
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
		{[]string{"data="}, 1, ".*storage constraints require at least one.*"},
		{[]string{"data=pool"}, 1, `.*only count can be specified for "data".*`},
		{[]string{"data=pool,1M"}, 1, `.*only count can be specified for "data".*`},
		{[]string{"data=1M"}, 1, `.*only count can be specified for "data".*`},
		{[]string{"data=2,1M"}, 1, `.*only count can be specified for "data".*`},
		{[]string{"cache", "data=2,1M"}, 1, `.*only count can be specified for "data".*`},
	}
	for i, t := range tests {
		c.Logf("test %d: %#v", i, t.args)
		com := s.getStorageUnitAddCommand(c)
		testing.TestInit(c, com, t.args, t.err)
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
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, gc.Equals, t.code)
		s.assertOutput(c, ctx, "", t.err)
	}
}
