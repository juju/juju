// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/testing"
)

type downloadSuite struct {
	BaseBackupsSuite
	subcommand *backups.DownloadCommand
}

var _ = gc.Suite(&downloadSuite{})

func (s *downloadSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.subcommand = &backups.DownloadCommand{}
}

func (s *downloadSuite) TearDownTest(c *gc.C) {
	filename := s.subcommand.ResolveFilename()
	if s.subcommand.Filename == "" {
		filename = s.filename
	}

	if s.filename == "" {
		s.filename = filename
	} else {
		c.Check(filename, gc.Equals, s.filename)
	}

	s.BaseBackupsSuite.TearDownTest(c)
}

func (s *downloadSuite) setSuccess() *fakeAPIClient {
	s.subcommand.ID = s.metaresult.ID
	client := s.BaseBackupsSuite.setDownload()
	return client
}

func (s *downloadSuite) TestHelp(c *gc.C) {
	ctx, err := testing.RunCommand(c, s.command, "download", "--help")
	c.Assert(err, jc.ErrorIsNil)

	info := s.subcommand.Info()
	expected := `(?sm)usage: juju backups download \[options] ` + info.Args + `$.*`
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^purpose: " + info.Purpose + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^" + info.Doc + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
}

func (s *downloadSuite) TestOkay(c *gc.C) {
	s.setSuccess()
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)
	c.Check(err, jc.ErrorIsNil)

	s.filename = "juju-backup-" + s.metaresult.ID + ".tar.gz"
	s.checkStd(c, ctx, s.filename+"\n", "")
	s.checkArchive(c)
}

func (s *downloadSuite) TestFilename(c *gc.C) {
	s.setSuccess()
	s.subcommand.Filename = "backup.tar.gz"
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)
	c.Check(err, jc.ErrorIsNil)

	s.filename = "backup.tar.gz"
	s.checkStd(c, ctx, s.filename+"\n", "")
	s.checkArchive(c)
}

func (s *downloadSuite) TestError(c *gc.C) {
	s.setFailure("failed!")
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)

	c.Check(errors.Cause(err), gc.ErrorMatches, "failed!")
}
