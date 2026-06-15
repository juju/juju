// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"

	coredatabase "github.com/juju/juju/core/database"
	schematesting "github.com/juju/juju/domain/schema/testing"
	sshbootstrap "github.com/juju/juju/domain/ssh/bootstrap"
	sshcontrollerstate "github.com/juju/juju/domain/ssh/state/controller"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

func TestStateSuite(t *stdtesting.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestGetSSHServerHostKeyMissing(c *tc.C) {
	st := sshcontrollerstate.NewState(txRunnerFactory(s.ControllerTxnRunner()))

	key, found, err := st.GetSSHServerHostKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsFalse)
	c.Check(key, tc.Equals, "")
}

func (s *stateSuite) TestGetSSHServerHostKeyExisting(c *tc.C) {
	err := sshbootstrap.InsertInitialSSHServerHostKey(testPrivateKey)(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	st := sshcontrollerstate.NewState(txRunnerFactory(s.ControllerTxnRunner()))

	key, found, err := st.GetSSHServerHostKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.IsTrue)
	c.Check(key, tc.Equals, testPrivateKey)
}

func txRunnerFactory(runner coredatabase.TxnRunner) coredatabase.TxnRunnerFactory {
	return func(context.Context) (coredatabase.TxnRunner, error) {
		return runner, nil
	}
}

const testPrivateKey = "test-private-key"
