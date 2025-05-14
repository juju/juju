// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"database/sql"
	"encoding/hex"
	"io"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

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

var _ = tc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory())
}

// TestAddSuccess asserts the happy path of adding agent binary metadata.
func (s *stateSuite) TestAddSuccess(c *tc.C) {
	archID := s.addArchitecture(c, "amd64")
	objStoreUUID, _ := s.addObjectStore(c)

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
func (s *stateSuite) TestAddAlreadyExists(c *tc.C) {
	archID := s.addArchitecture(c, "amd64")
	objStoreUUID1, _ := s.addObjectStore(c)

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
func (s *stateSuite) TestAddFailedUpdateExistingWithDifferentSHA(c *tc.C) {
	archID := s.addArchitecture(c, "amd64")
	objStoreUUID1, _ := s.addObjectStore(c)
	objStoreUUID2, _ := s.addObjectStore(c)

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
func (s *stateSuite) TestAddErrorArchitectureNotFound(c *tc.C) {
	objStoreUUID, _ := s.addObjectStore(c)

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
func (s *stateSuite) TestAddErrorObjectStoreUUIDNotFound(c *tc.C) {
	s.addArchitecture(c, "amd64")

	err := s.state.RegisterAgentBinary(c.Context(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "amd64",
		ObjectStoreUUID: objectstore.UUID(uuid.MustNewUUID().String()),
	})
	c.Check(err, tc.ErrorIs, agentbinaryerrors.ObjectNotFound)
}

func (s *stateSuite) addArchitecture(c *tc.C, name string) int {
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

func (s *stateSuite) addObjectStore(c *tc.C) (objectstore.UUID, string) {
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
	c.Assert(err, tc.ErrorIsNil)

	hasher256 := sha256.New()
	hasher384 := sha512.New384()
	_, err = io.Copy(io.MultiWriter(hasher256, hasher384), strings.NewReader(storeUUID))
	c.Assert(err, tc.ErrorIsNil)
	sha256Hash := hex.EncodeToString(hasher256.Sum(nil))
	sha384Hash := hex.EncodeToString(hasher384.Sum(nil))

	metaRecord := objectStoreMeta{
		UUID:   storeUUID,
		SHA256: sha256Hash,
		SHA384: sha384Hash,
		Size:   1234,
	}
	err = runner.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, metaRecord).Run()
	})
	c.Assert(err, tc.ErrorIsNil)

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
	c.Assert(err, tc.ErrorIsNil)
	err = runner.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, pathStmt, pathRecord).Run()
	})
	c.Assert(err, tc.ErrorIsNil)
	return objectstore.UUID(storeUUID), path
}

func (s *stateSuite) getAgentBinaryRecord(c *tc.C, version string, archID int) agentBinaryRecord {
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

func (s *stateSuite) TestGetObjectUUID(c *tc.C) {
	objStoreUUID, path := s.addObjectStore(c)
	gotUUID, err := s.state.GetObjectUUID(c.Context(), path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotUUID.String(), tc.Equals, objStoreUUID.String())
}

func (s *stateSuite) TestGetObjectUUIDFailedObjectNotFound(c *tc.C) {
	_, err := s.state.GetObjectUUID(c.Context(), "non-existent-path")
	c.Check(err, tc.ErrorIs, agentbinaryerrors.ObjectNotFound)
}

func getMetadata(c *tc.C, db *sql.DB, objStoreUUID objectstore.UUID) agentbinary.Metadata {
	var data agentbinary.Metadata
	err := db.QueryRow(`
SELECT version, architecture_name, size, sha_256
FROM   v_agent_binary_store
WHERE  object_store_uuid = ?`, objStoreUUID).Scan(&data.Version, &data.Arch, &data.Size, &data.SHA256)
	c.Assert(err, tc.ErrorIsNil)
	return data
}

func getObjectSHA256(c *tc.C, db *sql.DB, objStoreUUID objectstore.UUID) string {
	var sha string
	err := db.QueryRow(`
SELECT sha_256
FROM   object_store_metadata
WHERE  uuid = ?`, objStoreUUID).Scan(&sha)
	c.Assert(err, tc.ErrorIsNil)
	return sha
}

func (s *stateSuite) TestListAgentBinaries(c *tc.C) {
	_ = s.addArchitecture(c, "amd64")

	objStoreUUID, _ := s.addObjectStore(c)
	err := s.state.RegisterAgentBinary(c.Context(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "amd64",
		ObjectStoreUUID: objStoreUUID,
	})
	c.Assert(err, tc.ErrorIsNil)
	binary1 := getMetadata(c, s.DB(), objStoreUUID)

	objStoreUUID, _ = s.addObjectStore(c)
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

func (s *stateSuite) TestListAgentBinariesEmpty(c *tc.C) {
	binaries, err := s.state.ListAgentBinaries(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(binaries, tc.HasLen, 0)
}

func (s *stateSuite) TestCheckAgentBinarySHA256Exists(c *tc.C) {
	objStoreUUID, _ := s.addObjectStore(c)

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

func (s *stateSuite) TestCheckAgentBinarySHA256NoExists(c *tc.C) {
	objStoreUUID, _ := s.addObjectStore(c)
	sha := getObjectSHA256(c, s.DB(), objStoreUUID)
	exists, err := s.state.CheckAgentBinarySHA256Exists(c.Context(), sha)
	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}
