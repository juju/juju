// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	credentialstate "github.com/juju/juju/domain/credential/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainstatus "github.com/juju/juju/domain/status"
)

type controllerStateSuite struct {
	schematesting.ControllerSuite

	modelUUID coremodel.UUID
}

func TestControllerStateSuite(t *testing.T) {
	tc.Run(t, &controllerStateSuite{})
}

func (s *controllerStateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
}

// TestGetModelState is asserting the happy path of getting a model's state for
// status. The model is in a normal state and so we are asserting the response
// from the point of the model having nothing interesting to report.
func (s *controllerStateSuite) TestGetModelState(c *tc.C) {
	st := NewControllerState(s.TxnRunnerFactory(), s.modelUUID)

	mSt, err := st.GetModelState(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(mSt, tc.DeepEquals, domainstatus.ModelState{
		Destroying:                   false,
		Migrating:                    false,
		HasInvalidCloudCredential:    false,
		InvalidCloudCredentialReason: "",
	})
}

// TestGetModelStateinvalidCredentials is here to assert  that when the model's
// cloud credential is invalid, the model state is updated to indicate this with
// the invalid reason.
func (s *controllerStateSuite) TestGetModelStateInvalidCredentials(c *tc.C) {
	st := NewControllerState(s.TxnRunnerFactory(), s.modelUUID)

	credentialSt := credentialstate.NewState(s.TxnRunnerFactory())
	err := credentialSt.InvalidateModelCloudCredential(
		c.Context(),
		s.modelUUID,
		"test-invalid",
	)
	c.Assert(err, tc.ErrorIsNil)

	mSt, err := st.GetModelState(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(mSt, tc.DeepEquals, domainstatus.ModelState{
		Destroying:                   false,
		Migrating:                    false,
		HasInvalidCloudCredential:    true,
		InvalidCloudCredentialReason: "test-invalid",
	})
}

// TestGetModelStateDestroying is asserting that when the model's life is set to
// destroying that the model state is updated to reflect this.
func (s *controllerStateSuite) TestGetModelStateDestroying(c *tc.C) {
	st := NewControllerState(s.TxnRunnerFactory(), s.modelUUID)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE model SET life_id = 1 WHERE uuid = ?
	`, s.modelUUID)
		return err
	})
	c.Check(err, tc.ErrorIsNil)

	mSt, err := st.GetModelState(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(mSt, tc.DeepEquals, domainstatus.ModelState{
		Destroying:                   true,
		Migrating:                    false,
		HasInvalidCloudCredential:    false,
		InvalidCloudCredentialReason: "",
	})
}
