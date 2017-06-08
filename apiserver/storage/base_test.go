// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/storage"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	jujustorage "github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
)

type baseStorageSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer

	api   *storage.API
	state *mockState

	storageTag      names.StorageTag
	storageInstance *mockStorageInstance
	unitTag         names.UnitTag
	machineTag      names.MachineTag

	volumeTag            names.VolumeTag
	volume               *mockVolume
	volumeAttachment     *mockVolumeAttachment
	filesystemTag        names.FilesystemTag
	filesystem           *mockFilesystem
	filesystemAttachment *mockFilesystemAttachment
	stub                 testing.Stub

	registry    jujustorage.StaticProviderRegistry
	poolManager *mockPoolManager
	pools       map[string]*jujustorage.Config

	blocks map[state.BlockType]state.Block
}

func (s *baseStorageSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.authorizer = apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin"), Controller: true}
	s.stub.ResetCalls()
	s.state = s.constructState()

	s.registry = jujustorage.StaticProviderRegistry{map[jujustorage.ProviderType]jujustorage.Provider{}}
	s.pools = make(map[string]*jujustorage.Config)
	s.poolManager = s.constructPoolManager()

	var err error
	s.api, err = storage.NewAPI(s.state, s.registry, s.poolManager, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

// TODO(axw) get rid of assertCalls, use stub directly everywhere.
func (s *baseStorageSuite) assertCalls(c *gc.C, expectedCalls []string) {
	s.stub.CheckCallNames(c, expectedCalls...)
}

const (
	allStorageInstancesCall                 = "allStorageInstances"
	storageInstanceAttachmentsCall          = "storageInstanceAttachments"
	unitAssignedMachineCall                 = "UnitAssignedMachine"
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
	getBlockForTypeCall                     = "getBlockForType"
	volumeAttachmentCall                    = "volumeAttachment"
	attachStorageCall                       = "attachStorage"
	detachStorageCall                       = "detachStorage"
	destroyStorageInstanceCall              = "destroyStorageInstance"
)

func (s *baseStorageSuite) constructState() *mockState {
	s.unitTag = names.NewUnitTag("mysql/0")
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
		VolumeTag:  s.volumeTag,
		MachineTag: s.machineTag,
		life:       state.Alive,
	}

	s.blocks = make(map[state.BlockType]state.Block)
	return &mockState{
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
		storageInstanceFilesystemAttachment: func(m names.MachineTag, f names.FilesystemTag) (state.FilesystemAttachment, error) {
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
		volumeAttachment: func(names.MachineTag, names.VolumeTag) (state.VolumeAttachment, error) {
			s.stub.AddCall(volumeAttachmentCall)
			return s.volumeAttachment, nil
		},
		unitAssignedMachine: func(u names.UnitTag) (names.MachineTag, error) {
			s.stub.AddCall(unitAssignedMachineCall)
			if u == s.unitTag {
				return s.machineTag, nil
			}
			return names.MachineTag{}, errors.NotFoundf("%s", names.ReadableString(u))
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
		modelName: "storagetest",
		addStorageForUnit: func(u names.UnitTag, name string, cons state.StorageConstraints) error {
			s.stub.AddCall(addStorageForUnitCall)
			return nil
		},
		getBlockForType: func(t state.BlockType) (state.Block, bool, error) {
			s.stub.AddCall(getBlockForTypeCall, t)
			val, found := s.blocks[t]
			return val, found, nil
		},
		detachStorage: func(storage names.StorageTag, unit names.UnitTag) error {
			s.stub.AddCall(detachStorageCall, storage, unit)
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
		destroyStorageInstance: func(tag names.StorageTag) error {
			s.stub.AddCall(destroyStorageInstanceCall)
			return errors.New("cannae do it")
		},
	}
}

func (s *baseStorageSuite) addBlock(c *gc.C, t state.BlockType, msg string) {
	s.blocks[t] = mockBlock{
		t:   t,
		msg: msg,
	}
}

func (s *baseStorageSuite) blockAllChanges(c *gc.C, msg string) {
	s.addBlock(c, state.ChangeBlock, msg)
}

func (s *baseStorageSuite) blockDestroyEnvironment(c *gc.C, msg string) {
	s.addBlock(c, state.DestroyBlock, msg)
}

func (s *baseStorageSuite) blockRemoveObject(c *gc.C, msg string) {
	s.addBlock(c, state.RemoveBlock, msg)
}

func (s *baseStorageSuite) assertBlocked(c *gc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, msg)
}

func (s *baseStorageSuite) constructPoolManager() *mockPoolManager {
	return &mockPoolManager{
		getPool: func(name string) (*jujustorage.Config, error) {
			if one, ok := s.pools[name]; ok {
				return one, nil
			}
			return nil, errors.NotFoundf("mock pool manager: get pool %v", name)
		},
		createPool: func(name string, providerType jujustorage.ProviderType, attrs map[string]interface{}) (*jujustorage.Config, error) {
			pool, err := jujustorage.NewConfig(name, providerType, attrs)
			s.pools[name] = pool
			return pool, err
		},
		deletePool: func(name string) error {
			delete(s.pools, name)
			return nil
		},
		listPools: func() ([]*jujustorage.Config, error) {
			result := make([]*jujustorage.Config, len(s.pools))
			i := 0
			for _, v := range s.pools {
				result[i] = v
				i++
			}
			return result, nil
		},
	}
}
