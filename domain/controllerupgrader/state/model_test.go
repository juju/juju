// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"strings"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain"
	domainagentbinary "github.com/juju/juju/domain/agentbinary"
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
	c *tc.C, stream domainagentbinary.Stream,
) {
	var val domainagentbinary.Stream
	err := s.DB().QueryRow("SELECT stream_id FROM agent_version").Scan(&val)
	c.Check(err, tc.ErrorIsNil)
	c.Check(val, tc.Equals, stream)
}

// seedControllerModel establishes a controller's model information in the
// database.
func (s *controllerModelStateSuite) seedControllerModel(c *tc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
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
	modelUUID := modeltesting.GenModelUUID(c)
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
	s.setModelTargetAgentVersionAndStream(c, vers, domainagentbinary.AgentStreamReleased)
}

// setModelTargetAgentVersionAndStream is a testing utility for establishing an
// initial target agent version and stream for the model.
func (s *controllerModelStateSuite) setModelTargetAgentVersionAndStream(
	c *tc.C, vers string, stream domainagentbinary.Stream,
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

// addObjectStore inserts a new row to `object_store_metadata` table. Its UUID is returned.
func (s *controllerModelStateSuite) addObjectStore(c *tc.C) objectstore.UUID {
	storeUUID := tc.Must(c, objectstore.NewUUID)
	hasher256 := sha256.New()
	hasher384 := sha512.New384()
	_, err := io.Copy(io.MultiWriter(hasher256, hasher384), strings.NewReader(storeUUID.String()))
	c.Assert(err, tc.ErrorIsNil)
	sha256Hash := hex.EncodeToString(hasher256.Sum(nil))
	sha384Hash := hex.EncodeToString(hasher384.Sum(nil))

	_, err = s.DB().Exec(`
INSERT INTO object_store_metadata (uuid, sha_256, sha_384, size)
VALUES (?, ?, ?, ?)
`, storeUUID, sha256Hash, sha384Hash, 1234)
	c.Assert(err, tc.ErrorIsNil)

	return storeUUID
}

// addAgentBinaryStore inserts a new row to `agent_binary_store` table.
// It is dependent upon architecture and object store metadata for its foreign keys.
// Architecture is auto seeded in the DDL. However, addObjectStore must be invoked prior to
// addAgentBinaryStore.
func (s *controllerModelStateSuite) addAgentBinaryStore(c *tc.C, version semversion.Number, architecture domainagentbinary.Architecture, storeUUID objectstore.UUID) {
	_, err := s.DB().Exec(`
INSERT INTO agent_binary_store(version, architecture_id, object_store_uuid) VALUES(?, ?, ?)
`, version.String(), int(architecture), storeUUID.String())
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
		c.Context(), preCondition, toVersion, domainagentbinary.AgentStreamTesting,
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
	s.setModelTargetAgentVersionAndStream(c, "4.1.1", domainagentbinary.AgentStreamTesting)
	st := NewControllerModelState(s.TxnRunnerFactory())

	err = st.SetModelTargetAgentVersionAndStream(
		c.Context(), preCondition, toVersion, domainagentbinary.AgentStreamDevel,
	)
	c.Check(err, tc.NotNil)

	ver, err := st.GetModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver.String(), tc.Equals, "4.1.1")

	s.checkModelAgentStream(c, domainagentbinary.AgentStreamTesting)
}

