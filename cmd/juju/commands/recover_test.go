// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/modelcmd"
)

type recoverySuite struct{}

func TestRecoverySuite(t *testing.T) { tc.Run(t, &recoverySuite{}) }

func (s *recoverySuite) TestInit(c *tc.C) {
	for _, test := range []struct {
		args    []string
		pattern string
	}{
		{nil, "a backup archive is required"},
		{[]string{"backup.tar.gz"}, "a backup assertion file is required"},
		{[]string{"", "backup.tar.gz.assert"}, "a backup archive is required"},
		{[]string{"backup.tar.gz", ""}, "a backup assertion file is required"},
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
