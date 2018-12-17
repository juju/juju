// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"bytes"
	"io/ioutil"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type ContainerspecSetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&ContainerspecSetSuite{})

var containerSpecYaml = `
containerspec:
  foo: bar
`[1:]

var containerSpecSetInitTests = []struct {
	args []string
	err  string
}{
	{[]string{"--file", "file", "extra"}, `unrecognized args: \["extra"\]`},
}

func (s *ContainerspecSetSuite) TestContainerSpecSetInit(c *gc.C) {
	for i, t := range containerSpecSetInitTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx := s.GetHookContext(c, -1, "")
		com, err := jujuc.NewCommand(hctx, "pod-spec-set")
		c.Assert(err, jc.ErrorIsNil)
		cmdtesting.TestInit(c, jujuc.NewJujucCommandWrappedForTest(com), t.args, t.err)
	}
}

func (s *ContainerspecSetSuite) TestHelp(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "pod-spec-set")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)
	expectedHelp := "" +
		"Usage: pod-spec-set [options] --file <pod spec file>\n" +
		"\n" +
		"Summary:\n" +
		"set pod spec information\n" +
		"\n" +
		"Options:\n" +
		"--file  (= -)\n" +
		"    file containing pod spec\n" +
		"\n" +
		"Details:\n" +
		"Sets configuration data to use for a pod.\n" +
		"The spec applies to all units for the application.\n"

	c.Assert(bufferString(ctx.Stdout), gc.Equals, expectedHelp)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *ContainerspecSetSuite) TestContainerSpecSetNoData(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "pod-spec-set")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)

	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, nil)
	c.Check(code, gc.Equals, 1)
	c.Assert(bufferString(
		ctx.Stderr), gc.Matches,
		".*no pod spec specified: pipe pod spec to command, or specify a file with --file\n")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
}

func (s *ContainerspecSetSuite) TestContainerSpecSet(c *gc.C) {
	s.assertContainerSpecSet(c, "specfile.yaml")
}

func (s *ContainerspecSetSuite) TestContainerSpecSetStdIn(c *gc.C) {
	s.assertContainerSpecSet(c, "-")
}

func (s *ContainerspecSetSuite) assertContainerSpecSet(c *gc.C, filename string) {
	hctx := s.GetHookContext(c, -1, "")
	com, args, ctx := s.initCommand(c, hctx, containerSpecYaml, filename)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, args)
	c.Check(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
	c.Assert(hctx.info.ContainerSpec, gc.Equals, containerSpecYaml)
}

func (s *ContainerspecSetSuite) initCommand(
	c *gc.C, hctx jujuc.Context, yaml string, filename string,
) (cmd.Command, []string, *cmd.Context) {
	com, err := jujuc.NewCommand(hctx, "pod-spec-set")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)

	var args []string
	if filename == "-" {
		ctx.Stdin = bytes.NewBufferString(yaml)
	} else if filename != "" {
		filename = filepath.Join(c.MkDir(), filename)
		args = append(args, "--file", filename)
		err := ioutil.WriteFile(filename, []byte(yaml), 0644)
		c.Assert(err, jc.ErrorIsNil)
	}
	return jujuc.NewJujucCommandWrappedForTest(com), args, ctx
}
