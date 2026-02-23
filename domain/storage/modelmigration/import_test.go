// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v11"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	corestorage "github.com/juju/juju/core/storage"
	domainstorage "github.com/juju/juju/domain/storage"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
)

type importSuite struct {
	coordinator             *MockCoordinator
	service                 *MockImportService
	storageProviderRegistry *MockProviderRegistry
	storageRegistryGetter   *MockModelStorageRegistryGetter
	storageProvider         *MockProvider
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)
	s.storageProviderRegistry = NewMockProviderRegistry(ctrl)
	s.storageRegistryGetter = NewMockModelStorageRegistryGetter(ctrl)
	s.storageProvider = NewMockProvider(ctrl)

	c.Cleanup(func() {
		s.coordinator = nil
		s.service = nil
		s.storageProviderRegistry = nil
		s.storageRegistryGetter = nil
		s.storageProvider = nil
	})

	return ctrl
}

func (s *importSuite) newImportOperation() *importOperation {
	return &importOperation{
		storageRegistryGetter: s.storageRegistryGetter,
		service:               s.service,
	}
}

func (s *importSuite) TestRegisterImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
		return s.storageProviderRegistry
	}), loggertesting.WrapCheckLog(c))
}

func (s *importSuite) TestImportEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	s.noopStoragePoolImport()

	// Act
	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportStoragePools tests that Execute imports both user-defined and provider default
// storage pools and sets the recommended pools.
func (s *importSuite) TestImportStoragePools(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	model.AddStoragePool(description.StoragePoolArgs{
		Name:       "ebs-fast",
		Provider:   "ebs",
		Attributes: map[string]any{"foo": "bar"},
	})

	ctx := c.Context()

	poolsToImport := []domainstorage.UserStoragePoolParams{
		{
			Name:       "ebs-fast",
			Provider:   "ebs",
			Attributes: map[string]interface{}{"foo": "bar"},
		},
	}
	s.service.EXPECT().ImportStoragePools(ctx, poolsToImport)

	op := s.newImportOperation()
	err := op.Execute(ctx, model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportStorageInstances(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	expected := []domainstorage.ImportStorageInstanceParams{
		{
			PoolName:         "testpool",
			RequestedSizeMiB: uint64(1024),
			StorageID:        "multi-fs/1",
			StorageKind:      "block",
			StorageName:      "multi-fs",
			UnitName:         "unit/3",
		},
	}
	s.noopStoragePoolImport()
	s.service.EXPECT().ImportStorageInstances(gomock.Any(), expected).Return(nil)
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	model.AddStorage(description.StorageArgs{
		ID:          "multi-fs/1",
		Kind:        "block",
		UnitOwner:   "unit/3",
		Name:        "multi-fs",
		Attachments: nil,
		Constraints: &description.StorageInstanceConstraints{
			Pool: "testpool",
			Size: 1024,
		},
	})

	// Act
	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportStorageInstancesValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	model.AddStorage(description.StorageArgs{
		Kind:        "block",
		UnitOwner:   "unit/3",
		Name:        "multi-fs",
		Attachments: nil,
		Constraints: &description.StorageInstanceConstraints{
			Pool: "testpool",
			Size: 1024,
		},
	})
	s.noopStoragePoolImport()

	// Act
	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportFilesystems(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	model.AddFilesystem(description.FilesystemArgs{
		ID:           "fs-1",
		Size:         2048,
		Storage:      "multi-fs/1",
		Pool:         "testpool",
		FilesystemID: "provider-fs-1",
	})
	model.AddFilesystem(description.FilesystemArgs{
		ID:           "fs-2",
		Size:         4096,
		Storage:      "multi-fs/2",
		Pool:         "testpool",
		FilesystemID: "provider-fs-2",
	})

	s.noopStoragePoolImport()
	s.service.EXPECT().ImportFilesystems(gomock.Any(), tc.Bind(tc.SameContents, []domainstorage.ImportFilesystemParams{{
		ID:                "fs-1",
		SizeInMiB:         2048,
		StorageInstanceID: "multi-fs/1",
		PoolName:          "testpool",
		ProviderID:        "provider-fs-1",
	}, {
		ID:                "fs-2",
		SizeInMiB:         4096,
		StorageInstanceID: "multi-fs/2",
		PoolName:          "testpool",
		ProviderID:        "provider-fs-2",
	}})).Return(nil)

	// Act
	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) noopStoragePoolImport() {
	s.service.EXPECT().ImportStoragePools(gomock.Any(), gomock.Any()).Return(nil)
}
