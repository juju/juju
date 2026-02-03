// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	corestorage "github.com/juju/juju/core/storage"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageinternal "github.com/juju/juju/domain/storage/internal"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	internalstorage "github.com/juju/juju/internal/storage"
)

// adoptFilesystemSuite is a test suite for asserting the functionality of
// [StorageService.AdoptFilesystem].
type adoptFilesystemSuite struct {
	state            *MockState
	registry         *MockProviderRegistry
	provider         *MockProvider
	volumeSource     *mockVolumeSourceAndImporter
	filesystemSource *mockFilesystemSourceAndImporter
}

type mockVolumeSourceAndImporter struct {
	*MockVolumeSource
	*MockVolumeImporter
}

type mockFilesystemSourceAndImporter struct {
	*MockFilesystemSource
	*MockFilesystemImporter
}

func TestAdoptFilesystemSuite(t *testing.T) {
	tc.Run(t, &adoptFilesystemSuite{})
}

func (s *adoptFilesystemSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.registry = NewMockProviderRegistry(ctrl)
	s.provider = NewMockProvider(ctrl)
	s.filesystemSource = &mockFilesystemSourceAndImporter{
		MockFilesystemSource:   NewMockFilesystemSource(ctrl),
		MockFilesystemImporter: NewMockFilesystemImporter(ctrl),
	}
	s.volumeSource = &mockVolumeSourceAndImporter{
		MockVolumeSource:   NewMockVolumeSource(ctrl),
		MockVolumeImporter: NewMockVolumeImporter(ctrl),
	}
	c.Cleanup(func() {
		s.state = nil
		s.registry = nil
		s.provider = nil
		s.filesystemSource = nil
		s.volumeSource = nil
	})
	return ctrl
}

func (s *adoptFilesystemSuite) makeService() *StorageService {
	return &StorageService{
		st: s.state,
		registryGetter: modelStorageRegistryGetter(
			func() internalstorage.ProviderRegistry {
				return s.registry
			},
		),
	}
}

func (s *adoptFilesystemSuite) TestAdoptFilesystemInvalidStorageName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	svc := s.makeService()
	_, err := svc.AdoptFilesystem(
		c.Context(),
		"",
		poolUUID,
		"provider-id",
		false,
	)
	c.Assert(err, tc.ErrorIs, domainstorageerrors.InvalidStorageName)
}

