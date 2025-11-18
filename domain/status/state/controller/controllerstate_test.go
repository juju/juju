// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	"github.com/juju/tc"

	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	credentialstate "github.com/juju/juju/domain/credential/state"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modeltesting "github.com/juju/juju/domain/model/state/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainstatus "github.com/juju/juju/domain/status"
)

type controllerStateSuite struct {
	schematesting.ControllerSuite
}

func TestControllerStateSuite(t *testing.T) {
	tc.Run(t, &controllerStateSuite{})
}

func (s *controllerStateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
}

func (s *controllerStateSuite) TestGetModelStatusContext(c *tc.C) {
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-model")
	st := NewControllerState(s.TxnRunnerFactory(), modelUUID)

	mSt, err := st.GetModelStatusContext(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mSt, tc.DeepEquals, domainstatus.ModelStatusContext{
		IsDestroying:                 false,
		IsMigrating:                  false,
		HasInvalidCloudCredential:    false,
		InvalidCloudCredentialReason: "",
	})
}

func (s *controllerStateSuite) TestGetModelStatusContextModelNotFound(c *tc.C) {
	st := NewControllerState(s.TxnRunnerFactory(), "non-existent-model-uuid")

	_, err := st.GetModelStatusContext(c.Context())
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *controllerStateSuite) TestGetModelStatusContextDestroying(c *tc.C) {
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-model")
	st := NewControllerState(s.TxnRunnerFactory(), modelUUID)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE model SET life_id = 1 WHERE uuid = ?
		`, modelUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	mSt, err := st.GetModelStatusContext(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mSt, tc.DeepEquals, domainstatus.ModelStatusContext{
		IsDestroying:                 true,
		IsMigrating:                  false,
		HasInvalidCloudCredential:    false,
		InvalidCloudCredentialReason: "",
	})
}

func (s *controllerStateSuite) TestGetModelStatusContextMigrating(c *tc.C) {
	c.Skip("TODO: Implement this test when v_model_state model migration information is given in the database")
}

func (s *controllerStateSuite) TestGetModelStatusContextInvalidCredentials(c *tc.C) {
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-model")
	st := NewControllerState(s.TxnRunnerFactory(), modelUUID)

	credentialSt := credentialstate.NewState(s.TxnRunnerFactory())
	err := credentialSt.InvalidateModelCloudCredential(
		c.Context(),
		modelUUID,
		"invalid cloud credential",
	)
	c.Assert(err, tc.ErrorIsNil)

	mSt, err := st.GetModelStatusContext(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mSt, tc.DeepEquals, domainstatus.ModelStatusContext{
		IsDestroying:                 false,
		IsMigrating:                  false,
		HasInvalidCloudCredential:    true,
		InvalidCloudCredentialReason: "invalid cloud credential",
	})
}

func (s *controllerStateSuite) TestGetControllerNodeIDs(c *tc.C) {
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-model")
	st := NewControllerState(s.TxnRunnerFactory(), modelUUID)

	// There already exists a controller node with controller_id "0" created
	// during controller setup in SetUpTest. We add two more controller nodes
	// with controller_id "1" and "2" here.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for i := 1; i < 3; i++ {
			controllerID := strconv.Itoa(i)
			nodeID := uint64(1523785546583723502 + i)

			_, err := tx.ExecContext(ctx, `
INSERT INTO controller_node (controller_id, dqlite_node_id, dqlite_bind_address)
VALUES (?, ?, ?)
			`, controllerID, nodeID, "10.0.0."+strconv.Itoa(i))
			if err != nil {
				return err
			}
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	controllerIDs, err := st.GetControllerNodeIDs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerIDs, tc.HasLen, 3)
	c.Check(controllerIDs, tc.DeepEquals, []domainstatus.ControllerNode{{
		ControllerID: "0",
		DqliteNodeID: schematesting.DqliteNodeID,
	}, {
		ControllerID: "1",
		DqliteNodeID: 1523785546583723503,
	}, {
		ControllerID: "2",
		DqliteNodeID: 1523785546583723504,
	}})
}

func (s *controllerStateSuite) TestGetControllerNodeIDsEmpty(c *tc.C) {
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-model")
	st := NewControllerState(s.TxnRunnerFactory(), modelUUID)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM controller_node`)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	controllerIDs, err := st.GetControllerNodeIDs(c.Context())
	c.Assert(err, tc.ErrorIs, controllernodeerrors.EmptyControllerIDs)
	c.Check(controllerIDs, tc.HasLen, 0)
}
