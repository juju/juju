// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/cmd/modelcmd"
	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
)

type createSuite struct {
	BaseBackupsSuite
	wrappedCommand cmd.Command
	command        *backups.CreateCommand

	defaultFilename string
}

func TestCreateSuite(t *testing.T) {
	tc.Run(t, &createSuite{})
}

func (s *createSuite) SetUpTest(c *tc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.wrappedCommand, s.command = backups.NewCreateCommandForTest(s.store)
	s.defaultFilename = "juju-backup-00010101-000000.tar.gz"
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

// checkDownload verifies that a successful run reports both saved files.
// It returns the decoded assertion, with the checksum already
// cross-checked against the archive's actual digest.
func (s *createSuite) checkDownload(c *tc.C, ctx *cmd.Context, filename string) backups.Assertion {
	c.Check(cmdtesting.Stderr(ctx), tc.Contains, filename+" ")
	c.Check(cmdtesting.Stderr(ctx), tc.Contains, filename+".assert")
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, "")

	s.filename = filename
	s.checkArchive(c)
	return s.checkAssertion(c, filename)
}

// checkAssertion decodes the assertion saved beside the archive and
// cross-checks its checksum against the archive's actual digest.
func (s *createSuite) checkAssertion(c *tc.C, filename string) backups.Assertion {
	archive, err := os.ReadFile(filename)
	c.Assert(err, tc.ErrorIsNil)
	sum := sha256.Sum256(archive)

	assertion := s.readAssertion(c, filename+".assert")
	// The checksum keeps the controller's hex spelling, so compare
	// without case.
	c.Check(strings.EqualFold(assertion.Checksum, corebackups.ChecksumSHA256(hex.EncodeToString(sum[:]))), tc.IsTrue)
	c.Check(assertion.ChecksumFormat, tc.Equals, corebackups.ChecksumFormatSHA256)
	return assertion
}

func (s *createSuite) checkNoAssertion(c *tc.C, filename string) {
	_, err := os.Stat(filename + ".assert")
	c.Check(err, tc.Satisfies, os.IsNotExist)
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
	assertion := s.checkDownload(c, ctx, s.defaultFilename)
	c.Check(assertion.Notes, tc.Equals, "")
	c.Check(s.command.Filename, tc.Equals, backups.NotSet)
}

// TestDefaultQuiet verifies that quiet mode is unchanged in effect: no
// output at all, and the assertion is still created.
func (s *createSuite) TestDefaultQuiet(c *tc.C) {
	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.createCommandForGlobalOptionTesting(s.wrappedCommand), "create-backup", "--quiet")
	c.Assert(err, tc.ErrorIsNil)

	client.Check(c, "", "Create")
	client.CheckArgs(c, "")

	c.Check(ctx.Stderr.(*bytes.Buffer).String(), tc.Equals, "")
	c.Check(ctx.Stdout.(*bytes.Buffer).String(), tc.Equals, "")

	s.filename = s.defaultFilename
	s.checkArchive(c)
	s.checkAssertion(c, s.defaultFilename)
}

// TestNotes verifies that arbitrary notes — colons, quotes, Unicode and
// embedded newlines that resemble assertion keys — survive the YAML
// assertion intact.
func (s *createSuite) TestNotes(c *tc.C) {
	notes := "checked: yes\nsay \"hi\" — café\nchecksum: sha256-not-a-real-digest"
	client := s.setDownload()
	client.metaresult.Notes = notes
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, notes)
	c.Assert(err, tc.ErrorIsNil)

	client.Check(c, notes, "Create")
	client.CheckArgs(c, notes)
	assertion := s.checkDownload(c, ctx, s.defaultFilename)
	c.Check(assertion.Notes, tc.Equals, notes)
}

func (s *createSuite) TestFilename(c *tc.C) {
	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--filename", "backup.tgz")
	c.Assert(err, tc.ErrorIsNil)

	client.Check(c, "", "Create")
	client.CheckArgs(c, "")
	s.checkDownload(c, ctx, "backup.tgz")
	c.Check(s.command.Filename, tc.Equals, "backup.tgz")
}

