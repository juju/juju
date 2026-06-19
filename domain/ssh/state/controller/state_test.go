// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"

	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainssh "github.com/juju/juju/domain/ssh"
	sshbootstrap "github.com/juju/juju/domain/ssh/bootstrap"
	sshcontrollerstate "github.com/juju/juju/domain/ssh/state/controller"
	jujutesting "github.com/juju/juju/internal/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

func TestStateSuite(t *stdtesting.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestGetSSHServerHostKeyMissing(c *tc.C) {
	st := sshcontrollerstate.NewState(txRunnerFactory(s.ControllerTxnRunner()))

	key, err := st.GetSSHServerHostKey(c.Context())
	c.Check(key, tc.Equals, "")
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

func (s *stateSuite) TestGetSSHServerHostKeyExisting(c *tc.C) {
	err := sshbootstrap.InsertInitialSSHServerHostKey(jujutesting.SSHServerHostKey)(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	st := sshcontrollerstate.NewState(txRunnerFactory(s.ControllerTxnRunner()))

	key, err := st.GetSSHServerHostKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, jujutesting.SSHServerHostKey)

	var (
		storedID        string
		algorithmTypeID int
	)
	row := s.DB().QueryRow(`SELECT id, algorithm_type_id FROM controller_ssh_host_key`)
	c.Assert(row.Scan(&storedID, &algorithmTypeID), tc.ErrorIsNil)
	c.Check(storedID, tc.Equals, domainssh.SSHServerHostKeyUUID)
	c.Check(algorithmTypeID, tc.Equals, domainssh.SSHKeyAlgorithmTypeED25519ID)
}

func txRunnerFactory(runner coredatabase.TxnRunner) coredatabase.TxnRunnerFactory {
	return func(context.Context) (coredatabase.TxnRunner, error) {
		return runner, nil
	}
}
