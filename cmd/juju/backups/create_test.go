// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
)

type createSuite struct {
	BaseBackupsSuite
	wrappedCommand  cmd.Command
	command         *backups.CreateCommand
	defaultFilename string

	expectedOut string
	expectedErr string
}

var _ = tc.Suite(&createSuite{})

func (s *createSuite) SetUpTest(c *tc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.wrappedCommand, s.command = backups.NewCreateCommandForTest(s.store)
	s.defaultFilename = "juju-backup-<date>-<time>.tar.gz"

	s.expectedOut = MetaResultString
	s.expectedErr = `
Downloaded to juju-backup-00010101-000000.tar.gz
`[1:]
}

func (s *createSuite) TearDownTest(c *tc.C) {
	// We do not need to cater here for s.BaseBackupsSuite.filename as it will be deleted by the base suite.
	// However, in situations where s.command.Filename is defined, we want to remove it as well.
	if s.command.Filename != backups.NotSet && s.command.Filename != s.filename {
		err := os.Remove(s.command.Filename)
		c.Assert(err, tc.ErrorIsNil)
	}
	s.BaseBackupsSuite.TearDownTest(c)
}

func (s *createSuite) setSuccess() *fakeAPIClient {
	client := &fakeAPIClient{metaresult: s.metaresult}
	s.patchGetAPI(client)
	return client
}

func (s *createSuite) setFailure(failure string) *fakeAPIClient {
	client := &fakeAPIClient{err: errors.New(failure)}
	s.patchGetAPI(client)
	return client
}

func (s *createSuite) setDownload() *fakeAPIClient {
	client := s.setSuccess()
	client.archive = io.NopCloser(bytes.NewBufferString(s.data))
	return client
}

func (s *createSuite) checkDownloadStd(c *tc.C, ctx *cmd.Context) {
	c.Check(cmdtesting.Stderr(ctx), tc.Equals, s.expectedErr)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, s.expectedOut)

	out := cmdtesting.Stderr(ctx)
	parts := strings.Split(out, "\n")
	c.Assert(parts, tc.HasLen, 2)

	// Check the download message.
	parts = strings.Split(parts[0], "Downloaded to ")
	c.Assert(parts, tc.HasLen, 2)
	c.Assert(parts[0], tc.Equals, "")
	s.filename = parts[1][:len(parts[1])]
}

func (s *createSuite) checkDownload(c *tc.C, ctx *cmd.Context) {
	s.checkDownloadStd(c, ctx)
	s.checkArchive(c)
}

type createBackupArgParsing struct {
	title      string
	args       []string
	errMatch   string
	filename   string
	noDownload bool
	notes      string
}

var testCreateBackupArgParsing = []createBackupArgParsing{
	{
		title:      "no args",
		args:       []string{},
		filename:   backups.NotSet,
		noDownload: false,
		notes:      "",
	},
	{
		title:      "filename",
		args:       []string{"--filename", "testname"},
		filename:   "testname",
		noDownload: false,
		notes:      "",
	},
	{
		title:      "filename flag, no name",
		args:       []string{"--filename"},
		errMatch:   "option needs an argument: --filename",
		filename:   backups.NotSet,
		noDownload: false,
		notes:      "",
	},
	{
		title:      "filename && no-download",
		args:       []string{"--filename", "testname", "--no-download"},
		errMatch:   "cannot mix --no-download and --filename",
		filename:   backups.NotSet,
		noDownload: false,
		notes:      "",
	},
	{
		title:      "notes",
		args:       []string{"note for the backup"},
		errMatch:   "",
		filename:   backups.NotSet,
		noDownload: false,
		notes:      "note for the backup",
	},
}

func (s *createSuite) TestArgParsing(c *tc.C) {
	for i, test := range testCreateBackupArgParsing {
		c.Logf("%d: %s", i, test.title)
		err := cmdtesting.InitCommand(s.wrappedCommand, test.args)
		if test.errMatch == "" {
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(s.command.Filename, tc.Equals, test.filename)
			c.Assert(s.command.NoDownload, tc.Equals, test.noDownload)
			c.Assert(s.command.Notes, tc.Equals, test.notes)
		} else {
			c.Assert(err, tc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *createSuite) TestDefault(c *tc.C) {
	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand)
	c.Assert(err, tc.ErrorIsNil)

	client.CheckCalls(c, "Create", "Download")
	client.CheckArgs(c, "", "false", "backup-filename")
	s.checkDownload(c, ctx)
	c.Check(s.command.Filename, tc.Equals, backups.NotSet)
}

func (s *createSuite) TestDefaultQuiet(c *tc.C) {
	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.createCommandForGlobalOptionTesting(s.wrappedCommand), "create-backup", "--quiet")
	c.Assert(err, tc.ErrorIsNil)

	client.CheckCalls(c, "Create", "Download")
	client.CheckArgs(c, "", "false", "backup-filename")

	c.Check(ctx.Stderr.(*bytes.Buffer).String(), tc.Equals, "")
	c.Check(ctx.Stdout.(*bytes.Buffer).String(), tc.Equals, "")
}

func (s *createSuite) TestNotes(c *tc.C) {
	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "test notes")
	c.Assert(err, tc.ErrorIsNil)

	client.CheckCalls(c, "Create", "Download")
	client.CheckArgs(c, "test notes", "false", "backup-filename")
	s.checkDownload(c, ctx)
}

func (s *createSuite) TestFilename(c *tc.C) {
	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--filename", "backup.tgz")
	c.Assert(err, tc.ErrorIsNil)

	client.CheckCalls(c, "Create", "Download")
	client.CheckArgs(c, "", "false", "backup-filename")
	s.expectedErr = `
Downloaded to backup.tgz
`[1:]
	s.checkDownload(c, ctx)
	c.Check(s.command.Filename, tc.Equals, "backup.tgz")
}

func (s *createSuite) TestNoDownload(c *tc.C) {
	client := s.setSuccess()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--no-download")
	c.Assert(err, tc.ErrorIsNil)

	client.CheckCalls(c, "Create")
	client.CheckArgs(c, "", "true")
	c.Check(cmdtesting.Stderr(ctx), tc.Equals, "Remote backup stored on the controller as backup-filename\n")
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, s.expectedOut)
	c.Check(s.command.Filename, tc.Equals, backups.NotSet)
}

func (s *createSuite) TestFilenameAndNoDownload(c *tc.C) {
	s.setSuccess()
	_, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--no-download", "--filename", "backup.tgz")

	c.Check(err, tc.ErrorMatches, "cannot mix --no-download and --filename")
}

func (s *createSuite) TestError(c *tc.C) {
	s.setFailure("failed!")
	_, err := cmdtesting.RunCommand(c, s.wrappedCommand)

	c.Check(errors.Cause(err), tc.ErrorMatches, "failed!")
}