// TestNestedFilename verifies that an archive requested inside a
// subdirectory keeps its assertion beside it there.
func (s *createSuite) TestNestedFilename(c *tc.C) {
	s.setDownload()
	c.Assert(os.MkdirAll("nested/backups", 0o755), tc.ErrorIsNil)
	filename := "nested/backups/custom.tgz"

	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--filename", filename)
	c.Assert(err, tc.ErrorIsNil)
	s.checkDownload(c, ctx, filename)
}

// TestFullMetadataPreserved verifies that every summary value the
// controller reports survives the YAML encoding: versions with build
// metadata, timestamps, identities and counts.
func (s *createSuite) TestFullMetadataPreserved(c *tc.C) {
	started := time.Date(2026, 10, 30, 12, 34, 56, 0, time.UTC)
	finished := started.Add(2 * time.Minute)
	stored := finished.Add(time.Second)
	// The archive is named after the recorded start time.
	filename := started.Format(corebackups.FilenameTemplate)

	client := s.setDownload()
	client.metaresult.FormatVersion = 2
	client.metaresult.Size = 12345
	client.metaresult.Stored = stored
	client.metaresult.Started = started
	client.metaresult.Finished = finished
	client.metaresult.ControllerUUID = "9d0e1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b"
	client.metaresult.HANodes = 3
	client.metaresult.Model = "1f2e3d4c-5b6a-7988-9900-aabbccddeeff"
	client.metaresult.Machine = "0"
	client.metaresult.Hostname = "juju-9d0e1a-machine-0"
	client.metaresult.Version = semversion.MustParse("4.1.2.1")
	client.metaresult.Base = "ubuntu@24.04"

	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand)
	c.Assert(err, tc.ErrorIsNil)
	client.CheckCalls(c, "Create")
	assertion := s.checkDownload(c, ctx, filename)

	c.Check(assertion.FormatVersion, tc.Equals, int64(2))
	c.Check(assertion.Size, tc.Equals, int64(12345))
	c.Check(assertion.Stored.Equal(stored), tc.IsTrue)
	c.Check(assertion.Started.Equal(started), tc.IsTrue)
	c.Check(assertion.Finished.Equal(finished), tc.IsTrue)
	c.Check(assertion.ControllerUUID, tc.Equals, "9d0e1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b")
	c.Check(assertion.HANodes, tc.Equals, int64(3))
	c.Check(assertion.ModelUUID, tc.Equals, "1f2e3d4c-5b6a-7988-9900-aabbccddeeff")
	c.Check(assertion.MachineID, tc.Equals, "0")
	c.Check(assertion.Hostname, tc.Equals, "juju-9d0e1a-machine-0")
	c.Check(assertion.JujuVersion, tc.Equals, semversion.MustParse("4.1.2.1"))
	c.Check(assertion.Base, tc.Equals, "ubuntu@24.04")
}

// TestUnverifiableChecksumFormat verifies that a controller reporting a
// checksum format this client cannot verify — an older controller still
// recording SHA-1, or one predating the format field — fails the
// download and keeps no archive whose integrity is unconfirmed, and
// that no assertion is created.
func (s *createSuite) TestUnverifiableChecksumFormat(c *tc.C) {
	for _, format := range []string{"SHA-1, base64 encoded", ""} {
		c.Logf("checksum format %q", format)
		client := s.setDownload()
		client.metaresult.ChecksumFormat = format
		client.metaresult.Checksum = "dGhpcyBpcyBub3QgYSBzaGEyNTYgc3Vt"

		_, err := cmdtesting.RunCommand(c, s.wrappedCommand)
		c.Assert(err, tc.ErrorMatches,
			`controller reported checksum format ".*"; the downloaded archive cannot be verified`)
		client.CheckCalls(c, "Create")

		// The unverifiable archive is removed, never kept or renamed.
		_, err = os.Stat("juju-backup-00010101-000000.tar.gz")
		c.Check(err, tc.Satisfies, os.IsNotExist)
		_, err = os.Stat("juju-backup-00010101-000000.tar.gz.corrupt")
		c.Check(err, tc.Satisfies, os.IsNotExist)
		s.checkNoAssertion(c, "juju-backup-00010101-000000.tar.gz")
	}
}

