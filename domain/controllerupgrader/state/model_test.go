// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/modelagent"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/uuid"
)

// controllerModelStateSuite is a collection of tests to assert the contracts
// offered by [ControllerModelState].
type controllerModelStateSuite struct {
	schematesting.ModelSuite
}

// TestControllerModelStateSuite runs the tests defined in
// [controllerModelStateSuite].
func TestControllerModelStateSuite(t *testing.T) {
	tc.Run(t, &controllerModelStateSuite{})
}

// checkModelAgentStream is a testing utility for asserting the current value
// of the model's agent stream is equal to the supplied value.
func (s *controllerModelStateSuite) checkModelAgentStream(
	c *tc.C, stream modelagent.AgentStream,
) {
	var val modelagent.AgentStream
	err := s.DB().QueryRow("SELECT stream_id FROM agent_version").Scan(&val)
	c.Check(err, tc.ErrorIsNil)
	c.Check(val, tc.Equals, stream)
}

// seedControllerModel establishes a controller's model information in the
// database.
func (s *controllerModelStateSuite) seedControllerModel(c *tc.C) {
	modelUUID := coremodel.GenUUID(c)
	controllerUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type, qualifier, is_controller_model)
VALUES            (?, ?, "test-model", "iaas", "test-cloud", "ec2", "testq", true)
`,
		modelUUID, controllerUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
}

// seedModel establishes a non controller model's information in the database.
func (s *controllerModelStateSuite) seedModel(c *tc.C) {
	modelUUID := coremodel.GenUUID(c)
	controllerUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type, qualifier)
VALUES            (?, ?, "test-model", "iaas", "test-cloud", "ec2", "testq")
`,
		modelUUID, controllerUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
}

// setModelTargetAgentVersion is a testing utility for establishing an initial
// target agent version for the model.
func (s *controllerModelStateSuite) setModelTargetAgentVersion(c *tc.C, vers string) {
	s.setModelTargetAgentVersionAndStream(c, vers, modelagent.AgentStreamReleased)
}

// setModelTargetAgentVersionAndStream is a testing utility for establishing an
// initial target agent version and stream for the model.
func (s *controllerModelStateSuite) setModelTargetAgentVersionAndStream(
	c *tc.C, vers string, stream modelagent.AgentStream,
) {
	db, err := domain.NewStateBase(s.TxnRunnerFactory()).DB(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	q := "INSERT INTO agent_version (*) VALUES ($M.stream_id, $M.target_version, $M.latest_version)"

	args := sqlair.M{
		"latest_version": vers,
		"target_version": vers,
		"stream_id":      int(stream),
	}
	stmt, err := sqlair.Prepare(q, args)
	c.Assert(err, tc.ErrorIsNil)

	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, args).Run()
	})
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetModelTargetAgentVersionNotSet asserts that if no target agent version
// has been set for the model previously the operation produces an error.
//
// This isn't an expected error condition that a caller should ever have to care
// about. But we do want to see that it fails instead of being opinionated.
func (s *controllerModelStateSuite) TestSetModelTargetAgentVersionNotSet(c *tc.C) {
	preCondition, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)
	toVersion, err := semversion.Parse("4.1.0")
	c.Assert(err, tc.ErrorIsNil)
	s.seedControllerModel(c)
	st := NewControllerModelState(s.TxnRunnerFactory())

	err = st.SetModelTargetAgentVersion(c.Context(), preCondition, toVersion)
	c.Check(err, tc.NotNil)
}

// TestSetModelTargetAgentVersionPreconditionFail asserts that in an attempt to
// set the model target agent version and the precondition fails the caller gets
// back an error and the operation does not succeed.
func (s *controllerModelStateSuite) TestSetModelTargetAgentVersionPreconditionFail(c *tc.C) {
	preCondition, err := semversion.Parse("4.1.0")
	c.Assert(err, tc.ErrorIsNil)
	toVersion, err := semversion.Parse("4.2.0")
	c.Assert(err, tc.ErrorIsNil)
	s.seedControllerModel(c)
	s.setModelTargetAgentVersion(c, "4.1.1")
	st := NewControllerModelState(s.TxnRunnerFactory())

	err = st.SetModelTargetAgentVersion(c.Context(), preCondition, toVersion)
	c.Check(err, tc.NotNil)

	ver, err := st.GetModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver.String(), tc.Equals, "4.1.1")
}

// TestSetModelTargetAgentVersionNotController asserts that if this model is not
// the model that hosts the controller then an error is returned when setting
// the model's target agent version.
func (s *controllerModelStateSuite) TestSetModelTargetAgentVersionNotController(c *tc.C) {
	preCondition, err := semversion.Parse("4.1.0")
	c.Assert(err, tc.ErrorIsNil)
	toVersion, err := semversion.Parse("4.2.0")
	c.Assert(err, tc.ErrorIsNil)
	s.seedModel(c)
	s.setModelTargetAgentVersion(c, "4.1.0")
	st := NewControllerModelState(s.TxnRunnerFactory())

	err = st.SetModelTargetAgentVersion(c.Context(), preCondition, toVersion)
	c.Check(err, tc.NotNil)
}

