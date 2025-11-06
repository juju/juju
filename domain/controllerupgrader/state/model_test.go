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
	"github.com/juju/juju/domain/agentbinary"
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
	c *tc.C, stream agentbinary.Stream,
) {
	var val agentbinary.Stream
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
	s.setModelTargetAgentVersionAndStream(c, vers, agentbinary.AgentStreamReleased)
}

// setModelTargetAgentVersionAndStream is a testing utility for establishing an
// initial target agent version and stream for the model.
func (s *controllerModelStateSuite) setModelTargetAgentVersionAndStream(
	c *tc.C, vers string, stream agentbinary.Stream,
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
func (s *controllerModelStateSuite) addAgentBinaryStore(c *tc.C, version semversion.Number, architecture agentbinary.Architecture, storeUUID objectstore.UUID) {
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
		c.Context(), preCondition, toVersion, agentbinary.AgentStreamTesting,
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
	s.setModelTargetAgentVersionAndStream(c, "4.1.1", agentbinary.AgentStreamTesting)
	st := NewControllerModelState(s.TxnRunnerFactory())

	err = st.SetModelTargetAgentVersionAndStream(
		c.Context(), preCondition, toVersion, agentbinary.AgentStreamDevel,
	)
	c.Check(err, tc.NotNil)

	ver, err := st.GetModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver.String(), tc.Equals, "4.1.1")

	s.checkModelAgentStream(c, agentbinary.AgentStreamTesting)
}

// TestSetModelTargetAgentVersionAndStream is a happy path test for
// [State.SetModelTargetAgentVersionAndStream].
func (s *controllerModelStateSuite) TestSetModelTargetAgentVersionAndStream(c *tc.C) {
	preCondition, err := semversion.Parse("4.1.0")
	c.Assert(err, tc.ErrorIsNil)
	toVersion, err := semversion.Parse("4.2.0")
	c.Assert(err, tc.ErrorIsNil)
	s.seedControllerModel(c)
	s.setModelTargetAgentVersionAndStream(c, "4.1.0", agentbinary.AgentStreamReleased)
	st := NewControllerModelState(s.TxnRunnerFactory())

	err = st.SetModelTargetAgentVersionAndStream(
		c.Context(), preCondition, toVersion, agentbinary.AgentStreamDevel,
	)
	c.Check(err, tc.ErrorIsNil)

	ver, err := st.GetModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver.String(), tc.Equals, "4.2.0")

	s.checkModelAgentStream(c, agentbinary.AgentStreamDevel)
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
	s.setModelTargetAgentVersionAndStream(c, "4.1.0", agentbinary.AgentStreamDevel)
	st := NewControllerModelState(s.TxnRunnerFactory())

	err = st.SetModelTargetAgentVersionAndStream(
		c.Context(), preCondition, toVersion, agentbinary.AgentStreamReleased,
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
	s.setModelTargetAgentVersionAndStream(c, "4.1.0", agentbinary.AgentStreamReleased)
	st := NewControllerModelState(s.TxnRunnerFactory())

	err = st.SetModelTargetAgentVersionAndStream(
		c.Context(), preCondition, toVersion, agentbinary.AgentStreamReleased,
	)
	c.Check(err, tc.ErrorIsNil)

	ver, err := st.GetModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver.String(), tc.Equals, "4.2.0")

	s.checkModelAgentStream(c, agentbinary.AgentStreamReleased)
}

// TestHasAgentBinaryForVersionAndArchitectures tests that the given version and architectures
// exists without returning errors.
//func (s *controllerModelStateSuite) TestHasAgentBinaryForVersionAndArchitectures(c *tc.C) {
//	version, err := semversion.Parse("4.0.0")
//	c.Assert(err, tc.ErrorIsNil)
//	storeUUID := s.addObjectStore(c)
//	s.addAgentBinaryStore(c, version, agentbinary.AMD64, storeUUID)
//	s.addAgentBinaryStore(c, version, agentbinary.ARM64, storeUUID)
//
//	st := NewControllerModelState(s.TxnRunnerFactory())
//
//	agents, err := st.HasAgentBinariesForVersionAndArchitectures(c.Context(), version, []agentbinary.Architecture{agentbinary.AMD64, agentbinary.ARM64})
//
//	c.Assert(err, tc.ErrorIsNil)
//	c.Assert(agents, tc.DeepEquals, map[agentbinary.Architecture]bool{
//		agentbinary.AMD64: true,
//		agentbinary.ARM64: true,
//	})
//}

