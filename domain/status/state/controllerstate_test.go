// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/tc"

	corecloud "github.com/juju/juju/core/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	credentialstate "github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/domain/model"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type controllerStateSuite struct {
	schematesting.ControllerSuite

	uuid     coremodel.UUID
	userUUID user.UUID
	userName user.Name

	cloudUUID      corecloud.UUID
	credentialUUID corecredential.UUID
}

var _ = tc.Suite(&controllerStateSuite{})

// TestGetModelState is asserting the happy path of getting a model's state for
// status. The model is in a normal state and so we are asserting the response
// from the point of the model having nothing interesting to report.
func (s *controllerStateSuite) TestGetModelState(c *tc.C) {
	st := NewControllerState(s.TxnRunnerFactory())

	mSt, err := st.GetModelState(c.Context(), s.uuid)
	c.Check(err, tc.ErrorIsNil)
	c.Check(mSt, tc.DeepEquals, model.ModelState{
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
	st := NewControllerState(s.TxnRunnerFactory())
	m, err := st.GetModel(c.Context(), s.uuid)
	c.Assert(err, tc.ErrorIsNil)

	credentialSt := credentialstate.NewState(s.TxnRunnerFactory())
	err = credentialSt.InvalidateModelCloudCredential(
		c.Context(),
		m.UUID,
		"test-invalid",
	)
	c.Assert(err, tc.ErrorIsNil)

	mSt, err := st.GetModelState(c.Context(), s.uuid)
	c.Check(err, tc.ErrorIsNil)
	c.Check(mSt, tc.DeepEquals, model.ModelState{
		Destroying:                   false,
		Migrating:                    false,
		HasInvalidCloudCredential:    true,
		InvalidCloudCredentialReason: "test-invalid",
	})
}

// TestGetModelStateDestroying is asserting that when the model's life is set to
// destroying that the model state is updated to reflect this.
func (s *controllerStateSuite) TestGetModelStateDestroying(c *tc.C) {
	st := NewControllerState(s.TxnRunnerFactory())

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE model SET life_id = 1 WHERE uuid = ?
	`, s.uuid)
		return err
	})
	c.Check(err, tc.ErrorIsNil)

	mSt, err := st.GetModelState(c.Context(), s.uuid)
	c.Check(err, tc.ErrorIsNil)
	c.Check(mSt, tc.DeepEquals, model.ModelState{
		Destroying:                   true,
		Migrating:                    false,
		HasInvalidCloudCredential:    false,
		InvalidCloudCredentialReason: "",
	})
}
