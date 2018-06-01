// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io/ioutil"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
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
	s.wrappedCommand, s.command = backups.NewCreateCommandForTest(jujuclienttesting.MinimalStore())
	s.defaultFilename = "juju-backup-<date>-<time>.tar.gz"
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
	client.archive = ioutil.NopCloser(bytes.NewBufferString(s.data))
	return client
}

func (s *createSuite) checkDownloadStd(c *gc.C, ctx *cmd.Context) {
	c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, MetaResultString)

	out := ctx.Stderr.(*bytes.Buffer).String()

	parts := strings.Split(out, "\n")
	i := 0
	if s.command.KeepCopy {
		c.Assert(parts, gc.HasLen, 3)
		c.Check(parts[0], gc.Equals, s.metaresult.ID)
		i = 1
	} else {
		c.Assert(parts, gc.HasLen, 2)
	}

	// Check the download message.
	parts = strings.Split(parts[i], "downloading to ")
	c.Assert(parts, gc.HasLen, 2)
	c.Assert(parts[0], gc.Equals, "")
	s.filename = parts[1]
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
	keepCopy   bool
	noDownload bool
	notes      string
}

var testCreateBackupArgParsing = []createBackupArgParsing{
	{
		title:      "no args",
		args:       []string{},
		filename:   backups.NotSet,
		keepCopy:   false,
		noDownload: false,
		notes:      "",
	},
	{
		title:      "filename",
		args:       []string{"--filename", "testname"},
		filename:   "testname",
		keepCopy:   false,
		noDownload: false,
		notes:      "",
	},
	{
		title:      "filename flag, no name",
		args:       []string{"--filename"},
		errMatch:   "flag needs an argument: --filename",
		filename:   backups.NotSet,
		keepCopy:   false,
		noDownload: false,
		notes:      "",
	},
	{
		title:      "filename && no-download",
		args:       []string{"--filename", "testname", "--no-download"},
		errMatch:   "cannot mix --no-download and --filename",
		filename:   backups.NotSet,
		keepCopy:   false,
		noDownload: false,
		notes:      "",
	},
	{
		title:      "keep-copy",
		args:       []string{"--keep-copy"},
		errMatch:   "",
		filename:   backups.NotSet,
		keepCopy:   true,
		noDownload: false,
		notes:      "",
	},
	{
		title:      "notes",
		args:       []string{"note for the backup"},
		errMatch:   "",
		filename:   backups.NotSet,
		keepCopy:   false,
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
			c.Assert(s.command.KeepCopy, gc.Equals, test.keepCopy)
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
	client.CheckArgs(c, "", "false", "false", "filename")
	s.checkDownload(c, ctx)
	c.Check(s.command.Filename, gc.Equals, backups.NotSet)
}

func (s *createSuite) TestDefaultV1(c *gc.C) {
	s.apiVersion = 1
	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand)
	c.Assert(err, jc.ErrorIsNil)

	client.CheckCalls(c, "Create", "Download")
	client.CheckArgs(c, "", "true", "false", "spam")
	c.Assert(s.command.KeepCopy, jc.IsTrue)
	s.checkDownload(c, ctx)
	c.Check(s.command.Filename, gc.Equals, backups.NotSet)
}

func (s *createSuite) TestDefaultQuiet(c *gc.C) {
	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--quiet")
	c.Assert(err, jc.ErrorIsNil)

	client.CheckCalls(c, "Create", "Download")
	client.CheckArgs(c, "", "false", "false", "filename")

	c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
	c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, "")
}

func (s *createSuite) TestNotes(c *gc.C) {
	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "test notes")
	c.Assert(err, jc.ErrorIsNil)

	client.CheckCalls(c, "Create", "Download")
	client.CheckArgs(c, "test notes", "false", "false", "filename")
	s.checkDownload(c, ctx)
}

func (s *createSuite) TestFilename(c *gc.C) {
	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--filename", "backup.tgz")
	c.Assert(err, jc.ErrorIsNil)

	client.CheckCalls(c, "Create", "Download")
	client.CheckArgs(c, "", "false", "false", "filename")
	s.checkDownload(c, ctx)
	c.Check(s.command.Filename, gc.Equals, "backup.tgz")
}

func (s *createSuite) TestNoDownload(c *gc.C) {
	client := s.setSuccess()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--no-download")
	c.Assert(err, jc.ErrorIsNil)

	client.CheckCalls(c, "Create")
	client.CheckArgs(c, "", "true", "true")
	out := MetaResultString
	s.checkStd(c, ctx, out, "WARNING "+backups.DownloadWarning+"\n"+s.metaresult.ID+"\n")
	c.Check(s.command.Filename, gc.Equals, backups.NotSet)
}

func (s *createSuite) TestKeepCopy(c *gc.C) {
	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--keep-copy")
	c.Assert(err, jc.ErrorIsNil)

	client.CheckCalls(c, "Create", "Download")
	client.CheckArgs(c, "", "true", "false", "filename")

	s.checkDownload(c, ctx)
}

func (s *createSuite) TestKeepCopyV1Fail(c *gc.C) {
	s.apiVersion = 1
	s.setDownload()
	_, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--keep-copy")

	c.Assert(err, gc.ErrorMatches, "--keep-copy is not supported by this controller")
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