// TestChecksumUppercase verifies that the checksum comparison tolerates
// hex case: an uppercase SHA-256 checksum still verifies the download,
// and the recorded assertion checksum keeps the controller's spelling.
func (s *createSuite) TestChecksumUppercase(c *tc.C) {
	client := s.setDownload()
	sum := sha256.Sum256([]byte(s.data))
	lower := hex.EncodeToString(sum[:])
	client.metaresult.Checksum = "sha256-" + strings.ToUpper(lower)

	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand)
	c.Assert(err, tc.ErrorIsNil)
	client.CheckCalls(c, "Create")
	assertion := s.checkDownload(c, ctx, s.defaultFilename)
	c.Check(assertion.Checksum, tc.Equals, "sha256-"+strings.ToUpper(lower))
}

// TestCreateOnContainerController pins the enablement of create-backup
// on container controllers: the command used to refuse non-IAAS
// controller models outright.
func (s *createSuite) TestCreateOnContainerController(c *tc.C) {
	models := s.store.Models["arthur"]
	details := models.Models["admin/controller"]
	details.ModelType = model.CAAS
	models.Models["admin/controller"] = details
	s.store.Models["arthur"] = models

	client := s.setDownload()
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand)
	c.Assert(err, tc.ErrorIsNil)
	client.CheckCalls(c, "Create")
	s.checkDownload(c, ctx, s.defaultFilename)
}

func (s *createSuite) TestChecksumMismatch(c *tc.C) {
	client := s.setDownload()
	client.metaresult.Checksum = "wrong-checksum"

	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand)
	c.Assert(err, tc.ErrorMatches, `checksum mismatch for downloaded backup .*`)
	client.CheckCalls(c, "Create")

	// The assertion must not be created: the archive was never
	// verified, so no assertion can describe it.
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, "")
	s.checkNoAssertion(c, "juju-backup-00010101-000000.tar.gz")

	// The corrupt archive is kept under a suffix for inspection.
	_, err = os.Stat("juju-backup-00010101-000000.tar.gz")
	c.Check(err, tc.Satisfies, os.IsNotExist)
	data, err := os.ReadFile("juju-backup-00010101-000000.tar.gz.corrupt")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, s.data)
}

// TestStaleAssertionSurvivesFailedRerun verifies that a later failed
// backup to the same filename leaves the assertion from an earlier
// successful run in place: the command replaces the assertion only
// with a newly written one, never with nothing.
func (s *createSuite) TestStaleAssertionSurvivesFailedRerun(c *tc.C) {
	client := s.setDownload()
	client.metaresult.Notes = "first backup"
	_, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--filename", "backup.tgz")
	c.Assert(err, tc.ErrorIsNil)
	first := s.checkAssertion(c, "backup.tgz")
	c.Check(first.Notes, tc.Equals, "first backup")

	// A second run reporting a checksum that does not match the
	// archive fails after the download: the archive is kept under
	// .corrupt for inspection, and the earlier assertion is untouched.
	client.metaresult.Checksum = "sha256-" + strings.Repeat("f", 64)
	_, err = cmdtesting.RunCommand(c, s.wrappedCommand, "--filename", "backup.tgz")
	c.Assert(err, tc.ErrorMatches, `checksum mismatch for downloaded backup .*`)
	client.CheckCalls(c, "Create", "Create")

	_, err = os.Stat("backup.tgz")
	c.Check(err, tc.Satisfies, os.IsNotExist)
	data, err := os.ReadFile("backup.tgz.corrupt")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, s.data)

	stale := s.readAssertion(c, "backup.tgz.assert")
	c.Check(stale.Checksum, tc.Equals, first.Checksum)
	c.Check(stale.Notes, tc.Equals, first.Notes)
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
// for a complete backup, and no assertion either.
func (s *createSuite) TestPartialArchiveRemoved(c *tc.C) {
	client := s.setDownload()
	client.createHook = func() (io.ReadCloser, error) {
		return io.NopCloser(&flakyReader{data: s.data}), nil
	}

	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--filename", "backup.tgz")
	c.Assert(err, tc.ErrorMatches,
		`while copying to local archive file backup.tgz: connection reset by peer`)
	client.CheckCalls(c, "Create")

	// No assertion either: the transfer failed.
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, "")

	_, err = os.Stat("backup.tgz")
	c.Check(err, tc.Satisfies, os.IsNotExist)
	_, err = os.Stat("backup.tgz.corrupt")
	c.Check(err, tc.Satisfies, os.IsNotExist)
	s.checkNoAssertion(c, "backup.tgz")
}

