// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/testing"
)

type downloadSuite struct {
	BaseBackupsSuite
	wrappedCommand cmd.Command
	command        *backups.DownloadCommand
}

var _ = gc.Suite(&downloadSuite{})

func (s *downloadSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.wrappedCommand, s.command = backups.NewDownloadCommand()
}

func (s *downloadSuite) TearDownTest(c *gc.C) {
	filename := s.command.ResolveFilename()
	if s.command.Filename == "" {
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
	client := s.BaseBackupsSuite.setDownload()
	return client
}

func (s *downloadSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, s.wrappedCommand)
}

func (s *downloadSuite) TestOkay(c *gc.C) {
	s.setSuccess()
	ctx, err := testing.RunCommand(c, s.wrappedCommand, s.metaresult.ID)
	c.Check(err, jc.ErrorIsNil)

	s.filename = "juju-backup-" + s.metaresult.ID + ".tar.gz"
	s.checkStd(c, ctx, s.filename+"\n", "")
	s.checkArchive(c)
}

func (s *downloadSuite) TestFilename(c *gc.C) {
	s.setSuccess()
	ctx, err := testing.RunCommand(c, s.wrappedCommand, s.metaresult.ID, "--filename", "backup.tar.gz")
	c.Check(err, jc.ErrorIsNil)

	s.filename = "backup.tar.gz"
	s.checkStd(c, ctx, s.filename+"\n", "")
	s.checkArchive(c)
}

func (s *downloadSuite) TestError(c *gc.C) {
	s.setFailure("failed!")
	_, err := testing.RunCommand(c, s.wrappedCommand, s.metaresult.ID)
	c.Check(errors.Cause(err), gc.ErrorMatches, "failed!")
}