// TestHasAgentBinaryForVersionAndArchitectures tests that some of the architectures doesn't exist
// and no errors are returned.
//func (s *controllerModelStateSuite) TestHasAgentBinaryForVersionAndArchitecturesSomeArchitecturesDoesntExist(c *tc.C) {
//	version, err := semversion.Parse("4.0.0")
//	c.Assert(err, tc.ErrorIsNil)
//	storeUUID := s.addObjectStore(c)
//	s.addAgentBinaryStore(c, version, agentbinary.AMD64, storeUUID)
//	s.addAgentBinaryStore(c, version, agentbinary.ARM64, storeUUID)
//
//	st := NewControllerModelState(s.TxnRunnerFactory())
//
//	agents, err := st.HasAgentBinariesForVersionAndArchitectures(c.Context(), version, []agentbinary.Architecture{agentbinary.AMD64, agentbinary.PPC64EL, agentbinary.RISCV64})
//
//	c.Assert(err, tc.ErrorIsNil)
//	c.Assert(agents, tc.DeepEquals, map[agentbinary.Architecture]bool{
//		agentbinary.AMD64:   true,
//		agentbinary.PPC64EL: false,
//		agentbinary.RISCV64: false,
//	})
//}

// TestGetModelAgentStream tests getting the stream currently in use for the agent.
func (s *controllerModelStateSuite) TestGetModelAgentStream(c *tc.C) {
	version, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)
	s.setModelTargetAgentVersionAndStream(c, version.String(), agentbinary.AgentStreamReleased)

	st := NewControllerModelState(s.TxnRunnerFactory())

	agent, err := st.GetModelAgentStream(c.Context())

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(agent, tc.DeepEquals, agentbinary.AgentStreamReleased)
}

// TestGetModelAgentStreamDoesntExist tests getting the stream when it is missing.
func (s *controllerModelStateSuite) TestGetModelAgentStreamDoesntExist(c *tc.C) {
	st := NewControllerModelState(s.TxnRunnerFactory())

	_, err := st.GetModelAgentStream(c.Context())

	c.Assert(err, tc.ErrorMatches, "no agent stream has been set for the controller model")
}

// TestGetAllAgentStoreBinariesForStreamEmpty tests that then the model's object
// store has no binaries available
// [ControllerModelState.GetAllAgentStoreBinariesForStream] returns an empty
// list.
func (s *controllerModelStateSuite) TestGetAllAgentStoreBinariesForStreamEmpty(c *tc.C) {
	s.seedModel(c)
	s.setModelTargetAgentVersionAndStream(
		c, "4.1.0", agentbinary.AgentStreamReleased,
	)

	st := NewControllerModelState(s.TxnRunnerFactory())
	vals, err := st.GetAllAgentStoreBinariesForStream(
		c.Context(), agentbinary.AgentStreamReleased,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(vals, tc.HasLen, 0)
}

// TestGetAllAgentStoreBinariesForNotCurrentStream tests that if we ask for the
// agent binaries of a stream that differs from that of the model an empty
// slice of results is returned.
func (s *controllerModelStateSuite) TestGetAllAgentStoreBinariesForNotCurrentStream(c *tc.C) {
	s.seedModel(c)
	s.setModelTargetAgentVersionAndStream(
		c, "4.0.0", agentbinary.AgentStreamDevel,
	)

	ver, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)
	objectUUID := s.addObjectStore(c)
	s.addAgentBinaryStore(c, ver, agentbinary.AMD64, objectUUID)

	st := NewControllerModelState(s.TxnRunnerFactory())
	vals, err := st.GetAllAgentStoreBinariesForStream(
		c.Context(), agentbinary.AgentStreamReleased,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(vals, tc.HasLen, 0)
}

func (s *controllerModelStateSuite) TestGetAllAgentStoreBinariesForStream(c *tc.C) {
	s.seedModel(c)
	s.setModelTargetAgentVersionAndStream(
		c, "4.0.0", agentbinary.AgentStreamReleased,
	)

	ver1, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)
	ver2, err := semversion.Parse("4.0.2")
	c.Assert(err, tc.ErrorIsNil)
	ver3, err := semversion.Parse("4.0.3")
	c.Assert(err, tc.ErrorIsNil)

	objectUUID1 := s.addObjectStore(c)
	objectUUID2 := s.addObjectStore(c)
	objectUUID3 := s.addObjectStore(c)
	s.addAgentBinaryStore(c, ver1, agentbinary.AMD64, objectUUID1)
	s.addAgentBinaryStore(c, ver2, agentbinary.AMD64, objectUUID2)
	s.addAgentBinaryStore(c, ver3, agentbinary.AMD64, objectUUID3)

	st := NewControllerModelState(s.TxnRunnerFactory())
	vals, err := st.GetAllAgentStoreBinariesForStream(
		c.Context(), agentbinary.AgentStreamReleased,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(vals, tc.SameContents, []agentbinary.AgentBinary{
		{
			Architecture: agentbinary.AMD64,
			Stream:       agentbinary.AgentStreamReleased,
			Version:      ver3,
		},
		{
			Architecture: agentbinary.AMD64,
			Stream:       agentbinary.AgentStreamReleased,
			Version:      ver1,
		},
		{
			Architecture: agentbinary.AMD64,
			Stream:       agentbinary.AgentStreamReleased,
			Version:      ver2,
		},
	})
}
