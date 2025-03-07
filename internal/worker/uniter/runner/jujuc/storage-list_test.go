// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"encoding/json"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc/jujuctesting"
)

type storageListSuite struct {
	storageSuite
}

var _ = gc.Suite(&storageListSuite{})

func (s *storageListSuite) newHookContext() *jujuctesting.Context {
	ctx, info := s.NewHookContext()
	info.SetBlockStorage("data/0", "/dev/sda", s.Stub)
	info.SetBlockStorage("data/1", "/dev/sdb", s.Stub)
	info.SetBlockStorage("data/2", "/dev/sdc", s.Stub)
	info.SetBlockStorage("other/0", "/dev/sdd", s.Stub)
	info.SetBlockStorage("other/1", "/dev/sde", s.Stub)
	return ctx
}

func (s *storageListSuite) TestOutputFormatYAML(c *gc.C) {
	s.testOutputFormat(c,
		[]string{"--format", "yaml"},
		formatYaml,
		[]string{"data/0", "data/1", "data/2", "other/0", "other/1"},
	)
}

func (s *storageListSuite) TestOutputFormatJSON(c *gc.C) {
	s.testOutputFormat(c,
		[]string{"--format", "json"},
		formatJson,
		[]string{"data/0", "data/1", "data/2", "other/0", "other/1"},
	)
}

func (s *storageListSuite) TestOutputFormatDefault(c *gc.C) {
	// The default output format is "smart", which is
	// a newline-separated list of strings.
	s.testOutputFormat(c,
		[]string{},
		-1, // don't specify format
		"data/0\ndata/1\ndata/2\nother/0\nother/1\n",
	)
}

func (s *storageListSuite) TestOutputFiltered(c *gc.C) {
	s.testOutputFormat(c,
		[]string{"--format", "yaml", "data"},
		formatYaml,
		[]string{"data/0", "data/1", "data/2"},
	)
}

func (s *storageListSuite) TestOutputNoMatches(c *gc.C) {
	s.testOutputFormat(c,
		[]string{"--format", "yaml", "dat"},
		formatYaml,
		[]string{},
	)
}

func (s *storageListSuite) testOutputFormat(c *gc.C, args []string, format int, expect interface{}) {
	hctx := s.newHookContext()
	com, err := jujuc.NewCommand(hctx, "storage-list")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, args)
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")

	var out interface{}
	var outSlice []string
	switch format {
	case formatYaml:
		c.Assert(goyaml.Unmarshal(bufferBytes(ctx.Stdout), &outSlice), gc.IsNil)
		out = outSlice
	case formatJson:
		c.Assert(json.Unmarshal(bufferBytes(ctx.Stdout), &outSlice), gc.IsNil)
		out = outSlice
	default:
		out = string(bufferBytes(ctx.Stdout))
	}
	c.Assert(out, jc.DeepEquals, expect)
}
