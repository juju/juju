// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// instanceSuite is a test suite for asserting the parts of the [Service]
// interface that relate to storage instances.
type instanceSuite struct {
	state                 *MockState
	storageRegistryGetter *MockModelStorageRegistryGetter
}

// TestInstanceSuite runs all of the tests contained within [instanceSuite].
func TestInstanceSuite(t *testing.T) {
	tc.Run(t, &instanceSuite{})
}

func (s *instanceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.storageRegistryGetter = NewMockModelStorageRegistryGetter(ctrl)

	c.Cleanup(func() {
		s.state = nil
		s.storageRegistryGetter = nil
	})
	return ctrl
}

// TestGetStorageInstanceUUIDForIDNotFound tests getting a storage instance
// uuid for a storage id and then when the storage id does not exist in the
// model the caller gets back a error satisfying
// [domainstorageerrors.StorageInstanceNotFound].
func (s *instanceSuite) TestGetStorageInstanceUUIDForIDNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateExp := s.state.EXPECT()
	stateExp.GetStorageInstanceUUIDByID(gomock.Any(), "id1").Return(
		"", domainstorageerrors.StorageInstanceNotFound,
	)

	svc := NewService(
		s.state, loggertesting.WrapCheckLog(c), s.storageRegistryGetter,
	)
	_, err := svc.GetStorageInstanceUUIDForID(c.Context(), "id1")
	c.Check(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotFound)
}

// TestGetStorageInstanceUUIDForID is a happy path test for
// [Service.GetStorageInstanceUUIDForID].
func (s *instanceSuite) TestGetStorageInstanceUUIDForID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	stateExp := s.state.EXPECT()
	stateExp.GetStorageInstanceUUIDByID(gomock.Any(), "id1").Return(
		storageInstanceUUID, nil,
	)

	svc := NewService(
		s.state, loggertesting.WrapCheckLog(c), s.storageRegistryGetter,
	)
	uuid, err := svc.GetStorageInstanceUUIDForID(c.Context(), "id1")
	c.Check(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, storageInstanceUUID)
}
