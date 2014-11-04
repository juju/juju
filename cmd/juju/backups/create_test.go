// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/testing"
)

type createSuite struct {
	BaseBackupsSuite
	subcommand *backups.CreateCommand
}

var _ = gc.Suite(&createSuite{})

func (s *createSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.subcommand = &backups.CreateCommand{}
	s.subcommand.Filename = "juju-backup-<date>-<time>.tar.gz"
}

func (s *createSuite) setDownload() *fakeAPIClient {
	client := s.BaseBackupsSuite.setDownload()
	s.subcommand.Quiet = true
	return client
}

func (s *createSuite) checkDownload(c *gc.C, ctx *cmd.Context) {
	c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")

	out := ctx.Stdout.(*bytes.Buffer).String()
	parts := strings.Split(out, "\n")
	c.Assert(parts, gc.HasLen, 3)
	c.Assert(parts[2], gc.Equals, "")

	c.Check(parts[0], gc.Equals, s.metaresult.ID)

	// Check the download message.
	parts = strings.Split(parts[1], "downloading to ")
	c.Assert(parts, gc.HasLen, 2)
	c.Assert(parts[0], gc.Equals, "")

	// Check the filename.
	s.filename = parts[1]
	s.checkArchive(c)
}

func (s *createSuite) TestHelp(c *gc.C) {
	ctx, err := testing.RunCommand(c, s.command, "create", "--help")
	c.Assert(err, gc.IsNil)

	info := s.subcommand.Info()
	expected := "(?sm)usage: juju backups create [options] " + info.Args + "$.*"
	expected = strings.Replace(expected, "[", `\[`, -1)
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^purpose: " + info.Purpose + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^" + info.Doc + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
}

func (s *createSuite) TestOkay(c *gc.C) {
	client := s.setSuccess()
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)
	c.Assert(err, gc.IsNil)

	out := MetaResultString + s.metaresult.ID + "\n"
	s.checkStd(c, ctx, out, "")
	c.Check(client.args, gc.DeepEquals, []string{"notes"})
	c.Check(client.notes, gc.Equals, "")
}

func (s *createSuite) TestQuiet(c *gc.C) {
	s.setSuccess()
	s.subcommand.Quiet = true
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)
	c.Assert(err, gc.IsNil)

	out := s.metaresult.ID + "\n"
	s.checkStd(c, ctx, out, "")
}

func (s *createSuite) TestNotes(c *gc.C) {
	client := s.setSuccess()
	_, err := testing.RunCommand(c, s.command, "create", "spam")
	c.Assert(err, gc.IsNil)

	c.Check(client.args, gc.DeepEquals, []string{"notes"})
	c.Check(client.notes, gc.Equals, "spam")
}

func (s *createSuite) TestFilename(c *gc.C) {
	s.setDownload()
	s.subcommand.Filename = "backup.tgz"
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)
	c.Assert(err, gc.IsNil)

	s.checkDownload(c, ctx)
	c.Check(s.subcommand.Filename, gc.Equals, s.filename)
}

func (s *createSuite) TestDownload(c *gc.C) {
	s.setDownload()
	s.subcommand.Download = true
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)
	c.Assert(err, gc.IsNil)

	s.checkDownload(c, ctx)
	c.Check(s.subcommand.Filename, gc.Equals, backups.NotSet)
}

func (s *createSuite) TestError(c *gc.C) {
	s.setFailure("failed!")
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)

	c.Check(errors.Cause(err), gc.ErrorMatches, "failed!")
}
