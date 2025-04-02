// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreobjectstore "github.com/juju/juju/core/objectstore"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestGetMetadataNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetMetadata(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrNotFound)
}

func (s *stateSuite) TestGetMetadataFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo",
		Size:   666,
	}

	_, err := st.PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIsNil)

	received, err := st.GetMetadata(context.Background(), metadata.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, gc.DeepEquals, metadata)
}

func (s *stateSuite) TestGetMetadataBySHA256Found(c *gc.C) {
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

	_, err := st.PutMetadata(context.Background(), metadata1)
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.PutMetadata(context.Background(), metadata2)
	c.Assert(err, jc.ErrorIsNil)

	received, err := st.GetMetadataBySHA256(context.Background(), "41af286dc0b172ed2f1ca934fd2278de4a1192302ffa07087cea2682e7d372e3")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, gc.DeepEquals, metadata1)

	received, err = st.GetMetadataBySHA256(context.Background(), "b867951a18e694f3415cbef36be5a05de2d43f795f87c87756749e7bb6545b11")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, gc.DeepEquals, metadata2)
}

func (s *stateSuite) TestGetMetadataBySHA256NotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetMetadataBySHA256(context.Background(), "deadbeef")
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrNotFound)
}

func (s *stateSuite) TestGetMetadataBySHA256PrefixFound(c *gc.C) {
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

	_, err := st.PutMetadata(context.Background(), metadata1)
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.PutMetadata(context.Background(), metadata2)
	c.Assert(err, jc.ErrorIsNil)

	received, err := st.GetMetadataBySHA256Prefix(context.Background(), "41af286")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, gc.DeepEquals, metadata1)

	received, err = st.GetMetadataBySHA256Prefix(context.Background(), "b867951")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, gc.DeepEquals, metadata2)

	received, err = st.GetMetadataBySHA256Prefix(context.Background(), "b867951a18e")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, gc.DeepEquals, metadata2)
}

func (s *stateSuite) TestGetMetadataBySHA256PrefixNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetMetadataBySHA256Prefix(context.Background(), "deadbeef")
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrNotFound)
}

func (s *stateSuite) TestListMetadataFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo",
		Size:   666,
	}

	_, err := st.PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIsNil)

	received, err := st.ListMetadata(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, gc.DeepEquals, []coreobjectstore.Metadata{metadata})
}

