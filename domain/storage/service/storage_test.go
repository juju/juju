// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	modeltesting "github.com/juju/juju/core/model/testing"
	corestatus "github.com/juju/juju/core/status"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	domainstoragestate "github.com/juju/juju/domain/storage/state"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type storageSuite struct {
	testhelpers.IsolationSuite

	state              *MockState
	registry           *MockProviderRegistry
	provider           *MockProvider
	volumeSource       *MockVolumeSource
	volumeImporter     *MockVolumeImporter
	filesystemSource   *MockFilesystemSource
	filesystemImporter *MockFilesystemImporter
}

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

type volumeImporter struct {
	*MockVolumeSource
	*MockVolumeImporter
}

type filesystemImporter struct {
	*MockFilesystemSource
	*MockFilesystemImporter
}

func (s *storageSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.volumeSource = NewMockVolumeSource(ctrl)
	s.volumeImporter = NewMockVolumeImporter(ctrl)
	s.filesystemSource = NewMockFilesystemSource(ctrl)
	s.filesystemImporter = NewMockFilesystemImporter(ctrl)
	s.provider = NewMockProvider(ctrl)

	s.registry = NewMockProviderRegistry(ctrl)
	s.registry.EXPECT().StorageProvider(storage.ProviderType("ebs")).Return(s.provider, nil).AnyTimes()
	s.registry.EXPECT().StorageProvider(storage.ProviderType("elastic")).Return(s.provider, nil).AnyTimes()

	c.Cleanup(func() {
		s.state = nil
		s.volumeSource = nil
		s.volumeImporter = nil
		s.filesystemSource = nil
		s.filesystemImporter = nil
		s.provider = nil
		s.registry = nil
	})

	return ctrl
}

func (s *storageSuite) service(c *tc.C) *Service {
	return NewService(s.state, loggertesting.WrapCheckLog(c), modelStorageRegistryGetter(func() storage.ProviderRegistry {
		return s.registry
	}))
}

func (s *storageSuite) TestImportFilesystemValidate(c *tc.C) {
	c.Skip("TODO: Implement ImportFilesystem in the service")

	defer s.setupMocks(c).Finish()

	_, err := s.service(c).ImportFilesystem(c.Context(), ImportStorageParams{
		Kind:        storage.StorageKindFilesystem,
		Pool:        "elastic",
		ProviderId:  "provider-id",
		StorageName: "0",
	})
	c.Check(err, tc.ErrorIs, corestorage.InvalidStorageName)

	_, err = s.service(c).ImportFilesystem(c.Context(), ImportStorageParams{
		Kind:        storage.StorageKindFilesystem,
		Pool:        "0",
		ProviderId:  "provider-id",
		StorageName: "pgdata",
	})
	c.Check(err, tc.ErrorIs, storageerrors.InvalidPoolNameError)

	_, err = s.service(c).ImportFilesystem(c.Context(), ImportStorageParams{
		Kind:        storage.StorageKindBlock,
		Pool:        "elastic",
		ProviderId:  "provider-id",
		StorageName: "pgdata",
	})
	c.Check(err, tc.ErrorIs, errors.NotSupported)
}

