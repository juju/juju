// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/juju/tc"

	coreobjectstore "github.com/juju/juju/core/objectstore"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestGetMetadataNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetMetadata(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrNotFound)
}

func (s *stateSuite) TestGetMetadataFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo",
		Size:   666,
	}

	_, err := st.PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)

	received, err := st.GetMetadata(c.Context(), metadata.Path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, metadata)
}

func (s *stateSuite) TestGetMetadataBySHA256Found(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

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

	_, err := st.PutMetadata(c.Context(), metadata1)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.PutMetadata(c.Context(), metadata2)
	c.Assert(err, tc.ErrorIsNil)

	received, err := st.GetMetadataBySHA256(c.Context(), "41af286dc0b172ed2f1ca934fd2278de4a1192302ffa07087cea2682e7d372e3")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, metadata1)

	received, err = st.GetMetadataBySHA256(c.Context(), "b867951a18e694f3415cbef36be5a05de2d43f795f87c87756749e7bb6545b11")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, metadata2)
}

func (s *stateSuite) TestGetMetadataBySHA256NotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetMetadataBySHA256(c.Context(), "deadbeef")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrNotFound)
}

func (s *stateSuite) TestGetMetadataBySHA256PrefixFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

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

	_, err := st.PutMetadata(c.Context(), metadata1)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.PutMetadata(c.Context(), metadata2)
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
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetMetadataBySHA256Prefix(c.Context(), "deadbeef")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrNotFound)
}

func (s *stateSuite) TestListMetadataFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo",
		Size:   666,
	}

	_, err := st.PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)

	received, err := st.ListMetadata(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, []coreobjectstore.Metadata{metadata})
}

