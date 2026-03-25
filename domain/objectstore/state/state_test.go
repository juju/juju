// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/juju/clock"
	"github.com/juju/tc"

	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain/life"
	domainobjectstore "github.com/juju/juju/domain/objectstore"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestGetMetadataNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	_, err := st.GetMetadata(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrNotFound)
}

func (s *stateSuite) TestGetMetadataFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	uuid := tc.Must(c, coreobjectstore.NewUUID).String()

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo",
		Size:   666,
	}

	_, err := st.PutMetadata(c.Context(), uuid, metadata)
	c.Assert(err, tc.ErrorIsNil)

	received, err := st.GetMetadata(c.Context(), metadata.Path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, metadata)
}

func (s *stateSuite) TestGetMetadataBySHA256Found(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	uuid1 := tc.Must(c, coreobjectstore.NewUUID).String()
	uuid2 := tc.Must(c, coreobjectstore.NewUUID).String()

	metadata1 := coreobjectstore.Metadata{
		SHA256: "41af286dc0b172ed2f1ca934fd2278de4a1192302ffa07087cea2682e7d372e3",
		SHA384: "sha384-1",
		Path:   "blah-foo",
		Size:   666,
	}

	metadata2 := coreobjectstore.Metadata{
		SHA256: "b867951a18e694f3415cbef36be5a05de2d43f795f87c87756749e7bb6545b11",
		SHA384: "sha384-2",
		Path:   "blah-foo-2",
		Size:   666,
	}

	_, err := st.PutMetadata(c.Context(), uuid1, metadata1)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.PutMetadata(c.Context(), uuid2, metadata2)
	c.Assert(err, tc.ErrorIsNil)

	received, err := st.GetMetadataBySHA256(c.Context(), "41af286dc0b172ed2f1ca934fd2278de4a1192302ffa07087cea2682e7d372e3")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, metadata1)

	received, err = st.GetMetadataBySHA256(c.Context(), "b867951a18e694f3415cbef36be5a05de2d43f795f87c87756749e7bb6545b11")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, metadata2)
}

func (s *stateSuite) TestGetMetadataBySHA256NotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	_, err := st.GetMetadataBySHA256(c.Context(), "deadbeef")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrNotFound)
}

func (s *stateSuite) TestGetMetadataBySHA256PrefixFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	uuid1 := tc.Must(c, coreobjectstore.NewUUID).String()
	uuid2 := tc.Must(c, coreobjectstore.NewUUID).String()

	metadata1 := coreobjectstore.Metadata{
		SHA256: "41af286dc0b172ed2f1ca934fd2278de4a1192302ffa07087cea2682e7d372e3",
		SHA384: "sha384-1",
		Path:   "blah-foo",
		Size:   666,
	}

	metadata2 := coreobjectstore.Metadata{
		SHA256: "b867951a18e694f3415cbef36be5a05de2d43f795f87c87756749e7bb6545b11",
		SHA384: "sha384-2",
		Path:   "blah-foo-2",
		Size:   666,
	}

	_, err := st.PutMetadata(c.Context(), uuid1, metadata1)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.PutMetadata(c.Context(), uuid2, metadata2)
	c.Assert(err, tc.ErrorIsNil)

	received, err := st.GetMetadataBySHA256Prefix(c.Context(), "41af286")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, metadata1)

	received, err = st.GetMetadataBySHA256Prefix(c.Context(), "b867951")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, metadata2)

	received, err = st.GetMetadataBySHA256Prefix(c.Context(), "b867951a18e")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, metadata2)
}

func (s *stateSuite) TestGetMetadataBySHA256PrefixNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	_, err := st.GetMetadataBySHA256Prefix(c.Context(), "deadbeef")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrNotFound)
}

func (s *stateSuite) TestListMetadataFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	uuid := tc.Must(c, coreobjectstore.NewUUID).String()

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo",
		Size:   666,
	}

	_, err := st.PutMetadata(c.Context(), uuid, metadata)
	c.Assert(err, tc.ErrorIsNil)

	received, err := st.ListMetadata(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, []coreobjectstore.Metadata{metadata})
}

