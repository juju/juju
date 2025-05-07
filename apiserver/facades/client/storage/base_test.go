// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/storage"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/blockcommand"
	jujustorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type baseStorageSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer

	controllerUUID string
	modelUUID      coremodel.UUID

	api                 *storage.StorageAPI
	apiCaas             *storage.StorageAPI
	storageAccessor     *mockStorageAccessor
	blockDeviceGetter   *mockBlockDeviceGetter
	blockCommandService *storage.MockBlockCommandService

	storageTag      names.StorageTag
	storageInstance *mockStorageInstance
	unitTag         names.UnitTag
	machineTag      names.MachineTag

	volumeTag            names.VolumeTag
	volume               *mockVolume
	volumeAttachment     *mockVolumeAttachment
	volumeAttachmentPlan *mockVolumeAttachmentPlan
	filesystemTag        names.FilesystemTag
	filesystem           *mockFilesystem
	filesystemAttachment *mockFilesystemAttachment
	stub                 testhelpers.Stub

	storageService     *storage.MockStorageService
	applicationService *storage.MockApplicationService
	registry           jujustorage.StaticProviderRegistry
	poolsInUse         []string
}

func (s *baseStorageSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.unitTag = names.NewUnitTag("mysql/0")
	s.machineTag = names.NewMachineTag("1234")

	s.resources = common.NewResources()
	s.authorizer = apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin"), Controller: true}
	s.stub.ResetCalls()
	s.storageAccessor = s.constructStorageAccessor()
	s.blockDeviceGetter = &mockBlockDeviceGetter{}

	s.blockCommandService = storage.NewMockBlockCommandService(ctrl)
	s.storageService = storage.NewMockStorageService(ctrl)
	s.applicationService = storage.NewMockApplicationService(ctrl)
	s.applicationService.EXPECT().GetUnitMachineName(gomock.Any(), unit.Name("mysql/0")).DoAndReturn(func(ctx context.Context, u unit.Name) (machine.Name, error) {
		c.Assert(u.String(), tc.Equals, s.unitTag.Id())
		return machine.Name(s.machineTag.Id()), nil
	}).AnyTimes()

	s.registry = jujustorage.StaticProviderRegistry{Providers: map[jujustorage.ProviderType]jujustorage.Provider{}}
	s.poolsInUse = []string{}

	s.controllerUUID = uuid.MustNewUUID().String()
	s.modelUUID = modeltesting.GenModelUUID(c)
	s.api = storage.NewStorageAPI(
		s.controllerUUID, s.modelUUID, coremodel.IAAS,
		s.storageAccessor, s.blockDeviceGetter,
		s.storageService, s.applicationService, s.storageRegistryGetter,
		s.authorizer, s.blockCommandService)
	s.apiCaas = storage.NewStorageAPI(
		s.controllerUUID, s.modelUUID, coremodel.CAAS,
		s.storageAccessor, s.blockDeviceGetter,
		s.storageService, s.applicationService, s.storageRegistryGetter,
		s.authorizer, s.blockCommandService)

	return ctrl
}

func (s *baseStorageSuite) storageRegistryGetter(context.Context) (jujustorage.ProviderRegistry, error) {
	return s.registry, nil
}

// TODO(axw) get rid of assertCalls, use stub directly everywhere.
func (s *baseStorageSuite) assertCalls(c *tc.C, expectedCalls []string) {
	s.stub.CheckCallNames(c, expectedCalls...)
}

const (
	allStorageInstancesCall                 = "allStorageInstances"
	storageInstanceAttachmentsCall          = "storageInstanceAttachments"
	storageInstanceCall                     = "StorageInstance"
	storageInstanceFilesystemCall           = "StorageInstanceFilesystem"
	storageInstanceFilesystemAttachmentCall = "storageInstanceFilesystemAttachment"
	storageInstanceVolumeCall               = "storageInstanceVolume"
	volumeCall                              = "volumeCall"
	machineVolumeAttachmentsCall            = "machineVolumeAttachments"
	volumeAttachmentsCall                   = "volumeAttachments"
	allVolumesCall                          = "allVolumes"
	filesystemCall                          = "filesystemCall"
	machineFilesystemAttachmentsCall        = "machineFilesystemAttachments"
	filesystemAttachmentsCall               = "filesystemAttachments"
	allFilesystemsCall                      = "allFilesystems"
	addStorageForUnitCall                   = "addStorageForUnit"
	volumeAttachmentCall                    = "volumeAttachment"
	volumeAttachmentPlanCall                = "volumeAttachmentPlan"
	volumeAttachmentPlansCall               = "volumeAttachmentPlans"
	attachStorageCall                       = "attachStorage"
	detachStorageCall                       = "detachStorage"
	destroyStorageInstanceCall              = "destroyStorageInstance"
	releaseStorageInstanceCall              = "releaseStorageInstance"
	addExistingFilesystemCall               = "addExistingFilesystem"
)