// TestNoChecksum verifies that a response without a checksum, which
// would make the downloaded archive unverifiable, fails closed.
func (s *createSuite) TestNoChecksum(c *tc.C) {
	client := s.setSuccess()

	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand)
	c.Assert(err, tc.ErrorMatches,
		`controller returned no checksum for the backup, refusing to download it unverified`)
	client.CheckCalls(c, "Create")

	// No assertion: nothing was downloaded.
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, "")
	s.checkNoAssertion(c, s.defaultFilename)
}

// TestAssertionPublicationFailureKeepsArchive verifies that a destination
// directory blocks publication without removing it or the verified archive.
func (s *createSuite) TestAssertionPublicationFailureKeepsArchive(c *tc.C) {
	client := s.setDownload()
	err := os.Mkdir("backup.tgz.assert", 0o755)
	c.Assert(err, tc.ErrorIsNil)

	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--filename", "backup.tgz")
	c.Assert(err, tc.ErrorMatches, `while publishing local assertion file backup.tgz.assert: .*`)
	client.CheckCalls(c, "Create")

	// The verified archive is kept and the obstructing directory is
	// left alone.
	data, err := os.ReadFile("backup.tgz")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, s.data)
	info, err := os.Stat("backup.tgz.assert")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.IsDir(), tc.IsTrue)
	entries, err := os.ReadDir(".")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entries, tc.HasLen, 2)

	c.Check(cmdtesting.Stdout(ctx), tc.Equals, "")
	c.Check(cmdtesting.Stderr(ctx), tc.Equals, "")
}

// hookedCreateTempFS scripts failures while retaining real temporary files.
type hookedCreateTempFS struct {
	modelcmd.Filesystem
	createTemp func(dir, pattern string) (modelcmd.NamedWriteCloser, error)
}

func (f *hookedCreateTempFS) CreateTemp(dir, pattern string) (modelcmd.NamedWriteCloser, error) {
	return f.createTemp(dir, pattern)
}

type failingWriteWriter struct {
	modelcmd.NamedWriteCloser
	err error
}