func (s *stateSuite) TestPutMetadata(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	uuid := tc.Must(c, coreobjectstore.NewUUID).String()

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo",
		Size:   666,
	}

	uuid, err := st.PutMetadata(c.Context(), uuid, metadata)
	c.Assert(err, tc.ErrorIsNil)

	var received coreobjectstore.Metadata
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT path, size, sha_256, sha_384 FROM v_object_store_metadata WHERE uuid = ?`, uuid)
		return row.Scan(&received.Path, &received.Size, &received.SHA256, &received.SHA384)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, metadata)
}

func (s *stateSuite) TestPutMetadataConflict(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	// UUID does not matter in this test, because we are testing the conflict of
	// the hash and size, which is independent of the UUID.

	uuid1 := tc.Must(c, coreobjectstore.NewUUID).String()
	uuid2 := tc.Must(c, coreobjectstore.NewUUID).String()

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo",
		Size:   666,
	}

	_, err := st.PutMetadata(c.Context(), uuid1, metadata)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.PutMetadata(c.Context(), uuid2, metadata)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
	c.Check(err, tc.ErrorIs, objectstoreerrors.ErrHashAndSizeAlreadyExists)
}

func (s *stateSuite) TestPutMetadataConflictDifferentHash(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	// UUID does not matter in this test, because we are testing the conflict of
	// the hash and size, which is independent of the UUID.

	uuid1 := tc.Must(c, coreobjectstore.NewUUID).String()
	uuid2 := tc.Must(c, coreobjectstore.NewUUID).String()

	metadata1 := coreobjectstore.Metadata{
		SHA256: "sha256-a",
		SHA384: "sha384-a",
		Path:   "blah-foo",
		Size:   666,
	}

	metadata2 := coreobjectstore.Metadata{
		SHA256: "sha256-b",
		SHA384: "sha384-b",
		Path:   "blah-foo",
		Size:   666,
	}

	_, err := st.PutMetadata(c.Context(), uuid1, metadata1)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.PutMetadata(c.Context(), uuid2, metadata2)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
	c.Check(err, tc.ErrorIs, objectstoreerrors.ErrPathAlreadyExistsDifferentHash)
}

func (s *stateSuite) TestPutMetadataWithSameHashesAndSize(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	// UUID does not matter in this test, because we are testing the conflict of
	// the hash and size, which is independent of the UUID.

	uuid1 := tc.Must(c, coreobjectstore.NewUUID).String()
	uuid2 := tc.Must(c, coreobjectstore.NewUUID).String()

	metadata1 := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo-1",
		Size:   666,
	}
	metadata2 := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo-2",
		Size:   666,
	}

	_, err := st.PutMetadata(c.Context(), uuid1, metadata1)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.PutMetadata(c.Context(), uuid2, metadata2)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestPutMetadataWithSameSHA256AndSize(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	// UUID does not matter in this test, because we are testing the conflict of
	// the hash and size, which is independent of the UUID.

	uuid1 := tc.Must(c, coreobjectstore.NewUUID).String()
	uuid2 := tc.Must(c, coreobjectstore.NewUUID).String()

	metadata1 := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "foo",
		Path:   "blah-foo-1",
		Size:   666,
	}
	metadata2 := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "bar",
		Path:   "blah-foo-2",
		Size:   666,
	}

	rUUID1, err := st.PutMetadata(c.Context(), uuid1, metadata1)
	c.Assert(err, tc.ErrorIsNil)

	rUUID2, err := st.PutMetadata(c.Context(), uuid2, metadata2)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(rUUID1, tc.Equals, rUUID2)
}

func (s *stateSuite) TestPutMetadataWithSameSHA384AndSize(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	// UUID does not matter in this test, because we are testing the conflict of
	// the hash and size, which is independent of the UUID.

	uuid1 := tc.Must(c, coreobjectstore.NewUUID).String()
	uuid2 := tc.Must(c, coreobjectstore.NewUUID).String()

	metadata1 := coreobjectstore.Metadata{
		SHA256: "foo",
		SHA384: "sha384",
		Path:   "blah-foo-1",
		Size:   666,
	}
	metadata2 := coreobjectstore.Metadata{
		SHA256: "bar",
		SHA384: "sha384",
		Path:   "blah-foo-2",
		Size:   666,
	}

	rUUID1, err := st.PutMetadata(c.Context(), uuid1, metadata1)
	c.Assert(err, tc.ErrorIsNil)

	rUUID2, err := st.PutMetadata(c.Context(), uuid2, metadata2)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(rUUID1, tc.Equals, rUUID2)
}

func (s *stateSuite) TestPutMetadataWithSameHashDifferentSize(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	// Test if the hash is the same but the size is different. The root
	// cause of this, is if the hash is the same, but the size is different.
	// There is a broken hash function somewhere.

	uuid1 := tc.Must(c, coreobjectstore.NewUUID).String()
	uuid2 := tc.Must(c, coreobjectstore.NewUUID).String()

	metadata1 := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo-1",
		Size:   666,
	}
	metadata2 := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo-2",
		Size:   42,
	}

	_, err := st.PutMetadata(c.Context(), uuid1, metadata1)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.PutMetadata(c.Context(), uuid2, metadata2)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrHashAndSizeAlreadyExists)
}

func (s *stateSuite) TestPutMetadataMultipleTimes(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	// Ensure that we can add the same metadata multiple times.
	metadatas := make([]coreobjectstore.Metadata, 10)

	for i := range 10 {
		uuid := tc.Must(c, coreobjectstore.NewUUID).String()

		metadatas[i] = coreobjectstore.Metadata{
			SHA256: fmt.Sprintf("hash-256-%d", i),
			SHA384: fmt.Sprintf("hash-384-%d", i),
			Path:   fmt.Sprintf("blah-foo-%d", i),
			Size:   666,
		}

		_, err := st.PutMetadata(c.Context(), uuid, metadatas[i])
		c.Assert(err, tc.ErrorIsNil)
	}

	for i := range 10 {
		metadata, err := st.GetMetadata(c.Context(), fmt.Sprintf("blah-foo-%d", i))
		c.Assert(err, tc.ErrorIsNil)
		c.Check(metadata, tc.DeepEquals, metadatas[i])
	}
}

func (s *stateSuite) TestPutMetadataWithControllerIDHint(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	uuid := tc.Must(c, coreobjectstore.NewUUID).String()

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo",
		Size:   666,
	}

	uuid, err := st.PutMetadataWithControllerIDHint(c.Context(), uuid, metadata, "1")
	c.Assert(err, tc.ErrorIsNil)

	var nodeID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT node_id FROM object_store_placement WHERE uuid = ?`, uuid)
		return row.Scan(&nodeID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(nodeID, tc.Equals, "1")
}

