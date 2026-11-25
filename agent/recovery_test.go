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
