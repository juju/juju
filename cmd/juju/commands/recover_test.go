// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/api/jujuclient"
	"github.com/juju/juju/caas"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/arch"
	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
	coretesting "github.com/juju/juju/internal/testing"
)

type recoverySuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

func (s *recoverySuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
}

func (s *recoverySuite) TearDownTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
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
		{[]string{"", "backup.assert"}, "a backup archive is required"},
		{[]string{"backup.tar.gz", ""}, "a backup assertion file is required"},
		{[]string{"", "backup.assert", "extra"}, "a backup archive is required"},
		{[]string{"backup.tar.gz", "backup.assert", "extra"}, "unrecognized args.*"},
		{[]string{"backup.tar.gz", "backup.assert", "--sha256", "bad"}, "(flag|option) provided but not defined.*"},
		{[]string{"backup.tar.gz", "backup.assert", "--credential", "source"}, "(flag|option) provided but not defined.*"},
		{[]string{"backup.tar.gz", "backup.assert", "--config", "ca-cert=override"}, "(flag|option) provided but not defined.*"},
	} {
		err := cmdtesting.InitCommand(newRecoveryCommand(), test.args)
		c.Check(err, tc.ErrorMatches, test.pattern)
	}
	// Both paths are recorded for recovery, as given.
	command := &recoveryCommand{}
	wrapped := modelcmd.Wrap(command, modelcmd.WrapSkipModelFlags, modelcmd.WrapSkipDefaultModel)
	err := cmdtesting.InitCommand(wrapped, []string{"backup.tar.gz", "elsewhere/backup.assert"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(command.archivePath, tc.Equals, "backup.tar.gz")
	c.Check(command.assertionPath, tc.Equals, "elsewhere/backup.assert")
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
		assertionName = "backup.assert"
		// TODO(recovery): expect success once the controller is finalised.
		terminalError = "controller recovery database import is not available in this build; " +
			"initialised replacement database on instance test-instance"
	)
	newProvisionedCommand := func() cmd.Command {
		return newRecoveryCommandWithProvisioner(func(
			_ *cmd.Context, _ jujuclient.ClientStore, _ string, _ *recovery.ArchiveInfo,
		) error {
			return errors.New(terminalError)
		})
	}

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
			return terminalError
		},
	}, {
		summary:   "assertion records the checksum in uppercase hex",
		assertion: &uppercaseAssertion,
		archive:   true,
		expect: func(_, _ string) string {
			return terminalError
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

		newCmd := newProvisionedCommand
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

// TestRunPassesArchiveInfoToProvisioner checks the handoff after validation.
func (s *recoverySuite) TestRunPassesArchiveInfoToProvisioner(c *tc.C) {
	archive := recoveryArchive(c)
	sum := sha256.Sum256(archive)
	checksum := corebackups.ChecksumSHA256(hex.EncodeToString(sum[:]))
	assertion := "checksum: " + checksum +
		"\nchecksum-format: " + corebackups.ChecksumFormatSHA256 + "\n"

	dir := c.MkDir()
	c.Assert(os.WriteFile(filepath.Join(dir, "backup.tar.gz"), archive, 0o600), tc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(dir, "backup.assert"), []byte(assertion), 0o600), tc.ErrorIsNil)

	terminalErr := errors.New("recovery stopped after allocation")
	var got *recovery.ArchiveInfo
	command := newRecoveryCommandWithProvisioner(func(
		_ *cmd.Context, _ jujuclient.ClientStore, archivePath string, info *recovery.ArchiveInfo,
	) error {
		c.Check(archivePath, tc.Equals, filepath.Join(dir, "backup.tar.gz"))
		got = info
		return terminalErr
	})
	_, err := cmdtesting.RunCommandInDir(
		c, command, []string{"backup.tar.gz", "backup.assert"}, dir,
	)
	c.Assert(err, tc.ErrorIs, terminalErr)
	c.Assert(got, tc.NotNil)
	c.Check(got.ControllerUUID, tc.Equals, "c0ffee00-1111-2222-3333-444455556666")
	c.Check(got.ControllerName, tc.Equals, "source-ctrl")
	c.Check(got.ControllerModelUUID, tc.Equals, "c0ffee00-aaaa-2222-3333-444455556666")
	c.Check(got.CACert, tc.Equals, "source-ca-cert")
	c.Check(got.SystemIdentity, tc.Equals, "source-system-identity")
	c.Check(got.Checksum, tc.Equals, checksum)
	c.Check(got.AgentVersion.String(), tc.Equals, "4.1.0")
	c.Check(got.SourceBase, tc.Equals, "ubuntu@24.04/stable")
	c.Check(got.SourceArch, tc.Equals, "amd64")
	c.Check(got.MachineName, tc.Equals, "0")
	c.Check(got.CloudName, tc.Equals, "lxd")
	c.Check(got.CloudType, tc.Equals, "lxd")
}

func (s *recoverySuite) TestRunPassesKubernetesArchiveInfoToProvisioner(c *tc.C) {
	archive := recoveryArchiveForControllerModel(c, "myk8s", "kubernetes", "caas")
	sum := sha256.Sum256(archive)
	checksum := corebackups.ChecksumSHA256(hex.EncodeToString(sum[:]))
	assertion := "checksum: " + checksum +
		"\nchecksum-format: " + corebackups.ChecksumFormatSHA256 + "\n"

	dir := c.MkDir()
	c.Assert(os.WriteFile(filepath.Join(dir, "backup.tar.gz"), archive, 0o600), tc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(dir, "backup.assert"), []byte(assertion), 0o600), tc.ErrorIsNil)

	terminalErr := errors.New("recovery stopped after allocation")
	var got *recovery.ArchiveInfo
	command := newRecoveryCommandWithProvisioner(func(
		_ *cmd.Context, _ jujuclient.ClientStore, archivePath string, info *recovery.ArchiveInfo,
	) error {
		c.Check(archivePath, tc.Equals, filepath.Join(dir, "backup.tar.gz"))
		got = info
		return terminalErr
	})
	_, err := cmdtesting.RunCommandInDir(
		c, command, []string{"backup.tar.gz", "backup.assert"}, dir,
	)
	c.Assert(err, tc.ErrorIs, terminalErr)
	c.Assert(got, tc.NotNil)
	c.Check(got.ControllerUUID, tc.Equals, "c0ffee00-1111-2222-3333-444455556666")
	c.Check(got.ControllerName, tc.Equals, "source-ctrl")
	c.Check(got.CloudName, tc.Equals, "myk8s")
	c.Check(got.CloudType, tc.Equals, "kubernetes")
}