func (s *stateSuite) TestPutMetadataWithControllerIDHintMultipleTimes(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	// Ensure that we can add the same metadata multiple times.
	metadatas := make([]coreobjectstore.Metadata, 10)

	for i := range 10 {
		uuid := tc.Must(c, coreobjectstore.NewUUID).String()

		metadatas[i] = coreobjectstore.Metadata{
			SHA256: fmt.Sprintf("hash-256-%d", i),
			SHA384: fmt.Sprintf("hash-384-%d", i),
			Path:   fmt.Sprintf("blah-foo-%d", i),
			Size:   666,
		}

		_, err := st.PutMetadataWithControllerIDHint(c.Context(), uuid, metadatas[i], "1")
		c.Assert(err, tc.ErrorIsNil)
	}

	for i := range 10 {
		metadata, err := st.GetMetadata(c.Context(), fmt.Sprintf("blah-foo-%d", i))
		c.Assert(err, tc.ErrorIsNil)
		c.Check(metadata, tc.DeepEquals, metadatas[i])
	}
}

func (s *stateSuite) TestAddControllerIDHint(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	uuid := tc.Must(c, coreobjectstore.NewUUID).String()

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo",
		Size:   666,
	}

	_, err := st.PutMetadataWithControllerIDHint(c.Context(), uuid, metadata, "1")
	c.Assert(err, tc.ErrorIsNil)

	err = st.AddControllerIDHint(c.Context(), "sha384", "2")
	c.Assert(err, tc.ErrorIsNil)

	var nodes []string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT p.node_id
