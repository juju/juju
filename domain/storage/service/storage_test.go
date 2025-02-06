// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/environs/envcontext"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	coretesting "github.com/juju/juju/internal/testing"
)

type storageSuite struct {
	testing.IsolationSuite

	state              *MockState
	registry           storage.ProviderRegistry
	provider           *MockProvider
	volumeSource       *MockVolumeSource
	volumeImporter     *MockVolumeImporter
	filesystemSource   *MockFilesystemSource
	filesystemImporter *MockFilesystemImporter
}

var _ = gc.Suite(&storageSuite{})

type volumeImporter struct {
	*MockVolumeSource
	*MockVolumeImporter
}

type filesystemImporter struct {
	*MockFilesystemSource
	*MockFilesystemImporter
}

func (s *storageSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.volumeSource = NewMockVolumeSource(ctrl)
	s.volumeImporter = NewMockVolumeImporter(ctrl)
	s.filesystemSource = NewMockFilesystemSource(ctrl)
	s.filesystemImporter = NewMockFilesystemImporter(ctrl)
	s.provider = NewMockProvider(ctrl)

	registry := NewMockProviderRegistry(ctrl)
	registry.EXPECT().StorageProvider(storage.ProviderType("ebs")).Return(s.provider, nil).AnyTimes()
	registry.EXPECT().StorageProvider(storage.ProviderType("elastic")).Return(s.provider, nil).AnyTimes()

	s.registry = storage.ChainedProviderRegistry{registry, provider.CommonStorageProviders()}

	return ctrl
}

func (s *storageSuite) service(c *gc.C) *Service {
	return NewService(s.state, loggertesting.WrapCheckLog(c), modelStorageRegistryGetter(func() storage.ProviderRegistry {
		return s.registry
	}))
}

func noopModelCredentialInvalidatorGetter() (envcontext.ModelCredentialInvalidatorFunc, error) {
	return func(context.Context, string) error {
		return nil
	}, nil
}