// TestRecoveryConfigs checks that the replacement controller and
// controller-model configuration are built from the archived identity,
// CA material and agent version, with the controller model named after
// the bootstrap controller model.
func (s *recoverySuite) TestRecoveryConfigs(c *tc.C) {
	info := &recovery.ArchiveInfo{
		ControllerUUID:      "c0ffee00-1111-2222-3333-444455556666",
		ControllerName:      "source-ctrl",
		ControllerModelUUID: "c0ffee00-aaaa-2222-3333-444455556666",
		CACert:              coretesting.CACert,
		AgentVersion:        semversion.MustParse("4.1.0"),
		CloudType:           "lxd",
	}
	controllerConfig, modelConfig, err := recoveryConfigs(c.Context(), info, recoveryProvider(c, "lxd"))
	c.Assert(err, tc.ErrorIsNil)

	c.Check(controllerConfig.ControllerUUID(), tc.Equals, info.ControllerUUID)
	caCert, ok := controllerConfig.CACert()
	c.Check(ok, tc.IsTrue)
	c.Check(caCert, tc.Equals, coretesting.CACert)
	c.Check(controllerConfig.ControllerName(), tc.Equals, info.ControllerName)

	c.Check(modelConfig.Name(), tc.Equals, bootstrap.ControllerModelName)
	c.Check(modelConfig.Type(), tc.Equals, info.CloudType)
	c.Check(modelConfig.UUID(), tc.Equals, info.ControllerModelUUID)
	agentVersion, ok := modelConfig.AgentVersion()
	c.Check(ok, tc.IsTrue)
	c.Check(agentVersion, tc.Equals, info.AgentVersion)
}

// TestProvisionRecoveryRequiresSourceBase checks that a machine archive
// which does not record a usable controller base is rejected before any
// cloud access.
func (s *recoverySuite) TestProvisionRecoveryRequiresSourceBase(c *tc.C) {
	for _, sourceBase := range []string{"", "not-a-base"} {
		err := provisionRecovery(cmdtesting.Context(c), nil, "", &recovery.ArchiveInfo{
			CloudType:    "lxd",
			AgentVersion: version.Current,
			SourceBase:   sourceBase,
			SourceArch:   "amd64",
		})
		c.Check(err, tc.ErrorMatches, "archive does not record a usable controller base.*")
	}
}

// TestProvisionRecoveryRequiresSourceArch checks that an archive which does
// not record a usable controller architecture — empty, or a value no Juju
// release supports — is rejected before any cloud access, so recovery never
// provisions from the bootstrap client's host architecture.
func (s *recoverySuite) TestProvisionRecoveryRequiresSourceArch(c *tc.C) {
	for _, sourceArch := range []string{"", "not-an-arch"} {
		err := provisionRecovery(cmdtesting.Context(c), nil, "", &recovery.ArchiveInfo{
			CloudType:    "lxd",
			AgentVersion: version.Current,
			SourceBase:   "ubuntu@24.04",
			SourceArch:   sourceArch,
		})
		c.Check(err, tc.ErrorMatches, `archive does not record a usable controller architecture: ".*"`)
	}
}