FROM object_store_placement AS p
JOIN object_store_metadata AS m ON p.uuid = m.uuid
WHERE m.sha_384 = ?`, "sha384")
		if err != nil {
			return errors.Errorf("querying placement hints: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var nodeID string
			if err := rows.Scan(&nodeID); err != nil {
				return errors.Errorf("scanning placement hint: %w", err)
			}
			nodes = append(nodes, nodeID)
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(nodes, tc.SameContents, []string{"1", "2"})
}

func (s *stateSuite) TestAddControllerIDHintNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	err := st.AddControllerIDHint(c.Context(), "non-existent-sha384", "1")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrNotFound)
}

func (s *stateSuite) TestGetControllerIDHints(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	uuid := tc.Must(c, coreobjectstore.NewUUID).String()

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo",
		Size:   666,
	}

	_, err := st.PutMetadataWithControllerIDHint(c.Context(), uuid, metadata, "1")
	c.Assert(err, tc.ErrorIsNil)

	err = st.AddControllerIDHint(c.Context(), "sha384", "2")
	c.Assert(err, tc.ErrorIsNil)

	hints, err := st.GetControllerIDHints(c.Context(), "sha384")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(hints, tc.SameContents, []string{"1", "2"})
}

func (s *stateSuite) TestGetControllerIDHintsNoHints(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	hints, err := st.GetControllerIDHints(c.Context(), "sha384")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hints, tc.HasLen, 0)
}

func (s *stateSuite) TestRemoveMetadataNotExists(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	err := st.RemoveMetadata(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrNotFound)
}

func (s *stateSuite) TestRemoveMetadataDoesNotRemoveMetadataIfReferenced(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	uuid1 := tc.Must(c, coreobjectstore.NewUUID).String()
	uuid2 := tc.Must(c, coreobjectstore.NewUUID).String()

	metadata1 := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo-1",
		Size:   666,
	}
	metadata2 := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo-2",
		Size:   666,
	}

	_, err := st.PutMetadata(c.Context(), uuid1, metadata1)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.PutMetadata(c.Context(), uuid2, metadata2)
	c.Assert(err, tc.ErrorIsNil)

	err = st.RemoveMetadata(c.Context(), metadata2.Path)
	c.Assert(err, tc.ErrorIsNil)

	received, err := st.GetMetadata(c.Context(), metadata1.Path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, metadata1)
}

func (s *stateSuite) TestRemoveMetadataCleansUpEverything(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	uuid1 := tc.Must(c, coreobjectstore.NewUUID).String()
	uuid2 := tc.Must(c, coreobjectstore.NewUUID).String()
	uuid3 := tc.Must(c, coreobjectstore.NewUUID).String()

	metadata1 := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo-1",
		Size:   666,
	}
	metadata2 := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo-2",
		Size:   666,
	}

	// Add both metadata.
	_, err := st.PutMetadata(c.Context(), uuid1, metadata1)
	c.Assert(err, tc.ErrorIsNil)
	_, err = st.PutMetadata(c.Context(), uuid2, metadata2)
	c.Assert(err, tc.ErrorIsNil)

	// Remove both metadata.
	err = st.RemoveMetadata(c.Context(), metadata1.Path)
	c.Assert(err, tc.ErrorIsNil)
	err = st.RemoveMetadata(c.Context(), metadata2.Path)
	c.Assert(err, tc.ErrorIsNil)

	// Ensure that both metadata have been removed.
	_, err = st.GetMetadata(c.Context(), metadata1.Path)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrNotFound)
	_, err = st.GetMetadata(c.Context(), metadata2.Path)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrNotFound)

	// Add a new metadata with the same hash and size.
	metadata3 := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo-3",
		Size:   666,
	}
	_, err = st.PutMetadata(c.Context(), uuid3, metadata3)
	c.Assert(err, tc.ErrorIsNil)

	// We guarantee that the metadata has been added is unique, because
	// the UUID would be UUID from metadata1 if the metadata has not been
	// removed.
	received, err := st.GetMetadata(c.Context(), metadata3.Path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, metadata3)
}

func (s *stateSuite) TestRemoveMetadataThenAddAgain(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	uuid1 := tc.Must(c, coreobjectstore.NewUUID).String()
	uuid2 := tc.Must(c, coreobjectstore.NewUUID).String()

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo-1",
		Size:   666,
	}

	_, err := st.PutMetadata(c.Context(), uuid1, metadata)
	c.Assert(err, tc.ErrorIsNil)

	err = st.RemoveMetadata(c.Context(), metadata.Path)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.PutMetadata(c.Context(), uuid2, metadata)
	c.Assert(err, tc.ErrorIsNil)

	received, err := st.GetMetadata(c.Context(), metadata.Path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, metadata)
}

func (s *stateSuite) TestListMetadata(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	uuid1 := tc.Must(c, coreobjectstore.NewUUID).String()

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo-1",
		Size:   666,
	}

	_, err := st.PutMetadata(c.Context(), uuid1, metadata)
	c.Assert(err, tc.ErrorIsNil)

	metadatas, err := st.ListMetadata(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(metadatas, tc.HasLen, 1)

	c.Check(metadatas[0], tc.DeepEquals, metadata)
}

func (s *stateSuite) TestListMetadataNoRows(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	metadatas, err := st.ListMetadata(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(metadatas, tc.HasLen, 0)
}

func (s *stateSuite) TestGetActiveDrainingInfo(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	_, err := st.GetActiveDrainingInfo(c.Context())
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrDrainingPhaseNotFound)

	backendUUID := tc.Must(c, coreobjectstore.NewUUID).String()
	creds := domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "access-key",
		SecretKey: "secret-key",
	}

	err = st.SetObjectStoreBackendToS3(c.Context(), backendUUID, creds)
	c.Assert(err, tc.ErrorIsNil)

	err = st.StartDraining(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	info, err := st.GetActiveDrainingInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Phase, tc.Equals, string(coreobjectstore.PhaseDraining))
	c.Check(info.UUID, tc.Equals, "foo")
	c.Check(info.ActiveBackendUUID, tc.Equals, backendUUID)

	var fromBackendUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT uuid FROM object_store_backend
WHERE life_id = 1`)
		return row.Scan(&fromBackendUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.FromBackendUUID, tc.NotNil)
	c.Check(*info.FromBackendUUID, tc.Equals, fromBackendUUID)
}

