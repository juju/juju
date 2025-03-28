// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type storageGetSuite struct {
	storageSuite
}

var _ = gc.Suite(&storageGetSuite{})

var storageGetTests = []struct {
	args   []string
	format int
	out    interface{}
}{
	{[]string{"--format", "yaml"}, formatYaml, storageAttributes},
	{[]string{"--format", "json"}, formatJson, storageAttributes},
	{[]string{}, formatYaml, storageAttributes},
	{[]string{"location"}, -1, "/dev/sda\n"},
}

func (s *storageGetSuite) TestOutputFormatKey(c *gc.C) {
	for i, t := range storageGetTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx, _ := s.newHookContext()
		com, err := jujuc.NewHookCommand(hctx, "storage-get")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)
		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")

		var out interface{}
		var outMap map[string]interface{}
		switch t.format {
		case formatYaml:
			c.Assert(goyaml.Unmarshal(bufferBytes(ctx.Stdout), &outMap), gc.IsNil)
			out = outMap
		case formatJson:
			c.Assert(json.Unmarshal(bufferBytes(ctx.Stdout), &outMap), gc.IsNil)
			out = outMap
		default:
			out = string(bufferBytes(ctx.Stdout))
		}
		c.Assert(out, gc.DeepEquals, t.out)
	}
}

func (s *storageGetSuite) TestOutputPath(c *gc.C) {
	hctx, _ := s.newHookContext()
	com, err := jujuc.NewHookCommand(hctx, "storage-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--format", "yaml", "--output", "some-file", "-s", "data/0"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
	content, err := os.ReadFile(filepath.Join(ctx.Dir, "some-file"))
	c.Assert(err, jc.ErrorIsNil)

	var out map[string]interface{}
	c.Assert(goyaml.Unmarshal(content, &out), gc.IsNil)
	c.Assert(out, gc.DeepEquals, storageAttributes)
}
