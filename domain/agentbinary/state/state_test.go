// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain/agentbinary"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ControllerSuite

	state *State
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory())
}

// TestAddSuccess asserts the happy path of adding agent binary metadata.
func (s *stateSuite) TestAddSuccess(c *gc.C) {
	archID := s.addArchitecture(c, "amd64")
	objStoreUUID := s.addObjectStoreMetadata(c)

	err := s.state.Add(context.Background(), agentbinary.Metadata{
		Version:         "2.9.0",
		Arch:            "amd64",
		ObjectStoreUUID: objStoreUUID,
	})
	c.Assert(err, jc.ErrorIsNil)

	record := s.getAgentBinaryRecord(c, "2.9.0", archID)
	c.Assert(record.Version, gc.Equals, "2.9.0")
	c.Assert(record.ArchitectureID, gc.Equals, archID)
	c.Assert(record.ObjectStoreUUID, gc.Equals, string(objStoreUUID))
}

// TestAddUpdatesExisting asserts that Add will update the metadata if it already exists.
func (s *stateSuite) TestAddUpdatesExisting(c *gc.C) {
	archID := s.addArchitecture(c, "amd64")
	objStoreUUID1 := s.addObjectStoreMetadata(c)
	objStoreUUID2 := s.addObjectStoreMetadata(c)

	s.addAgentBinary(c, "2.9.0", archID, string(objStoreUUID1))

	err := s.state.Add(context.Background(), agentbinary.Metadata{
		Version:         "2.9.0",
		Arch:            "amd64",
		ObjectStoreUUID: objStoreUUID2,
	})
	c.Assert(err, jc.ErrorIsNil)

	record := s.getAgentBinaryRecord(c, "2.9.0", archID)
	c.Assert(record.Version, gc.Equals, "2.9.0")
	c.Assert(record.ArchitectureID, gc.Equals, archID)
	c.Assert(record.ObjectStoreUUID, gc.Equals, string(objStoreUUID2))
}

// TestAddErrorArchitectureNotFound asserts that a NotSupported error is returned
// when the architecture is not found.
func (s *stateSuite) TestAddErrorArchitectureNotFound(c *gc.C) {
	objStoreUUID := s.addObjectStoreMetadata(c)

	err := s.state.Add(context.Background(), agentbinary.Metadata{
		Version:         "2.9.0",
		Arch:            "non-existent-arch",
		ObjectStoreUUID: objStoreUUID,
	})
	c.Assert(err, jc.ErrorIs, coreerrors.NotSupported)
}

// TestAddErrorObjectStoreUUIDNotFound asserts that a NotFound error is returned
// when the object store UUID is not found.
func (s *stateSuite) TestAddErrorObjectStoreUUIDNotFound(c *gc.C) {
	s.addArchitecture(c, "amd64")

	err := s.state.Add(context.Background(), agentbinary.Metadata{
		Version:         "2.9.0",
		Arch:            "amd64",
		ObjectStoreUUID: objectstore.UUID(uuid.MustNewUUID().String()),
	})
	c.Assert(err, jc.ErrorIs, coreerrors.NotFound)
}

// Helper methods

func (s *stateSuite) addArchitecture(c *gc.C, name string) int {
	runner := s.TxnRunner()

	// First check if the architecture already exists
	selectStmt, err := sqlair.Prepare(`
		SELECT id AS &architectureRecord.id
		FROM architecture
		WHERE name = $architectureRecord.name
	`, architectureRecord{})
	c.Assert(err, jc.ErrorIsNil)

	record := architectureRecord{Name: name}
	err = runner.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
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
	c.Assert(err, jc.ErrorIsNil)

	err = runner.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, insertStmt, record).Get(&record)
	})
	c.Assert(err, jc.ErrorIsNil)
	return record.ID
}

func (s *stateSuite) addObjectStoreMetadata(c *gc.C) objectstore.UUID {
	runner := s.TxnRunner()

	storeUUID := uuid.MustNewUUID().String()
	stmt, err := sqlair.Prepare(`
		INSERT INTO object_store_metadata (uuid, sha_256, sha_384, size)
		VALUES ($objectStoreMeta.uuid, $objectStoreMeta.sha_256, $objectStoreMeta.sha_384, $objectStoreMeta.size)
	`, objectStoreMeta{})
	c.Assert(err, jc.ErrorIsNil)

	// Generate unique SHA values using UUIDs
	sha256Val := "sha256-" + uuid.MustNewUUID().String()
	sha384Val := "sha384-" + uuid.MustNewUUID().String()

	record := objectStoreMeta{
		UUID:   storeUUID,
		SHA256: sha256Val,
		SHA384: sha384Val,
		Size:   1234,
	}
	err = runner.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, record).Run()
	})
	c.Assert(err, jc.ErrorIsNil)
	return objectstore.UUID(storeUUID)
}

func (s *stateSuite) addAgentBinary(c *gc.C, version string, archID int, objStoreUUID string) {
	runner := s.TxnRunner()

	stmt, err := sqlair.Prepare(`
		INSERT INTO agent_binary_store (version, architecture_id, object_store_uuid)
		VALUES ($agentBinaryRecord.version, $agentBinaryRecord.architecture_id, $agentBinaryRecord.object_store_uuid)
	`, agentBinaryRecord{})
	c.Assert(err, jc.ErrorIsNil)

	record := agentBinaryRecord{
		Version:         version,
		ArchitectureID:  archID,
		ObjectStoreUUID: objStoreUUID,
	}
	err = runner.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, record).Run()
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) getAgentBinaryRecord(c *gc.C, version string, archID int) agentBinaryRecord {
	runner := s.TxnRunner()

	stmt, err := sqlair.Prepare(`
		SELECT version AS &agentBinaryRecord.version, 
		       architecture_id AS &agentBinaryRecord.architecture_id, 
		       object_store_uuid AS &agentBinaryRecord.object_store_uuid
		FROM agent_binary_store
		WHERE version = $agentBinaryRecord.version AND architecture_id = $agentBinaryRecord.architecture_id
	`, agentBinaryRecord{})
	c.Assert(err, jc.ErrorIsNil)

	params := agentBinaryRecord{
		Version:        version,
		ArchitectureID: archID,
	}
	var record agentBinaryRecord
	err = runner.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, params).Get(&record)
	})
	c.Assert(err, jc.ErrorIsNil)
	return record
}