// TestProvisionRecoveryMachineAcceptsArchivedAgentVersion checks that the client
// does not have to run the archived agent version: the replacement agent
// is taken from packaged tools or, failing that, from the archive itself.
// A missing machine name makes the command stop after the version check,
// before looking for SSH keys or contacting a cloud.
func (s *recoverySuite) TestProvisionRecoveryMachineAcceptsArchivedAgentVersion(c *tc.C) {
	for _, test := range []struct {
		summary  string
		current  string
		archived string
	}{{
		summary:  "pre-release build",
		current:  "4.1-beta3",
		archived: "4.1-beta3.1",
	}, {
		summary:  "release patch",
		current:  "4.1.0",
		archived: "4.1.7",
	}} {
		c.Logf("%s", test.summary)
		s.PatchValue(&version.Current, semversion.MustParse(test.current))
		err := provisionRecovery(cmdtesting.Context(c), nil, "", &recovery.ArchiveInfo{
			CloudType:    "lxd",
			AgentVersion: semversion.MustParse(test.archived),
			SourceBase:   "ubuntu@24.04",
			SourceArch:   "amd64",
		})
		c.Check(err, tc.ErrorMatches, "archive does not record a usable controller machine")
	}
}

// TestProvisionRecoveryMachineRejectsOtherVersionLine checks that the client
// still refuses an archive from another version line, before any cloud
// access: the client writes the parameters the archived agent consumes
// and keeps talking to the recovered controller, so major.minor and the
// pre-release tag must match.
func (s *recoverySuite) TestProvisionRecoveryMachineRejectsOtherVersionLine(c *tc.C) {
	for _, test := range []struct {
		summary  string
		current  string
		archived string
	}{{
		summary:  "different minor",
		current:  "4.1.0",
		archived: "4.2.0",
	}, {
		summary:  "different major",
		current:  "4.1.0",
		archived: "5.1.0",
	}, {
		summary:  "different pre-release tag",
		current:  "4.1-beta3",
		archived: "4.1-beta4",
	}, {
		summary:  "pre-release and release",
		current:  "4.1-beta3",
		archived: "4.1.0",
	}} {
		c.Logf("%s", test.summary)
		s.PatchValue(&version.Current, semversion.MustParse(test.current))
		err := provisionRecovery(cmdtesting.Context(c), nil, "", &recovery.ArchiveInfo{
			CloudType:    "lxd",
			CloudName:    "not-a-cloud",
			AgentVersion: semversion.MustParse(test.archived),
			SourceBase:   "ubuntu@24.04",
			SourceArch:   "amd64",
			MachineName:  "0",
		})
		c.Check(err, tc.ErrorMatches,
			"archive was created by agent version .* but this binary is .*; "+
				"recovery requires the same major.minor version line and pre-release tag")
	}
}

// TestProvisionRecoveryRequiresMachineName checks that an archive which does
// not record a usable controller machine is rejected before any cloud access.
func (s *recoverySuite) TestProvisionRecoveryRequiresMachineName(c *tc.C) {
	for _, machineName := range []string{"", "not-a-machine"} {
		err := provisionRecovery(cmdtesting.Context(c), nil, "", &recovery.ArchiveInfo{
			CloudType:    "lxd",
			AgentVersion: version.Current,
			SourceBase:   "ubuntu@24.04",
			SourceArch:   "amd64",
			MachineName:  machineName,
		})
		c.Check(err, tc.ErrorMatches, "archive does not record a usable controller machine")
	}
}

// The recovery client must request the archived platform even when its
// own architecture differs from the controller's.
func (s *recoverySuite) TestAllocationTargetFromArchive(c *tc.C) {
	nonClientArch := "arm64"
	if arch.HostArch() == "arm64" {
		nonClientArch = "amd64"
	}
	info := &recovery.ArchiveInfo{
		AgentVersion: version.Current,
		SourceBase:   "ubuntu@24.04/stable",
		SourceArch:   nonClientArch,
	}

	params, err := recoveryParamsFromArchive(info)
	c.Assert(err, tc.ErrorIsNil)
	wantBase, err := base.ParseBaseFromString("ubuntu@24.04/stable")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(params.BootstrapBase, tc.Equals, wantBase)
	c.Check(params.SupportedBootstrapBases, tc.DeepEquals, []base.Base{wantBase})
	c.Assert(params.BootstrapConstraints.Arch, tc.NotNil)
	c.Check(*params.BootstrapConstraints.Arch, tc.Equals, nonClientArch)
	c.Check(params.Arch, tc.Equals, nonClientArch)
	c.Check(params.AgentVersion, tc.Equals, info.AgentVersion)
}

func (s *recoverySuite) TestProvisionRecoveryKubernetesRequiresExactAgentVersion(c *tc.C) {
	for _, test := range []struct{ current, archived string }{
		{"4.1.0", "4.1.7"},
		{"4.1-beta3", "4.1-beta3.1"},
	} {
		s.PatchValue(&version.Current, semversion.MustParse(test.current))
		err := provisionRecovery(cmdtesting.Context(c), nil, "", &recovery.ArchiveInfo{
			CloudType:    jujucloud.CloudTypeKubernetes,
			AgentVersion: semversion.MustParse(test.archived),
		})
		c.Check(err, tc.ErrorMatches,
			"archive was created by agent version .* but this binary is .*; recovery requires the exact same version")
	}
}

