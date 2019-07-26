// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"bytes"
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/os/series"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	jujucmd "github.com/juju/juju/cmd"
	jujuversion "github.com/juju/juju/version"
)

type SuperCommandSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&SuperCommandSuite{})

func (s *SuperCommandSuite) TestVersion(c *gc.C) {
	s.PatchValue(&jujuversion.Current, version.MustParse("2.99.0"))
	params := cmd.SuperCommandParams{
		Name: "juju-test-command",
	}
	command := jujucmd.NewSuperCommand(params)
	cctx, err := cmdtesting.RunCommand(c, command, "version")
	c.Assert(err, jc.ErrorIsNil)
	output := fmt.Sprintf("2.99.0-%s-%s\n",
		series.MustHostSeries(), arch.HostArch())

	c.Assert(cctx.Stdout.(*bytes.Buffer).String(), gc.Equals, output)
	c.Assert(cctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
}

func (s *SuperCommandSuite) TestVersionDetail(c *gc.C) {
	s.PatchValue(&jujuversion.Current, version.MustParse("2.99.0"))
	s.PatchValue(&jujuversion.GitCommit, "0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f")
	s.PatchValue(&jujuversion.GitTreeState, "clean")
	s.PatchValue(&jujuversion.Compiler, "gc")
	params := cmd.SuperCommandParams{
		Name: "juju-test-command",
	}
	command := jujucmd.NewSuperCommand(params)
	cctx, err := cmdtesting.RunCommand(c, command, "version", "--all")
	c.Assert(err, jc.ErrorIsNil)
	outputTemplate := `
version: 2.99.0-%s-%s
git-commit: 0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f
git-tree-state: clean
compiler: gc
`[1:]
	output := fmt.Sprintf(outputTemplate, series.MustHostSeries(), arch.HostArch())

	c.Assert(cctx.Stdout.(*bytes.Buffer).String(), gc.Equals, output)
	c.Assert(cctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
}

func (s *SuperCommandSuite) TestVersionDetailJSON(c *gc.C) {
	s.PatchValue(&jujuversion.Current, version.MustParse("2.99.0"))
	s.PatchValue(&jujuversion.GitCommit, "0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f")
	s.PatchValue(&jujuversion.GitTreeState, "clean")
	s.PatchValue(&jujuversion.Compiler, "gc")
	params := cmd.SuperCommandParams{
		Name: "juju-test-command",
	}
	command := jujucmd.NewSuperCommand(params)
	cctx, err := cmdtesting.RunCommand(c, command, "version", "--all", "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	outputTemplate := `
{"version":"2.99.0-%s-%s","git-commit":"0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f","git-tree-state":"clean","compiler":"gc"}
`[1:]
	output := fmt.Sprintf(outputTemplate, series.MustHostSeries(), arch.HostArch())

	c.Assert(cctx.Stdout.(*bytes.Buffer).String(), gc.Equals, output)
	c.Assert(cctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
}

func (s *SuperCommandSuite) TestVersionDetailYAML(c *gc.C) {
	s.PatchValue(&jujuversion.Current, version.MustParse("2.99.0"))
	s.PatchValue(&jujuversion.GitCommit, "0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f")
	s.PatchValue(&jujuversion.GitTreeState, "clean")
	s.PatchValue(&jujuversion.Compiler, "gc")
	params := cmd.SuperCommandParams{
		Name: "juju-test-command",
	}
	command := jujucmd.NewSuperCommand(params)
	cctx, err := cmdtesting.RunCommand(c, command, "version", "--all", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	outputTemplate := `
version: 2.99.0-%s-%s
git-commit: 0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f
git-tree-state: clean
compiler: gc
`[1:]
	output := fmt.Sprintf(outputTemplate, series.MustHostSeries(), arch.HostArch())

	c.Assert(cctx.Stdout.(*bytes.Buffer).String(), gc.Equals, output)
	c.Assert(cctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
}