func (s *stateSuite) TestPutMetadata(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo",
		Size:   666,
	}

	uuid, err := st.PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIsNil)

	runner, err := s.TxnRunnerFactory()()
	c.Assert(err, jc.ErrorIsNil)

	var received coreobjectstore.Metadata
	err = runner.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT path, size, sha_256, sha_384 FROM v_object_store_metadata WHERE uuid = ?`, uuid)
		return row.Scan(&received.Path, &received.Size, &received.SHA256, &received.SHA384)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, gc.DeepEquals, metadata)
}

func (s *stateSuite) TestPutMetadataConflict(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo",
		Size:   666,
	}

	_, err := st.PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.PutMetadata(context.Background(), metadata)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
	c.Check(err, jc.ErrorIs, objectstoreerrors.ErrHashAndSizeAlreadyExists)
}

func (s *stateSuite) TestPutMetadataConflictDifferentHash(c *gc.C) {
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

	_, err := st.PutMetadata(context.Background(), metadata1)
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.PutMetadata(context.Background(), metadata2)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
	c.Check(err, jc.ErrorIs, objectstoreerrors.ErrPathAlreadyExistsDifferentHash)
}

func (s *stateSuite) TestPutMetadataWithSameHashesAndSize(c *gc.C) {
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

	_, err := st.PutMetadata(context.Background(), metadata1)
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.PutMetadata(context.Background(), metadata2)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestPutMetadataWithSameSHA256AndSize(c *gc.C) {
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

	uuid1, err := st.PutMetadata(context.Background(), metadata1)
	c.Assert(err, jc.ErrorIsNil)

	uuid2, err := st.PutMetadata(context.Background(), metadata2)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(uuid1, gc.Equals, uuid2)
}

func (s *stateSuite) TestPutMetadataWithSameSHA384AndSize(c *gc.C) {
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

	uuid1, err := st.PutMetadata(context.Background(), metadata1)
	c.Assert(err, jc.ErrorIsNil)

	uuid2, err := st.PutMetadata(context.Background(), metadata2)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(uuid1, gc.Equals, uuid2)
}

func (s *stateSuite) TestPutMetadataWithSameHashDifferentSize(c *gc.C) {
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

	_, err := st.PutMetadata(context.Background(), metadata1)
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.PutMetadata(context.Background(), metadata2)
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrHashAndSizeAlreadyExists)
}

func (s *stateSuite) TestPutMetadataMultipleTimes(c *gc.C) {
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

		_, err := st.PutMetadata(context.Background(), metadatas[i])
		c.Assert(err, jc.ErrorIsNil)
	}

	for i := 0; i < 10; i++ {
		metadata, err := st.GetMetadata(context.Background(), fmt.Sprintf("blah-foo-%d", i))
		c.Assert(err, jc.ErrorIsNil)
		c.Check(metadata, jc.DeepEquals, metadatas[i])
	}
}

func (s *stateSuite) TestRemoveMetadataNotExists(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.RemoveMetadata(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrNotFound)
}

func (s *stateSuite) TestRemoveMetadataDoesNotRemoveMetadataIfReferenced(c *gc.C) {
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

	_, err := st.PutMetadata(context.Background(), metadata1)
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.PutMetadata(context.Background(), metadata2)
	c.Assert(err, jc.ErrorIsNil)

	err = st.RemoveMetadata(context.Background(), metadata2.Path)
	c.Assert(err, jc.ErrorIsNil)

	received, err := st.GetMetadata(context.Background(), metadata1.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, gc.DeepEquals, metadata1)
}

func (s *stateSuite) TestRemoveMetadataCleansUpEverything(c *gc.C) {
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
	_, err := st.PutMetadata(context.Background(), metadata1)
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.PutMetadata(context.Background(), metadata2)
	c.Assert(err, jc.ErrorIsNil)

	// Remove both metadata.
	err = st.RemoveMetadata(context.Background(), metadata1.Path)
	c.Assert(err, jc.ErrorIsNil)
	err = st.RemoveMetadata(context.Background(), metadata2.Path)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that both metadata have been removed.
	_, err = st.GetMetadata(context.Background(), metadata1.Path)
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrNotFound)
	_, err = st.GetMetadata(context.Background(), metadata2.Path)
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrNotFound)

	// Add a new metadata with the same hash and size.
	metadata3 := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo-3",
		Size:   666,
	}
	_, err = st.PutMetadata(context.Background(), metadata3)
	c.Assert(err, jc.ErrorIsNil)

	// We guarantee that the metadata has been added is unique, because
	// the UUID would be UUID from metadata1 if the metadata has not been
	// removed.
	received, err := st.GetMetadata(context.Background(), metadata3.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, gc.DeepEquals, metadata3)
}

func (s *stateSuite) TestRemoveMetadataThenAddAgain(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo-1",
		Size:   666,
	}

	_, err := st.PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIsNil)

	err = st.RemoveMetadata(context.Background(), metadata.Path)
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIsNil)

	received, err := st.GetMetadata(context.Background(), metadata.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, gc.DeepEquals, metadata)
}

func (s *stateSuite) TestListMetadata(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata := coreobjectstore.Metadata{
		SHA256: "sha256",
		SHA384: "sha384",
		Path:   "blah-foo-1",
		Size:   666,
	}

	_, err := st.PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIsNil)

	metadatas, err := st.ListMetadata(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadatas, gc.HasLen, 1)

	c.Check(metadatas[0], gc.DeepEquals, metadata)
}

func (s *stateSuite) TestListMetadataNoRows(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadatas, err := st.ListMetadata(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadatas, gc.HasLen, 0)
}
