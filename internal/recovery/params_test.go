// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery_test

import (
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/recovery"
	"github.com/juju/juju/internal/testhelpers"
)

type paramsSuite struct {
	testhelpers.IsolationSuite
}

func TestParamsSuite(t *stdtesting.T) {
	tc.Run(t, &paramsSuite{})
}

func (s *paramsSuite) TestIsRecoveryMissing(c *tc.C) {
	isRecovery, err := recovery.IsRecovery(c.MkDir())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isRecovery, tc.IsFalse)
}

func (s *paramsSuite) TestIsRecoveryEmptyFile(c *tc.C) {
	dataDir := c.MkDir()
	writeRecoveryFile(c, dataDir, recovery.ParamsFile, "")
	isRecovery, err := recovery.IsRecovery(dataDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isRecovery, tc.IsTrue)
}

func (s *paramsSuite) TestIsRecoveryPopulatedFile(c *tc.C) {
	dataDir := c.MkDir()
	writeRecoveryFile(c, dataDir, recovery.ParamsFile, "archive-path: /var/lib/juju/recovery/archive.tar.gz\n")
	isRecovery, err := recovery.IsRecovery(dataDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isRecovery, tc.IsTrue)
}

func (s *paramsSuite) TestIsRecoveryDirectory(c *tc.C) {
	dataDir := c.MkDir()
	paramsPath := filepath.Join(dataDir, recovery.ParamsFile)
	c.Assert(os.MkdirAll(paramsPath, 0o700), tc.ErrorIsNil)
	isRecovery, err := recovery.IsRecovery(dataDir)
	c.Check(isRecovery, tc.IsFalse)
	c.Check(err, tc.ErrorMatches,
		`invalid recovery params path ".*/recovery/params": expected regular file`)
}

func (s *paramsSuite) TestIsRecoverySymlink(c *tc.C) {
	dataDir := c.MkDir()
	writeRecoveryFile(c, dataDir, "recovery/target", "")
	c.Assert(os.Symlink("target", filepath.Join(dataDir, recovery.ParamsFile)), tc.ErrorIsNil)
	isRecovery, err := recovery.IsRecovery(dataDir)
	c.Check(isRecovery, tc.IsFalse)
	c.Check(err, tc.ErrorMatches, `invalid recovery params path ".*/recovery/params": expected regular file`)
}

func (s *paramsSuite) TestReadParamsRoundTrip(c *tc.C) {
	dataDir := c.MkDir()
	params := s.validParams(c)
	writeParams(c, dataDir, params)
	got, err := recovery.ReadParams(dataDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.ArchivePath, tc.Equals, params.ArchivePath)
	c.Check(got.SHA256, tc.Equals, params.SHA256)
	c.Check(got.AgentVersion, tc.Equals, params.AgentVersion)
	c.Check(got.MachineName, tc.Equals, params.MachineName)
	c.Check(got.Node.AgentVersion, tc.Equals, params.AgentVersion)
	c.Check(got.Node.ControllerCloud.Name, tc.Equals, params.Node.ControllerCloud.Name)
}

func (s *paramsSuite) TestReadParamsAcceptsPrefixedChecksum(c *tc.C) {
	dataDir := c.MkDir()
	params := s.validParams(c)
	params.SHA256 = "sha256-" + params.SHA256
	writeParams(c, dataDir, params)
	_, err := recovery.ReadParams(dataDir)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *paramsSuite) TestReadParamsRejectsMissingArchive(c *tc.C) {
	params := s.validParams(c)
	params.ArchivePath = ""
	s.assertInvalidParams(c, params, "incomplete recovery parameters")
}

func (s *paramsSuite) TestReadParamsRejectsBadChecksum(c *tc.C) {
	for _, checksum := range []string{
		"", "not-a-checksum", "0123456789abcdef",
		"zz23456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	} {
		params := s.validParams(c)
		params.SHA256 = checksum
		s.assertInvalidParams(c, params, "incomplete recovery parameters|invalid recovery checksum")
	}
}

func (s *paramsSuite) TestReadParamsRejectsVersionDisagreement(c *tc.C) {
	params := s.validParams(c)
	params.Node.AgentVersion = semversion.MustParse("4.1.1")
	s.assertInvalidParams(c, params, "incomplete recovery parameters")
}

func (s *paramsSuite) TestReadParamsRejectsInvalidMachine(c *tc.C) {
	params := s.validParams(c)
	params.MachineName = "not a machine"
	dataDir := c.MkDir()
	writeParams(c, dataDir, params)
	_, err := recovery.ReadParams(dataDir)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *paramsSuite) TestReadParamsMissingFile(c *tc.C) {
	_, err := recovery.ReadParams(c.MkDir())
	c.Check(err, tc.ErrorMatches, "reading recovery parameters: .*")
}

