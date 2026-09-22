// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
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

func TestCreateSuite(t *testing.T) {
	tc.Run(t, &createSuite{})
}

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
		c.Assert(err, tc.Or(tc.ErrorIsNil, tc.ErrorIs), os.ErrNotExist)
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
	client.data = s.data
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
	c.Cleanup(func() {
		s.filename = ""
	})
}

func (s *createSuite) checkDownload(c *tc.C, ctx *cmd.Context) {
	s.checkDownloadStd(c, ctx)
	s.checkArchive(c)
}

type createBackupArgParsing struct {
	title    string
	args     []string
	errMatch string
	filename string
	notes    string
}

var testCreateBackupArgParsing = []createBackupArgParsing{
	{
		title:    "no args",
		args:     []string{},
		filename: backups.NotSet,
		notes:    "",
	},
	{
		title:    "filename",
		args:     []string{"--filename", "testname"},
		filename: "testname",
		notes:    "",
	},
	{
		title:    "filename flag, no name",
		args:     []string{"--filename"},
		errMatch: "option needs an argument: --filename",
		filename: backups.NotSet,
		notes:    "",
	},
	{
		title:    "notes",
		args:     []string{"note for the backup"},
		errMatch: "",
		filename: backups.NotSet,
		notes:    "note for the backup",
	},
}

func (s *createSuite) TestArgParsing(c *tc.C) {
	for i, test := range testCreateBackupArgParsing {
		c.Logf("%d: %s", i, test.title)
		err := cmdtesting.InitCommand(s.wrappedCommand, test.args)
		if test.errMatch == "" {
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(s.command.Filename, tc.Equals, test.filename)
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

	client.Check(c, "", "Create")
	client.CheckArgs(c, "")
	s.checkDownload(c, ctx)
	c.Check(s.command.Filename, tc.Equals, backups.NotSet)
}

func (s *createSuite) TestDefaultQuiet(c *tc.C) {
	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.createCommandForGlobalOptionTesting(s.wrappedCommand), "create-backup", "--quiet")
	c.Assert(err, tc.ErrorIsNil)

	client.Check(c, "", "Create")
	client.CheckArgs(c, "")

	c.Check(ctx.Stderr.(*bytes.Buffer).String(), tc.Equals, "")
	c.Check(ctx.Stdout.(*bytes.Buffer).String(), tc.Equals, "")
}

func (s *createSuite) TestNotes(c *tc.C) {
	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "test notes")
	c.Assert(err, tc.ErrorIsNil)

	client.Check(c, "test notes", "Create")
	client.CheckArgs(c, "test notes")
	s.checkDownload(c, ctx)
}

func (s *createSuite) TestFilename(c *tc.C) {
	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--filename", "backup.tgz")
	c.Assert(err, tc.ErrorIsNil)

	client.Check(c, "", "Create")
	client.CheckArgs(c, "")
	s.expectedErr = `
Downloaded to backup.tgz
`[1:]
	s.checkDownload(c, ctx)
	c.Check(s.command.Filename, tc.Equals, "backup.tgz")
}

func (s *createSuite) TestChecksumMismatch(c *tc.C) {
	client := s.setDownload()
	client.metaresult.Checksum = "wrong-checksum"

	_, err := cmdtesting.RunCommand(c, s.wrappedCommand)
	c.Assert(err, tc.ErrorMatches, `checksum mismatch for downloaded backup .*`)
	client.CheckCalls(c, "Create")

	// The corrupt archive is kept under a suffix for inspection.
	_, err = os.Stat("juju-backup-00010101-000000.tar.gz")
	c.Check(err, tc.Satisfies, os.IsNotExist)
	data, err := os.ReadFile("juju-backup-00010101-000000.tar.gz.corrupt")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, s.data)
}

// flakyReader returns its data once and then fails, simulating a
// transfer that breaks part way through.
type flakyReader struct {
	data string
	read bool
}

func (r *flakyReader) Read(p []byte) (int, error) {
	if r.read {
		return 0, errors.New("connection reset by peer")
	}
	r.read = true
	return copy(p, r.data), nil
}

// TestPartialArchiveRemoved verifies that a transfer that breaks part
// way through leaves no partial archive behind that could be mistaken
// for a complete backup.
func (s *createSuite) TestPartialArchiveRemoved(c *tc.C) {
	client := s.setDownload()
	client.createHook = func() (io.ReadCloser, error) {
		return io.NopCloser(&flakyReader{data: s.data}), nil
	}

	_, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--filename", "backup.tgz")
	c.Assert(err, tc.ErrorMatches,
		`while copying to local archive file backup.tgz: connection reset by peer`)
	client.CheckCalls(c, "Create")

	_, err = os.Stat("backup.tgz")
	c.Check(err, tc.Satisfies, os.IsNotExist)
	_, err = os.Stat("backup.tgz.corrupt")
	c.Check(err, tc.Satisfies, os.IsNotExist)
}

// TestNoChecksum verifies that a response without a checksum, which
// would make the downloaded archive unverifiable, fails closed.
func (s *createSuite) TestNoChecksum(c *tc.C) {
	client := s.setSuccess()

	_, err := cmdtesting.RunCommand(c, s.wrappedCommand)
	c.Assert(err, tc.ErrorMatches,
		`controller returned no checksum for the backup, refusing to download it unverified`)
	client.CheckCalls(c, "Create")
}

func (s *createSuite) TestError(c *tc.C) {
	s.setFailure("failed!")
	_, err := cmdtesting.RunCommand(c, s.wrappedCommand)
	c.Check(errors.Cause(err), tc.ErrorMatches, "failed!")
}