func (s *stateSuite) TestSetDrainingPhase(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	backendUUID := tc.Must(c, coreobjectstore.NewUUID).String()
	creds := domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "access-key",
		SecretKey: "secret-key",
	}

	err := st.SetObjectStoreBackendToS3(c.Context(), backendUUID, creds)
	c.Assert(err, tc.ErrorIsNil)

	err = st.StartDraining(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	info, err := st.GetActiveDrainingInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Phase, tc.Equals, string(coreobjectstore.PhaseDraining))

	err = st.SetDrainingPhase(c.Context(), "foo", coreobjectstore.PhaseCompleted)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.GetActiveDrainingInfo(c.Context())
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrDrainingPhaseNotFound)
}

func (s *stateSuite) TestStartDrainingMissingFromBackend(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	err := st.StartDraining(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, ".*migrating from: backend not found")
}

func (s *stateSuite) TestStartDrainingMissingToBackend(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE object_store_backend
SET life_id = 1`)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = st.StartDraining(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, ".*migrating to: backend not found")
}

func (s *stateSuite) TestStartDraining(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	backendUUID := tc.Must(c, coreobjectstore.NewUUID).String()
	creds := domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "access-key",
		SecretKey: "secret-key",
	}

	err := st.SetObjectStoreBackendToS3(c.Context(), backendUUID, creds)
	c.Assert(err, tc.ErrorIsNil)

	err = st.StartDraining(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	err = st.StartDraining(c.Context(), "bar")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrDrainingAlreadyInProgress)
}

func (s *stateSuite) TestStartDrainingAndSetDrainingPhase(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	backendUUID := tc.Must(c, coreobjectstore.NewUUID).String()
	creds := domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "access-key",
		SecretKey: "secret-key",
	}

	err := st.SetObjectStoreBackendToS3(c.Context(), backendUUID, creds)
	c.Assert(err, tc.ErrorIsNil)

	err = st.StartDraining(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	err = st.SetDrainingPhase(c.Context(), "foo", coreobjectstore.PhaseCompleted)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestSetObjectStoreBackendToS3CalledTwice(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	backendUUID := tc.Must(c, coreobjectstore.NewUUID).String()
	creds := domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "access-key",
		SecretKey: "secret-key",
	}

	err := st.SetObjectStoreBackendToS3(c.Context(), backendUUID, creds)
	c.Assert(err, tc.ErrorIsNil)

	// Force the old backend to be marked as dead.
	s.markBackendAsDead(c, "653813f9-2896-5332-8cbe-629a337a56a3")

	err = st.SetObjectStoreBackendToS3(c.Context(), backendUUID, creds)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrBackendAlreadyExists)
}

func (s *stateSuite) TestSetObjectStoreBackendToS3MultipleTimes(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	backendUUID0 := tc.Must(c, coreobjectstore.NewUUID).String()
	backendUUID1 := tc.Must(c, coreobjectstore.NewUUID).String()
	backendUUID2 := tc.Must(c, coreobjectstore.NewUUID).String()

	creds := domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "access-key",
		SecretKey: "secret-key",
	}

	err := st.SetObjectStoreBackendToS3(c.Context(), backendUUID0, creds)
	c.Assert(err, tc.ErrorIsNil)

	// Force the file backend to be marked as dead.
	s.markBackendAsDead(c, "653813f9-2896-5332-8cbe-629a337a56a3")

	err = st.SetObjectStoreBackendToS3(c.Context(), backendUUID1, creds)
	c.Assert(err, tc.ErrorIsNil)

	err = st.StartDraining(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	// Force the first backend to be marked as dead.
	s.markBackendAsDead(c, backendUUID0)

	err = st.SetObjectStoreBackendToS3(c.Context(), backendUUID2, creds)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrDrainingAlreadyInProgress)

	err = st.SetDrainingPhase(c.Context(), "foo", coreobjectstore.PhaseCompleted)
	c.Assert(err, tc.ErrorIsNil)

	err = st.SetObjectStoreBackendToS3(c.Context(), backendUUID2, creds)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestSetObjectStoreBackendToS3WithActiveDrainingBackend(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	backendUUID0 := tc.Must(c, coreobjectstore.NewUUID).String()
	backendUUID1 := tc.Must(c, coreobjectstore.NewUUID).String()

	creds := domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "access-key",
		SecretKey: "secret-key",
	}

	// This backend is ignored, as the draining phase is not active.
	err := st.SetObjectStoreBackendToS3(c.Context(), backendUUID0, creds)
	c.Assert(err, tc.ErrorIsNil)

	// Force the file backend to be marked as dead.
	s.markBackendAsDead(c, "653813f9-2896-5332-8cbe-629a337a56a3")

	err = st.SetObjectStoreBackendToS3(c.Context(), backendUUID1, creds)
	c.Assert(err, tc.ErrorIsNil)

	err = st.StartDraining(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	info, err := st.GetActiveDrainingInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info, tc.DeepEquals, domainobjectstore.DrainingInfo{
		Phase:             string(coreobjectstore.PhaseDraining),
		UUID:              "foo",
		FromBackendUUID:   new(backendUUID0),
		ActiveBackendUUID: backendUUID1,
	})
}

func (s *stateSuite) TestSetObjectStoreBackendToS3(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	backendUUID := tc.Must(c, coreobjectstore.NewUUID).String()
	creds := domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "access-key",
		SecretKey: "secret-key",
	}

	err := st.SetObjectStoreBackendToS3(c.Context(), backendUUID, creds)
	c.Assert(err, tc.ErrorIsNil)

	var lifeID, typeID int
	var dyingTypeID int
	var endpoint, accessKey, secretKey string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT life_id, type_id FROM object_store_backend
WHERE uuid = ?`, backendUUID)
		if err := row.Scan(&lifeID, &typeID); err != nil {
			return errors.Errorf("querying s3 backend: %w", err)
		}

		row = tx.QueryRowContext(ctx, `
SELECT type_id FROM object_store_backend
WHERE life_id = 1`)
		if err := row.Scan(&dyingTypeID); err != nil {
			return errors.Errorf("querying dying backend: %w", err)
		}

		row = tx.QueryRowContext(ctx, `
SELECT endpoint, static_key, static_secret
FROM object_store_backend_s3_credential
WHERE object_store_backend_uuid = ?`, backendUUID)
		if err := row.Scan(&endpoint, &accessKey, &secretKey); err != nil {
			return errors.Errorf("querying backend credentials: %w", err)
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(lifeID, tc.Equals, 0)
	c.Check(typeID, tc.Equals, 1)

	c.Check(dyingTypeID, tc.Equals, 0)

	c.Check(endpoint, tc.Equals, creds.Endpoint)
	c.Check(accessKey, tc.Equals, creds.AccessKey)
	c.Check(secretKey, tc.Equals, creds.SecretKey)
}

