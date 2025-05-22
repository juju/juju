// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type storageAddSuite struct {
	storageSuite
}

func TestStorageAddSuite(t *testing.T) {
	tc.Run(t, &storageAddSuite{})
}

func (s *storageAddSuite) getStorageUnitAddCommand(c *tc.C) cmd.Command {
	hctx, _ := s.ContextSuite.NewHookContext()
	com, err := jujuc.NewCommand(hctx, "storage-add")
	c.Assert(err, tc.ErrorIsNil)
	return jujuc.NewJujucCommandWrappedForTest(com)
}

func (s *storageAddSuite) assertOutput(c *tc.C, ctx *cmd.Context, o, e string) {
	c.Assert(bufferString(ctx.Stdout), tc.Equals, o)
	c.Assert(bufferString(ctx.Stderr), tc.Equals, e)
}

type tstData struct {
	args []string
	code int
	err  string
}

func (s *storageAddSuite) TestStorageAddInit(c *tc.C) {
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

func (s *storageAddSuite) TestAddUnitStorage(c *tc.C) {
	tests := []tstData{
		{[]string{"data=676"}, 0, ""},
		{[]string{"data"}, 0, ``},
	}

	for i, t := range tests {
		c.Logf("test %d: %#v", i, t.args)
		com := s.getStorageUnitAddCommand(c)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, tc.Equals, t.code)
		s.assertOutput(c, ctx, "", t.err)
	}
}