func (s *adoptFilesystemSuite) TestAdoptFilesystemInvalidPoolUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := s.makeService()
	_, err := svc.AdoptFilesystem(
		c.Context(),
		"mystorage",
		domainstorage.StoragePoolUUID("invalid"),
		"provider-id",
		false,
	)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *adoptFilesystemSuite) TestAdoptFilesystemEmptyProviderID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	svc := s.makeService()
	_, err := svc.AdoptFilesystem(c.Context(), "mystorage", poolUUID, "", false)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *adoptFilesystemSuite) TestAdoptFilesystemPoolNotFound(c *tc.C) {
	ctx := c.Context()
	defer s.setupMocks(c).Finish()

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	s.state.EXPECT().GetStoragePool(
		ctx, poolUUID,
	).Return(
		domainstorage.StoragePool{}, domainstorageerrors.StoragePoolNotFound,
	)

	svc := s.makeService()
	_, err := svc.AdoptFilesystem(
		ctx, "mystorage", poolUUID, "provider-id", false)
	c.Assert(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
}

func (s *adoptFilesystemSuite) TestAdoptFilesystemProviderTypeNotFound(c *tc.C) {
	ctx := c.Context()
	defer s.setupMocks(c).Finish()

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	pool := domainstorage.StoragePool{
		Name:     "pool1",
		Provider: "provider1",
		Attrs:    map[string]string{},
	}

	s.state.EXPECT().GetStoragePool(ctx, poolUUID).Return(pool, nil)
	s.registry.EXPECT().StorageProvider(
		internalstorage.ProviderType("provider1"),
	).Return(nil, coreerrors.NotFound)

	svc := s.makeService()
	_, err := svc.AdoptFilesystem(
		ctx, "mystorage", poolUUID, "provider-id", false)
	c.Assert(err, tc.ErrorIs, domainstorageerrors.ProviderTypeNotFound)
}

// TestAdoptFilesystemSuccessFilesystem tests successfully adopting a
// model-scoped filesystem.
func (s *adoptFilesystemSuite) TestAdoptFilesystemSuccessFilesystem(c *tc.C) {
	ctx := c.Context()
	defer s.setupMocks(c).Finish()

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	pool := domainstorage.StoragePool{
		Name:     "pool1",
		Provider: "provider1",
		Attrs:    map[string]string{},
	}
	providerID := "fs-123"
	storageName := domainstorage.Name("mystorage")

	s.state.EXPECT().GetStoragePool(ctx, poolUUID).Return(pool, nil)
	s.registry.EXPECT().StorageProvider(
		internalstorage.ProviderType("provider1"),
	).Return(s.provider, nil)
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindFilesystem).Return(true).AnyTimes()
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindBlock).Return(false).AnyTimes()
	s.provider.EXPECT().Scope().Return(internalstorage.ScopeEnviron)
	s.provider.EXPECT().FilesystemSource(
		gomock.Any()).Return(s.filesystemSource, nil)

	// Set up resource tags
	s.state.EXPECT().GetStorageResourceTagInfoForModel(
		ctx, config.ResourceTagsKey,
	).Return(domainstorageprovisioning.ModelResourceTagInfo{
		ControllerUUID:   "controller-uuid",
		ModelUUID:        "model-uuid",
		BaseResourceTags: "env=test user=admin",
	}, nil)

	expectedTags := map[string]string{
		"env":               "test",
		"user":              "admin",
		tags.JujuController: "controller-uuid",
		tags.JujuModel:      "model-uuid",
	}

	fsInfo := internalstorage.FilesystemInfo{
		ProviderId: providerID,
		Size:       1024,
	}

	s.filesystemSource.MockFilesystemImporter.EXPECT().ImportFilesystem(
		gomock.Any(), providerID, storageName.String(), expectedTags, false,
	).Return(fsInfo, nil)

	args := domainstorageinternal.CreateStorageInstanceWithExistingFilesystem{
		Name:                     storageName,
		Kind:                     domainstorage.StorageKindFilesystem,
		StoragePoolUUID:          poolUUID,
		FilesystemSize:           1024,
		FilesystemProviderID:     providerID,
		RequestedSizeMiB:         1024,
		FilesystemProvisionScope: domainstorageprovisioning.ProvisionScopeModel,
	}
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.UUID`, tc.IsNonZeroUUID)
	mc.AddExpr(`_.FilesystemUUID`, tc.IsNonZeroUUID)
	s.state.EXPECT().CreateStorageInstanceWithExistingFilesystem(
		gomock.Any(), tc.Bind(mc, args),
	).Return("instance-1", nil)

	svc := s.makeService()
	id, err := svc.AdoptFilesystem(
		ctx, storageName, poolUUID, providerID, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(id, tc.Equals, corestorage.ID("instance-1"))
}

// TestAdoptFilesystemSuccessVolumeBacked tests successfully adopting a
// filesystem backed by a model-scoped volume.
func (s *adoptFilesystemSuite) TestAdoptFilesystemSuccessVolumeBacked(c *tc.C) {
	ctx := c.Context()
	defer s.setupMocks(c).Finish()

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	pool := domainstorage.StoragePool{
		Name:     "pool1",
		Provider: "provider1",
		Attrs:    map[string]string{},
	}
	providerID := "vol-123"
	storageName := domainstorage.Name("mystorage")

	s.state.EXPECT().GetStoragePool(ctx, poolUUID).Return(pool, nil)
	s.registry.EXPECT().StorageProvider(
		internalstorage.ProviderType("provider1"),
	).Return(s.provider, nil)
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindFilesystem).Return(false).AnyTimes()
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindBlock).Return(true).AnyTimes()
	s.provider.EXPECT().Scope().Return(internalstorage.ScopeEnviron)
	s.provider.EXPECT().VolumeSource(gomock.Any()).Return(s.volumeSource, nil)
	s.state.EXPECT().GetStorageResourceTagInfoForModel(
		ctx, config.ResourceTagsKey,
	).Return(domainstorageprovisioning.ModelResourceTagInfo{
		ControllerUUID:   "controller-uuid",
		ModelUUID:        "model-uuid",
		BaseResourceTags: "",
	}, nil)

	expectedTags := map[string]string{
		tags.JujuController: "controller-uuid",
		tags.JujuModel:      "model-uuid",
	}

	volInfo := internalstorage.VolumeInfo{
		VolumeId:   providerID,
		Size:       2048,
		HardwareId: "hw-id",
		WWN:        "wwn-123",
		Persistent: true,
	}

	s.volumeSource.MockVolumeImporter.EXPECT().ImportVolume(
		ctx, providerID, storageName.String(), expectedTags, false,
	).Return(volInfo, nil)

	fsArgs := domainstorageinternal.CreateStorageInstanceWithExistingFilesystem{
		Name:                     storageName,
		Kind:                     domainstorage.StorageKindFilesystem,
		StoragePoolUUID:          poolUUID,
		FilesystemSize:           2048,
		RequestedSizeMiB:         2048,
		FilesystemProvisionScope: domainstorageprovisioning.ProvisionScopeMachine,
	}
	args := domainstorageinternal.CreateStorageInstanceWithExistingVolumeBackedFilesystem{
		CreateStorageInstanceWithExistingFilesystem: fsArgs,

		VolumeProvisionScope: domainstorageprovisioning.ProvisionScopeModel,
		VolumeProviderID:     providerID,
		VolumeSize:           2048,
		VolumeHardwareID:     "hw-id",
		VolumeWWN:            "wwn-123",
		VolumePersistent:     true,
	}
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_._.UUID`, tc.IsNonZeroUUID)
	mc.AddExpr(`_._.FilesystemUUID`, tc.IsNonZeroUUID)
	mc.AddExpr(`_.VolumeUUID`, tc.IsNonZeroUUID)
	s.state.EXPECT().CreateStorageInstanceWithExistingVolumeBackedFilesystem(
		ctx, tc.Bind(mc, args),
	).Return("instance-1", nil)

	svc := s.makeService()
	id, err := svc.AdoptFilesystem(
		ctx, storageName, poolUUID, providerID, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(id, tc.Equals, corestorage.ID("instance-1"))
}

