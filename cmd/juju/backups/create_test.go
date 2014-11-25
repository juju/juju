// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
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

func (s *createSuite) checkDownloadStd(c *gc.C, ctx *cmd.Context) {
	c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")

	out := ctx.Stdout.(*bytes.Buffer).String()
	if !s.subcommand.Quiet {
		parts := strings.Split(out, MetaResultString)
		c.Assert(parts, gc.HasLen, 2)
		c.Assert(parts[0], gc.Equals, "")
		out = parts[1]
	}

	parts := strings.Split(out, "\n")
	c.Assert(parts, gc.HasLen, 3)
	c.Assert(parts[2], gc.Equals, "")

	c.Check(parts[0], gc.Equals, s.metaresult.ID)

	// Check the download message.
	parts = strings.Split(parts[1], "downloading to ")
	c.Assert(parts, gc.HasLen, 2)
	c.Assert(parts[0], gc.Equals, "")
	s.filename = parts[1]
}

func (s *createSuite) checkDownload(c *gc.C, ctx *cmd.Context) {
	s.checkDownloadStd(c, ctx)
	s.checkArchive(c)
}

func (s *createSuite) TestHelp(c *gc.C) {
	ctx, err := testing.RunCommand(c, s.command, "create", "--help")
	c.Assert(err, jc.ErrorIsNil)

	info := s.subcommand.Info()
	expected := "(?sm)usage: juju backups create [options] " + info.Args + "$.*"
	expected = strings.Replace(expected, "[", `\[`, -1)
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^purpose: " + info.Purpose + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^" + info.Doc + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
}

func (s *createSuite) TestNoArgs(c *gc.C) {
	client := s.BaseBackupsSuite.setDownload()
	_, err := testing.RunCommand(c, s.command, "create")
	c.Assert(err, jc.ErrorIsNil)

	client.Check(c, s.metaresult.ID, "", "Create", "Download")
}

func (s *createSuite) TestDefaultDownload(c *gc.C) {
	s.setDownload()
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.checkDownload(c, ctx)
	c.Check(s.filename, gc.Not(gc.Equals), "")
	c.Check(s.subcommand.Filename, gc.Equals, backups.NotSet)
}

func (s *createSuite) TestQuiet(c *gc.C) {
	client := s.BaseBackupsSuite.setDownload()
	s.subcommand.Quiet = true
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)

	client.Check(c, s.metaresult.ID, "", "Create", "Download")

	c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
	out := ctx.Stdout.(*bytes.Buffer).String()
	c.Check(out, gc.Not(jc.Contains), MetaResultString)
	c.Check(out, jc.HasPrefix, s.metaresult.ID+"\n")
	s.checkDownloadStd(c, ctx)
}

func (s *createSuite) TestNotes(c *gc.C) {
	client := s.BaseBackupsSuite.setDownload()
	_, err := testing.RunCommand(c, s.command, "create", "spam")
	c.Assert(err, jc.ErrorIsNil)

	client.Check(c, s.metaresult.ID, "spam", "Create", "Download")
}

func (s *createSuite) TestFilename(c *gc.C) {
	client := s.setDownload()
	s.subcommand.Filename = "backup.tgz"
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)

	client.Check(c, s.metaresult.ID, "", "Create", "Download")
	s.checkDownload(c, ctx)
	c.Check(s.subcommand.Filename, gc.Equals, s.filename)
}

func (s *createSuite) TestNoDownload(c *gc.C) {
	client := s.setSuccess()
	s.subcommand.NoDownload = true
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)

	client.Check(c, "", "", "Create")
	out := MetaResultString + s.metaresult.ID + "\n"
	s.checkStd(c, ctx, out, backups.DownloadWarning+"\n")
	c.Check(s.subcommand.Filename, gc.Equals, backups.NotSet)
}

func (s *createSuite) TestFilenameAndNoDownload(c *gc.C) {
	s.setSuccess()
	_, err := testing.RunCommand(c, s.command, "create", "--no-download", "--filename", "backup.tgz")

	c.Check(err, gc.ErrorMatches, "cannot mix --no-download and --filename")
}

func (s *createSuite) TestError(c *gc.C) {
	s.setFailure("failed!")
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)

	c.Check(errors.Cause(err), gc.ErrorMatches, "failed!")
}
