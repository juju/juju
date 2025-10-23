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