// TestAdoptFilesystemMachineScopedFilesystemNotSupported tests that adopting
// a machine-scoped filesystem without a volume is not supported.
func (s *adoptFilesystemSuite) TestAdoptFilesystemMachineScopedFilesystemNotSupported(c *tc.C) {
	ctx := c.Context()
	defer s.setupMocks(c).Finish()

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	pool := domainstorage.StoragePool{
		Name:     "pool1",
		Provider: "provider1",
		Attrs:    map[string]string{},
	}
	storageName := domainstorage.Name("mystorage")

	s.state.EXPECT().GetStoragePool(ctx, poolUUID).Return(pool, nil)
	s.registry.EXPECT().StorageProvider(
		internalstorage.ProviderType("provider1")).Return(s.provider, nil)
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindFilesystem).Return(true).AnyTimes()
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindBlock).Return(false).AnyTimes()
	s.provider.EXPECT().Scope().Return(internalstorage.ScopeMachine)

	svc := s.makeService()
	_, err := svc.AdoptFilesystem(
		ctx, storageName, poolUUID, "provider-id", false)
	c.Assert(err, tc.ErrorIs, domainstorageerrors.AdoptionNotSupported)
}

// TestAdoptFilesystemMachineScopedVolumeNotSupported tests that adopting
// a filesystem backed by a machine-scoped volume is not supported.
func (s *adoptFilesystemSuite) TestAdoptFilesystemMachineScopedVolumeNotSupported(c *tc.C) {
	ctx := c.Context()
	defer s.setupMocks(c).Finish()

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	pool := domainstorage.StoragePool{
		Name:     "pool1",
		Provider: "provider1",
		Attrs:    map[string]string{},
	}
	storageName := domainstorage.Name("mystorage")

	s.state.EXPECT().GetStoragePool(ctx, poolUUID).Return(pool, nil)
	s.registry.EXPECT().StorageProvider(
		internalstorage.ProviderType("provider1")).Return(s.provider, nil)
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindFilesystem).Return(false).AnyTimes()
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindBlock).Return(true).AnyTimes()
	s.provider.EXPECT().Scope().Return(internalstorage.ScopeMachine)

	svc := s.makeService()
	_, err := svc.AdoptFilesystem(
		ctx, storageName, poolUUID, "provider-id", false)
	c.Assert(err, tc.ErrorIs, domainstorageerrors.AdoptionNotSupported)
}

