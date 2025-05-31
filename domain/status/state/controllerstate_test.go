// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	credentialstate "github.com/juju/juju/domain/credential/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/uuid"
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

	s.modelUUID = modeltesting.GenModelUUID(c)
}

// insertModelDirectly inserts a model record directly using SQL
func (s *controllerStateSuite) insertModelDirectly(c *tc.C, modelUUID coremodel.UUID) {
	s.insertModelDirectlyWithCredential(c, modelUUID, false)
}

// insertModelDirectlyWithCredential inserts a model record directly using SQL, optionally with a credential
func (s *controllerStateSuite) insertModelDirectlyWithCredential(c *tc.C, modelUUID coremodel.UUID, withCredential bool) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		cloudUUID := uuid.MustNewUUID().String()
		userUUID := uuid.MustNewUUID().String()

		_, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO cloud_type (id, type) VALUES (1, "ec2")
		`)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO cloud (uuid, name, cloud_type_id, endpoint, skip_tls_verify) 
VALUES (?, "test-cloud", 1, "", FALSE)
		`, cloudUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO user (uuid, name, display_name, external, removed, created_by_uuid, created_at) 
VALUES (?, "test-user", "Test User", FALSE, FALSE, ?, datetime('now'))
		`, userUUID, userUUID)
		if err != nil {
			return err
		}

		var credentialUUID *string
		if withCredential {
			_, err = tx.ExecContext(ctx, `
INSERT OR IGNORE INTO auth_type (id, type) VALUES (1, "access-key")
			`)
			if err != nil {
				return err
			}

			credUUID := uuid.MustNewUUID().String()
			_, err = tx.ExecContext(ctx, `
INSERT INTO cloud_credential (uuid, cloud_uuid, auth_type_id, owner_uuid, name, invalid, invalid_reason)
VALUES (?, ?, "1", ?, "test-cred", FALSE, "")
			`, credUUID, cloudUUID, userUUID)
			if err != nil {
				return err
			}
			credentialUUID = &credUUID
		}

		if withCredential {
			_, err = tx.ExecContext(ctx, `
INSERT INTO model (uuid, activated, cloud_uuid, cloud_credential_uuid, model_type_id, life_id, name, owner_uuid)
VALUES (?, TRUE, ?, ?, 0, 0, "test-model", ?)
			`, modelUUID.String(), cloudUUID, *credentialUUID, userUUID)
		} else {
			_, err = tx.ExecContext(ctx, `
INSERT INTO model (uuid, activated, cloud_uuid, model_type_id, life_id, name, owner_uuid)
VALUES (?, TRUE, ?, 0, 0, "test-model", ?)
			`, modelUUID.String(), cloudUUID, userUUID)
		}
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// updateModelLifeDirectly updates model life status directly using SQL
func (s *controllerStateSuite) updateModelLifeDirectly(c *tc.C, modelUUID coremodel.UUID, lifeID int) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE model SET life_id = ? WHERE uuid = ?
		`, lifeID, modelUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// TestGetModelState is asserting the happy path of getting a model's state for
// status. The model is in a normal state and so we are asserting the response
// from the point of the model having nothing interesting to report.
func (s *controllerStateSuite) TestGetModelState(c *tc.C) {
	s.insertModelDirectly(c, s.modelUUID)

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

// TestGetModelStateInvalidCredentials is here to assert that when the model's
// cloud credential is invalid, the model state is updated to indicate this with
// the invalid reason.
func (s *controllerStateSuite) TestGetModelStateInvalidCredentials(c *tc.C) {
	s.insertModelDirectlyWithCredential(c, s.modelUUID, true)

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
	s.insertModelDirectly(c, s.modelUUID)

	st := NewControllerState(s.TxnRunnerFactory(), s.modelUUID)

	s.SeedControllerTable(c, s.modelUUID)

	s.updateModelLifeDirectly(c, s.modelUUID, 1) // 1 = destroying

	mSt, err := st.GetModelState(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(mSt, tc.DeepEquals, domainstatus.ModelState{
		Destroying:                   true,
		Migrating:                    false,
		HasInvalidCloudCredential:    false,
		InvalidCloudCredentialReason: "",
	})
}
