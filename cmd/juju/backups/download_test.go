// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
)

type downloadSuite struct {
	BaseBackupsSuite
	wrappedCommand cmd.Command
	command        *backups.DownloadCommand
}

func TestDownloadSuite(t *testing.T) {
	tc.Run(t, &downloadSuite{})
}

func (s *downloadSuite) SetUpTest(c *tc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.wrappedCommand, s.command = backups.NewDownloadCommandForTest(s.store)
}

func (s *downloadSuite) TearDownTest(c *tc.C) {
	filename := s.command.ResolveFilename()
	if s.command.LocalFilename == "" {
		filename = s.filename
	}

	if s.filename == "" {
		s.filename = filename
	} else {
		c.Check(filename, tc.Equals, s.filename)
	}

	s.BaseBackupsSuite.TearDownTest(c)
}

func (s *downloadSuite) setSuccess() *fakeAPIClient {
	client := s.BaseBackupsSuite.setDownload()
	return client
}

func (s *downloadSuite) TestOkay(c *tc.C) {
	s.setSuccess()
	s.filename = "juju-backup-" + s.metaresult.ID + ".tar.gz"
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, s.filename)
	c.Check(err, tc.ErrorIsNil)

	c.Check(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, s.filename+"\n")
	s.checkArchive(c)
}

func (s *downloadSuite) TestFilename(c *tc.C) {
	s.setSuccess()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, s.metaresult.ID, "--filename", "backup.tar.gz")
	c.Check(err, tc.ErrorIsNil)

	s.filename = "backup.tar.gz"
	c.Check(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, s.filename+"\n")
	s.checkArchive(c)
}

func (s *downloadSuite) TestError(c *tc.C) {
	s.setFailure("failed!")
	_, err := cmdtesting.RunCommand(c, s.wrappedCommand, s.metaresult.ID)
	c.Check(errors.Cause(err), tc.ErrorMatches, "failed!")
}