func (s *stateSuite) TestMarkObjectStoreBackendAsDrained(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	drainingUUID := tc.Must(c, coreobjectstore.NewUUID).String()
	activeUUID := tc.Must(c, coreobjectstore.NewUUID).String()

	creds := domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "access-key",
		SecretKey: "secret-key",
	}

	// First call promotes an S3 backend and marks the default file backend as
	// dying.
	err := st.SetObjectStoreBackendToS3(c.Context(), drainingUUID, creds)
	c.Assert(err, tc.ErrorIsNil)

	// Force the old backend to be marked as dead.
	s.markBackendAsDead(c, "653813f9-2896-5332-8cbe-629a337a56a3")

	// Second call marks the first S3 backend as dying and activates a new one.
	err = st.SetObjectStoreBackendToS3(c.Context(), activeUUID, creds)
	c.Assert(err, tc.ErrorIsNil)

	err = st.MarkObjectStoreBackendAsDrained(c.Context(), drainingUUID)
	c.Assert(err, tc.ErrorIsNil)

	var drainingLifeID, activeLifeID int
	var credsCount int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT life_id FROM object_store_backend
WHERE uuid = ?`, drainingUUID)
		if err := row.Scan(&drainingLifeID); err != nil {
			return errors.Errorf("querying drained backend: %w", err)
		}

		row = tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM object_store_backend_s3_credential
WHERE object_store_backend_uuid = ?`, drainingUUID)
		if err := row.Scan(&credsCount); err != nil {
			return errors.Errorf("counting drained backend credentials: %w", err)
		}

		row = tx.QueryRowContext(ctx, `
SELECT life_id FROM object_store_backend
WHERE uuid = ?`, activeUUID)
		if err := row.Scan(&activeLifeID); err != nil {
			return errors.Errorf("querying active backend: %w", err)
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(drainingLifeID, tc.Equals, 2)
	c.Check(credsCount, tc.Equals, 0)
	c.Check(activeLifeID, tc.Equals, 0)
}