func (s *paramsSuite) TestHasMarker(c *tc.C) {
	dataDir := c.MkDir()
	checksum := s.validParams(c).SHA256
	found, err := recovery.HasMarker(dataDir, recovery.StartedFile, checksum)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsFalse)
	writeRecoveryFile(c, dataDir, recovery.StartedFile, checksum+"\n")
	found, err = recovery.HasMarker(dataDir, recovery.StartedFile, checksum)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsTrue)
	_, err = recovery.HasMarker(dataDir, recovery.StartedFile, "another-checksum")
	c.Check(err, tc.ErrorMatches, "recovery marker belongs to another archive")
}

func (s *paramsSuite) TestBeginInitialisationIsExclusive(c *tc.C) {
	dataDir := c.MkDir()
	checksum := s.validParams(c).SHA256
	c.Assert(recovery.BeginInitialisation(dataDir, checksum), tc.ErrorIsNil)
	contents, err := os.ReadFile(filepath.Join(dataDir, recovery.StartedFile))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(contents), tc.Equals, checksum+"\n")
	err = recovery.BeginInitialisation(dataDir, checksum)
	c.Check(err, tc.ErrorMatches, "claiming recovery initialisation: .*")
}

// TestMarkerChecksumSpelling checks that markers record and compare one
// canonical digest form: the checksum legitimately arrives with or
// without a "sha256-" prefix.
func (s *paramsSuite) TestMarkerChecksumSpelling(c *tc.C) {
	bare := s.validParams(c).SHA256
	prefixed := "sha256-" + bare

	// A prefixed input is recorded in its bare form and is read back
	// with either spelling.
	dataDir := c.MkDir()
	c.Assert(recovery.BeginInitialisation(dataDir, prefixed), tc.ErrorIsNil)
	contents, err := os.ReadFile(filepath.Join(dataDir, recovery.StartedFile))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(contents), tc.Equals, bare+"\n")
	for _, spelling := range []string{bare, prefixed} {
		found, err := recovery.HasMarker(dataDir, recovery.StartedFile, spelling)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(found, tc.IsTrue, tc.Commentf("spelling %q", spelling))
	}

	// A pre-existing marker that records the prefixed spelling matches a
	// bare checksum, and reading it does not rewrite it.
	dataDir = c.MkDir()
	writeRecoveryFile(c, dataDir, recovery.StartedFile, prefixed+"\n")
	found, err := recovery.HasMarker(dataDir, recovery.StartedFile, bare)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsTrue)
	contents, err = os.ReadFile(filepath.Join(dataDir, recovery.StartedFile))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(contents), tc.Equals, prefixed+"\n")

	// A different digest is another archive under any spelling.
	_, err = recovery.HasMarker(dataDir, recovery.StartedFile, "sha256-"+strings.Repeat("0", 64))
	c.Check(err, tc.ErrorMatches, "recovery marker belongs to another archive")
}

func (s *paramsSuite) validParams(c *tc.C) recovery.Params {
	modelConfig, err := config.New(config.UseDefaults, map[string]any{
		"name": "controller", "type": "lxd", "uuid": "c0ffee00-aaaa-2222-3333-444455556666",
	})
	c.Assert(err, tc.ErrorIsNil)
	return recovery.Params{
		ArchivePath:  "/var/lib/juju/recovery/archive.tar.gz",
		SHA256:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		AgentVersion: semversion.MustParse("4.1.0"), MachineName: "0",
		Node: instancecfg.StateInitializationParams{
			AgentVersion: semversion.MustParse("4.1.0"), ControllerModelConfig: modelConfig,
		},
	}
}

func (s *paramsSuite) assertInvalidParams(c *tc.C, params recovery.Params, match string) {
	dataDir := c.MkDir()
	writeParams(c, dataDir, params)
	_, err := recovery.ReadParams(dataDir)
	c.Check(err, tc.ErrorMatches, match)
}

func writeParams(c *tc.C, dataDir string, params recovery.Params) {
	data, err := params.Marshal()
	c.Assert(err, tc.ErrorIsNil)
	writeRecoveryFile(c, dataDir, recovery.ParamsFile, string(data))
}

func writeRecoveryFile(c *tc.C, dataDir, name, contents string) {
	filename := filepath.Join(dataDir, name)
	c.Assert(os.MkdirAll(filepath.Dir(filename), 0o700), tc.ErrorIsNil)
	c.Assert(os.WriteFile(filename, []byte(contents), 0o600), tc.ErrorIsNil)
}
