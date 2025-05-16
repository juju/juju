// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"fmt"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/arch"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
)

type VersionSuite struct {
	testhelpers.IsolationSuite
}

func TestVersionSuite(t *stdtesting.T) { tc.Run(t, &VersionSuite{}) }
func (s *VersionSuite) TestVersion(c *tc.C) {
	s.PatchValue(&jujuversion.Current, semversion.MustParse("2.99.0"))
	command := newVersionCommand()
	cctx, err := cmdtesting.RunCommand(c, command)
	c.Assert(err, tc.ErrorIsNil)
	output := fmt.Sprintf("2.99.0-%s-%s\n",
		coreos.HostOSTypeName(), arch.HostArch())

	c.Assert(cctx.Stdout.(*bytes.Buffer).String(), tc.Equals, output)
	c.Assert(cctx.Stderr.(*bytes.Buffer).String(), tc.Equals, "")
}

func (s *VersionSuite) TestVersionDetail(c *tc.C) {
	s.PatchValue(&jujuversion.Current, semversion.MustParse("2.99.0"))
	s.PatchValue(&jujuversion.GitCommit, "0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f")
	s.PatchValue(&jujuversion.GitTreeState, "clean")
	s.PatchValue(&jujuversion.Compiler, "gc")
	s.PatchValue(&jujuversion.GoBuildTags, "a,b,c,d")
	command := newVersionCommand()
	cctx, err := cmdtesting.RunCommand(c, command, "--all")
	c.Assert(err, tc.ErrorIsNil)
	outputTemplate := `
version: 2.99.0-%s-%s
git-commit: 0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f
git-tree-state: clean
compiler: gc
go-build-tags: a,b,c,d
`[1:]
	output := fmt.Sprintf(outputTemplate, coreos.HostOSTypeName(), arch.HostArch())

	c.Assert(cctx.Stdout.(*bytes.Buffer).String(), tc.Equals, output)
	c.Assert(cctx.Stderr.(*bytes.Buffer).String(), tc.Equals, "")
}

func (s *VersionSuite) TestVersionDetailJSON(c *tc.C) {
	s.PatchValue(&jujuversion.Current, semversion.MustParse("2.99.0"))
	s.PatchValue(&jujuversion.GitCommit, "0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f")
	s.PatchValue(&jujuversion.GitTreeState, "clean")
	s.PatchValue(&jujuversion.Compiler, "gc")
	s.PatchValue(&jujuversion.GoBuildTags, "a,b,c,d")
	command := newVersionCommand()
	cctx, err := cmdtesting.RunCommand(c, command, "--all", "--format", "json")
	c.Assert(err, tc.ErrorIsNil)
	outputTemplate := `
{"version":"2.99.0-%s-%s","git-commit":"0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f","git-tree-state":"clean","compiler":"gc","go-build-tags":"a,b,c,d"}
`[1:]
	output := fmt.Sprintf(outputTemplate, coreos.HostOSTypeName(), arch.HostArch())

	c.Assert(cctx.Stdout.(*bytes.Buffer).String(), tc.Equals, output)
	c.Assert(cctx.Stderr.(*bytes.Buffer).String(), tc.Equals, "")
}

func (s *VersionSuite) TestVersionDetailYAML(c *tc.C) {
	s.PatchValue(&jujuversion.Current, semversion.MustParse("2.99.0"))
	s.PatchValue(&jujuversion.GitCommit, "0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f")
	s.PatchValue(&jujuversion.GitTreeState, "clean")
	s.PatchValue(&jujuversion.Compiler, "gc")
	s.PatchValue(&jujuversion.GoBuildTags, "a,b,c,d")
	command := newVersionCommand()
	cctx, err := cmdtesting.RunCommand(c, command, "--all", "--format", "yaml")
	c.Assert(err, tc.ErrorIsNil)
	outputTemplate := `
version: 2.99.0-%s-%s
git-commit: 0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f
git-tree-state: clean
compiler: gc
go-build-tags: a,b,c,d
`[1:]
	output := fmt.Sprintf(outputTemplate, coreos.HostOSTypeName(), arch.HostArch())

	c.Assert(cctx.Stdout.(*bytes.Buffer).String(), tc.Equals, output)
	c.Assert(cctx.Stderr.(*bytes.Buffer).String(), tc.Equals, "")
}
