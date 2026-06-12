// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	stdtesting "testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite
}

func TestBootstrapSuite(t *stdtesting.T) {
	tc.Run(t, &bootstrapSuite{})
}

func (s *bootstrapSuite) TestInsertInitialSSHServerHostKey(c *tc.C) {
	err := InsertInitialSSHServerHostKey(testPrivateKey)(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	var key string
	row := s.DB().QueryRow(`SELECT ssh_key FROM controller_ssh_host_key WHERE id = ?`, sshServerHostKeyID)
	c.Assert(row.Scan(&key), tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
}

func (s *bootstrapSuite) TestInsertInitialSSHServerHostKeyValidatesEmpty(c *tc.C) {
	err := InsertInitialSSHServerHostKey("")(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorMatches, `empty SSHServerHostKey`)
}

const testPrivateKey = "test-private-key"