// TestAdoptFilesystemVolumeSourceNotImporter tests that when the volume source
// does not implement VolumeImporter, an error is returned.
func (s *adoptFilesystemSuite) TestAdoptFilesystemVolumeSourceNotImporter(c *tc.C) {
	ctx := c.Context()
	defer s.setupMocks(c).Finish()

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	pool := domainstorage.StoragePool{
		Name:     "pool1",
		Provider: "provider1",
		Attrs:    map[string]string{},
	}
	storageName := domainstorage.Name("mystorage")

	s.state.EXPECT().GetStoragePool(ctx, poolUUID).Return(pool, nil)
	s.registry.EXPECT().StorageProvider(
		internalstorage.ProviderType("provider1")).Return(s.provider, nil)
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindFilesystem).Return(false).AnyTimes()
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindBlock).Return(true).AnyTimes()
	s.provider.EXPECT().Scope().Return(internalstorage.ScopeEnviron)

	// Return a volume source that doesn't implement VolumeImporter
	s.provider.EXPECT().VolumeSource(
		gomock.Any()).Return(s.volumeSource.MockVolumeSource, nil)

	s.state.EXPECT().GetStorageResourceTagInfoForModel(
		ctx, config.ResourceTagsKey,
	).Return(domainstorageprovisioning.ModelResourceTagInfo{
		ControllerUUID: "controller-uuid",
		ModelUUID:      "model-uuid",
	}, nil)

	svc := s.makeService()
	_, err := svc.AdoptFilesystem(
		ctx, storageName, poolUUID, "provider-id", false)
	c.Assert(err, tc.ErrorIs, domainstorageerrors.AdoptionNotSupported)
}

// TestAdoptFilesystemFilesystemSourceNotImporter tests that when the filesystem
// source does not implement FilesystemImporter, an error is returned.
func (s *adoptFilesystemSuite) TestAdoptFilesystemFilesystemSourceNotImporter(c *tc.C) {
	ctx := c.Context()
	defer s.setupMocks(c).Finish()

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	pool := domainstorage.StoragePool{
		Name:     "pool1",
		Provider: "provider1",
		Attrs:    map[string]string{},
	}
	storageName := domainstorage.Name("mystorage")

	s.state.EXPECT().GetStoragePool(ctx, poolUUID).Return(pool, nil)
	s.registry.EXPECT().StorageProvider(
		internalstorage.ProviderType("provider1")).Return(s.provider, nil)
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindFilesystem).Return(true).AnyTimes()
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindBlock).Return(false).AnyTimes()
	s.provider.EXPECT().Scope().Return(internalstorage.ScopeEnviron)

	// Return a filesystem source that doesn't implement FilesystemImporter
	s.provider.EXPECT().FilesystemSource(
		gomock.Any()).Return(s.filesystemSource.MockFilesystemSource, nil)

	s.state.EXPECT().GetStorageResourceTagInfoForModel(
		ctx, config.ResourceTagsKey,
	).Return(domainstorageprovisioning.ModelResourceTagInfo{
		ControllerUUID: "controller-uuid",
		ModelUUID:      "model-uuid",
	}, nil)

	svc := s.makeService()
	_, err := svc.AdoptFilesystem(
		ctx, storageName, poolUUID, "provider-id", false)
	c.Assert(err, tc.ErrorIs, domainstorageerrors.AdoptionNotSupported)
}

