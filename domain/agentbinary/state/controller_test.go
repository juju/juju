// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/agentbinary"
	agentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/uuid"
)

type controllerStateSuite struct {
	schematesting.ControllerSuite

	state *ControllerState
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &controllerStateSuite{})
}

func (s *controllerStateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.state = NewControllerState(s.TxnRunnerFactory())
}

// TestAddSuccess asserts the happy path of adding agent binary metadata.
func (s *controllerStateSuite) TestAddSuccess(c *tc.C) {
	archID := s.addArchitecture(c, "amd64")
	objStoreUUID, _ := addObjectStore(c, s.TxnRunner())

	err := s.state.RegisterAgentBinary(c.Context(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "amd64",
		ObjectStoreUUID: objStoreUUID,
	})
	c.Assert(err, tc.ErrorIsNil)

	record := s.getAgentBinaryRecord(c, "4.0.0", archID)
	c.Check(record.Version, tc.Equals, "4.0.0")
	c.Check(record.ArchitectureID, tc.Equals, archID)
	c.Check(record.ObjectStoreUUID, tc.Equals, objStoreUUID.String())
}

// TestAddAlreadyExists asserts that an error is returned when the agent binary
// already exists. The error will satisfy [agentbinaryerrors.AlreadyExists].
func (s *controllerStateSuite) TestAddAlreadyExists(c *tc.C) {
	archID := s.addArchitecture(c, "amd64")
	objStoreUUID1, _ := addObjectStore(c, s.TxnRunner())

	err := s.state.RegisterAgentBinary(c.Context(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "amd64",
		ObjectStoreUUID: objStoreUUID1,
	})
	c.Check(err, tc.ErrorIsNil)

	err = s.state.RegisterAgentBinary(c.Context(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "amd64",
		ObjectStoreUUID: objStoreUUID1,
	})
	c.Check(err, tc.ErrorIs, agentbinaryerrors.AlreadyExists)

	record := s.getAgentBinaryRecord(c, "4.0.0", archID)
	c.Check(record.Version, tc.Equals, "4.0.0")
	c.Check(record.ArchitectureID, tc.Equals, archID)
	c.Check(record.ObjectStoreUUID, tc.Equals, objStoreUUID1.String())
}

// TestAddFailedUpdateExistingWithDifferentSHA asserts that an error is returned
// when the agent binary already exists with a different SHA. The error will
// satisfy [agentbinaryerrors.AgentBinaryImmutable].
func (s *controllerStateSuite) TestAddFailedUpdateExistingWithDifferentSHA(c *tc.C) {
	archID := s.addArchitecture(c, "amd64")
	objStoreUUID1, _ := addObjectStore(c, s.TxnRunner())
	objStoreUUID2, _ := addObjectStore(c, s.TxnRunner())

	err := s.state.RegisterAgentBinary(c.Context(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "amd64",
		ObjectStoreUUID: objStoreUUID1,
	})
	c.Check(err, tc.ErrorIsNil)

	err = s.state.RegisterAgentBinary(c.Context(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "amd64",
		ObjectStoreUUID: objStoreUUID2,
	})
	c.Check(err, tc.ErrorIs, agentbinaryerrors.AgentBinaryImmutable)

	record := s.getAgentBinaryRecord(c, "4.0.0", archID)
	c.Check(record.Version, tc.Equals, "4.0.0")
	c.Check(record.ArchitectureID, tc.Equals, archID)
	c.Check(record.ObjectStoreUUID, tc.Equals, objStoreUUID1.String())
}

// TestAddErrorArchitectureNotFound asserts that a [coreerrors.NotSupported]
// error is returned when the architecture is not found.
func (s *controllerStateSuite) TestAddErrorArchitectureNotFound(c *tc.C) {
	objStoreUUID, _ := addObjectStore(c, s.TxnRunner())

	err := s.state.RegisterAgentBinary(c.Context(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "non-existent-arch",
		ObjectStoreUUID: objStoreUUID,
	})
	c.Check(err, tc.ErrorIs, coreerrors.NotSupported)
}

// TestAddErrorObjectStoreUUIDNotFound asserts that a
// [agentbinaryerrors.ObjectNotFound] error is returned when the object store
// UUID is not found.
func (s *controllerStateSuite) TestAddErrorObjectStoreUUIDNotFound(c *tc.C) {
	s.addArchitecture(c, "amd64")

	err := s.state.RegisterAgentBinary(c.Context(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "amd64",
		ObjectStoreUUID: objectstore.UUID(uuid.MustNewUUID().String()),
	})
	c.Check(err, tc.ErrorIs, agentbinaryerrors.ObjectNotFound)
}

func (s *controllerStateSuite) addArchitecture(c *tc.C, name string) int {
	runner := s.TxnRunner()

	// First check if the architecture already exists
	selectStmt, err := sqlair.Prepare(`
SELECT id AS &architectureRecord.id
FROM architecture
WHERE name = $architectureRecord.name
`, architectureRecord{})
	c.Assert(err, tc.ErrorIsNil)

	record := architectureRecord{Name: name}
	err = runner.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, selectStmt, record).Get(&record)
	})

	// If architecture exists, return its ID
	if err == nil {
		return record.ID
	}

	// Otherwise insert the new architecture
	insertStmt, err := sqlair.Prepare(`
INSERT INTO architecture (name)
VALUES ($architectureRecord.name)
RETURNING id AS &architectureRecord.id
`, architectureRecord{})
	c.Assert(err, tc.ErrorIsNil)

	err = runner.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, insertStmt, record).Get(&record)
	})
	c.Assert(err, tc.ErrorIsNil)
	return record.ID
}