func (s *stateSuite) TestMarkObjectStoreBackendAsDrainedReentrant(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	drainingUUID := tc.Must(c, coreobjectstore.NewUUID).String()
	activeUUID := tc.Must(c, coreobjectstore.NewUUID).String()

	creds := domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "access-key",
		SecretKey: "secret-key",
	}

	// First promotion marks the default file backend as dying.
	err := st.SetObjectStoreBackendToS3(c.Context(), drainingUUID, creds)
	c.Assert(err, tc.ErrorIsNil)

	// Force the old backend to be marked as dead.
	s.markBackendAsDead(c, "653813f9-2896-5332-8cbe-629a337a56a3")

	// Second promotion marks the first S3 backend as dying and activates a new
	// one.
	err = st.SetObjectStoreBackendToS3(c.Context(), activeUUID, creds)
	c.Assert(err, tc.ErrorIsNil)

	// First call should mark the draining backend dead.
	err = st.MarkObjectStoreBackendAsDrained(c.Context(), drainingUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Second call should be a no-op.
	err = st.MarkObjectStoreBackendAsDrained(c.Context(), drainingUUID)
	c.Assert(err, tc.ErrorIsNil)

	var lifeID int
	var credsCount int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT life_id FROM object_store_backend
WHERE uuid = ?`, drainingUUID)
		if err := row.Scan(&lifeID); err != nil {
			return errors.Errorf("querying drained backend: %w", err)
		}

		row = tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM object_store_backend_s3_credential
WHERE object_store_backend_uuid = ?`, drainingUUID)
		if err := row.Scan(&credsCount); err != nil {
			return errors.Errorf("counting drained backend credentials: %w", err)
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(lifeID, tc.Equals, 2)
	c.Check(credsCount, tc.Equals, 0)
}