// TestSetModelTargetAgentVersionAndStream is a happy path test for
// [State.SetModelTargetAgentVersionAndStream].
func (s *controllerModelStateSuite) TestSetModelTargetAgentVersionAndStream(c *tc.C) {
	preCondition, err := semversion.Parse("4.1.0")
	c.Assert(err, tc.ErrorIsNil)
	toVersion, err := semversion.Parse("4.2.0")
	c.Assert(err, tc.ErrorIsNil)
	s.seedControllerModel(c)
	s.setModelTargetAgentVersionAndStream(c, "4.1.0", domainagentbinary.AgentStreamReleased)
	st := NewControllerModelState(s.TxnRunnerFactory())

	err = st.SetModelTargetAgentVersionAndStream(
		c.Context(), preCondition, toVersion, domainagentbinary.AgentStreamDevel,
	)
	c.Check(err, tc.ErrorIsNil)

	ver, err := st.GetModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver.String(), tc.Equals, "4.2.0")

	s.checkModelAgentStream(c, domainagentbinary.AgentStreamDevel)
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
	s.setModelTargetAgentVersionAndStream(c, "4.1.0", domainagentbinary.AgentStreamDevel)
	st := NewControllerModelState(s.TxnRunnerFactory())

	err = st.SetModelTargetAgentVersionAndStream(
		c.Context(), preCondition, toVersion, domainagentbinary.AgentStreamReleased,
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
	s.setModelTargetAgentVersionAndStream(c, "4.1.0", domainagentbinary.AgentStreamReleased)
	st := NewControllerModelState(s.TxnRunnerFactory())

	err = st.SetModelTargetAgentVersionAndStream(
		c.Context(), preCondition, toVersion, domainagentbinary.AgentStreamReleased,
	)
	c.Check(err, tc.ErrorIsNil)

	ver, err := st.GetModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver.String(), tc.Equals, "4.2.0")

	s.checkModelAgentStream(c, domainagentbinary.AgentStreamReleased)
}

// TestHasAgentBinaryForVersionAndArchitectures tests that the given version and architectures
// exists without returning errors.
func (s *controllerModelStateSuite) TestHasAgentBinaryForVersionAndArchitectures(c *tc.C) {
	version, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)
	storeUUID := s.addObjectStore(c)
	s.addAgentBinaryStore(c, version, domainagentbinary.AMD64, storeUUID)
	s.addAgentBinaryStore(c, version, domainagentbinary.ARM64, storeUUID)

	st := NewControllerModelState(s.TxnRunnerFactory())

	agents, err := st.HasAgentBinariesForVersionAndArchitectures(c.Context(), version, []domainagentbinary.Architecture{domainagentbinary.AMD64, domainagentbinary.ARM64})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(agents, tc.DeepEquals, map[domainagentbinary.Architecture]bool{
		domainagentbinary.AMD64: true,
		domainagentbinary.ARM64: true,
	})
}

// TestHasAgentBinaryForVersionAndArchitectures tests that some of the architectures doesn't exist
// and no errors are returned.
func (s *controllerModelStateSuite) TestHasAgentBinaryForVersionAndArchitecturesSomeArchitecturesDoesntExist(c *tc.C) {
	version, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)
	storeUUID := s.addObjectStore(c)
	s.addAgentBinaryStore(c, version, domainagentbinary.AMD64, storeUUID)
	s.addAgentBinaryStore(c, version, domainagentbinary.ARM64, storeUUID)

	st := NewControllerModelState(s.TxnRunnerFactory())

	agents, err := st.HasAgentBinariesForVersionAndArchitectures(c.Context(), version, []domainagentbinary.Architecture{domainagentbinary.AMD64, domainagentbinary.PPC64EL, domainagentbinary.RISCV64})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(agents, tc.DeepEquals, map[domainagentbinary.Architecture]bool{
		domainagentbinary.AMD64:   true,
		domainagentbinary.PPC64EL: false,
		domainagentbinary.RISCV64: false,
	})
}

// TestGetModelAgentStream tests getting the stream currently in use for the agent.
func (s *controllerModelStateSuite) TestGetModelAgentStream(c *tc.C) {
	version, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)
	s.setModelTargetAgentVersionAndStream(c, version.String(), domainagentbinary.AgentStreamReleased)

	st := NewControllerModelState(s.TxnRunnerFactory())

	agent, err := st.GetModelAgentStream(c.Context())

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(agent, tc.DeepEquals, domainagentbinary.AgentStreamReleased)
}

// TestGetModelAgentStreamDoesntExist tests getting the stream when it is missing.
func (s *controllerModelStateSuite) TestGetModelAgentStreamDoesntExist(c *tc.C) {
	st := NewControllerModelState(s.TxnRunnerFactory())

	_, err := st.GetModelAgentStream(c.Context())

	c.Assert(err, tc.ErrorMatches, "no agent stream has been set for the controller model")
}