// TestProvisionRecoveryRequiresIAASMachineName checks that a machine
// archive which does not record the source controller machine is rejected
// before any cloud access.
func (s *recoverySuite) TestProvisionRecoveryRequiresIAASMachineName(c *tc.C) {
	err := provisionRecovery(cmdtesting.Context(c), nil, "", &recovery.ArchiveInfo{
		CloudType:    "lxd",
		AgentVersion: version.Current,
		SourceBase:   "ubuntu@24.04",
		SourceArch:   "amd64",
	})
	c.Check(err, tc.ErrorMatches, "archive does not record a usable controller machine")
}

// recoveryProvider returns a registered provider so configuration tests use
// the same provider schema as recovery.
func recoveryProvider(c *tc.C, cloudType string) environs.EnvironProvider {
	provider, err := environs.Provider(cloudType)
	c.Assert(err, tc.ErrorIsNil)
	return provider
}

// TestRecoveryConfigsPreservesArchivedSettings checks that allocation keeps
// the archived controller and provider-specific model configuration.
func (s *recoverySuite) TestRecoveryConfigsPreservesArchivedSettings(c *tc.C) {
	info := &recovery.ArchiveInfo{
		ControllerUUID:      "c0ffee00-1111-2222-3333-444455556666",
		ControllerName:      "source-ctrl",
		ControllerModelUUID: "c0ffee00-aaaa-2222-3333-444455556666",
		CACert:              coretesting.CACert,
		AgentVersion:        semversion.MustParse("4.1.0"),
		CloudType:           "lxd",
		ControllerConfig: map[string]string{
			"api-port": "17071", "ssh-server-port": "17023",
			"auditing-enabled": "true", "audit-log-max-backups": "7",
			"max-debug-log-duration": "10m", "removed-setting": "ignored",
		},
		ControllerModelConfig: map[string]string{
			"project": "my-project", "image-stream": "daily", "firewall-mode": "global",
			"automatically-retry-hooks": "false", "resource-tags": "a=b c=d",
			"name": "stale-name", "type": "ec2",
			"uuid": "deadbeef-0000-0000-0000-000000000000", "agent-version": "1.0.0",
		},
	}
	controllerConfig, modelConfig, err := recoveryConfigs(c.Context(), info, recoveryProvider(c, "lxd"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerConfig.ControllerUUID(), tc.Equals, info.ControllerUUID)
	c.Check(controllerConfig.ControllerName(), tc.Equals, info.ControllerName)
	c.Check(controllerConfig.APIPort(), tc.Equals, 17071)
	c.Check(controllerConfig.SSHServerPort(), tc.Equals, 17023)
	c.Check(controllerConfig.AuditingEnabled(), tc.IsTrue)
	c.Check(controllerConfig.AuditLogMaxBackups(), tc.Equals, 7)
	c.Check(controllerConfig["removed-setting"], tc.IsNil)
	c.Check(modelConfig.Name(), tc.Equals, bootstrap.ControllerModelName)
	c.Check(modelConfig.Type(), tc.Equals, info.CloudType)
	c.Check(modelConfig.UUID(), tc.Equals, info.ControllerModelUUID)
	agentVersion, ok := modelConfig.AgentVersion()
	c.Assert(ok, tc.IsTrue)
	c.Check(agentVersion, tc.Equals, info.AgentVersion)
	c.Check(modelConfig.ImageStream(), tc.Equals, "daily")
	c.Check(modelConfig.AutomaticallyRetryHooks(), tc.IsFalse)
	tags, ok := modelConfig.ResourceTags()
	c.Assert(ok, tc.IsTrue)
	c.Check(tags, tc.DeepEquals, map[string]string{"a": "b", "c": "d"})
	c.Check(modelConfig.UnknownAttrs()["project"], tc.Equals, "my-project")
}

func (s *recoverySuite) TestRecoveryConfigsCoercesProviderTypes(c *tc.C) {
	info := &recovery.ArchiveInfo{
		ControllerUUID:      "c0ffee00-1111-2222-3333-444455556666",
		ControllerModelUUID: "c0ffee00-aaaa-2222-3333-444455556666",
		CACert:              coretesting.CACert,
		AgentVersion:        semversion.MustParse("4.1.0"),
		CloudType:           "ec2",
		ControllerModelConfig: map[string]string{
			"vpc-id": "vpc-1234", "vpc-id-force": "true",
		},
	}
	_, modelConfig, err := recoveryConfigs(c.Context(), info, recoveryProvider(c, "ec2"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelConfig.UnknownAttrs()["vpc-id-force"], tc.Equals, true)
}

// TestRecoveryParamsFromArchiveForwardsProvisioningInputs checks that every
// archived allocation input reaches the recovery parameters.
func (s *recoverySuite) TestRecoveryParamsFromArchiveForwardsProvisioningInputs(c *tc.C) {
	storagePools := map[string]internalstorage.Attrs{
		"pool-a": {
			corestorage.BootstrapStoragePoolNameKey: "pool-a",
			corestorage.BootstrapStoragePoolTypeKey: "lxd",
			"source":                                "/dev/sda1",
		},
	}
	info := &recovery.ArchiveInfo{
		AgentVersion: version.Current, SourceBase: "ubuntu@24.04/stable", SourceArch: "arm64",
		ControllerName: "source-ctrl", ModelConstraints: constraints.MustParse("cores=4"),
		ControllerConstraints: constraints.MustParse("mem=4096 zones=zone-a"), StoragePools: storagePools,
	}
	params, err := recoveryParamsFromArchive(info)
	c.Assert(err, tc.ErrorIsNil)
	wantBase, err := base.ParseBaseFromString(info.SourceBase)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(params.ControllerName, tc.Equals, "source-ctrl")
	c.Check(params.AgentVersion, tc.Equals, info.AgentVersion)
	c.Check(params.Arch, tc.Equals, "arm64")
	c.Check(params.BootstrapBase, tc.Equals, wantBase)
	c.Check(params.SupportedBootstrapBases, tc.DeepEquals, []base.Base{wantBase})
	c.Check(params.ModelConstraints, tc.DeepEquals, info.ModelConstraints)

	// The archived controller constraints are kept, with the archived
	// architecture pinned over them.
	c.Assert(params.BootstrapConstraints.Arch, tc.NotNil)
	c.Check(*params.BootstrapConstraints.Arch, tc.Equals, "arm64")
	c.Assert(params.BootstrapConstraints.Mem, tc.NotNil)
	c.Check(*params.BootstrapConstraints.Mem, tc.Equals, uint64(4096))
	c.Assert(params.BootstrapConstraints.Zones, tc.NotNil)
	c.Check(*params.BootstrapConstraints.Zones, tc.DeepEquals, []string{"zone-a"})

	c.Check(params.StoragePools, tc.DeepEquals, storagePools)

	// Allocation may rewrite the pool attributes it receives; the validated
	// archive summary must not change underneath it.
	params.StoragePools["pool-a"]["source"] = "other"
	c.Check(info.StoragePools["pool-a"]["source"], tc.Equals, "/dev/sda1")
}

// TestRecoveryCredentialsUsesArchivedRegion checks that the archived cloud
// region selects where the replacement is allocated, and that an archived
// region the local cloud definition does not know fails rather than silently
// allocating elsewhere.
func (s *recoverySuite) TestRecoveryCredentialsUsesArchivedRegion(c *tc.C) {
	cloudDef := jujucloud.Cloud{
		Name:      "recovery-cloud",
		Type:      "dummy",
		AuthTypes: jujucloud.AuthTypes{jujucloud.EmptyAuthType},
		Regions: []jujucloud.Region{
			{Name: "first"},
			{Name: "archived"},
		},
	}
	c.Assert(jujucloud.WritePersonalCloudMetadata(map[string]jujucloud.Cloud{
		cloudDef.Name: cloudDef,
	}), tc.ErrorIsNil)

	store := jujuclient.NewMemStore()
	c.Assert(store.UpdateCredential(cloudDef.Name, jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"recovery": jujucloud.NewEmptyCredential(),
		},
		DefaultCredential: "recovery",
		DefaultRegion:     "first",
	}), tc.ErrorIsNil)

	info := &recovery.ArchiveInfo{
		CloudName:   cloudDef.Name,
		CloudType:   "dummy",
		CloudRegion: "archived",
	}
	target, err := recoveryCredentials(cmdtesting.Context(c), store, info)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(target.spec.Region, tc.Equals, "archived")

	// Without an archived region the credential's default region is used.
	info.CloudRegion = ""
	target, err = recoveryCredentials(cmdtesting.Context(c), store, info)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(target.spec.Region, tc.Equals, "first")

	info.CloudRegion = "gone"
	_, err = recoveryCredentials(cmdtesting.Context(c), store, info)
	c.Assert(err, tc.ErrorMatches,
		`region "gone" not found \(expected one of .*\)`)
}

