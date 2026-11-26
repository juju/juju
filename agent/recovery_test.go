// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/internal/testhelpers"
)

type recoverySuite struct {
	testhelpers.IsolationSuite
}

func TestRecoverySuite(t *stdtesting.T) {
	tc.Run(t, &recoverySuite{})
}

func (s *recoverySuite) TestIsRecoveryMissingFile(c *tc.C) {
	isRecovery, err := agent.IsRecovery(c.MkDir())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isRecovery, tc.IsFalse)
}

func (s *recoverySuite) TestIsRecoveryEmptyFile(c *tc.C) {
	dataDir := c.MkDir()
	paramsFile := filepath.Join(dataDir, agent.RecoveryParamsFile)
	c.Assert(os.MkdirAll(filepath.Dir(paramsFile), 0755), tc.ErrorIsNil)
	c.Assert(os.WriteFile(paramsFile, nil, 0600), tc.ErrorIsNil)

	isRecovery, err := agent.IsRecovery(dataDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isRecovery, tc.IsTrue)
}

func (s *recoverySuite) TestIsRecoveryPopulatedFile(c *tc.C) {
	dataDir := c.MkDir()
	paramsFile := filepath.Join(dataDir, agent.RecoveryParamsFile)
	c.Assert(os.MkdirAll(filepath.Dir(paramsFile), 0755), tc.ErrorIsNil)
	c.Assert(os.WriteFile(paramsFile, []byte("params: value"), 0600), tc.ErrorIsNil)

	isRecovery, err := agent.IsRecovery(dataDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isRecovery, tc.IsTrue)
}

func (s *recoverySuite) TestIsRecoveryDirectory(c *tc.C) {
	dataDir := c.MkDir()
	c.Assert(os.MkdirAll(filepath.Join(dataDir, agent.RecoveryParamsFile), 0755), tc.ErrorIsNil)

	isRecovery, err := agent.IsRecovery(dataDir)
	c.Check(err, tc.ErrorMatches,
		`invalid recovery params path ".*/recovery/params": expected regular file`)
	c.Check(isRecovery, tc.IsFalse)
}

func (s *recoverySuite) TestIsRecoverySymlink(c *tc.C) {
	dataDir := c.MkDir()
	paramsFile := filepath.Join(dataDir, agent.RecoveryParamsFile)
	c.Assert(os.MkdirAll(filepath.Dir(paramsFile), 0755), tc.ErrorIsNil)
	c.Assert(os.Symlink("/nonexistent/params", paramsFile), tc.ErrorIsNil)

	isRecovery, err := agent.IsRecovery(dataDir)
	c.Check(err, tc.ErrorMatches,
		`invalid recovery params path ".*/recovery/params": expected regular file`)
	c.Check(isRecovery, tc.IsFalse)
}

func (s *recoverySuite) TestAcknowledgeRecoveryWaitCreatesAndIsIdempotent(c *tc.C) {
	dataDir := c.MkDir()
	waiting := filepath.Join(dataDir, agent.RecoveryAgentWaitingFile)
	c.Assert(os.MkdirAll(filepath.Dir(waiting), 0700), tc.ErrorIsNil)

	c.Assert(agent.AcknowledgeRecoveryWait(dataDir), tc.ErrorIsNil)
	info, err := os.Lstat(waiting)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Mode(), tc.Equals, os.FileMode(0600))

	c.Assert(agent.AcknowledgeRecoveryWait(dataDir), tc.ErrorIsNil)
}

func (s *recoverySuite) TestAcknowledgeRecoveryWaitMissingDirectory(c *tc.C) {
	dataDir := c.MkDir()
	err := agent.AcknowledgeRecoveryWait(dataDir)
	c.Check(err, tc.ErrorMatches, `.*no such file or directory`)
	_, statErr := os.Stat(filepath.Dir(filepath.Join(dataDir, agent.RecoveryAgentWaitingFile)))
	c.Check(statErr, tc.ErrorIs, os.ErrNotExist)
}

func (s *recoverySuite) TestAcknowledgeRecoveryWaitRejectsNonRegular(c *tc.C) {
	dataDir := c.MkDir()
	waiting := filepath.Join(dataDir, agent.RecoveryAgentWaitingFile)
	c.Assert(os.MkdirAll(waiting, 0700), tc.ErrorIsNil)
	c.Check(agent.AcknowledgeRecoveryWait(dataDir), tc.ErrorMatches,
		`invalid recovery acknowledgement ".*/recovery/agent-waiting": expected regular file`)

	c.Assert(os.Remove(waiting), tc.ErrorIsNil)
	c.Assert(os.Symlink("/nonexistent/waiting", waiting), tc.ErrorIsNil)
	c.Check(agent.AcknowledgeRecoveryWait(dataDir), tc.ErrorMatches,
		`invalid recovery acknowledgement ".*/recovery/agent-waiting": expected regular file`)
}
