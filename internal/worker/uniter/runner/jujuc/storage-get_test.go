// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/tc"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type storageGetSuite struct {
	storageSuite
}

func TestStorageGetSuite(t *stdtesting.T) {
	tc.Run(t, &storageGetSuite{})
}

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

func (s *storageGetSuite) TestOutputFormatKey(c *tc.C) {
	for i, t := range storageGetTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx, _ := s.newHookContext()
		com, err := jujuc.NewCommand(hctx, "storage-get")
		c.Assert(err, tc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)
		c.Assert(code, tc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), tc.Equals, "")

		var out interface{}
		var outMap map[string]interface{}
		switch t.format {
		case formatYaml:
			c.Assert(goyaml.Unmarshal(bufferBytes(ctx.Stdout), &outMap), tc.IsNil)
			out = outMap
		case formatJson:
			c.Assert(json.Unmarshal(bufferBytes(ctx.Stdout), &outMap), tc.IsNil)
			out = outMap
		default:
			out = string(bufferBytes(ctx.Stdout))
		}
		c.Assert(out, tc.DeepEquals, t.out)
	}
}

func (s *storageGetSuite) TestOutputPath(c *tc.C) {
	hctx, _ := s.newHookContext()
	com, err := jujuc.NewCommand(hctx, "storage-get")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--format", "yaml", "--output", "some-file", "-s", "data/0"})
	c.Assert(code, tc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), tc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), tc.Equals, "")
	content, err := os.ReadFile(filepath.Join(ctx.Dir, "some-file"))
	c.Assert(err, tc.ErrorIsNil)

	var out map[string]interface{}
	c.Assert(goyaml.Unmarshal(content, &out), tc.IsNil)
	c.Assert(out, tc.DeepEquals, storageAttributes)
}