func (s *stateSuite) TestPutMetadata(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo",
		Size:   666,
	}

	uuid, err := st.PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)

	runner, err := s.TxnRunnerFactory()()
	c.Assert(err, tc.ErrorIsNil)

	var received coreobjectstore.Metadata
	err = runner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT path, size, sha_256, sha_384 FROM v_object_store_metadata WHERE uuid = ?`, uuid)
		return row.Scan(&received.Path, &received.Size, &received.SHA256, &received.SHA384)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, metadata)
}

func (s *stateSuite) TestPutMetadataConflict(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo",
		Size:   666,
	}

	_, err := st.PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
	c.Check(err, tc.ErrorIs, objectstoreerrors.ErrHashAndSizeAlreadyExists)
}

func (s *stateSuite) TestPutMetadataConflictDifferentHash(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

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

	_, err := st.PutMetadata(c.Context(), metadata1)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.PutMetadata(c.Context(), metadata2)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
	c.Check(err, tc.ErrorIs, objectstoreerrors.ErrPathAlreadyExistsDifferentHash)
}

func (s *stateSuite) TestPutMetadataWithSameHashesAndSize(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

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

	_, err := st.PutMetadata(c.Context(), metadata1)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.PutMetadata(c.Context(), metadata2)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestPutMetadataWithSameSHA256AndSize(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

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

	uuid1, err := st.PutMetadata(c.Context(), metadata1)
	c.Assert(err, tc.ErrorIsNil)

	uuid2, err := st.PutMetadata(c.Context(), metadata2)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(uuid1, tc.Equals, uuid2)
}

func (s *stateSuite) TestPutMetadataWithSameSHA384AndSize(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

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

	uuid1, err := st.PutMetadata(c.Context(), metadata1)
	c.Assert(err, tc.ErrorIsNil)

	uuid2, err := st.PutMetadata(c.Context(), metadata2)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(uuid1, tc.Equals, uuid2)
}

func (s *stateSuite) TestPutMetadataWithSameHashDifferentSize(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Test if the hash is the same but the size is different. The root
	// cause of this, is if the hash is the same, but the size is different.
	// There is a broken hash function somewhere.

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

	_, err := st.PutMetadata(c.Context(), metadata1)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.PutMetadata(c.Context(), metadata2)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrHashAndSizeAlreadyExists)
}

func (s *stateSuite) TestPutMetadataMultipleTimes(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Ensure that we can add the same metadata multiple times.
	metadatas := make([]coreobjectstore.Metadata, 10)

	for i := 0; i < 10; i++ {
		metadatas[i] = coreobjectstore.Metadata{
			SHA256: fmt.Sprintf("hash-256-%d", i),
			SHA384: fmt.Sprintf("hash-384-%d", i),
			Path:   fmt.Sprintf("blah-foo-%d", i),
			Size:   666,
		}

		_, err := st.PutMetadata(c.Context(), metadatas[i])
		c.Assert(err, tc.ErrorIsNil)
	}

	for i := 0; i < 10; i++ {
		metadata, err := st.GetMetadata(c.Context(), fmt.Sprintf("blah-foo-%d", i))
		c.Assert(err, tc.ErrorIsNil)
		c.Check(metadata, tc.DeepEquals, metadatas[i])
	}
}

func (s *stateSuite) TestRemoveMetadataNotExists(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.RemoveMetadata(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrNotFound)
}

func (s *stateSuite) TestRemoveMetadataDoesNotRemoveMetadataIfReferenced(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

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

	_, err := st.PutMetadata(c.Context(), metadata1)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.PutMetadata(c.Context(), metadata2)
	c.Assert(err, tc.ErrorIsNil)

	err = st.RemoveMetadata(c.Context(), metadata2.Path)
	c.Assert(err, tc.ErrorIsNil)

	received, err := st.GetMetadata(c.Context(), metadata1.Path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, metadata1)
}

func (s *stateSuite) TestRemoveMetadataCleansUpEverything(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

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
	_, err := st.PutMetadata(c.Context(), metadata1)
	c.Assert(err, tc.ErrorIsNil)
	_, err = st.PutMetadata(c.Context(), metadata2)
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
	_, err = st.PutMetadata(c.Context(), metadata3)
	c.Assert(err, tc.ErrorIsNil)

	// We guarantee that the metadata has been added is unique, because
	// the UUID would be UUID from metadata1 if the metadata has not been
	// removed.
	received, err := st.GetMetadata(c.Context(), metadata3.Path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, metadata3)
}

func (s *stateSuite) TestRemoveMetadataThenAddAgain(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo-1",
		Size:   666,
	}

	_, err := st.PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)

	err = st.RemoveMetadata(c.Context(), metadata.Path)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)

	received, err := st.GetMetadata(c.Context(), metadata.Path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received, tc.DeepEquals, metadata)
}

func (s *stateSuite) TestListMetadata(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo-1",
		Size:   666,
	}

	_, err := st.PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)

	metadatas, err := st.ListMetadata(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(metadatas, tc.HasLen, 1)

	c.Check(metadatas[0], tc.DeepEquals, metadata)
}

func (s *stateSuite) TestListMetadataNoRows(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadatas, err := st.ListMetadata(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(metadatas, tc.HasLen, 0)
}

func (s *stateSuite) TestGetActiveDrainingPhase(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, _, err := st.GetActiveDrainingPhase(context.Background())
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrDrainingPhaseNotFound)

	err = st.SetDrainingPhase(context.Background(), "foo", coreobjectstore.PhaseDraining)
	c.Assert(err, jc.ErrorIsNil)

	_, phase, err := st.GetActiveDrainingPhase(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(phase, gc.Equals, coreobjectstore.PhaseDraining)
}

func (s *stateSuite) TestSetDrainingPhase(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.SetDrainingPhase(context.Background(), "foo", coreobjectstore.PhaseDraining)
	c.Assert(err, jc.ErrorIsNil)

	_, phase, err := st.GetActiveDrainingPhase(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(phase, gc.Equals, coreobjectstore.PhaseDraining)

	err = st.SetDrainingPhase(context.Background(), "foo", coreobjectstore.PhaseCompleted)
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = st.GetActiveDrainingPhase(context.Background())
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrDrainingPhaseNotFound)
}

func (s *stateSuite) TestSetDrainingPhaseWithMultipleActive(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.SetDrainingPhase(context.Background(), "foo", coreobjectstore.PhaseDraining)
	c.Assert(err, jc.ErrorIsNil)

	_, phase, err := st.GetActiveDrainingPhase(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(phase, gc.Equals, coreobjectstore.PhaseDraining)

	err = st.SetDrainingPhase(context.Background(), "bar", coreobjectstore.PhaseDraining)
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrDrainingAlreadyInProgress)
}
