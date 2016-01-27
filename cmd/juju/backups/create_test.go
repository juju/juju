// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/testing"
)

type createSuite struct {
	BaseBackupsSuite
	wrappedCommand  cmd.Command
	command         *backups.CreateCommand
	defaultFilename string
}

var _ = gc.Suite(&createSuite{})

func (s *createSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.wrappedCommand, s.command = backups.NewCreateCommand()
	s.defaultFilename = "juju-backup-<date>-<time>.tar.gz"
}

func (s *createSuite) setDownload() *fakeAPIClient {
	client := s.BaseBackupsSuite.setDownload()
	return client
}

func (s *createSuite) checkDownloadStd(c *gc.C, ctx *cmd.Context) {
	c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")

	out := ctx.Stdout.(*bytes.Buffer).String()
	if !s.command.Log.Quiet {
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
	s.checkHelp(c, s.wrappedCommand)
}

func (s *createSuite) TestNoArgs(c *gc.C) {
	client := s.BaseBackupsSuite.setDownload()
	_, err := testing.RunCommand(c, s.wrappedCommand, "--quiet")
	c.Assert(err, jc.ErrorIsNil)

	client.Check(c, s.metaresult.ID, "", "Create", "Download")
}

func (s *createSuite) TestDefaultDownload(c *gc.C) {
	s.setDownload()
	ctx, err := testing.RunCommand(c, s.wrappedCommand, "--quiet", "--filename", s.defaultFilename)
	c.Assert(err, jc.ErrorIsNil)

	s.checkDownload(c, ctx)
	c.Check(s.command.Filename, gc.Not(gc.Equals), "")
	c.Check(s.command.Filename, gc.Equals, backups.NotSet)
}

func (s *createSuite) TestQuiet(c *gc.C) {
	client := s.BaseBackupsSuite.setDownload()
	ctx, err := testing.RunCommand(c, s.wrappedCommand, "--quiet")
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
	_, err := testing.RunCommand(c, s.wrappedCommand, "spam", "--quiet")
	c.Assert(err, jc.ErrorIsNil)

	client.Check(c, s.metaresult.ID, "spam", "Create", "Download")
}

func (s *createSuite) TestFilename(c *gc.C) {
	client := s.setDownload()
	ctx, err := testing.RunCommand(c, s.wrappedCommand, "--filename", "backup.tgz", "--quiet")
	c.Assert(err, jc.ErrorIsNil)

	client.Check(c, s.metaresult.ID, "", "Create", "Download")
	s.checkDownload(c, ctx)
	c.Check(s.command.Filename, gc.Equals, "backup.tgz")
}

func (s *createSuite) TestNoDownload(c *gc.C) {
	client := s.setSuccess()
	ctx, err := testing.RunCommand(c, s.wrappedCommand, "--no-download")
	c.Assert(err, jc.ErrorIsNil)

	client.Check(c, "", "", "Create")
	out := MetaResultString + s.metaresult.ID + "\n"
	s.checkStd(c, ctx, out, backups.DownloadWarning+"\n")
	c.Check(s.command.Filename, gc.Equals, backups.NotSet)
}

func (s *createSuite) TestFilenameAndNoDownload(c *gc.C) {
	s.setSuccess()
	_, err := testing.RunCommand(c, s.wrappedCommand, "--no-download", "--filename", "backup.tgz")

	c.Check(err, gc.ErrorMatches, "cannot mix --no-download and --filename")
}

func (s *createSuite) TestError(c *gc.C) {
	s.setFailure("failed!")
	_, err := testing.RunCommand(c, s.wrappedCommand)

	c.Check(errors.Cause(err), gc.ErrorMatches, "failed!")
}
