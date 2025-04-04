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

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain/agentbinary"
	agentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
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
	objStoreUUID, _ := s.addObjectStore(c)

	err := s.state.AddAgentBinary(context.Background(), agentbinary.AddAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "amd64",
		ObjectStoreUUID: objStoreUUID,
	})
	c.Assert(err, jc.ErrorIsNil)

	record := s.getAgentBinaryRecord(c, "4.0.0", archID)
	c.Check(record.Version, gc.Equals, "4.0.0")
	c.Check(record.ArchitectureID, gc.Equals, archID)
	c.Check(record.ObjectStoreUUID, gc.Equals, objStoreUUID.String())
}

// TestAddAlreadyExists asserts that an error is returned when the agent binary
// already exists. The error will satisfy [agentbinaryerrors.AlreadyExists].
func (s *stateSuite) TestAddAlreadyExists(c *gc.C) {
	archID := s.addArchitecture(c, "amd64")
	objStoreUUID1, _ := s.addObjectStore(c)

	err := s.state.AddAgentBinary(context.Background(), agentbinary.AddAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "amd64",
		ObjectStoreUUID: objStoreUUID1,
	})
	c.Check(err, jc.ErrorIsNil)

	err = s.state.AddAgentBinary(context.Background(), agentbinary.AddAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "amd64",
		ObjectStoreUUID: objStoreUUID1,
	})
	c.Check(err, jc.ErrorIs, agentbinaryerrors.AlreadyExists)

	record := s.getAgentBinaryRecord(c, "4.0.0", archID)
	c.Check(record.Version, gc.Equals, "4.0.0")
	c.Check(record.ArchitectureID, gc.Equals, archID)
	c.Check(record.ObjectStoreUUID, gc.Equals, objStoreUUID1.String())
}

// TestAddFailedUpdateExistingWithDifferentSHA asserts that an error is returned
// when the agent binary already exists with a different SHA. The error will
// satisfy [agentbinaryerrors.AgentBinaryImmutable].
func (s *stateSuite) TestAddFailedUpdateExistingWithDifferentSHA(c *gc.C) {
	archID := s.addArchitecture(c, "amd64")
	objStoreUUID1, _ := s.addObjectStore(c)
	objStoreUUID2, _ := s.addObjectStore(c)

	err := s.state.AddAgentBinary(context.Background(), agentbinary.AddAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "amd64",
		ObjectStoreUUID: objStoreUUID1,
	})
	c.Check(err, jc.ErrorIsNil)

	err = s.state.AddAgentBinary(context.Background(), agentbinary.AddAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "amd64",
		ObjectStoreUUID: objStoreUUID2,
	})
	c.Check(err, jc.ErrorIs, agentbinaryerrors.AgentBinaryImmutable)

	record := s.getAgentBinaryRecord(c, "4.0.0", archID)
	c.Check(record.Version, gc.Equals, "4.0.0")
	c.Check(record.ArchitectureID, gc.Equals, archID)
	c.Check(record.ObjectStoreUUID, gc.Equals, objStoreUUID1.String())
}

// TestAddErrorArchitectureNotFound asserts that a [coreerrors.NotSupported]
// error is returned when the architecture is not found.
func (s *stateSuite) TestAddErrorArchitectureNotFound(c *gc.C) {
	objStoreUUID, _ := s.addObjectStore(c)

	err := s.state.AddAgentBinary(context.Background(), agentbinary.AddAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "non-existent-arch",
		ObjectStoreUUID: objStoreUUID,
	})
	c.Check(err, jc.ErrorIs, coreerrors.NotSupported)
}

// TestAddErrorObjectStoreUUIDNotFound asserts that a
// [agentbinaryerrors.ObjectNotFound] error is returned when the object store
// UUID is not found.
func (s *stateSuite) TestAddErrorObjectStoreUUIDNotFound(c *gc.C) {
	s.addArchitecture(c, "amd64")

	err := s.state.AddAgentBinary(context.Background(), agentbinary.AddAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "amd64",
		ObjectStoreUUID: objectstore.UUID(uuid.MustNewUUID().String()),
	})
	c.Check(err, jc.ErrorIs, agentbinaryerrors.ObjectNotFound)
}

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

func (s *stateSuite) addObjectStore(c *gc.C) (objectstore.UUID, string) {
	runner := s.TxnRunner()

	type objectStoreMeta struct {
		UUID   string `db:"uuid"`
		SHA256 string `db:"sha_256"`
		SHA384 string `db:"sha_384"`
		Size   int    `db:"size"`
	}

	storeUUID := uuid.MustNewUUID().String()
	stmt, err := sqlair.Prepare(`
INSERT INTO object_store_metadata (uuid, sha_256, sha_384, size)
VALUES ($objectStoreMeta.uuid, $objectStoreMeta.sha_256, $objectStoreMeta.sha_384, $objectStoreMeta.size)
`, objectStoreMeta{})
	c.Assert(err, jc.ErrorIsNil)

	hasher256 := sha256.New()
	hasher384 := sha512.New384()
	_, err = io.Copy(io.MultiWriter(hasher256, hasher384), strings.NewReader(storeUUID))
	c.Assert(err, jc.ErrorIsNil)
	sha256Hash := hex.EncodeToString(hasher256.Sum(nil))
	sha384Hash := hex.EncodeToString(hasher384.Sum(nil))

	metaRecord := objectStoreMeta{
		UUID:   storeUUID,
		SHA256: sha256Hash,
		SHA384: sha384Hash,
		Size:   1234,
	}
	err = runner.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, metaRecord).Run()
	})
	c.Assert(err, jc.ErrorIsNil)

	type dbMetadataPath struct {
		// UUID is the uuid for the metadata.
		UUID string `db:"metadata_uuid"`
		// Path is the path to the object.
		Path string `db:"path"`
	}
	path := "/path/" + storeUUID
	pathRecord := dbMetadataPath{
		UUID: storeUUID,
		Path: path,
	}
	pathStmt, err := sqlair.Prepare(`
INSERT INTO object_store_metadata_path (path, metadata_uuid)
VALUES ($dbMetadataPath.*)`, pathRecord)
	c.Assert(err, jc.ErrorIsNil)
	err = runner.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, pathStmt, pathRecord).Run()
	})
	c.Assert(err, jc.ErrorIsNil)
	return objectstore.UUID(storeUUID), path
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

func (s *stateSuite) TestGetObjectUUID(c *gc.C) {
	objStoreUUID, path := s.addObjectStore(c)
	gotUUID, err := s.state.GetObjectUUID(context.Background(), path)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotUUID.String(), gc.Equals, objStoreUUID.String())
}

func (s *stateSuite) TestGetObjectUUIDFailedObjectNotFound(c *gc.C) {
	_, err := s.state.GetObjectUUID(context.Background(), "non-existent-path")
	c.Check(err, jc.ErrorIs, agentbinaryerrors.ObjectNotFound)
}
