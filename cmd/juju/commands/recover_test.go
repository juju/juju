// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/modelcmd"
	corebackups "github.com/juju/juju/core/backups"
	coretesting "github.com/juju/juju/internal/testing"
)

type recoverySuite struct {
	coretesting.BaseSuite
}

func TestRecoverySuite(t *testing.T) { tc.Run(t, &recoverySuite{}) }

// TestInit verifies that argument validation rejects the failing
// shapes in the table, and that both given paths are recorded as
// provided for recovery. An empty archive argument is reported before
// leftover arguments are rejected: it is the more useful error.
func (s *recoverySuite) TestInit(c *tc.C) {
	for _, test := range []struct {
		args    []string
		pattern string
	}{
		{nil, "a backup archive is required"},
		{[]string{"backup.tar.gz"}, "a backup assertion file is required"},
		{[]string{"", "backup.tar.gz.assert"}, "a backup archive is required"},
		{[]string{"backup.tar.gz", ""}, "a backup assertion file is required"},
		{[]string{"", "backup.tar.gz.assert", "extra"}, "a backup archive is required"},
		{[]string{"backup.tar.gz", "backup.tar.gz.assert", "extra"}, "unrecognized args.*"},
		{[]string{"backup.tar.gz", "backup.tar.gz.assert", "--sha256", "bad"}, "(flag|option) provided but not defined.*"},
		{[]string{"backup.tar.gz", "backup.tar.gz.assert", "--credential", "source"}, "(flag|option) provided but not defined.*"},
		{[]string{"backup.tar.gz", "backup.tar.gz.assert", "--config", "ca-cert=override"}, "(flag|option) provided but not defined.*"},
	} {
		err := cmdtesting.InitCommand(newRecoveryCommand(), test.args)
		c.Check(err, tc.ErrorMatches, test.pattern)
	}
	// Both paths are recorded for recovery, as given.
	command := &recoveryCommand{}
	wrapped := modelcmd.Wrap(command, modelcmd.WrapSkipModelFlags, modelcmd.WrapSkipDefaultModel)
	err := cmdtesting.InitCommand(wrapped, []string{"backup.tar.gz", "elsewhere/backup.tar.gz.assert"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(command.archivePath, tc.Equals, "backup.tar.gz")
	c.Check(command.assertionPath, tc.Equals, "elsewhere/backup.tar.gz.assert")
}

// TestRun drives the wrapped recover command through every Run path:
// a missing, empty, malformed or uncloseable assertion file, an
// assertion whose checksum does not match the archive, a missing
// archive, and a valid archive, which reaches this build's terminal
// error. The paths are given relative to the command's working
// directory, so every case also exercises the relative to absolute
// path resolution: the expected messages quote the resolved
// absolute paths.
func (s *recoverySuite) TestRun(c *tc.C) {
	archive := recoveryArchive(c)
	sum := sha256.Sum256(archive)
	checksum := corebackups.ChecksumSHA256(hex.EncodeToString(sum[:]))
	assertionYAML := "checksum: " + checksum +
		"\nchecksum-format: " + corebackups.ChecksumFormatSHA256 + "\n"
	// A well-formed assertion recording some other archive's digest.
	mismatchedAssertion := "checksum: " + corebackups.ChecksumSHA256(strings.Repeat("0", sha256.Size*2)) +
		"\nchecksum-format: " + corebackups.ChecksumFormatSHA256 + "\n"
	// The same archive pinned by an assertion whose digest is recorded
	// in uppercase hex, as create-backup may preserve the controller's
	// spelling.
	uppercaseAssertion := "checksum: sha256-" + strings.ToUpper(hex.EncodeToString(sum[:])) +
		"\nchecksum-format: " + corebackups.ChecksumFormatSHA256 + "\n"
	emptyAssertion := ""
	malformedAssertion := "checksum: [unclosed\n"
	closeErr := errors.New("input/output error")

	const (
		archiveName   = "backup.tar.gz"
		assertionName = "backup.tar.gz.assert"
	)

	for i, test := range []struct {
		summary        string
		assertion      *string
		assertionIsDir bool
		archive        bool
		archiveIsDir   bool
		archiveEmpty   bool
		newCommand     func() cmd.Command
		expect         func(archivePath, assertionPath string) string
	}{{
		summary: "assertion file is missing",
		expect: func(_, assertionPath string) string {
			return `reading backup assertion "` + regexp.QuoteMeta(assertionPath) +
				`": open ` + regexp.QuoteMeta(assertionPath) + `: no such file or directory`
		},
	}, {
		summary:   "assertion file is empty",
		assertion: &emptyAssertion,
		expect: func(_, assertionPath string) string {
			return `reading backup assertion "` + regexp.QuoteMeta(assertionPath) +
				`": backup assertion is empty`
		},
	}, {
		summary:   "assertion file is malformed",
		assertion: &malformedAssertion,
		expect: func(_, assertionPath string) string {
			return `reading backup assertion "` + regexp.QuoteMeta(assertionPath) +
				`": cannot parse backup assertion: .*`
		},
	}, {
		summary:        "assertion path is a directory",
		assertionIsDir: true,
		expect: func(_, assertionPath string) string {
			return `reading backup assertion "` + regexp.QuoteMeta(assertionPath) +
				`": cannot parse backup assertion: yaml: input error: read ` +
				regexp.QuoteMeta(assertionPath) + `: is a directory`
		},
	}, {
		summary:   "assertion file fails to close",
		assertion: &assertionYAML,
		newCommand: func() cmd.Command {
			command := &recoveryCommand{}
			command.CanClearCurrentModel = true
			base := command.Filesystem()
			command.SetFilesystem(&hookedOpenFS{
				Filesystem: base,
				open: func(name string) (modelcmd.ReadSeekCloser, error) {
					file, err := base.Open(name)
					if err != nil {
						return nil, err
					}
					return &failingCloseReader{ReadSeekCloser: file, err: closeErr}, nil
				},
			})
			return modelcmd.Wrap(command, modelcmd.WrapSkipModelFlags, modelcmd.WrapSkipDefaultModel)
		},
		expect: func(_, assertionPath string) string {
			return `reading backup assertion "` + regexp.QuoteMeta(assertionPath) +
				`": input/output error`
		},
	}, {
		summary:   "archive is missing",
		assertion: &assertionYAML,
		expect: func(archivePath, _ string) string {
			return `invalid recovery archive: opening archive: open ` +
				regexp.QuoteMeta(archivePath) + `: no such file or directory`
		},
	}, {
		summary:   "archive does not match the assertion checksum",
		assertion: &mismatchedAssertion,
		archive:   true,
		expect: func(_, _ string) string {
			return `invalid recovery archive: archive checksum mismatch: expected sha256 "` +
				corebackups.ChecksumSHA256(strings.Repeat("0", sha256.Size*2)) +
				`", archive is "` + checksum + `"`
		},
	}, {
		summary:      "archive path is a directory",
		assertion:    &assertionYAML,
		archiveIsDir: true,
		expect: func(archivePath, _ string) string {
			return `invalid recovery archive: reading archive: read ` +
				regexp.QuoteMeta(archivePath) + `: is a directory`
		},
	}, {
		summary:      "archive is empty",
		assertion:    &assertionYAML,
		archiveEmpty: true,
		expect: func(_, _ string) string {
			return `invalid recovery archive: reading archive: EOF`
		},
	}, {
		summary:   "valid archive reaches the terminal error",
		assertion: &assertionYAML,
		archive:   true,
		expect: func(_, _ string) string {
			return `controller recovery provisioning is not available in this build`
		},
	}, {
		summary:   "assertion records the checksum in uppercase hex",
		assertion: &uppercaseAssertion,
		archive:   true,
		expect: func(_, _ string) string {
			return `controller recovery provisioning is not available in this build`
		},
	}} {
		c.Logf("test %d: %s", i, test.summary)
		dir := c.MkDir()
		assertionPath := filepath.Join(dir, assertionName)
		if test.assertionIsDir {
			err := os.Mkdir(assertionPath, 0o755)
			c.Assert(err, tc.ErrorIsNil)
		} else if test.assertion != nil {
			err := os.WriteFile(assertionPath, []byte(*test.assertion), 0o600)
			c.Assert(err, tc.ErrorIsNil)
		}
		archivePath := filepath.Join(dir, archiveName)
		var archiveInfo os.FileInfo
		if test.archiveIsDir {
			err := os.Mkdir(archivePath, 0o755)
			c.Assert(err, tc.ErrorIsNil)
		} else if test.archiveEmpty {
			err := os.WriteFile(archivePath, nil, 0o600)
			c.Assert(err, tc.ErrorIsNil)
		} else if test.archive {
			err := os.WriteFile(archivePath, archive, 0o600)
			c.Assert(err, tc.ErrorIsNil)
			archiveInfo, err = os.Stat(archivePath)
			c.Assert(err, tc.ErrorIsNil)
		}

		newCmd := newRecoveryCommand
		if test.newCommand != nil {
			newCmd = test.newCommand
		}
		_, err := cmdtesting.RunCommandInDir(c, newCmd(),
			[]string{archiveName, assertionName}, dir)
		c.Check(err, tc.ErrorMatches, test.expect(archivePath, assertionPath))

		// The command never writes its inputs: the archive, when
		// present, is byte-identical afterwards with its mode
		// unchanged, and the workspace neither gains nor loses files.
		want := make([]string, 0, 2)
		if test.archive || test.archiveIsDir || test.archiveEmpty {
			want = append(want, archiveName)
		}
		if test.archive {
			data, err := os.ReadFile(archivePath)
			c.Assert(err, tc.ErrorIsNil)
			c.Check(data, tc.DeepEquals, archive)
			info, err := os.Stat(archivePath)
			c.Assert(err, tc.ErrorIsNil)
			c.Check(info.Mode(), tc.Equals, archiveInfo.Mode())
		}
		if test.assertion != nil || test.assertionIsDir {
			want = append(want, assertionName)
		}
		entries, err := os.ReadDir(dir)
		c.Assert(err, tc.ErrorIsNil)
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		sort.Strings(names)
		sort.Strings(want)
		c.Check(names, tc.DeepEquals, want)
	}
}

// hookedOpenFS delegates every filesystem operation, except Open, to
// the wrapped filesystem, so a test can return a handle that behaves
// in ways a regular file cannot, such as failing to close.
type hookedOpenFS struct {
	modelcmd.Filesystem
	open func(name string) (modelcmd.ReadSeekCloser, error)
}

// Open implements Filesystem.
func (f *hookedOpenFS) Open(name string) (modelcmd.ReadSeekCloser, error) {
	return f.open(name)
}

// failingCloseReader stands in for the assertion file, serving its
// contents normally but failing to close, as a failing filesystem
// can report.
type failingCloseReader struct {
	modelcmd.ReadSeekCloser
	err error
}

// Close implements io.Closer; the wrapped handle is closed regardless
// and the scripted failure is reported.
func (r *failingCloseReader) Close() error {
	_ = r.ReadSeekCloser.Close()
	return r.err
}

// recoveryArchive renders a minimal valid recovery archive: the
// provenance metadata, a controller dump recording one controller
// model on an lxd cloud, that model's database dump, and the
// data-directory files archive. It is the smallest archive the
// recover command's validation accepts, so reaching the terminal
// error proves the whole read and verify path ran.
func recoveryArchive(c *tc.C) []byte {
	const (
		controllerUUID      = "c0ffee00-1111-2222-3333-444455556666"
		controllerModelUUID = "c0ffee00-aaaa-2222-3333-444455556666"
	)
	metadata := `{` +
		`"ID":"20261104-120000.aabb-ccdd-eeff",` +
		`"FormatVersion":2,` +
		`"Stored":"0001-01-01T00:00:00Z",` +
		`"Started":"2026-11-04T12:00:00Z",` +
		`"Finished":"2026-11-04T12:00:34Z",` +
		`"Notes":"",` +
		`"ModelUUID":"` + controllerModelUUID + `",` +
		`"Machine":"0",` +
		`"Hostname":"myhost",` +
		`"Version":"4.1.0",` +
		`"ControllerUUID":"` + controllerUUID + `",` +
		`"HANodes":1,` +
		`"ControllerMachineID":"0",` +
		`"ControllerMachineInstanceID":"inst-10101010"` +
		`}` + "\n"
	controllerDump := `version: 4.1.0
payload:
  controller:
  - uuid: ` + controllerUUID + `
    model_uuid: ` + controllerModelUUID + `
    ca_cert: source-ca-cert
    ca_private_key: source-ca-key
  cloud:
  - uuid: cloud-lxd
    name: lxd
    cloud_type_id: 1
  cloud_type:
  - id: 1
    type: lxd
  model_type:
  - id: 0
    type: iaas
  controller_config:
  - key: controller-name
    value: source-ctrl
  model:
  - uuid: ` + controllerModelUUID + `
    name: controller
    cloud_uuid: cloud-lxd
    model_type_id: 0
`
	files := map[string][]byte{
		"juju-backup/metadata.json":                                []byte(metadata),
		"juju-backup/dump/controller.yaml":                         []byte(controllerDump),
		"juju-backup/dump/models/" + controllerModelUUID + ".yaml": []byte("payload: {}\n"),
		"juju-backup/root.tar":                                     []byte("blobs"),
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data := files[name]
		hdr := &tar.Header{
			Name:     name,
			Mode:     0o600,
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
		}
		c.Assert(tw.WriteHeader(hdr), tc.ErrorIsNil)
		_, err := tw.Write(data)
		c.Assert(err, tc.ErrorIsNil)
	}
	c.Assert(tw.Close(), tc.ErrorIsNil)
	c.Assert(gz.Close(), tc.ErrorIsNil)
	return buf.Bytes()
}
