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
	ContextSuite
}

var _ = gc.Suite(&storageAddSuite{})

func (s *storageAddSuite) SetUpTest(c *gc.C) {
	s.ContextSuite.SetUpTest(c)
}

func (s *storageAddSuite) getStorageUnitAddCommand(c *gc.C) cmd.Command {
	hctx := s.GetStorageAddHookContext(c)
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
usage: storage-add <charm storage name>=<constraints> ...
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
		{[]string{"data"}, 1, `expected "key=value", got "data"`},
		{[]string{"data="}, 1, ".*storage constraints require at least one.*"},
		{[]string{"data=-676"}, 1, `.*cannot parse count: count must be gre.*`},
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