func (s *storageSuite) TestImportFilesystem(c *tc.C) {
	c.Skip("TODO: Implement ImportFilesystem in the service")

	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().Supports(storage.StorageKindFilesystem).Return(true)
	cfg, err := storage.NewConfig("elastic", "elastic", nil)
	c.Assert(err, tc.ErrorIsNil)
	s.provider.EXPECT().FilesystemSource(cfg).Return(filesystemImporter{
		MockFilesystemSource:   s.filesystemSource,
		MockFilesystemImporter: s.filesystemImporter,
	}, nil)

	controllerUUID := uuid.MustNewUUID().String()
	modelUUID := modeltesting.GenModelUUID(c).String()
	// s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "elastic").Return(domainstorage.StoragePool{}, storageerrors.PoolNotFoundError)
	s.state.EXPECT().GetModelDetails().Return(domainstorage.ModelDetails{
		ModelUUID:      modelUUID,
		ControllerUUID: controllerUUID,
	}, nil)
	s.filesystemImporter.EXPECT().ImportFilesystem(gomock.Any(), "provider-id", map[string]string{
		"juju-model-uuid":      modelUUID,
		"juju-controller-uuid": controllerUUID,
	}).Return(storage.FilesystemInfo{
		ProviderId: "filesystem-id",
		Size:       123,
	}, nil)

	s.state.EXPECT().ImportFilesystem(gomock.Any(), corestorage.Name("pgdata"), domainstorage.FilesystemInfo{
		FilesystemInfo: storage.FilesystemInfo{
			ProviderId: "filesystem-id",
			Size:       123,
		},
		Pool: "elastic",
	}).Return("pgdata/0", nil)

	result, err := s.service(c).ImportFilesystem(c.Context(), ImportStorageParams{
		Kind:        storage.StorageKindFilesystem,
		Pool:        "elastic",
		ProviderId:  "provider-id",
		StorageName: "pgdata",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, corestorage.ID("pgdata/0"))
}

func (s *storageSuite) TestImportFilesystemUsingStoragePool(c *tc.C) {
	c.Skip("TODO: Implement ImportFilesystem in the service")

	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().Supports(storage.StorageKindFilesystem).Return(true)
	cfg, err := storage.NewConfig("fast-elastic", "elastic", nil)
	c.Assert(err, tc.ErrorIsNil)
	s.provider.EXPECT().FilesystemSource(cfg).Return(filesystemImporter{
		MockFilesystemSource:   s.filesystemSource,
		MockFilesystemImporter: s.filesystemImporter,
	}, nil)

	// s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "fast-elastic").Return(domainstorage.StoragePool{
	// 	Name:     "fast-elastic",
	// 	Provider: "elastic",
	// }, nil)
	controllerUUID := uuid.MustNewUUID().String()
	modelUUID := modeltesting.GenModelUUID(c).String()
	s.state.EXPECT().GetModelDetails().Return(domainstorage.ModelDetails{
		ModelUUID:      modelUUID,
		ControllerUUID: controllerUUID,
	}, nil)
	s.filesystemImporter.EXPECT().ImportFilesystem(gomock.Any(), "provider-id", map[string]string{
		"juju-model-uuid":      modelUUID,
		"juju-controller-uuid": controllerUUID,
	}).Return(storage.FilesystemInfo{
		ProviderId: "provider-id",
		Size:       123,
	}, nil)

	s.state.EXPECT().ImportFilesystem(gomock.Any(), corestorage.Name("pgdata"), domainstorage.FilesystemInfo{
		FilesystemInfo: storage.FilesystemInfo{
			ProviderId: "provider-id",
			Size:       123,
		},
		Pool: "fast-elastic",
	}).Return("pgdata/0", nil)

	result, err := s.service(c).ImportFilesystem(c.Context(), ImportStorageParams{
		Kind:        storage.StorageKindFilesystem,
		Pool:        "fast-elastic",
		ProviderId:  "provider-id",
		StorageName: "pgdata",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, corestorage.ID("pgdata/0"))
}

func (s *storageSuite) TestImportFilesystemNotSupported(c *tc.C) {
	c.Skip("TODO: Implement ImportFilesystem in the service")

	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().Supports(storage.StorageKindFilesystem).Return(true)
	cfg, err := storage.NewConfig("elastic", "elastic", nil)
	c.Assert(err, tc.ErrorIsNil)
	s.provider.EXPECT().FilesystemSource(cfg).Return(s.filesystemSource, nil)

	// s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "elastic").Return(domainstorage.StoragePool{}, storageerrors.PoolNotFoundError)
	s.state.EXPECT().GetModelDetails().Return(domainstorage.ModelDetails{
		ModelUUID:      modeltesting.GenModelUUID(c).String(),
		ControllerUUID: uuid.MustNewUUID().String(),
	}, nil)
	_, err = s.service(c).ImportFilesystem(c.Context(), ImportStorageParams{
		Kind:        storage.StorageKindFilesystem,
		Pool:        "elastic",
		ProviderId:  "provider-id",
		StorageName: "pgdata",
	})
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *storageSuite) TestImportFilesystemVolumeBacked(c *tc.C) {
	c.Skip("TODO: Implement ImportFilesystem in the service")

	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().Supports(storage.StorageKindFilesystem).Return(false)
	cfg, err := storage.NewConfig("ebs", "ebs", nil)
	c.Assert(err, tc.ErrorIsNil)
	s.provider.EXPECT().VolumeSource(cfg).Return(volumeImporter{
		MockVolumeSource:   s.volumeSource,
		MockVolumeImporter: s.volumeImporter,
	}, nil)

	controllerUUID := uuid.MustNewUUID().String()
	modelUUID := modeltesting.GenModelUUID(c).String()
	// s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "ebs").Return(domainstorage.StoragePool{}, storageerrors.PoolNotFoundError)
	s.state.EXPECT().GetModelDetails().Return(domainstorage.ModelDetails{
		ModelUUID:      modelUUID,
		ControllerUUID: controllerUUID,
	}, nil)
	s.volumeImporter.EXPECT().ImportVolume(gomock.Any(), "provider-id", "", map[string]string{
		"juju-model-uuid":      modelUUID,
		"juju-controller-uuid": controllerUUID,
	}, false).Return(storage.VolumeInfo{
		VolumeId:   "provider-id",
		HardwareId: "hw",
		WWN:        "wwn",
		Size:       123,
		Persistent: true,
	}, nil)

	s.state.EXPECT().ImportFilesystem(gomock.Any(), corestorage.Name("pgdata"), domainstorage.FilesystemInfo{
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

	result, err := s.service(c).ImportFilesystem(c.Context(), ImportStorageParams{
		Kind:        storage.StorageKindFilesystem,
		Pool:        "ebs",
		ProviderId:  "provider-id",
		StorageName: "pgdata",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, corestorage.ID("pgdata/0"))
}

func (s *storageSuite) TestImportFilesystemVolumeBackedNotSupported(c *tc.C) {
	c.Skip("TODO: Implement ImportFilesystem in the service")

	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().Supports(storage.StorageKindFilesystem).Return(false)
	cfg, err := storage.NewConfig("ebs", "ebs", nil)
	c.Assert(err, tc.ErrorIsNil)
	s.provider.EXPECT().VolumeSource(cfg).Return(s.volumeSource, nil)

	// s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "ebs").Return(domainstorage.StoragePool{}, storageerrors.PoolNotFoundError)
	s.state.EXPECT().GetModelDetails().Return(domainstorage.ModelDetails{
		ModelUUID:      modeltesting.GenModelUUID(c).String(),
		ControllerUUID: uuid.MustNewUUID().String(),
	}, nil)
	_, err = s.service(c).ImportFilesystem(c.Context(), ImportStorageParams{
		Kind:        storage.StorageKindFilesystem,
		Pool:        "ebs",
		ProviderId:  "provider-id",
		StorageName: "pgdata",
	})
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *storageSuite) TestListStorageInstances(c *tc.C) {
	defer s.setupMocks(c).Finish()

	u0n := unit.Name("mysql/0")
	u1n := unit.Name("mysql/1")
	s.state.EXPECT().ListStorageInstances(gomock.Any()).Return([]domainstorage.StorageInstanceDetails{
		{
			ID:         "pgdata-0",
			Owner:      &u0n,
			Kind:       domainstorage.StorageKindBlock,
			Life:       life.Alive,
			Persistent: true,
		},
		{
			ID:         "data-1",
			Owner:      &u1n,
			Kind:       domainstorage.StorageKindFilesystem,
			Life:       life.Alive,
			Persistent: false,
		},
	}, nil)

	result, err := s.service(c).ListStorageInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []domainstorage.StorageInstanceDetails{
		{
			ID:         "pgdata-0",
			Owner:      &u0n,
			Kind:       domainstorage.StorageKindBlock,
			Life:       life.Alive,
			Persistent: true,
		},
		{
			ID:         "data-1",
			Owner:      &u1n,
			Kind:       domainstorage.StorageKindFilesystem,
			Life:       life.Alive,
			Persistent: false,
		},
	})
}

func (s *storageSuite) TestListVolumeWithAttachments(c *tc.C) {
	defer s.setupMocks(c).Finish()

	u0n := unit.Name("mysql/0")
	m0 := machine.Name("0")
	u1n := unit.Name("mysql/1")
	s.state.EXPECT().ListVolumeWithAttachments(gomock.Any(), "pgdata-0", "data-1").
		Return(map[string]domainstoragestate.VolumeDetails{
			"pgdata-0": {
				StorageID: "pgdata-0",
				Status: status.StatusInfo[status.StorageVolumeStatusType]{
					Status:  status.StorageVolumeStatusTypeAttaching,
					Message: "attaching the volumez",
				},
				Attachments: []domainstoragestate.VolumeAttachmentDetails{
					{
						AttachmentDetails: domainstoragestate.AttachmentDetails{
							Life:    life.Alive,
							Unit:    u0n,
							Machine: &m0,
						},
						BlockDeviceUUID: "bk-uuid-1",
					},
				},
			},
			"data-1": {
				StorageID: "data-1",
				Status: status.StatusInfo[status.StorageVolumeStatusType]{
					Status:  status.StorageVolumeStatusTypeAttached,
					Message: "all good",
				},
				Attachments: []domainstoragestate.VolumeAttachmentDetails{
					{
						AttachmentDetails: domainstoragestate.AttachmentDetails{
							Life: life.Alive,
							Unit: u1n,
						},
						BlockDeviceUUID: "bk-uuid-2",
					},
				},
			},
		}, nil)

	result, err := s.service(c).ListVolumeWithAttachments(c.Context(),
		"pgdata-0",
		"data-1",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, map[string]domainstorage.VolumeDetails{
		"pgdata-0": {
			StorageID: "pgdata-0",
			Status: corestatus.StatusInfo{
				Status:  corestatus.Attaching,
				Message: "attaching the volumez",
			},
			Attachments: []domainstorage.VolumeAttachmentDetails{
				{
					AttachmentDetails: domainstorage.AttachmentDetails{
						Life:    life.Alive,
						Unit:    u0n,
						Machine: &m0,
					},
					BlockDeviceUUID: "bk-uuid-1",
				},
			},
		},
		"data-1": {
			StorageID: "data-1",
			Status: corestatus.StatusInfo{
				Status:  corestatus.Attached,
				Message: "all good",
			},
			Attachments: []domainstorage.VolumeAttachmentDetails{
				{
					AttachmentDetails: domainstorage.AttachmentDetails{
						Life: life.Alive,
						Unit: u1n,
					},
					BlockDeviceUUID: "bk-uuid-2",
				},
			},
		},
	})
}

func (s *storageSuite) TestListFilesystemWithAttachments(c *tc.C) {
	defer s.setupMocks(c).Finish()

	u0n := unit.Name("mysql/0")
	m0 := machine.Name("0")
	u1n := unit.Name("mysql/1")
	s.state.EXPECT().ListFilesystemWithAttachments(gomock.Any(), "pgdata-0", "data-1").
		Return(map[string]domainstoragestate.FilesystemDetails{
			"pgdata-0": {
				StorageID: "pgdata-0",
				Status: status.StatusInfo[status.StorageFilesystemStatusType]{
					Status:  status.StorageFilesystemStatusTypeAttaching,
					Message: "attaching the volumez",
				},
				Attachments: []domainstoragestate.FilesystemAttachmentDetails{
					{
						AttachmentDetails: domainstoragestate.AttachmentDetails{
							Life:    life.Alive,
							Unit:    u0n,
							Machine: &m0,
						},
						MountPoint: "/mnt/foo",
					},
				},
			},
			"data-1": {
				StorageID: "data-1",
				Status: status.StatusInfo[status.StorageFilesystemStatusType]{
					Status:  status.StorageFilesystemStatusTypeAttached,
					Message: "all good",
				},
				Attachments: []domainstoragestate.FilesystemAttachmentDetails{
					{
						AttachmentDetails: domainstoragestate.AttachmentDetails{
							Life: life.Alive,
							Unit: u1n,
						},
						MountPoint: "/mnt/bar",
					},
				},
			},
		}, nil)

	result, err := s.service(c).ListFilesystemWithAttachments(c.Context(),
		"pgdata-0",
		"data-1",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, map[string]domainstorage.FilesystemDetails{
		"pgdata-0": {
			StorageID: "pgdata-0",
			Status: corestatus.StatusInfo{
				Status:  corestatus.Attaching,
				Message: "attaching the volumez",
			},
			Attachments: []domainstorage.FilesystemAttachmentDetails{
				{
					AttachmentDetails: domainstorage.AttachmentDetails{
						Life:    life.Alive,
						Unit:    u0n,
						Machine: &m0,
					},
					MountPoint: "/mnt/foo",
				},
			},
		},
		"data-1": {
			StorageID: "data-1",
			Status: corestatus.StatusInfo{
				Status:  corestatus.Attached,
				Message: "all good",
			},
			Attachments: []domainstorage.FilesystemAttachmentDetails{
				{
					AttachmentDetails: domainstorage.AttachmentDetails{
						Life: life.Alive,
						Unit: u1n,
					},
					MountPoint: "/mnt/bar",
				},
			},
		},
	})
}