// TestRecoveryCredentialsUsesLocalKubernetesRegion checks that a Kubernetes
// recovery allocates in the locally resolved cluster region even when the
// archive records one: regions name local kubeconfig contexts, so the
// archived name can be stale once the cloud is re-imported on this client.
func (s *recoverySuite) TestRecoveryCredentialsUsesLocalKubernetesRegion(c *tc.C) {
	store := setupKubernetesRecoveryCloud(c)
	hostRegion := "microk8s/localhost"
	replaceKubernetesProviderForTest(c, &fakeK8sRecoveryProvider{
		hostCloudRegion: &hostRegion,
	})

	info := validKubernetesArchiveInfo(nil)
	info.CloudRegion = "archived"

	target, err := recoveryCredentials(cmdtesting.Context(c), store, info)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(target.spec.Region, tc.Equals, "localhost")
}

// TestProvisionRecoveryRequiresKubernetesControllerName checks that a
// Kubernetes archive which does not record the source controller name
// is rejected before any cloud access.
func (s *recoverySuite) TestProvisionRecoveryRequiresKubernetesControllerName(c *tc.C) {
	err := provisionRecovery(cmdtesting.Context(c), nil, "", &recovery.ArchiveInfo{
		CloudType:    "kubernetes",
		AgentVersion: version.Current,
	})
	c.Check(err, tc.ErrorMatches, "recovery archive does not record the source controller name")
}