func (s *storageSuite) TestImportFilesystem(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().Supports(storage.StorageKindFilesystem).Return(true)
	cfg, err := storage.NewConfig("elastic", "elastic", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.provider.EXPECT().FilesystemSource(cfg).Return(filesystemImporter{
		MockFilesystemSource:   s.filesystemSource,
		MockFilesystemImporter: s.filesystemImporter,
	}, nil)

	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "elastic").Return(domainstorage.StoragePoolDetails{}, storageerrors.PoolNotFoundError)
	s.state.EXPECT().GetModelDetails().Return(domainstorage.ModelDetails{
		ModelUUID:      coretesting.ModelTag.Id(),
		ControllerUUID: coretesting.ControllerTag.Id(),
	}, nil)
	s.filesystemImporter.EXPECT().ImportFilesystem(gomock.Any(), "provider-id", map[string]string{
		"juju-model-uuid":      coretesting.ModelTag.Id(),
		"juju-controller-uuid": coretesting.ControllerTag.Id(),
	}).Return(storage.FilesystemInfo{
		FilesystemId: "provider-id",
		Size:         123,
	}, nil)

	s.state.EXPECT().ImportFilesystem(gomock.Any(), storage.Name("pgdata"), domainstorage.FilesystemInfo{
		FilesystemInfo: storage.FilesystemInfo{
			FilesystemId: "provider-id",
			Size:         123,
		},
		Pool: "elastic",
	}).Return("pgdata/0", nil)

	result, err := s.service(c).ImportFilesystem(context.Background(), noopModelCredentialInvalidatorGetter, ImportStorageParams{
		Kind:        storage.StorageKindFilesystem,
		Pool:        "elastic",
		ProviderId:  "provider-id",
		StorageName: "pgdata",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, storage.ID("pgdata/0"))
}

func (s *storageSuite) TestImportFilesystemUsingStoragePool(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().Supports(storage.StorageKindFilesystem).Return(true)
	cfg, err := storage.NewConfig("fast-elastic", "elastic", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.provider.EXPECT().FilesystemSource(cfg).Return(filesystemImporter{
		MockFilesystemSource:   s.filesystemSource,
		MockFilesystemImporter: s.filesystemImporter,
	}, nil)

	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "fast-elastic").Return(domainstorage.StoragePoolDetails{
		Name:     "fast-elastic",
		Provider: "elastic",
	}, nil)
	s.state.EXPECT().GetModelDetails().Return(domainstorage.ModelDetails{
		ModelUUID:      coretesting.ModelTag.Id(),
		ControllerUUID: coretesting.ControllerTag.Id(),
	}, nil)
	s.filesystemImporter.EXPECT().ImportFilesystem(gomock.Any(), "provider-id", map[string]string{
		"juju-model-uuid":      coretesting.ModelTag.Id(),
		"juju-controller-uuid": coretesting.ControllerTag.Id(),
	}).Return(storage.FilesystemInfo{
		FilesystemId: "provider-id",
		Size:         123,
	}, nil)

	s.state.EXPECT().ImportFilesystem(gomock.Any(), storage.Name("pgdata"), domainstorage.FilesystemInfo{
		FilesystemInfo: storage.FilesystemInfo{
			FilesystemId: "provider-id",
			Size:         123,
		},
		Pool: "fast-elastic",
	}).Return("pgdata/0", nil)

	result, err := s.service(c).ImportFilesystem(context.Background(), noopModelCredentialInvalidatorGetter, ImportStorageParams{
		Kind:        storage.StorageKindFilesystem,
		Pool:        "fast-elastic",
		ProviderId:  "provider-id",
		StorageName: "pgdata",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, storage.ID("pgdata/0"))
}

func (s *storageSuite) TestImportFilesystemNotSupported(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().Supports(storage.StorageKindFilesystem).Return(true)
	cfg, err := storage.NewConfig("elastic", "elastic", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.provider.EXPECT().FilesystemSource(cfg).Return(s.filesystemSource, nil)

	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "elastic").Return(domainstorage.StoragePoolDetails{}, storageerrors.PoolNotFoundError)
	s.state.EXPECT().GetModelDetails().Return(domainstorage.ModelDetails{
		ModelUUID:      coretesting.ModelTag.Id(),
		ControllerUUID: coretesting.ControllerTag.Id(),
	}, nil)
	_, err = s.service(c).ImportFilesystem(context.Background(), noopModelCredentialInvalidatorGetter, ImportStorageParams{
		Kind:        storage.StorageKindFilesystem,
		Pool:        "elastic",
		ProviderId:  "provider-id",
		StorageName: "pgdata",
	})
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

func (s *storageSuite) TestImportFilesystemVolumeBacked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().Supports(storage.StorageKindFilesystem).Return(false)
	cfg, err := storage.NewConfig("ebs", "ebs", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.provider.EXPECT().VolumeSource(cfg).Return(volumeImporter{
		MockVolumeSource:   s.volumeSource,
		MockVolumeImporter: s.volumeImporter,
	}, nil)

	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "ebs").Return(domainstorage.StoragePoolDetails{}, storageerrors.PoolNotFoundError)
	s.state.EXPECT().GetModelDetails().Return(domainstorage.ModelDetails{
		ModelUUID:      coretesting.ModelTag.Id(),
		ControllerUUID: coretesting.ControllerTag.Id(),
	}, nil)
	s.volumeImporter.EXPECT().ImportVolume(gomock.Any(), "provider-id", map[string]string{
		"juju-model-uuid":      coretesting.ModelTag.Id(),
		"juju-controller-uuid": coretesting.ControllerTag.Id(),
	}).Return(storage.VolumeInfo{
		VolumeId:   "provider-id",
		HardwareId: "hw",
		WWN:        "wwn",
		Size:       123,
		Persistent: true,
	}, nil)

	s.state.EXPECT().ImportFilesystem(gomock.Any(), storage.Name("pgdata"), domainstorage.FilesystemInfo{
		FilesystemInfo: storage.FilesystemInfo{
			Size: 123,
		},
		Pool: "ebs",
		BackingVolume: &storage.VolumeInfo{
			VolumeId:   "provider-id",
			HardwareId: "hw",
			WWN:        "wwn",
			Size:       123,
			Persistent: true,
		},
	}).Return("pgdata/0", nil)

	result, err := s.service(c).ImportFilesystem(context.Background(), noopModelCredentialInvalidatorGetter, ImportStorageParams{
		Kind:        storage.StorageKindFilesystem,
		Pool:        "ebs",
		ProviderId:  "provider-id",
		StorageName: "pgdata",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, storage.ID("pgdata/0"))
}

func (s *storageSuite) TestImportFilesystemVolumeBackedNotSupported(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().Supports(storage.StorageKindFilesystem).Return(false)
	cfg, err := storage.NewConfig("ebs", "ebs", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.provider.EXPECT().VolumeSource(cfg).Return(s.volumeSource, nil)

	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "ebs").Return(domainstorage.StoragePoolDetails{}, storageerrors.PoolNotFoundError)
	s.state.EXPECT().GetModelDetails().Return(domainstorage.ModelDetails{
		ModelUUID:      coretesting.ModelTag.Id(),
		ControllerUUID: coretesting.ControllerTag.Id(),
	}, nil)
	_, err = s.service(c).ImportFilesystem(context.Background(), noopModelCredentialInvalidatorGetter, ImportStorageParams{
		Kind:        storage.StorageKindFilesystem,
		Pool:        "ebs",
		ProviderId:  "provider-id",
		StorageName: "pgdata",
	})
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}
