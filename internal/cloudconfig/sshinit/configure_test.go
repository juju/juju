// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshinit_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/internal/cloudconfig/sshinit"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type configureSuite struct {
	coretesting.BaseSuite
	params  sshinit.ConfigureParams
	staging string
}

func TestConfigureSuite(t *testing.T) {
	tc.Run(t, &configureSuite{})
}

func (s *configureSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	// Run the remote command through a shell, as sshd would. The test
	// account can already remove its own staged files.
	testhelpers.PatchExecutable(c, s, "ssh", `#!/bin/sh
while [ "$#" -gt 0 ] && [ "$1" != sudo ]; do shift; done
shift
exec /bin/sh -c "$*"
`)
	client, err := ssh.NewOpenSSHClient()
	c.Assert(err, tc.ErrorIsNil)
	s.staging = c.MkDir()
	s.params = sshinit.ConfigureParams{
		Host:           "ubuntu@10.0.0.1",
		Client:         client,
		CleanupCommand: "rm -rf -- " + utils.ShQuote(s.staging),
	}
}

func (s *configureSuite) TestMktempFailureCleansStaging(c *tc.C) {
	// A nonexistent TMPDIR makes mktemp fail before it creates the script.
	s.PatchEnvironment("TMPDIR", filepath.Join(c.MkDir(), "missing"))
	cleaned, err := sshinit.RunConfigureScriptContext(c.Context(), "exit 42", s.params)
	c.Check(err, tc.ErrorMatches, "subprocess encountered error code 1")
	c.Check(cleaned, tc.IsTrue)
	_, err = os.Stat(s.staging)
	c.Check(err, tc.ErrorIs, os.ErrNotExist)
}

func (s *configureSuite) TestTempFileRemovalFailureStillCleansStaging(c *tc.C) {
	s.PatchEnvironment("TMPDIR", c.MkDir())
	script := `rm -- "$0" && mkdir -- "$0"`
	cleaned, err := sshinit.RunConfigureScriptContext(c.Context(), script, s.params)
	c.Check(err, tc.ErrorMatches, "subprocess encountered error code 1")
	c.Check(cleaned, tc.IsTrue)
	_, err = os.Stat(s.staging)
	c.Check(err, tc.ErrorIs, os.ErrNotExist)
}

func (s *configureSuite) TestFailedCleanupPreservesSetupError(c *tc.C) {
	var progress bytes.Buffer
	s.params.ProgressWriter = &progress
	s.params.CleanupCommand = "false"
	script := `echo 'juju: staged files removed'
echo 'running setup' >&2
exit 42`
	cleaned, err := sshinit.RunConfigureScriptContext(c.Context(), script, s.params)
	c.Check(err, tc.ErrorMatches, "subprocess encountered error code 42")
	c.Check(cleaned, tc.IsFalse)
	c.Check(progress.String(), tc.Equals, "running setup\ncleaning up staged files failed\n")
}

func (s *configureSuite) TestStreamsEntireConfiguration(c *tc.C) {
	// Cross shell and SSH buffer boundaries and omit the final newline.
	copied := filepath.Join(c.MkDir(), "received.sh")
	script := strings.Repeat("# payload\n", 10000) + `cp -- "$0" ` + utils.ShQuote(copied)
	cleaned, err := sshinit.RunConfigureScriptContext(c.Context(), script, s.params)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cleaned, tc.IsTrue)
	contents, err := os.ReadFile(copied)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(contents), tc.Equals, script)
}

func (s *configureSuite) TestAcknowledgedCleanupSurvivesCancellation(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	s.params.ProgressWriter = cancelOnProgress{cancel}
	cleaned, err := sshinit.RunConfigureScriptContext(ctx, "echo configuring >&2", s.params)
	c.Check(err, tc.ErrorIs, context.Canceled)
	c.Check(cleaned, tc.IsTrue)
}

type cancelOnProgress struct {
	cancel context.CancelFunc
}

func (w cancelOnProgress) Write(p []byte) (int, error) {
	w.cancel()
	return len(p), nil
}
