// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"io/ioutil"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type WorkloadVersionGetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&WorkloadVersionGetSuite{})

func (s *WorkloadVersionGetSuite) createCommand(c *gc.C, version string, err error) cmd.Command {
	hctx := s.GetHookContext(c, -1, "")
	hctx.info.Version.WorkloadVersion = version
	s.Stub.SetErrors(err)

	com, err := jujuc.NewCommand(hctx, cmdString("workload-version-get"))
	c.Assert(err, jc.ErrorIsNil)
	return com
}

func (s *WorkloadVersionGetSuite) TestWorkloadVersionGetSomeArguments(c *gc.C) {
	com := s.createCommand(c, "", nil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"blah"})
	c.Check(code, gc.Equals, 2)
	c.Check(bufferString(ctx.Stdout), gc.Equals, "")
	c.Check(bufferString(ctx.Stderr), gc.Equals, "error: unrecognized args: [\"blah\"]\n")
}

func (s *WorkloadVersionGetSuite) TestWorkloadVersionGetNoArguments(c *gc.C) {
	com := s.createCommand(c, "beastmaster", nil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, nil)
	c.Check(code, gc.Equals, 0)
	c.Check(bufferString(ctx.Stdout), gc.Equals, "beastmaster\n")
	c.Check(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *WorkloadVersionGetSuite) TestWorkloadVersionGetFormatJson(c *gc.C) {
	com := s.createCommand(c, "misere", nil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--format", "json"})
	c.Check(code, gc.Equals, 0)
	c.Check(bufferString(ctx.Stdout), gc.Equals, "\"misere\"\n")
	c.Check(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *WorkloadVersionGetSuite) TestWorkloadVersionGetFormatYaml(c *gc.C) {
	com := s.createCommand(c, "notrumps", nil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--format", "yaml"})
	c.Check(code, gc.Equals, 0)
	c.Check(bufferString(ctx.Stdout), gc.Equals, "notrumps\n")
	c.Check(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *WorkloadVersionGetSuite) TestWorkloadVersionGetOutputFile(c *gc.C) {
	destFile, err := ioutil.TempFile("", "TestWorkloadVersionGetOutputFile")
	c.Assert(err, jc.ErrorIsNil)
	err = destFile.Close()
	c.Assert(err, jc.ErrorIsNil)
	fileName := destFile.Name()
	defer os.Remove(fileName)

	com := s.createCommand(c, "clubs", nil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--output", fileName})
	c.Check(code, gc.Equals, 0)
	c.Check(bufferString(ctx.Stdout), gc.Equals, "")
	c.Check(bufferString(ctx.Stderr), gc.Equals, "")

	fileContents, err := ioutil.ReadFile(fileName)
	c.Check(strings.TrimRight(string(fileContents), "\r\n"), gc.Equals, "clubs")
}

func (s *WorkloadVersionGetSuite) TestWorkloadVersionGetError(c *gc.C) {
	com := s.createCommand(c, "beastmaster", errors.New("uh oh spaghettio"))
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, nil)
	c.Check(code, gc.Equals, 1)
	c.Check(bufferString(ctx.Stdout), gc.Equals, "")
	c.Check(bufferString(ctx.Stderr), gc.Equals, "error: uh oh spaghettio\n")
}

func (s *WorkloadVersionGetSuite) TestHelp(c *gc.C) {

	var helpTemplate = `
Usage: workload-version-get [options]

Summary:
get workload version

Options:
--format  (= smart)
    Specify output format (json|smart|yaml)
-o, --output (= "")
    Specify an output file

Details:
workload-version-get returns the currently-set workload version for
this unit. It takes no arguments.
`[1:]

	com := s.createCommand(c, "", nil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Check(code, gc.Equals, 0)

	c.Check(bufferString(ctx.Stdout), gc.Equals, helpTemplate)
	c.Check(bufferString(ctx.Stderr), gc.Equals, "")
}