func (s *baseStorageSuite) constructStorageAccessor() *mockStorageAccessor {
	s.storageTag = names.NewStorageTag("data/0")

	s.storageInstance = &mockStorageInstance{
		kind:       state.StorageKindFilesystem,
		owner:      s.unitTag,
		storageTag: s.storageTag,
		life:       state.Dying,
	}

	storageInstanceAttachment := &mockStorageAttachment{
		storage: s.storageInstance,
		life:    state.Alive,
	}

	s.machineTag = names.NewMachineTag("66")
	s.filesystemTag = names.NewFilesystemTag("104")
	s.volumeTag = names.NewVolumeTag("22")
	s.filesystem = &mockFilesystem{
		tag:     s.filesystemTag,
		storage: &s.storageTag,
		life:    state.Alive,
	}
	s.filesystemAttachment = &mockFilesystemAttachment{
		filesystem: s.filesystemTag,
		machine:    s.machineTag,
		life:       state.Dead,
	}
	s.volume = &mockVolume{tag: s.volumeTag, storage: &s.storageTag}
	s.volumeAttachment = &mockVolumeAttachment{
		VolumeTag: s.volumeTag,
		HostTag:   s.machineTag,
		life:      state.Alive,
	}

	s.volumeAttachmentPlan = &mockVolumeAttachmentPlan{
		VolumeTag: s.volumeTag,
		HostTag:   s.machineTag,
		life:      state.Alive,
		info:      &state.VolumeAttachmentPlanInfo{},
		blk:       &state.BlockDeviceInfo{},
	}

	return &mockStorageAccessor{
		allStorageInstances: func() ([]state.StorageInstance, error) {
			s.stub.AddCall(allStorageInstancesCall)
			return []state.StorageInstance{s.storageInstance}, nil
		},
		storageInstance: func(sTag names.StorageTag) (state.StorageInstance, error) {
			s.stub.AddCall(storageInstanceCall, sTag)
			if sTag == s.storageTag {
				return s.storageInstance, nil
			}
			return nil, errors.NotFoundf("%s", names.ReadableString(sTag))
		},
		storageInstanceAttachments: func(tag names.StorageTag) ([]state.StorageAttachment, error) {
			s.stub.AddCall(storageInstanceAttachmentsCall, tag)
			if tag == s.storageTag {
				return []state.StorageAttachment{storageInstanceAttachment}, nil
			}
			return []state.StorageAttachment{}, nil
		},
		storageInstanceFilesystem: func(sTag names.StorageTag) (state.Filesystem, error) {
			s.stub.AddCall(storageInstanceFilesystemCall)
			if sTag == s.storageTag {
				return s.filesystem, nil
			}
			return nil, errors.NotFoundf("%s", names.ReadableString(sTag))
		},
		storageInstanceFilesystemAttachment: func(m names.Tag, f names.FilesystemTag) (state.FilesystemAttachment, error) {
			s.stub.AddCall(storageInstanceFilesystemAttachmentCall)
			if m == s.machineTag && f == s.filesystemTag {
				return s.filesystemAttachment, nil
			}
			return nil, errors.NotFoundf("filesystem attachment %s:%s", m, f)
		},
		storageInstanceVolume: func(t names.StorageTag) (state.Volume, error) {
			s.stub.AddCall(storageInstanceVolumeCall)
			if t == s.storageTag {
				return s.volume, nil
			}
			return nil, errors.NotFoundf("%s", names.ReadableString(t))
		},
		volumeAttachment: func(names.Tag, names.VolumeTag) (state.VolumeAttachment, error) {
			s.stub.AddCall(volumeAttachmentCall)
			return s.volumeAttachment, nil
		},
		volumeAttachmentPlan: func(names.Tag, names.VolumeTag) (state.VolumeAttachmentPlan, error) {
			s.stub.AddCall(volumeAttachmentPlanCall)
			return s.volumeAttachmentPlan, nil
		},
		volumeAttachmentPlans: func(names.VolumeTag) ([]state.VolumeAttachmentPlan, error) {
			s.stub.AddCall(volumeAttachmentPlansCall)
			return []state.VolumeAttachmentPlan{s.volumeAttachmentPlan}, nil
		},
		volume: func(tag names.VolumeTag) (state.Volume, error) {
			s.stub.AddCall(volumeCall)
			if tag == s.volumeTag {
				return s.volume, nil
			}
			return nil, errors.NotFoundf("%s", names.ReadableString(tag))
		},
		machineVolumeAttachments: func(machine names.MachineTag) ([]state.VolumeAttachment, error) {
			s.stub.AddCall(machineVolumeAttachmentsCall)
			if machine == s.machineTag {
				return []state.VolumeAttachment{s.volumeAttachment}, nil
			}
			return nil, nil
		},
		volumeAttachments: func(volume names.VolumeTag) ([]state.VolumeAttachment, error) {
			s.stub.AddCall(volumeAttachmentsCall)
			if volume == s.volumeTag {
				return []state.VolumeAttachment{s.volumeAttachment}, nil
			}
			return nil, nil
		},
		allVolumes: func() ([]state.Volume, error) {
			s.stub.AddCall(allVolumesCall)
			return []state.Volume{s.volume}, nil
		},
		filesystem: func(tag names.FilesystemTag) (state.Filesystem, error) {
			s.stub.AddCall(filesystemCall)
			if tag == s.filesystemTag {
				return s.filesystem, nil
			}
			return nil, errors.NotFoundf("%s", names.ReadableString(tag))
		},
		machineFilesystemAttachments: func(machine names.MachineTag) ([]state.FilesystemAttachment, error) {
			s.stub.AddCall(machineFilesystemAttachmentsCall)
			if machine == s.machineTag {
				return []state.FilesystemAttachment{s.filesystemAttachment}, nil
			}
			return nil, nil
		},
		filesystemAttachments: func(filesystem names.FilesystemTag) ([]state.FilesystemAttachment, error) {
			s.stub.AddCall(filesystemAttachmentsCall)
			if filesystem == s.filesystemTag {
				return []state.FilesystemAttachment{s.filesystemAttachment}, nil
			}
			return nil, nil
		},
		allFilesystems: func() ([]state.Filesystem, error) {
			s.stub.AddCall(allFilesystemsCall)
			return []state.Filesystem{s.filesystem}, nil
		},
		addStorageForUnit: func(u names.UnitTag, name string, cons state.StorageConstraints) ([]names.StorageTag, error) {
			s.stub.AddCall(addStorageForUnitCall)
			return nil, nil
		},
		detachStorage: func(storage names.StorageTag, unit names.UnitTag, force bool) error {
			s.stub.AddCall(detachStorageCall, storage, unit, force)
			if storage == s.storageTag && unit == s.unitTag {
				return nil
			}
			return errors.NotFoundf(
				"attachment of %s to %s",
				names.ReadableString(storage),
				names.ReadableString(unit),
			)
		},
		attachStorage: func(storage names.StorageTag, unit names.UnitTag) error {
			s.stub.AddCall(attachStorageCall, storage, unit)
			if storage == s.storageTag && unit == s.unitTag {
				return nil
			}
			return errors.Errorf(
				"cannot attach %s to %s",
				names.ReadableString(storage),
				names.ReadableString(unit),
			)
		},
		destroyStorageInstance: func(tag names.StorageTag, destroyAttached bool, force bool) error {
			s.stub.AddCall(destroyStorageInstanceCall, tag, destroyAttached, force)
			return errors.New("cannae do it")
		},
		releaseStorageInstance: func(tag names.StorageTag, destroyAttached bool, force bool) error {
			s.stub.AddCall(releaseStorageInstanceCall, tag, destroyAttached, force)
			return errors.New("cannae do it")
		},
		addExistingFilesystem: func(f state.FilesystemInfo, v *state.VolumeInfo, storageName string) (names.StorageTag, error) {
			s.stub.AddCall(addExistingFilesystemCall, f, v, storageName)
			return s.storageTag, s.stub.NextErr()
		},
	}
}

func (s *baseStorageSuite) addBlock(c *tc.C, t blockcommand.BlockType, msg string) {
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), t).Return(msg, nil)
}

func (s *baseStorageSuite) blockAllChanges(c *tc.C, msg string) {
	s.addBlock(c, blockcommand.ChangeBlock, msg)
}

func (s *baseStorageSuite) assertBlocked(c *tc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), tc.IsTrue)
	c.Assert(err, tc.ErrorMatches, msg)
}