func (s *controllerStateSuite) getAgentBinaryRecord(c *tc.C, version string, archID int) agentBinaryRecord {
	runner := s.TxnRunner()

	stmt, err := sqlair.Prepare(`
SELECT version AS &agentBinaryRecord.version,
       architecture_id AS &agentBinaryRecord.architecture_id,
       object_store_uuid AS &agentBinaryRecord.object_store_uuid
FROM agent_binary_store
WHERE version = $agentBinaryRecord.version AND architecture_id = $agentBinaryRecord.architecture_id
`, agentBinaryRecord{})
	c.Assert(err, tc.ErrorIsNil)

	params := agentBinaryRecord{
		Version:        version,
		ArchitectureID: archID,
	}
	var record agentBinaryRecord
	err = runner.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, params).Get(&record)
	})
	c.Assert(err, tc.ErrorIsNil)
	return record
}

func (s *controllerStateSuite) TestGetObjectUUID(c *tc.C) {
	objStoreUUID, path := addObjectStore(c, s.TxnRunner())
	gotUUID, err := s.state.GetObjectUUID(c.Context(), path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotUUID.String(), tc.Equals, objStoreUUID.String())
}

func (s *controllerStateSuite) TestGetObjectUUIDFailedObjectNotFound(c *tc.C) {
	_, err := s.state.GetObjectUUID(c.Context(), "non-existent-path")
	c.Check(err, tc.ErrorIs, agentbinaryerrors.ObjectNotFound)
}

func (s *controllerStateSuite) TestListAgentBinaries(c *tc.C) {
	_ = s.addArchitecture(c, "amd64")

	objStoreUUID, _ := addObjectStore(c, s.TxnRunner())
	err := s.state.RegisterAgentBinary(c.Context(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "amd64",
		ObjectStoreUUID: objStoreUUID,
	})
	c.Assert(err, tc.ErrorIsNil)
	binary1 := getMetadata(c, s.DB(), objStoreUUID)

	objStoreUUID, _ = addObjectStore(c, s.TxnRunner())
	err = s.state.RegisterAgentBinary(c.Context(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.0.1",
		Arch:            "amd64",
		ObjectStoreUUID: objStoreUUID,
	})
	c.Assert(err, tc.ErrorIsNil)
	binary2 := getMetadata(c, s.DB(), objStoreUUID)

	binaries, err := s.state.ListAgentBinaries(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(binaries, tc.SameContents, []agentbinary.Metadata{
		binary1,
		binary2,
	})
}

func (s *controllerStateSuite) TestListAgentBinariesEmpty(c *tc.C) {
	binaries, err := s.state.ListAgentBinaries(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(binaries, tc.HasLen, 0)
}

func (s *controllerStateSuite) TestCheckAgentBinarySHA256Exists(c *tc.C) {
	objStoreUUID, _ := addObjectStore(c, s.TxnRunner())

	err := s.state.RegisterAgentBinary(c.Context(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "amd64",
		ObjectStoreUUID: objStoreUUID,
	})
	c.Assert(err, tc.ErrorIsNil)

	sha := getMetadata(c, s.DB(), objStoreUUID).SHA256
	exists, err := s.state.CheckAgentBinarySHA256Exists(c.Context(), sha)
	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)
}

func (s *controllerStateSuite) TestCheckAgentBinarySHA256NoExists(c *tc.C) {
	objStoreUUID, _ := addObjectStore(c, s.TxnRunner())
	sha := getObjectSHA256(c, s.DB(), objStoreUUID)
	exists, err := s.state.CheckAgentBinarySHA256Exists(c.Context(), sha)
	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *controllerStateSuite) TestGetAgentBinarySHA256Exists(c *tc.C) {
	objStoreUUID, _ := addObjectStore(c, s.TxnRunner())
	sha := getObjectSHA256(c, s.DB(), objStoreUUID)
	err := s.state.RegisterAgentBinary(c.Context(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "amd64",
		ObjectStoreUUID: objStoreUUID,
	})
	c.Assert(err, tc.ErrorIsNil)

	num, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)

	ver := coreagentbinary.Version{Number: num, Arch: "amd64"}
	exists, shaRes, err := s.state.GetAgentBinarySHA256(c.Context(), ver, agentbinary.AgentStreamDevel)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exists, tc.Equals, true)
	c.Check(shaRes, tc.Equals, sha)
}

func (s *controllerStateSuite) TestGetAgentBinarySHA256NoExists(c *tc.C) {
	num, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)

	ver := coreagentbinary.Version{Number: num, Arch: "amd64"}
	exists, shaRes, err := s.state.GetAgentBinarySHA256(c.Context(), ver, agentbinary.AgentStreamDevel)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exists, tc.Equals, false)
	c.Check(shaRes, tc.Equals, "")
}