// TestAdoptFilesystemVolumeImportNotSupported tests that when the volume
// importer returns NotSupported, the appropriate error is returned.
func (s *adoptFilesystemSuite) TestAdoptFilesystemVolumeImportNotSupported(c *tc.C) {
	ctx := c.Context()
	defer s.setupMocks(c).Finish()

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	pool := domainstorage.StoragePool{
		Name:     "pool1",
		Provider: "provider1",
		Attrs:    map[string]string{},
	}
	providerID := "vol-123"
	storageName := domainstorage.Name("mystorage")

	s.state.EXPECT().GetStoragePool(ctx, poolUUID).Return(pool, nil)
	s.registry.EXPECT().StorageProvider(
		internalstorage.ProviderType("provider1")).Return(s.provider, nil)
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindFilesystem).Return(false).AnyTimes()
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindBlock).Return(true).AnyTimes()
	s.provider.EXPECT().Scope().Return(internalstorage.ScopeEnviron)
	s.provider.EXPECT().VolumeSource(gomock.Any()).Return(s.volumeSource, nil)
	s.state.EXPECT().GetStorageResourceTagInfoForModel(
		ctx, config.ResourceTagsKey,
	).Return(domainstorageprovisioning.ModelResourceTagInfo{
		ControllerUUID: "controller-uuid",
		ModelUUID:      "model-uuid",
	}, nil)

	s.volumeSource.MockVolumeImporter.EXPECT().ImportVolume(
		ctx, providerID, storageName.String(), gomock.Any(), false,
	).Return(internalstorage.VolumeInfo{}, coreerrors.NotSupported)

	svc := s.makeService()
	_, err := svc.AdoptFilesystem(ctx, storageName, poolUUID, providerID, false)
	c.Assert(err, tc.ErrorIs, domainstorageerrors.AdoptionNotSupported)
}

// TestAdoptFilesystemFilesystemImportNotSupported tests that when the
// filesystem importer returns NotSupported, the appropriate error is returned.
func (s *adoptFilesystemSuite) TestAdoptFilesystemFilesystemImportNotSupported(c *tc.C) {
	ctx := c.Context()
	defer s.setupMocks(c).Finish()

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	pool := domainstorage.StoragePool{
		Name:     "pool1",
		Provider: "provider1",
		Attrs:    map[string]string{},
	}
	providerID := "fs-123"
	storageName := domainstorage.Name("mystorage")

	s.state.EXPECT().GetStoragePool(ctx, poolUUID).Return(pool, nil)
	s.registry.EXPECT().StorageProvider(
		internalstorage.ProviderType("provider1")).Return(s.provider, nil)
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindFilesystem).Return(true).AnyTimes()
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindBlock).Return(false).AnyTimes()
	s.provider.EXPECT().Scope().Return(internalstorage.ScopeEnviron)
	s.provider.EXPECT().FilesystemSource(
		gomock.Any()).Return(s.filesystemSource, nil)
	s.state.EXPECT().GetStorageResourceTagInfoForModel(
		ctx, config.ResourceTagsKey,
	).Return(domainstorageprovisioning.ModelResourceTagInfo{
		ControllerUUID: "controller-uuid",
		ModelUUID:      "model-uuid",
	}, nil)

	s.filesystemSource.MockFilesystemImporter.EXPECT().ImportFilesystem(
		ctx, providerID, storageName.String(), gomock.Any(), false,
	).Return(internalstorage.FilesystemInfo{}, coreerrors.NotSupported)

	svc := s.makeService()
	_, err := svc.AdoptFilesystem(ctx, storageName, poolUUID, providerID, false)
	c.Assert(err, tc.ErrorIs, domainstorageerrors.AdoptionNotSupported)
}

