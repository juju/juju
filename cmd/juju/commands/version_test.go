// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"fmt"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/cmd/cmdtesting"
)

type VersionSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&VersionSuite{})

func (s *VersionSuite) TestVersion(c *gc.C) {
	s.PatchValue(&jujuversion.Current, semversion.MustParse("2.99.0"))
	command := newVersionCommand()
	cctx, err := cmdtesting.RunCommand(c, command)
	c.Assert(err, jc.ErrorIsNil)
	output := fmt.Sprintf("2.99.0-%s-%s\n",
		coreos.HostOSTypeName(), arch.HostArch())

	c.Assert(cctx.Stdout.(*bytes.Buffer).String(), gc.Equals, output)
	c.Assert(cctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
}

func (s *VersionSuite) TestVersionDetail(c *gc.C) {
	s.PatchValue(&jujuversion.Current, semversion.MustParse("2.99.0"))
	s.PatchValue(&jujuversion.GitCommit, "0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f")
	s.PatchValue(&jujuversion.GitTreeState, "clean")
	s.PatchValue(&jujuversion.Compiler, "gc")
	s.PatchValue(&jujuversion.GoBuildTags, "a,b,c,d")
	command := newVersionCommand()
	cctx, err := cmdtesting.RunCommand(c, command, "--all")
	c.Assert(err, jc.ErrorIsNil)
	outputTemplate := `
version: 2.99.0-%s-%s
git-commit: 0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f
git-tree-state: clean
compiler: gc
go-build-tags: a,b,c,d
`[1:]
	output := fmt.Sprintf(outputTemplate, coreos.HostOSTypeName(), arch.HostArch())

	c.Assert(cctx.Stdout.(*bytes.Buffer).String(), gc.Equals, output)
	c.Assert(cctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
}

func (s *VersionSuite) TestVersionDetailJSON(c *gc.C) {
	s.PatchValue(&jujuversion.Current, semversion.MustParse("2.99.0"))
	s.PatchValue(&jujuversion.GitCommit, "0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f")
	s.PatchValue(&jujuversion.GitTreeState, "clean")
	s.PatchValue(&jujuversion.Compiler, "gc")
	s.PatchValue(&jujuversion.GoBuildTags, "a,b,c,d")
	command := newVersionCommand()
	cctx, err := cmdtesting.RunCommand(c, command, "--all", "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	outputTemplate := `
{"version":"2.99.0-%s-%s","git-commit":"0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f","git-tree-state":"clean","compiler":"gc","go-build-tags":"a,b,c,d"}
`[1:]
	output := fmt.Sprintf(outputTemplate, coreos.HostOSTypeName(), arch.HostArch())

	c.Assert(cctx.Stdout.(*bytes.Buffer).String(), gc.Equals, output)
	c.Assert(cctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
}

func (s *VersionSuite) TestVersionDetailYAML(c *gc.C) {
	s.PatchValue(&jujuversion.Current, semversion.MustParse("2.99.0"))
	s.PatchValue(&jujuversion.GitCommit, "0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f")
	s.PatchValue(&jujuversion.GitTreeState, "clean")
	s.PatchValue(&jujuversion.Compiler, "gc")
	s.PatchValue(&jujuversion.GoBuildTags, "a,b,c,d")
	command := newVersionCommand()
	cctx, err := cmdtesting.RunCommand(c, command, "--all", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	outputTemplate := `
version: 2.99.0-%s-%s
git-commit: 0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f
git-tree-state: clean
compiler: gc
go-build-tags: a,b,c,d
`[1:]
	output := fmt.Sprintf(outputTemplate, coreos.HostOSTypeName(), arch.HostArch())

	c.Assert(cctx.Stdout.(*bytes.Buffer).String(), gc.Equals, output)
	c.Assert(cctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
}
