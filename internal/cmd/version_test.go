// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd_test

import (
	"fmt"
	"testing"

	"github.com/juju/loggo/v2"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
)

type VersionSuite struct {
	testhelpers.LoggingSuite

	ctx *cmd.Context
}

func TestVersionSuite(t *testing.T) {
	tc.Run(t, &VersionSuite{})
}

type versionDetail struct {
	Version       string `json:"version"`
	GitCommitHash string `json:"git-commit-hash"`
	GitTreeState  string `json:"git-tree-state"`
}

func (s *VersionSuite) SetUpTest(c *tc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.ctx = cmdtesting.Context(c)
	loggo.ReplaceDefaultWriter(cmd.NewWarningWriter(s.ctx.Stderr))
}

func (s *VersionSuite) TestVersion(c *tc.C) {
	const version = "999.888.777"

	code := cmd.Main(cmd.NewVersionCommand(version, nil), s.ctx, nil)
	c.Check(code, tc.Equals, 0)
	c.Assert(cmdtesting.Stderr(s.ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), tc.Equals, version+"\n")
}

func (s *VersionSuite) TestVersionExtraArgs(c *tc.C) {
	code := cmd.Main(cmd.NewVersionCommand("xxx", nil), s.ctx, []string{"foo"})
	c.Check(code, tc.Equals, 2)
	c.Assert(cmdtesting.Stdout(s.ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(s.ctx), tc.Matches, "ERROR unrecognized args.*\n")
}

func (s *VersionSuite) TestVersionJson(c *tc.C) {
	const version = "999.888.777"

	code := cmd.Main(cmd.NewVersionCommand(version, nil), s.ctx, []string{"--format", "json"})
	c.Check(code, tc.Equals, 0)
	c.Assert(cmdtesting.Stderr(s.ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), tc.Equals, fmt.Sprintf("%q\n", version))
}

func (s *VersionSuite) TestVersionDetailJson(c *tc.C) {
	const version = "999.888.777"
	detail := versionDetail{
		Version:       version,
		GitCommitHash: "46f1a0bd5592a2f9244ca321b129902a06b53e03",
		GitTreeState:  "dirty",
	}

	code := cmd.Main(cmd.NewVersionCommand(version, detail), s.ctx, []string{"--all", "--format", "json"})
	c.Check(code, tc.Equals, 0)
	c.Assert(cmdtesting.Stderr(s.ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(s.ctx), tc.Equals, `
{"version":"999.888.777","git-commit-hash":"46f1a0bd5592a2f9244ca321b129902a06b53e03","git-tree-state":"dirty"}
`[1:])
}