// TestAdoptFilesystemVolumeNotFoundOnProvider tests that when the volume is
// not found on the provider, the appropriate error is returned.
func (s *adoptFilesystemSuite) TestAdoptFilesystemVolumeNotFoundOnProvider(c *tc.C) {
	ctx := c.Context()
	defer s.setupMocks(c).Finish()

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	pool := domainstorage.StoragePool{
		Name:     "pool1",
		Provider: "provider1",
		Attrs:    map[string]string{},
	}
	providerID := "vol-123"
	storageName := domainstorage.Name("mystorage")

	s.state.EXPECT().GetStoragePool(ctx, poolUUID).Return(pool, nil)
	s.registry.EXPECT().StorageProvider(
		internalstorage.ProviderType("provider1")).Return(s.provider, nil)
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindFilesystem).Return(false).AnyTimes()
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindBlock).Return(true).AnyTimes()
	s.provider.EXPECT().Scope().Return(internalstorage.ScopeEnviron)
	s.provider.EXPECT().VolumeSource(gomock.Any()).Return(s.volumeSource, nil)
	s.state.EXPECT().GetStorageResourceTagInfoForModel(
		ctx, config.ResourceTagsKey,
	).Return(domainstorageprovisioning.ModelResourceTagInfo{
		ControllerUUID: "controller-uuid",
		ModelUUID:      "model-uuid",
	}, nil)

	s.volumeSource.MockVolumeImporter.EXPECT().ImportVolume(
		ctx, providerID, storageName.String(), gomock.Any(), false,
	).Return(internalstorage.VolumeInfo{}, coreerrors.NotFound)

	svc := s.makeService()
	_, err := svc.AdoptFilesystem(ctx, storageName, poolUUID, providerID, false)
	c.Assert(err, tc.ErrorIs, domainstorageerrors.PooledStorageEntityNotFound)
}

// TestAdoptFilesystemFilesystemNotFoundOnProvider tests that when the
// filesystem is not found on the provider, the appropriate error is returned.
func (s *adoptFilesystemSuite) TestAdoptFilesystemFilesystemNotFoundOnProvider(c *tc.C) {
	ctx := c.Context()
	defer s.setupMocks(c).Finish()

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	pool := domainstorage.StoragePool{
		Name:     "pool1",
		Provider: "provider1",
		Attrs:    map[string]string{},
	}
	providerID := "fs-123"
	storageName := domainstorage.Name("mystorage")

	s.state.EXPECT().GetStoragePool(ctx, poolUUID).Return(pool, nil)
	s.registry.EXPECT().StorageProvider(
		internalstorage.ProviderType("provider1")).Return(s.provider, nil)
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindFilesystem).Return(true).AnyTimes()
	s.provider.EXPECT().Supports(
		internalstorage.StorageKindBlock).Return(false).AnyTimes()
	s.provider.EXPECT().Scope().Return(internalstorage.ScopeEnviron)
	s.provider.EXPECT().FilesystemSource(
		gomock.Any()).Return(s.filesystemSource, nil)
	s.state.EXPECT().GetStorageResourceTagInfoForModel(
		ctx, config.ResourceTagsKey,
	).Return(domainstorageprovisioning.ModelResourceTagInfo{
		ControllerUUID: "controller-uuid",
		ModelUUID:      "model-uuid",
	}, nil)

	s.filesystemSource.MockFilesystemImporter.EXPECT().ImportFilesystem(
		ctx, providerID, storageName.String(), gomock.Any(), false,
	).Return(internalstorage.FilesystemInfo{}, coreerrors.NotFound)

	svc := s.makeService()
	_, err := svc.AdoptFilesystem(ctx, storageName, poolUUID, providerID, false)
	c.Assert(err, tc.ErrorIs, domainstorageerrors.PooledStorageEntityNotFound)
}