// TestProvisionRecoveryFinalisesKubernetesCloudHostRegion checks that
// FinalizeCloud runs before the host-cloud-region guard so built-in clouds
// such as microk8s (whose HostCloudRegion is discovered during finalisation)
// are accepted, while clouds that remain without a host cloud region after
// finalisation are still rejected.
func (s *recoverySuite) TestProvisionRecoveryFinalisesKubernetesCloudHostRegion(c *tc.C) {
	c.Assert(jujucloud.WritePersonalCloudMetadata(map[string]jujucloud.Cloud{
		"myk8s": {
			Name:    "myk8s",
			Type:    jujucloud.CloudTypeKubernetes,
			Regions: []jujucloud.Region{{Name: "localhost"}},
		},
	}), tc.ErrorIsNil)

	var hostRegion string
	replaceKubernetesProviderForTest(c, &fakeK8sRecoveryProvider{
		hostCloudRegion: &hostRegion,
	})

	info := &recovery.ArchiveInfo{
		ControllerName: "source-ctrl",
		CloudName:      "myk8s",
		CloudType:      jujucloud.CloudTypeKubernetes,
		AgentVersion:   version.Current,
	}
	err := provisionRecovery(cmdtesting.Context(c), jujuclient.NewMemStore(), "", info)
	c.Check(err, tc.ErrorMatches, "recovery cloud does not record a host cloud region.*")

	hostRegion = "microk8s/localhost"
	err = provisionRecovery(cmdtesting.Context(c), jujuclient.NewMemStore(), "", info)
	c.Check(err, tc.ErrorMatches, `detecting credentials for "myk8s" cloud provider: credentials not found`)
}

// TestProvisionRecoveryKubernetes checks the end-to-end Kubernetes recovery
// provisioning branch: opening the Kubernetes broker with the archived
// controller identity, preparing bootstrap with the archived controller name
// before allocation, forwarding constraints, storage pools and agent version
// into RecoverK8s, invoking the controller finaliser in recovery mode, and
// reporting the replacement namespace in the terminal error.
func (s *recoverySuite) TestProvisionRecoveryKubernetes(c *tc.C) {
	store := setupKubernetesRecoveryCloud(c)
	hostRegion := "microk8s/localhost"

	var openParams environs.OpenParams
	broker := &fakeK8sRecoveryBroker{c: c}
	replaceKubernetesProviderForTest(c, &fakeK8sRecoveryProvider{
		hostCloudRegion: &hostRegion,
		open: func(_ context.Context, args environs.OpenParams, _ environs.CredentialInvalidator) (caas.Broker, error) {
			openParams = args
			broker.cfg = args.Config
			return broker, nil
		},
	})

	storagePools := map[string]internalstorage.Attrs{
		"k8s-pool": {
			corestorage.BootstrapStoragePoolNameKey: "k8s-pool",
			corestorage.BootstrapStoragePoolTypeKey: "kubernetes",
			"storage-class":                         "fast",
		},
	}
	info := validKubernetesArchiveInfo(storagePools)

	err := provisionRecovery(cmdtesting.Context(c), store, "", info)
	c.Assert(err, tc.ErrorMatches,
		`controller recovery database import is not available in this build; started replacement agent in namespace controller-source-ctrl`)

	c.Check(openParams.ControllerUUID, tc.Equals, info.ControllerUUID)
	c.Assert(openParams.Config, tc.NotNil)
	c.Check(openParams.Config.Name(), tc.Equals, bootstrap.ControllerModelName)
	c.Check(openParams.Config.UUID(), tc.Equals, info.ControllerModelUUID)
	c.Check(openParams.Cloud.Region, tc.Equals, "localhost")
	c.Assert(openParams.Cloud.Credential, tc.NotNil)
	c.Check(openParams.Cloud.Credential.Attributes()["Token"], tc.Equals, "test-token")

	c.Check(broker.steps, tc.DeepEquals, []string{"prepare", "bootstrap", "finalize", "wait"})
	c.Check(broker.preparedControllerName, tc.Equals, "source-ctrl")
	c.Check(broker.bootstrapArgs.CloudName, tc.Equals, "myk8s")
	c.Check(broker.bootstrapArgs.CloudRegion, tc.Equals, "localhost")
	c.Check(broker.bootstrapArgs.ControllerConfig.ControllerUUID(), tc.Equals, info.ControllerUUID)
	c.Check(broker.bootstrapArgs.StoragePools, tc.DeepEquals, storagePools)
	c.Assert(broker.bootstrapArgs.BootstrapConstraints.Arch, tc.NotNil)
	c.Check(*broker.bootstrapArgs.BootstrapConstraints.Arch, tc.Equals, arch.ARM64)
	c.Assert(broker.bootstrapArgs.BootstrapConstraints.Mem, tc.NotNil)
	c.Check(*broker.bootstrapArgs.BootstrapConstraints.Mem, tc.Equals, uint64(2048))
	agentVersion, ok := broker.cfg.AgentVersion()
	c.Assert(ok, tc.IsTrue)
	c.Check(agentVersion, tc.Equals, info.AgentVersion)

	c.Assert(broker.podConfig, tc.NotNil)
	c.Check(broker.podConfig.Bootstrap.StateInitializationParams.RecoveryMode, tc.IsTrue)
	c.Check(broker.podConfig.JujuVersion, tc.Equals, info.AgentVersion)
	c.Check(broker.podConfig.Bootstrap.ControllerCloudCredentialName, tc.Equals, "k8s-cred")
	c.Check(broker.podConfig.Bootstrap.Timeout, tc.Equals, recoveryAllocationTimeout)
}