func (w *failingWriteWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type shortWriter struct {
	modelcmd.NamedWriteCloser
}

func (w *shortWriter) Write(p []byte) (int, error) {
	return w.NamedWriteCloser.Write(p[:len(p)/2])
}

type failingCloseWriter struct {
	modelcmd.NamedWriteCloser
	err error
}

func (w *failingCloseWriter) Close() error {
	_ = w.NamedWriteCloser.Close()
	return w.err
}

// TestAssertionFailureKeepsExistingFiles verifies that creation, write and
// close failures preserve the archive and old assertion without leaking
// temporary files.
func (s *createSuite) TestAssertionFailureKeepsExistingFiles(c *tc.C) {
	failure := errors.New("assertion I/O failure")
	for _, test := range []struct {
		name      string
		createErr bool
		wrap      func(modelcmd.NamedWriteCloser) modelcmd.NamedWriteCloser
		err       error
	}{
		{name: "create", createErr: true, err: failure},
		{
			name: "write",
			wrap: func(file modelcmd.NamedWriteCloser) modelcmd.NamedWriteCloser {
				return &failingWriteWriter{NamedWriteCloser: file, err: failure}
			},
			err: failure,
		},
		{
			name: "short write",
			wrap: func(file modelcmd.NamedWriteCloser) modelcmd.NamedWriteCloser {
				return &shortWriter{NamedWriteCloser: file}
			},
			err: io.ErrShortWrite,
		},
		{
			name: "close",
			wrap: func(file modelcmd.NamedWriteCloser) modelcmd.NamedWriteCloser {
				return &failingCloseWriter{NamedWriteCloser: file, err: failure}
			},
			err: failure,
		},
	} {
		c.Logf("failure during %s", test.name)
		dir := c.MkDir()
		filename := filepath.Join(dir, "backup.tgz")
		assertionPath := filename + ".assert"
		const previousAssertion = "previous assertion"
		c.Assert(os.WriteFile(assertionPath, []byte(previousAssertion), 0o600), tc.ErrorIsNil)
		s.setDownload()
		base := (&modelcmd.FilesystemCommand{}).Filesystem()
		s.command.SetFilesystem(&hookedCreateTempFS{
			Filesystem: base,
			createTemp: func(dir, pattern string) (modelcmd.NamedWriteCloser, error) {
				if test.createErr {
					return nil, failure
				}
				file, err := base.CreateTemp(dir, pattern)
				if err != nil {
					return nil, err
				}
				return test.wrap(file), nil
			},
		})

		ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--filename", filename)
		c.Assert(err, tc.ErrorIs, test.err)
		data, err := os.ReadFile(filename)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(string(data), tc.Equals, s.data)
		data, err = os.ReadFile(assertionPath)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(string(data), tc.Equals, previousAssertion)
		entries, err := os.ReadDir(dir)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(entries, tc.HasLen, 2)
		c.Check(cmdtesting.Stderr(ctx), tc.Equals, "")
	}
}

// TestAssertionLinksKeepArchive verifies that publishing an assertion
// replaces a linked destination without overwriting the verified archive.
func (s *createSuite) TestAssertionLinksKeepArchive(c *tc.C) {
	for _, test := range []struct {
		name string
		link func(string, string) error
	}{
		{name: "symlink", link: os.Symlink},
		{name: "hardlink", link: os.Link},
	} {
		c.Logf("assertion destination: %s", test.name)
		dir := c.MkDir()
		filename := filepath.Join(dir, "backup.tgz")
		c.Assert(os.WriteFile(filename, []byte("old backup"), 0o600), tc.ErrorIsNil)
		c.Assert(test.link(filename, filename+".assert"), tc.ErrorIsNil)
		s.setDownload()

		ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--filename", filename)
		c.Assert(err, tc.ErrorIsNil)
		s.checkDownload(c, ctx, filename)
		info, err := os.Lstat(filename + ".assert")
		c.Assert(err, tc.ErrorIsNil)
		c.Check(info.Mode().IsRegular(), tc.IsTrue)
		entries, err := os.ReadDir(dir)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(entries, tc.HasLen, 2)
	}
}

// TestAssertionReplaced verifies that a second successful backup to the
// same filename replaces the assertion rather than appending to it.
func (s *createSuite) TestAssertionReplaced(c *tc.C) {
	client := s.setDownload()
	client.metaresult.Notes = "first backup"
	_, err := cmdtesting.RunCommand(c, s.wrappedCommand, "--filename", "backup.tgz")
	c.Assert(err, tc.ErrorIsNil)
	assertion := s.checkAssertion(c, "backup.tgz")
	c.Check(assertion.Notes, tc.Equals, "first backup")

	client.metaresult.Notes = "second backup"
	_, err = cmdtesting.RunCommand(c, s.wrappedCommand, "--filename", "backup.tgz")
	c.Assert(err, tc.ErrorIsNil)
	assertion = s.checkAssertion(c, "backup.tgz")
	c.Check(assertion.Notes, tc.Equals, "second backup")

	data, err := os.ReadFile("backup.tgz.assert")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(strings.Count(string(data), "notes:"), tc.Equals, 1)
}

func (s *createSuite) TestError(c *tc.C) {
	s.setFailure("failed!")
	_, err := cmdtesting.RunCommand(c, s.wrappedCommand)
	c.Check(errors.Cause(err), tc.ErrorMatches, "failed!")
}