func (s *stateSuite) TestGetActiveObjectStoreBackend(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	var activeUUID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT uuid FROM object_store_backend
WHERE life_id = 0`)
		return row.Scan(&activeUUID)
	})
	c.Assert(err, tc.ErrorIsNil)

	info, err := st.GetActiveObjectStoreBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.UUID, tc.Equals, activeUUID)
	c.Check(info.ObjectStoreType, tc.Equals, "file")
	c.Check(info.LifeID, tc.Equals, life.Alive)
	c.Check(info.Endpoint, tc.IsNil)
	c.Check(info.AccessKey, tc.IsNil)
	c.Check(info.SecretKey, tc.IsNil)
}

func (s *stateSuite) TestGetActiveObjectStoreBackendS3(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	backendUUID := tc.Must(c, coreobjectstore.NewUUID).String()
	creds := domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "access-key",
		SecretKey: "secret-key",
	}

	err := st.SetObjectStoreBackendToS3(c.Context(), backendUUID, creds)
	c.Assert(err, tc.ErrorIsNil)

	info, err := st.GetActiveObjectStoreBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.UUID, tc.Equals, backendUUID)
	c.Check(info.ObjectStoreType, tc.Equals, "s3")
	c.Check(info.LifeID, tc.Equals, life.Alive)
	c.Assert(info.Endpoint, tc.NotNil)
	c.Check(*info.Endpoint, tc.Equals, creds.Endpoint)
	c.Assert(info.AccessKey, tc.NotNil)
	c.Check(*info.AccessKey, tc.Equals, creds.AccessKey)
	c.Assert(info.SecretKey, tc.NotNil)
	c.Check(*info.SecretKey, tc.Equals, creds.SecretKey)
}

func (s *stateSuite) TestGetActiveObjectStoreBackendNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE object_store_backend
SET life_id = 1`)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.GetActiveObjectStoreBackend(c.Context())
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrBackendNotFound)
}

func (s *stateSuite) TestGetObjectStoreBackend(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	var backendUUID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT uuid FROM object_store_backend
WHERE life_id = 0`)
		return row.Scan(&backendUUID)
	})
	c.Assert(err, tc.ErrorIsNil)

	info, err := st.GetObjectStoreBackend(c.Context(), backendUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.UUID, tc.Equals, backendUUID)
	c.Check(info.ObjectStoreType, tc.Equals, "file")
	c.Check(info.LifeID, tc.Equals, life.Alive)
	c.Check(info.Endpoint, tc.IsNil)
	c.Check(info.AccessKey, tc.IsNil)
	c.Check(info.SecretKey, tc.IsNil)
}

func (s *stateSuite) TestGetObjectStoreBackendS3(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	backendUUID := tc.Must(c, coreobjectstore.NewUUID).String()
	creds := domainobjectstore.S3Credentials{
		Endpoint:  "https://s3.example.com",
		AccessKey: "access-key",
		SecretKey: "secret-key",
	}

	err := st.SetObjectStoreBackendToS3(c.Context(), backendUUID, creds)
	c.Assert(err, tc.ErrorIsNil)

	info, err := st.GetObjectStoreBackend(c.Context(), backendUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.UUID, tc.Equals, backendUUID)
	c.Check(info.ObjectStoreType, tc.Equals, "s3")
	c.Check(info.LifeID, tc.Equals, life.Alive)

	c.Assert(info.Endpoint, tc.NotNil)
	c.Check(*info.Endpoint, tc.Equals, creds.Endpoint)
	c.Assert(info.AccessKey, tc.NotNil)
	c.Check(*info.AccessKey, tc.Equals, creds.AccessKey)
	c.Assert(info.SecretKey, tc.NotNil)
	c.Check(*info.SecretKey, tc.Equals, creds.SecretKey)
}

func (s *stateSuite) TestGetObjectStoreBackendNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), clock.WallClock)

	missingUUID := tc.Must(c, coreobjectstore.NewUUID).String()

	_, err := st.GetObjectStoreBackend(c.Context(), missingUUID)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrBackendNotFound)
}

func (s *stateSuite) TestDefaultFileBackendUUID(c *tc.C) {
	// Juju UUID namespace that we (should) use for all Juju well-known UUIDs.
	jujuUUIDNamespace := "96bb15e6-8b85-448b-9fce-ede1a1700e64"
	namespaceUUID, err := uuid.Parse(jujuUUIDNamespace)
	c.Assert(err, tc.ErrorIsNil)

	fileObjectStoreUUID := uuid.NewSHA1(namespaceUUID, []byte("juju.objectstore.file.backend"))
	c.Assert(fileObjectStoreUUID.String(), tc.Equals, defaultFileBackendUUID)

	var backendUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT uuid FROM object_store_backend
WHERE type_id = 0`)
		return row.Scan(&backendUUID)
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(backendUUID, tc.Equals, defaultFileBackendUUID)
}

func (s *stateSuite) markBackendAsDead(c *tc.C, uuid string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE object_store_backend
SET life_id = 2
WHERE uuid = ?`, uuid)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}
