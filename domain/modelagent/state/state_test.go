// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/domain"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modelstatetesting "github.com/juju/juju/domain/model/state/testing"
	modelagentstate "github.com/juju/juju/domain/modelagent/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

// TestGetModelAgentVersionSuccess tests that State.GetModelAgentVersion is
// correct in the expected case when the model exists.
func (s *stateSuite) TestGetModelAgentVersionSuccess(c *gc.C) {
	expectedVersion, err := version.Parse("4.21.54")
	c.Assert(err, jc.ErrorIsNil)

	txnRunner := s.TxnRunnerFactory()
	state := modelagentstate.NewState(txnRunner)
	modelID := modelstatetesting.CreateTestModel(c, txnRunner, "test")
	s.setAgentVersion(c, modelID, expectedVersion.String())

	obtainedVersion, err := state.GetModelAgentVersion(context.Background(), modelID)
	c.Check(err, jc.ErrorIsNil)
	c.Check(obtainedVersion, jc.DeepEquals, expectedVersion)
}

// TestGetModelAgentVersionModelNotFound tests that State.GetModelAgentVersion
// returns modelerrors.NotFound when the model does not exist in the DB.
func (s *stateSuite) TestGetModelAgentVersionModelNotFound(c *gc.C) {
	txnRunner := s.TxnRunnerFactory()
	state := modelagentstate.NewState(txnRunner)
	modelID := modeltesting.GenModelUUID(c)

	_, err := state.GetModelAgentVersion(context.Background(), modelID)
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestGetModelAgentVersionCantParseVersion tests that State.GetModelAgentVersion
// returns an appropriate error when the agent version in the DB is invalid.
func (s *stateSuite) TestGetModelAgentVersionCantParseVersion(c *gc.C) {
	txnRunner := s.TxnRunnerFactory()
	state := modelagentstate.NewState(txnRunner)
	modelID := modelstatetesting.CreateTestModel(c, txnRunner, "test")
	s.setAgentVersion(c, modelID, "invalid-version")

	_, err := state.GetModelAgentVersion(context.Background(), modelID)
	c.Check(err, gc.ErrorMatches, `cannot parse agent version "invalid-version".*`)
}

// Set the agent version for the given model in the DB.
func (s *stateSuite) setAgentVersion(c *gc.C, modelID model.UUID, vers string) {
	st := domain.NewStateBase(s.TxnRunnerFactory())
	db, err := st.DB()
	c.Assert(err, jc.ErrorIsNil)

	q := `
UPDATE model_agent
SET target_version = $M.target_agent_version
WHERE model_uuid = $M.model_id
`
	args := sqlair.M{
		"model_id":             modelID,
		"target_agent_version": vers,
	}

	stmt, err := st.Prepare(q, args)
	c.Assert(err, jc.ErrorIsNil)

	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, args).Run()
	})
	c.Assert(err, jc.ErrorIsNil)
}