// TestProvisionRecoveryKubernetesReportsNamespaceOnCancellation checks that
// post-creation cancellation surfaces the replacement namespace for manual
// cleanup instead of a bare cancelled error.
func (s *recoverySuite) TestProvisionRecoveryKubernetesReportsNamespaceOnCancellation(c *tc.C) {
	store := setupKubernetesRecoveryCloud(c)
	hostRegion := "microk8s/localhost"

	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	cmdCtx := cmdtesting.Context(c)
	cmdCtx.Context = ctx

	broker := &fakeK8sRecoveryBroker{
		c:             c,
		afterFinalize: cancel,
	}
	replaceKubernetesProviderForTest(c, &fakeK8sRecoveryProvider{
		hostCloudRegion: &hostRegion,
		open: func(_ context.Context, args environs.OpenParams, _ environs.CredentialInvalidator) (caas.Broker, error) {
			broker.cfg = args.Config
			return broker, nil
		},
	})

	err := provisionRecovery(cmdCtx, store, "", validKubernetesArchiveInfo(nil))
	c.Check(err, tc.ErrorIs, bootstrap.Cancelled())
	c.Check(err, tc.ErrorMatches,
		`replacement namespace "controller-source-ctrl" may require manual removal: cancelled`)
}

func setupKubernetesRecoveryCloud(c *tc.C) *jujuclient.MemStore {
	const endpoint = "https://127.0.0.1:16443"
	c.Assert(jujucloud.WritePersonalCloudMetadata(map[string]jujucloud.Cloud{
		"myk8s": {
			Name:      "myk8s",
			Type:      jujucloud.CloudTypeKubernetes,
			AuthTypes: jujucloud.AuthTypes{jujucloud.OAuth2AuthType},
			Endpoint:  endpoint,
			Regions:   []jujucloud.Region{{Name: "localhost", Endpoint: endpoint}},
		},
	}), tc.ErrorIsNil)

	store := jujuclient.NewMemStore()
	c.Assert(store.UpdateCredential("myk8s", jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"k8s-cred": jujucloud.NewCredential(jujucloud.OAuth2AuthType, map[string]string{
				"Token": "test-token",
			}),
		},
		DefaultCredential: "k8s-cred",
		DefaultRegion:     "localhost",
	}), tc.ErrorIsNil)
	return store
}

func validKubernetesArchiveInfo(
	storagePools map[string]internalstorage.Attrs,
) *recovery.ArchiveInfo {
	return &recovery.ArchiveInfo{
		ControllerUUID:        "c0ffee00-1111-2222-3333-444455556666",
		ControllerName:        "source-ctrl",
		ControllerModelUUID:   "c0ffee00-aaaa-2222-3333-444455556666",
		CACert:                coretesting.CACert,
		CAPrivateKey:          coretesting.CAKey,
		AgentVersion:          version.Current,
		CloudName:             "myk8s",
		CloudType:             jujucloud.CloudTypeKubernetes,
		ModelConstraints:      constraints.MustParse("mem=2048"),
		ControllerConstraints: constraints.MustParse("arch=arm64"),
		StoragePools:          storagePools,
	}
}

func replaceKubernetesProviderForTest(c *tc.C, p *fakeK8sRecoveryProvider) {
	reg := environs.GlobalProviderRegistry()
	orig, err := reg.Provider(jujucloud.CloudTypeKubernetes)
	c.Assert(err, tc.ErrorIsNil)
	containerOrig, ok := orig.(caas.ContainerEnvironProvider)
	c.Assert(ok, tc.IsTrue)
	p.ContainerEnvironProvider = containerOrig

	reg.UnregisterProvider(jujucloud.CloudTypeKubernetes)
	c.Cleanup(func() {
		reg.UnregisterProvider(jujucloud.CloudTypeKubernetes)
		c.Assert(reg.RegisterProvider(orig, jujucloud.CloudTypeKubernetes), tc.ErrorIsNil)
	})
	c.Assert(reg.RegisterProvider(p, jujucloud.CloudTypeKubernetes), tc.ErrorIsNil)
}

type fakeK8sRecoveryProvider struct {
	caas.ContainerEnvironProvider
	hostCloudRegion *string
	open            func(context.Context, environs.OpenParams, environs.CredentialInvalidator) (caas.Broker, error)
}