// TestSetModelTargetAgentVersion is a happy path test for
// [State.SetModelTargetAgentVersion].
func (s *controllerModelStateSuite) TestSetModelTargetAgentVersion(c *tc.C) {
	preCondition, err := semversion.Parse("4.1.0")
	c.Assert(err, tc.ErrorIsNil)
	toVersion, err := semversion.Parse("4.2.0")
	c.Assert(err, tc.ErrorIsNil)
	s.seedControllerModel(c)
	s.setModelTargetAgentVersion(c, "4.1.0")
	st := NewControllerModelState(s.TxnRunnerFactory())

	err = st.SetModelTargetAgentVersion(c.Context(), preCondition, toVersion)
	c.Check(err, tc.ErrorIsNil)

	ver, err := st.GetModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver.String(), tc.Equals, "4.2.0")
}

// TestSetModelTargetAgentVersionStreamNotSet asserts that if no target agent
// version has been set for the model previously the operation produces an
// error.
//
// This isn't an expected error condition that a caller should ever have to care
// about. But we do want to see that it fails instead of being opinionated.
func (s *controllerModelStateSuite) TestSetModelTargetAgentVersionAndStreamNotSet(c *tc.C) {
	preCondition, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)
	toVersion, err := semversion.Parse("4.1.0")
	c.Assert(err, tc.ErrorIsNil)
	s.seedControllerModel(c)
	st := NewControllerModelState(s.TxnRunnerFactory())

	err = st.SetModelTargetAgentVersionAndStream(
		c.Context(), preCondition, toVersion, modelagent.AgentStreamTesting,
	)
	c.Check(err, tc.NotNil)
}

// TestSetModelTargetAgentVersionAndStreamPreconditionFail asserts that in an
// attempt to set the model target agent version/stream and the precondition
// fails the caller gets back an error and the operation does not succeed.
func (s *controllerModelStateSuite) TestSetModelTargetAgentVersionAndStreamPreconditionFail(c *tc.C) {
	preCondition, err := semversion.Parse("4.1.0")
	c.Assert(err, tc.ErrorIsNil)
	toVersion, err := semversion.Parse("4.2.0")
	c.Assert(err, tc.ErrorIsNil)
	s.seedControllerModel(c)
	s.setModelTargetAgentVersionAndStream(c, "4.1.1", modelagent.AgentStreamTesting)
	st := NewControllerModelState(s.TxnRunnerFactory())

	err = st.SetModelTargetAgentVersionAndStream(
		c.Context(), preCondition, toVersion, modelagent.AgentStreamDevel,
	)
	c.Check(err, tc.NotNil)

	ver, err := st.GetModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver.String(), tc.Equals, "4.1.1")

	s.checkModelAgentStream(c, modelagent.AgentStreamTesting)
}

// TestSetModelTargetAgentVersionAndStream is a happy path test for
// [State.SetModelTargetAgentVersionAndStream].
func (s *controllerModelStateSuite) TestSetModelTargetAgentVersionAndStream(c *tc.C) {
	preCondition, err := semversion.Parse("4.1.0")
	c.Assert(err, tc.ErrorIsNil)
	toVersion, err := semversion.Parse("4.2.0")
	c.Assert(err, tc.ErrorIsNil)
	s.seedControllerModel(c)
	s.setModelTargetAgentVersionAndStream(c, "4.1.0", modelagent.AgentStreamReleased)
	st := NewControllerModelState(s.TxnRunnerFactory())

	err = st.SetModelTargetAgentVersionAndStream(
		c.Context(), preCondition, toVersion, modelagent.AgentStreamDevel,
	)
	c.Check(err, tc.ErrorIsNil)

	ver, err := st.GetModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver.String(), tc.Equals, "4.2.0")

	s.checkModelAgentStream(c, modelagent.AgentStreamDevel)
}

// TestSetModelTargetAgentVersionAndStreamNotController asserts that if this
// model is not the model that hosts the controller then an error is returned
// when setting the model's target agent version.
func (s *controllerModelStateSuite) TestSetModelTargetAgentVersionAndStreamNotController(c *tc.C) {
	preCondition, err := semversion.Parse("4.1.0")
	c.Assert(err, tc.ErrorIsNil)
	toVersion, err := semversion.Parse("4.2.0")
	c.Assert(err, tc.ErrorIsNil)
	s.seedModel(c)
	s.setModelTargetAgentVersionAndStream(c, "4.1.0", modelagent.AgentStreamDevel)
	st := NewControllerModelState(s.TxnRunnerFactory())

	err = st.SetModelTargetAgentVersionAndStream(
		c.Context(), preCondition, toVersion, modelagent.AgentStreamReleased,
	)
	c.Check(err, tc.NotNil)
}

// TestSetModelTargetAgentVersionAndStreamNoStreamChange is a happy path test
// for [State.SetModelTargetAgentVersionAndStream]. This test is setting the
// agent stream to the same value it currently is. This test expects no errors
// and for the operation to succeed.
func (s *controllerModelStateSuite) TestSetModelTargetAgentVersionAndStreamNoStreamChange(c *tc.C) {
	preCondition, err := semversion.Parse("4.1.0")
	c.Assert(err, tc.ErrorIsNil)
	toVersion, err := semversion.Parse("4.2.0")
	c.Assert(err, tc.ErrorIsNil)
	s.seedControllerModel(c)
	s.setModelTargetAgentVersionAndStream(c, "4.1.0", modelagent.AgentStreamReleased)
	st := NewControllerModelState(s.TxnRunnerFactory())

	err = st.SetModelTargetAgentVersionAndStream(
		c.Context(), preCondition, toVersion, modelagent.AgentStreamReleased,
	)
	c.Check(err, tc.ErrorIsNil)

	ver, err := st.GetModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver.String(), tc.Equals, "4.2.0")

	s.checkModelAgentStream(c, modelagent.AgentStreamReleased)
}
