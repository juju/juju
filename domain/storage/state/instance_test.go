// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
)

// instanceSuite is a test suite for asserting storage instance based interfaces
// in this package.
type instanceSuite struct {
	baseSuite
}

// TestInstanceSuite runs the tests contained within [instanceSuite].
func TestInstanceSuite(t *testing.T) {
	tc.Run(t, &instanceSuite{})
}

// TestGetStorageInstanceUUIDByID tests the happy path of getting a storage
// innstance uuid by it's id value.
func (s *instanceSuite) TestGetStorageInstanceUUIDByID(c *tc.C) {
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	uuid, id := s.newStorageInstanceForCharmWithPool(
		c, "ory-kratos", poolUUID, "token",
	)

	st := NewState(s.TxnRunnerFactory())
	gotUUID, err := st.GetStorageInstanceUUIDByID(c.Context(), id)
	c.Check(err, tc.ErrorIsNil)
	c.Check(gotUUID, tc.Equals, uuid)
}

// TestGetStorageInstanceUUIDByIDNotFound tests the case where a storage
// instance cannot be found for a given storage id. In this case the caller MUST
// get back an error satisfying [domainstorageerrors.StorageInstanceNotFound].
func (s *instanceSuite) TestGetStorageInstanceUUIDByIDNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetStorageInstanceUUIDByID(c.Context(), "non-existent-id")
	c.Check(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotFound)
}

func (s *instanceSuite) TestGetStorageInstanceUUIDsByIDs(c *tc.C) {
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	uuid1, id1 := s.newStorageInstanceForCharmWithPool(
		c, "foo", poolUUID, "token1",
	)
	uuid2, id2 := s.newStorageInstanceForCharmWithPool(
		c, "bar", poolUUID, "token2",
	)

	st := NewState(s.TxnRunnerFactory())
	uuidMap, err := st.GetStorageInstanceUUIDsByIDs(c.Context(), []string{id1, id2})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuidMap, tc.DeepEquals, map[string]string{
		id1: uuid1.String(),
		id2: uuid2.String(),
	})
}

func (s *instanceSuite) TestGetStorageInstanceUUIDsByIDsDuplicateIDs(c *tc.C) {
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	uuid1, id1 := s.newStorageInstanceForCharmWithPool(
		c, "foo", poolUUID, "token1",
	)
	uuid2, id2 := s.newStorageInstanceForCharmWithPool(
		c, "bar", poolUUID, "token2",
	)

	st := NewState(s.TxnRunnerFactory())
	uuidMap, err := st.GetStorageInstanceUUIDsByIDs(c.Context(), []string{id1, id2, id1})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuidMap, tc.DeepEquals, map[string]string{
		id1: uuid1.String(),
		id2: uuid2.String(),
	})
}

func (s *instanceSuite) TestGetStorageInstanceUUIDsByIDsMiss(c *tc.C) {
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	uuid1, id1 := s.newStorageInstanceForCharmWithPool(
		c, "foo", poolUUID, "token1",
	)
	uuid2, id2 := s.newStorageInstanceForCharmWithPool(
		c, "bar", poolUUID, "token2",
	)

	st := NewState(s.TxnRunnerFactory())
	uuidMap, err := st.GetStorageInstanceUUIDsByIDs(c.Context(), []string{id1, id2, "foo", "bar"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuidMap, tc.DeepEquals, map[string]string{
		id1: uuid1.String(),
		id2: uuid2.String(),
	})
}

func (s *instanceSuite) TestGetStorageInstanceUUIDsByVolumeIDs(c *tc.C) {
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	storage1UUID, _ := s.newStorageInstanceForCharmWithPool(
		c, "foo", poolUUID, "token1",
	)
	storage2UUID, _ := s.newStorageInstanceForCharmWithPool(
		c, "bar", poolUUID, "token2",
	)

	volume1UUID := s.newStorageVolume(c, "vol-1")
	s.newStorageInstanceVolume(c, storage1UUID, volume1UUID)

	volume2UUID := s.newStorageVolume(c, "vol-2")
	s.newStorageInstanceVolume(c, storage2UUID, volume2UUID)

	st := NewState(s.TxnRunnerFactory())
	uuidMap, err := st.GetStorageInstanceUUIDsByVolumeIDs(c.Context(), []string{"vol-1", "vol-2"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuidMap, tc.DeepEquals, map[string]string{
		"vol-1": storage1UUID.String(),
		"vol-2": storage2UUID.String(),
	})
}

func (s *instanceSuite) TestGetStorageInstanceUUIDsByVolumeIDsDuplicateIDs(c *tc.C) {
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	storageUUID, _ := s.newStorageInstanceForCharmWithPool(
		c, "foo", poolUUID, "token1",
	)

	volumeUUID := s.newStorageVolume(c, "vol-1")
	s.newStorageInstanceVolume(c, storageUUID, volumeUUID)

	st := NewState(s.TxnRunnerFactory())
	uuidMap, err := st.GetStorageInstanceUUIDsByVolumeIDs(c.Context(), []string{"vol-1", "vol-1"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuidMap, tc.DeepEquals, map[string]string{
		"vol-1": storageUUID.String(),
	})
}

func (s *instanceSuite) TestGetStorageInstanceUUIDsByVolumeIDsMiss(c *tc.C) {
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	storageUUID, _ := s.newStorageInstanceForCharmWithPool(
		c, "foo", poolUUID, "token1",
	)

	volumeUUID := s.newStorageVolume(c, "vol-1")
	s.newStorageInstanceVolume(c, storageUUID, volumeUUID)

	st := NewState(s.TxnRunnerFactory())
	uuidMap, err := st.GetStorageInstanceUUIDsByVolumeIDs(c.Context(), []string{"vol-1", "vol-missing"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuidMap, tc.DeepEquals, map[string]string{
		"vol-1": storageUUID.String(),
	})
}