func (p *fakeK8sRecoveryProvider) FinalizeCloud(_ environs.FinalizeCloudContext, cld jujucloud.Cloud) (jujucloud.Cloud, error) {
	if p.hostCloudRegion != nil {
		cld.HostCloudRegion = *p.hostCloudRegion
	}
	return cld, nil
}

func (p *fakeK8sRecoveryProvider) DetectCredentials(string) (*jujucloud.CloudCredential, error) {
	return nil, errors.Errorf("credentials %w", coreerrors.NotFound)
}

func (p *fakeK8sRecoveryProvider) Open(
	ctx context.Context,
	args environs.OpenParams,
	invalidator environs.CredentialInvalidator,
) (caas.Broker, error) {
	if p.open == nil {
		return nil, errors.New("unexpected Kubernetes broker open")
	}
	return p.open(ctx, args, invalidator)
}

type fakeK8sRecoveryBroker struct {
	caas.Broker
	c                      *tc.C
	cfg                    *config.Config
	preparedControllerName string
	namespace              string
	steps                  []string
	bootstrapArgs          environs.BootstrapParams
	podConfig              *podcfg.ControllerPodConfig
	afterFinalize          func()
}

func (b *fakeK8sRecoveryBroker) Config() *config.Config {
	return b.cfg
}

func (b *fakeK8sRecoveryBroker) SetConfig(_ context.Context, cfg *config.Config) error {
	b.cfg = cfg
	return nil
}

func (b *fakeK8sRecoveryBroker) PrepareForBootstrap(_ environs.BootstrapContext, controllerName string) error {
	b.steps = append(b.steps, "prepare")
	b.preparedControllerName = controllerName
	b.namespace = "controller-" + controllerName
	return nil
}

func (b *fakeK8sRecoveryBroker) ConstraintsValidator(context.Context) (constraints.Validator, error) {
	return constraints.NewValidator(), nil
}

func (b *fakeK8sRecoveryBroker) Bootstrap(
	_ environs.BootstrapContext,
	args environs.BootstrapParams,
) (*environs.BootstrapResult, error) {
	b.steps = append(b.steps, "bootstrap")
	b.bootstrapArgs = args
	return &environs.BootstrapResult{
		Arch: arch.AMD64,
		Base: base.MustParseBaseFromString("ubuntu@24.04"),
		CaasBootstrapFinalizer: func(
			_ environs.BootstrapContext, pcfg *podcfg.ControllerPodConfig, _ environs.BootstrapDialOpts,
		) error {
			b.steps = append(b.steps, "finalize")
			b.podConfig = pcfg
			if b.afterFinalize != nil {
				b.afterFinalize()
			}
			return nil
		},
	}, nil
}

func (b *fakeK8sRecoveryBroker) Namespace() string {
	return b.namespace
}

func (b *fakeK8sRecoveryBroker) WaitForRecoveryAgent(_ context.Context, cfg *podcfg.ControllerPodConfig) error {
	b.c.Check(cfg, tc.Equals, b.podConfig)
	b.steps = append(b.steps, "wait")
	return nil
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

// recoveryArchive renders the default minimal LXD recovery archive.
func recoveryArchive(c *tc.C) []byte {
	return recoveryArchiveForControllerModel(c, "lxd", "lxd", "iaas")
}

// recoveryArchiveForControllerModel renders a minimal valid recovery
// archive: the provenance metadata, a controller dump recording one
// controller model on the given cloud, that model's database dump, and the
// data-directory files archive. It is the smallest archive the recover
// command's validation accepts, so reaching the terminal error proves the
// whole read and verify path ran.
func recoveryArchiveForControllerModel(
	c *tc.C, cloudName, cloudType, modelType string,
) []byte {
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
		// The base is what the real producer records: the machine
		// service's base string, channel included.
		`"Base":"ubuntu@24.04/stable",` +
		`"Arch":"amd64",` +
		`"Hostname":"myhost",` +
		`"Version":"4.1.0",` +
		`"ControllerUUID":"` + controllerUUID + `",` +
		`"HANodes":1,` +
		`"ControllerMachineID":"0",` +
		`"ControllerMachineInstanceID":"inst-10101010"` +
		`}` + "\n"
	systemIdentity := ""
	if cloudType != "kubernetes" {
		systemIdentity = "    system_identity: source-system-identity\n"
	}
	controllerDump := `version: 4.1.0
payload:
  controller:
  - uuid: ` + controllerUUID + `
    model_uuid: ` + controllerModelUUID + `
    ca_cert: source-ca-cert
    ca_private_key: source-ca-key
` + systemIdentity + `  cloud:
  - uuid: cloud-source
    name: ` + cloudName + `
    cloud_type_id: 1
  cloud_type:
  - id: 1
    type: ` + cloudType + `
  model_type:
  - id: 0
    type: ` + modelType + `
  controller_config:
  - key: controller-name
    value: source-ctrl
  model:
  - uuid: ` + controllerModelUUID + `
    name: controller
    cloud_uuid: cloud-source
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
