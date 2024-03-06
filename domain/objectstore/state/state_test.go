// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/objectstore"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestGetMetadataNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetMetadata(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, sql.ErrNoRows)
}

func (s *stateSuite) TestGetMetadataFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata := objectstore.Metadata{
		UUID: uuid.MustNewUUID().String(),
		Hash: "hash",
		Path: "blah-foo",
		Size: 666,
	}

	err := st.PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIsNil)

	received, err := st.GetMetadata(context.Background(), metadata.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, gc.DeepEquals, metadata)
}

func (s *stateSuite) TestListMetadataFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata := objectstore.Metadata{
		UUID: uuid.MustNewUUID().String(),
		Hash: "hash",
		Path: "blah-foo",
		Size: 666,
	}

	err := st.PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIsNil)

	received, err := st.ListMetadata(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, gc.DeepEquals, []objectstore.Metadata{metadata})
}

func (s *stateSuite) TestPutMetadataConflict(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata := objectstore.Metadata{
		UUID: uuid.MustNewUUID().String(),
		Hash: "hash",
		Path: "blah-foo",
		Size: 666,
	}

	err := st.PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIsNil)

	err = st.PutMetadata(context.Background(), metadata)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
	c.Check(err, jc.ErrorIs, objectstoreerrors.ErrHashAlreadyExists)
}

func (s *stateSuite) TestPutMetadataWithSameHashAndSize(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata1 := objectstore.Metadata{
		UUID: uuid.MustNewUUID().String(),
		Hash: "hash",
		Path: "blah-foo-1",
		Size: 666,
	}
	metadata2 := objectstore.Metadata{
		UUID: uuid.MustNewUUID().String(),
		Hash: "hash",
		Path: "blah-foo-2",
		Size: 666,
	}

	err := st.PutMetadata(context.Background(), metadata1)
	c.Assert(err, jc.ErrorIsNil)

	err = st.PutMetadata(context.Background(), metadata2)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestPutMetadataWithSameHashDifferentSize(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Test if the hash is the same but the size is different. The root
	// cause of this, is if the hash is the same, but the size is different.
	// There is a broken hash function somewhere.

	metadata1 := objectstore.Metadata{
		UUID: uuid.MustNewUUID().String(),
		Hash: "hash",
		Path: "blah-foo-1",
		Size: 666,
	}
	metadata2 := objectstore.Metadata{
		UUID: uuid.MustNewUUID().String(),
		Hash: "hash",
		Path: "blah-foo-2",
		Size: 42,
	}

	err := st.PutMetadata(context.Background(), metadata1)
	c.Assert(err, jc.ErrorIsNil)

	err = st.PutMetadata(context.Background(), metadata2)
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrHashAndSizeAlreadyExists)
}

func (s *stateSuite) TestPutMetadataMultipleTimes(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Ensure that we can add the same metadata multiple times.
	metadatas := make([]objectstore.Metadata, 10)

	for i := 0; i < 10; i++ {
		metadatas[i] = objectstore.Metadata{
			UUID: uuid.MustNewUUID().String(),
			Hash: fmt.Sprintf("hash-%d", i),
			Path: fmt.Sprintf("blah-foo-%d", i),
			Size: 666,
		}

		err := st.PutMetadata(context.Background(), metadatas[i])
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
	c.Assert(err, jc.ErrorIs, sql.ErrNoRows)
}

func (s *stateSuite) TestRemoveMetadataDoesNotRemoveMetadataIfReferenced(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata1 := objectstore.Metadata{
		UUID: uuid.MustNewUUID().String(),
		Hash: "hash",
		Path: "blah-foo-1",
		Size: 666,
	}
	metadata2 := objectstore.Metadata{
		UUID: uuid.MustNewUUID().String(),
		Hash: "hash",
		Path: "blah-foo-2",
		Size: 666,
	}

	err := st.PutMetadata(context.Background(), metadata1)
	c.Assert(err, jc.ErrorIsNil)

	err = st.PutMetadata(context.Background(), metadata2)
	c.Assert(err, jc.ErrorIsNil)

	err = st.RemoveMetadata(context.Background(), metadata2.Path)
	c.Assert(err, jc.ErrorIsNil)

	received, err := st.GetMetadata(context.Background(), metadata1.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, gc.DeepEquals, metadata1)
}

func (s *stateSuite) TestRemoveMetadataCleansUpEverything(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata1 := objectstore.Metadata{
		UUID: uuid.MustNewUUID().String(),
		Hash: "hash",
		Path: "blah-foo-1",
		Size: 666,
	}
	metadata2 := objectstore.Metadata{
		UUID: uuid.MustNewUUID().String(),
		Hash: "hash",
		Path: "blah-foo-2",
		Size: 666,
	}

	// Add both metadata.
	err := st.PutMetadata(context.Background(), metadata1)
	c.Assert(err, jc.ErrorIsNil)
	err = st.PutMetadata(context.Background(), metadata2)
	c.Assert(err, jc.ErrorIsNil)

	// Remove both metadata.
	err = st.RemoveMetadata(context.Background(), metadata1.Path)
	c.Assert(err, jc.ErrorIsNil)
	err = st.RemoveMetadata(context.Background(), metadata2.Path)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that both metadata have been removed.
	_, err = st.GetMetadata(context.Background(), metadata1.Path)
	c.Assert(err, jc.ErrorIs, sql.ErrNoRows)
	_, err = st.GetMetadata(context.Background(), metadata2.Path)
	c.Assert(err, jc.ErrorIs, sql.ErrNoRows)

	// Add a new metadata with the same hash and size.
	metadata3 := objectstore.Metadata{
		UUID: uuid.MustNewUUID().String(),
		Hash: "hash",
		Path: "blah-foo-3",
		Size: 666,
	}
	err = st.PutMetadata(context.Background(), metadata3)
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

	metadata := objectstore.Metadata{
		UUID: uuid.MustNewUUID().String(),
		Hash: "hash",
		Path: "blah-foo-1",
		Size: 666,
	}

	err := st.PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIsNil)

	err = st.RemoveMetadata(context.Background(), metadata.Path)
	c.Assert(err, jc.ErrorIsNil)

	err = st.PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIsNil)

	received, err := st.GetMetadata(context.Background(), metadata.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, gc.DeepEquals, metadata)
}

func (s *stateSuite) TestListMetadata(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	metadata := objectstore.Metadata{
		UUID: uuid.MustNewUUID().String(),
		Hash: "hash",
		Path: "blah-foo-1",
		Size: 666,
	}

	err := st.PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIsNil)

	metadatas, err := st.ListMetadata(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadatas, gc.HasLen, 1)

	c.Check(metadatas[0], gc.DeepEquals, metadata)
}
