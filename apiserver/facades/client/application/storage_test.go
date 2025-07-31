// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	applicationservice "github.com/juju/juju/domain/application/service"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/uuid"
)

type storageSuite struct {
	storageService *MockStorageService
}

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.storageService = NewMockStorageService(ctrl)
	c.Cleanup(func() {
		s.storageService = nil
	})
	return ctrl
}

func (s *storageSuite) TestStorageDirectives(c *tc.C) {
	defer s.setupMocks(c).Finish()

	poolUUID := domainstorage.StoragePoolUUID(uuid.MustNewUUID().String())
	s.storageService.EXPECT().GetStoragePoolByName(gomock.Any(), "test-pool").Return(domainstorage.StoragePool{
		UUID: poolUUID.String(),
	}, nil)

	directives := map[string]storage.Directive{
		"a": {
			Pool: "test-pool",
		},
		"b": {
			Size: 123,
		},
		"c": {
			Count: 5,
		},
	}
	sdo, err := storageDirectives(c.Context(), s.storageService, directives)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sdo, tc.DeepEquals, map[string]applicationservice.ApplicationStorageDirectiveOverride{
		"a": {PoolUUID: &poolUUID},
		"b": {Size: ptr[uint64](123)},
		"c": {Count: ptr[uint32](5)},
	})
}
