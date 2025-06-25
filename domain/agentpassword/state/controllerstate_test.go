// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/agentpassword"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	internalpassword "github.com/juju/juju/internal/password"
)

type controllerModelState struct {
	schematesting.ControllerModelSuite
}

func TestControllerStateSuite(t *testing.T) {
	tc.Run(t, &controllerModelState{})
}

func (s *controllerModelState) TestSetControllerModelPassword(c *tc.C) {
	st := NewControllerState(s.TxnRunnerFactory())

	passwordHash := s.genPasswordHash(c)

	err := st.SetControllerNodePasswordHash(c.Context(), "0", passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the password hash was set correctly.
	var hash string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT password_hash FROM controller_node_password WHERE controller_id = 0").Scan(&hash)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hash, tc.Equals, string(passwordHash))
}

func (s *controllerModelState) TestSetControllerModelPasswordDoesNotExist(c *tc.C) {
	st := NewControllerState(s.TxnRunnerFactory())

	passwordHash := s.genPasswordHash(c)

	err := st.SetControllerNodePasswordHash(c.Context(), "1", passwordHash)
	c.Assert(err, tc.ErrorIs, controllernodeerrors.NotFound)
}

func (s *controllerModelState) TestMatchesUnitPasswordHash(c *tc.C) {
	st := NewControllerState(s.TxnRunnerFactory())

	passwordHash := s.genPasswordHash(c)

	err := st.SetControllerNodePasswordHash(c.Context(), "0", passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	valid, err := st.MatchesControllerNodePasswordHash(c.Context(), "0", passwordHash)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(valid, tc.IsTrue)
}

func (s *controllerModelState) TestMatchesUnitPasswordHashUnitNotFound(c *tc.C) {
	st := NewControllerState(s.TxnRunnerFactory())

	passwordHash := s.genPasswordHash(c)

	_, err := st.MatchesControllerNodePasswordHash(c.Context(), "1", passwordHash)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *controllerModelState) TestMatchesUnitPasswordHashInvalidPassword(c *tc.C) {
	st := NewControllerState(s.TxnRunnerFactory())

	passwordHash := s.genPasswordHash(c)

	err := st.SetControllerNodePasswordHash(c.Context(), "0", passwordHash)
	c.Assert(err, tc.ErrorIsNil)

	valid, err := st.MatchesControllerNodePasswordHash(c.Context(), "0", passwordHash+"1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(valid, tc.IsFalse)
}

func (s *controllerModelState) genPasswordHash(c *tc.C) agentpassword.PasswordHash {
	rand, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	return agentpassword.PasswordHash(internalpassword.AgentPasswordHash(rand))
}
