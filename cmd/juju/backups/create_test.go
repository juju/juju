// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/backups"
)

type createSuite struct {
	BaseBackupsSuite
	wrappedCommand  cmd.Command
	command         *backups.CreateCommand
	defaultFilename string

	expectedOut string
	expectedErr string
}

var _ = gc.Suite(&createSuite{})

func (s *createSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.wrappedCommand, s.command = backups.NewCreateCommandForTest(s.store)
	s.defaultFilename = "juju-backup-<date>-<time>.tar.gz"

	s.expectedOut = MetaResultString
	s.expectedErr = `
Downloaded to juju-backup-00010101-000000.tar.gz
`[1:]
}

func (s *createSuite) TearDownTest(c *gc.C) {
	// We do not need to cater here for s.BaseBackupsSuite.filename as it will be deleted by the base suite.
	// However, in situations where s.command.Filename is defined, we want to remove it as well.
	if s.command.Filename != backups.NotSet && s.command.Filename != s.filename {
		err := os.Remove(s.command.Filename)
		c.Assert(err, jc.ErrorIsNil)
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

func (s *createSuite) checkDownloadStd(c *gc.C, ctx *cmd.Context) {
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, s.expectedErr)
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, s.expectedOut)

	out := cmdtesting.Stderr(ctx)
	parts := strings.Split(out, "\n")
	c.Assert(parts, gc.HasLen, 2)

	// Check the download message.
	parts = strings.Split(parts[0], "Downloaded to ")
	c.Assert(parts, gc.HasLen, 2)
	c.Assert(parts[0], gc.Equals, "")
	s.filename = parts[1][:len(parts[1])]
}

func (s *createSuite) checkDownload(c *gc.C, ctx *cmd.Context) {
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

func (s *createSuite) TestArgParsing(c *gc.C) {
	for i, test := range testCreateBackupArgParsing {
		c.Logf("%d: %s", i, test.title)
		err := cmdtesting.InitCommand(s.wrappedCommand, test.args)
		if test.errMatch == "" {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(s.command.Filename, gc.Equals, test.filename)
			c.Assert(s.command.NoDownload, gc.Equals, test.noDownload)
			c.Assert(s.command.Notes, gc.Equals, test.notes)
		} else {
			c.Assert(err, gc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *createSuite) TestDefault(c *gc.C) {
	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand)
	c.Assert(err, jc.ErrorIsNil)

	client.CheckCalls(c, "Create", "Download")
	client.CheckArgs(c, "", "false", "backup-filename")
	s.checkDownload(c, ctx)
	c.Check(s.command.Filename, gc.Equals, backups.NotSet)
}

func (s *createSuite) TestDefaultQuiet(c *gc.C) {
	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.createCommandForGlobalOptionTesting(s.wrappedCommand), "create-backup", "--quiet")
	c.Assert(err, jc.ErrorIsNil)

	client.CheckCalls(c, "Create", "Download")
	client.CheckArgs(c, "", "false", "backup-filename")

	c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
	c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, "")
}

func (s *createSuite) TestNotes(c *gc.C) {
	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "test notes")
	c.Assert(err, jc.ErrorIsNil)

	client.CheckCalls(c, "Create", "Download")
	client.CheckArgs(c, "test notes", "false", "backup-filename")
	s.checkDownload(c, ctx)
}

func (s *createSuite) TestFilename(c *gc.C) {
	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--filename", "backup.tgz")
	c.Assert(err, jc.ErrorIsNil)

	client.CheckCalls(c, "Create", "Download")
	client.CheckArgs(c, "", "false", "backup-filename")
	s.expectedErr = `
Downloaded to backup.tgz
`[1:]
	s.checkDownload(c, ctx)
	c.Check(s.command.Filename, gc.Equals, "backup.tgz")
}

func (s *createSuite) TestNoDownload(c *gc.C) {
	client := s.setSuccess()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--no-download")
	c.Assert(err, jc.ErrorIsNil)

	client.CheckCalls(c, "Create")
	client.CheckArgs(c, "", "true")
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "Remote backup stored on the controller as backup-filename\n")
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, s.expectedOut)
	c.Check(s.command.Filename, gc.Equals, backups.NotSet)
}

func (s *createSuite) TestFilenameAndNoDownload(c *gc.C) {
	s.setSuccess()
	_, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--no-download", "--filename", "backup.tgz")

	c.Check(err, gc.ErrorMatches, "cannot mix --no-download and --filename")
}

func (s *createSuite) TestError(c *gc.C) {
	s.setFailure("failed!")
	_, err := cmdtesting.RunCommand(c, s.wrappedCommand)

	c.Check(errors.Cause(err), gc.ErrorMatches, "failed!")
}
