// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/storage"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	jujustorage "github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

type baseStorageSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer testing.FakeAuthorizer

	api   *storage.API
	state *mockState

	storageTag      names.StorageTag
	storageInstance *mockStorageInstance
	unitTag         names.UnitTag
	machineTag      names.MachineTag

	volumeTag        names.VolumeTag
	volume           state.Volume
	volumeAttachment state.VolumeAttachment
	calls            []string

	poolManager *mockPoolManager
	pools       map[string]*jujustorage.Config

	blocks map[state.BlockType]state.Block
}

func (s *baseStorageSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.authorizer = testing.FakeAuthorizer{names.NewUserTag("testuser"), true}
	s.calls = []string{}
	s.state = s.constructState(c)

	s.pools = make(map[string]*jujustorage.Config)
	s.poolManager = s.constructPoolManager(c)

	var err error
	s.api, err = storage.CreateAPI(s.state, s.poolManager, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *baseStorageSuite) assertCalls(c *gc.C, expectedCalls []string) {
	c.Assert(s.calls, jc.SameContents, expectedCalls)
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
	addStorageForUnitCall                   = "addStorageForUnit"
	getBlockForTypeCall                     = "getBlockForType"
)

func (s *baseStorageSuite) constructState(c *gc.C) *mockState {
	s.unitTag = names.NewUnitTag("mysql/0")
	s.storageTag = names.NewStorageTag("data/0")

	s.storageInstance = &mockStorageInstance{
		kind:       state.StorageKindFilesystem,
		owner:      s.unitTag,
		storageTag: s.storageTag,
	}

	storageInstanceAttachment := &mockStorageAttachment{storage: s.storageInstance}

	s.machineTag = names.NewMachineTag("66")
	filesystemTag := names.NewFilesystemTag("104")
	s.volumeTag = names.NewVolumeTag("22")
	filesystem := &mockFilesystem{tag: filesystemTag}
	filesystemAttachment := &mockFilesystemAttachment{}
	s.volume = &mockVolume{tag: s.volumeTag, storage: s.storageTag}
	s.volumeAttachment = &mockVolumeAttachment{
		VolumeTag:  s.volumeTag,
		MachineTag: s.machineTag,
	}

	s.blocks = make(map[state.BlockType]state.Block)
	return &mockState{
		allStorageInstances: func() ([]state.StorageInstance, error) {
			s.calls = append(s.calls, allStorageInstancesCall)
			return []state.StorageInstance{s.storageInstance}, nil
		},
		storageInstance: func(sTag names.StorageTag) (state.StorageInstance, error) {
			s.calls = append(s.calls, storageInstanceCall)
			c.Assert(sTag, gc.DeepEquals, s.storageTag)
			return s.storageInstance, nil
		},
		storageInstanceAttachments: func(tag names.StorageTag) ([]state.StorageAttachment, error) {
			s.calls = append(s.calls, storageInstanceAttachmentsCall)
			c.Assert(tag, gc.DeepEquals, s.storageTag)
			return []state.StorageAttachment{storageInstanceAttachment}, nil
		},
		storageInstanceFilesystem: func(sTag names.StorageTag) (state.Filesystem, error) {
			s.calls = append(s.calls, storageInstanceFilesystemCall)
			c.Assert(sTag, gc.DeepEquals, s.storageTag)
			return filesystem, nil
		},
		storageInstanceFilesystemAttachment: func(m names.MachineTag, f names.FilesystemTag) (state.FilesystemAttachment, error) {
			s.calls = append(s.calls, storageInstanceFilesystemAttachmentCall)
			c.Assert(m, gc.DeepEquals, s.machineTag)
			c.Assert(f, gc.DeepEquals, filesystemTag)
			return filesystemAttachment, nil
		},
		storageInstanceVolume: func(t names.StorageTag) (state.Volume, error) {
			s.calls = append(s.calls, storageInstanceVolumeCall)
			c.Assert(t, gc.DeepEquals, s.storageTag)
			return s.volume, nil
		},
		unitAssignedMachine: func(u names.UnitTag) (names.MachineTag, error) {
			s.calls = append(s.calls, unitAssignedMachineCall)
			c.Assert(u, gc.DeepEquals, s.unitTag)
			return s.machineTag, nil
		},
		volume: func(tag names.VolumeTag) (state.Volume, error) {
			s.calls = append(s.calls, volumeCall)
			c.Assert(tag, gc.DeepEquals, s.volumeTag)
			return s.volume, nil
		},
		machineVolumeAttachments: func(machine names.MachineTag) ([]state.VolumeAttachment, error) {
			s.calls = append(s.calls, machineVolumeAttachmentsCall)
			c.Assert(machine, gc.DeepEquals, s.machineTag)
			return []state.VolumeAttachment{s.volumeAttachment}, nil
		},
		volumeAttachments: func(volume names.VolumeTag) ([]state.VolumeAttachment, error) {
			s.calls = append(s.calls, volumeAttachmentsCall)
			c.Assert(volume, gc.DeepEquals, s.volumeTag)
			return []state.VolumeAttachment{s.volumeAttachment}, nil
		},
		allVolumes: func() ([]state.Volume, error) {
			s.calls = append(s.calls, allVolumesCall)
			return []state.Volume{s.volume}, nil
		},
		envName: "storagetest",
		addStorageForUnit: func(u names.UnitTag, name string, cons state.StorageConstraints) error {
			s.calls = append(s.calls, addStorageForUnitCall)
			return nil
		},
		getBlockForType: func(t state.BlockType) (state.Block, bool, error) {
			s.calls = append(s.calls, getBlockForTypeCall)
			val, found := s.blocks[t]
			return val, found, nil
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

func (s *baseStorageSuite) constructPoolManager(c *gc.C) *mockPoolManager {
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

type mockPoolManager struct {
	getPool    func(name string) (*jujustorage.Config, error)
	createPool func(name string, providerType jujustorage.ProviderType, attrs map[string]interface{}) (*jujustorage.Config, error)
	deletePool func(name string) error
	listPools  func() ([]*jujustorage.Config, error)
}

func (m *mockPoolManager) Get(name string) (*jujustorage.Config, error) {
	return m.getPool(name)
}

func (m *mockPoolManager) Create(name string, providerType jujustorage.ProviderType, attrs map[string]interface{}) (*jujustorage.Config, error) {
	return m.createPool(name, providerType, attrs)
}

func (m *mockPoolManager) Delete(name string) error {
	return m.deletePool(name)
}

func (m *mockPoolManager) List() ([]*jujustorage.Config, error) {
	return m.listPools()
}

type mockState struct {
	storageInstance                     func(names.StorageTag) (state.StorageInstance, error)
	allStorageInstances                 func() ([]state.StorageInstance, error)
	storageInstanceAttachments          func(names.StorageTag) ([]state.StorageAttachment, error)
	unitAssignedMachine                 func(u names.UnitTag) (names.MachineTag, error)
	storageInstanceVolume               func(names.StorageTag) (state.Volume, error)
	storageInstanceVolumeAttachment     func(names.MachineTag, names.VolumeTag) (state.VolumeAttachment, error)
	storageInstanceFilesystem           func(names.StorageTag) (state.Filesystem, error)
	storageInstanceFilesystemAttachment func(m names.MachineTag, f names.FilesystemTag) (state.FilesystemAttachment, error)
	watchStorageAttachment              func(names.StorageTag, names.UnitTag) state.NotifyWatcher
	watchFilesystemAttachment           func(names.MachineTag, names.FilesystemTag) state.NotifyWatcher
	watchVolumeAttachment               func(names.MachineTag, names.VolumeTag) state.NotifyWatcher
	envName                             string
	volume                              func(tag names.VolumeTag) (state.Volume, error)
	machineVolumeAttachments            func(machine names.MachineTag) ([]state.VolumeAttachment, error)
	volumeAttachments                   func(volume names.VolumeTag) ([]state.VolumeAttachment, error)
	allVolumes                          func() ([]state.Volume, error)
	addStorageForUnit                   func(u names.UnitTag, name string, cons state.StorageConstraints) error
	getBlockForType                     func(t state.BlockType) (state.Block, bool, error)
}

func (st *mockState) StorageInstance(s names.StorageTag) (state.StorageInstance, error) {
	return st.storageInstance(s)
}

func (st *mockState) AllStorageInstances() ([]state.StorageInstance, error) {
	return st.allStorageInstances()
}

func (st *mockState) StorageAttachments(tag names.StorageTag) ([]state.StorageAttachment, error) {
	return st.storageInstanceAttachments(tag)
}

func (st *mockState) UnitAssignedMachine(unit names.UnitTag) (names.MachineTag, error) {
	return st.unitAssignedMachine(unit)
}

func (st *mockState) FilesystemAttachment(m names.MachineTag, f names.FilesystemTag) (state.FilesystemAttachment, error) {
	return st.storageInstanceFilesystemAttachment(m, f)
}

func (st *mockState) StorageInstanceFilesystem(s names.StorageTag) (state.Filesystem, error) {
	return st.storageInstanceFilesystem(s)
}

func (st *mockState) StorageInstanceVolume(s names.StorageTag) (state.Volume, error) {
	return st.storageInstanceVolume(s)
}

func (st *mockState) VolumeAttachment(m names.MachineTag, v names.VolumeTag) (state.VolumeAttachment, error) {
	return st.storageInstanceVolumeAttachment(m, v)
}

func (st *mockState) WatchStorageAttachment(s names.StorageTag, u names.UnitTag) state.NotifyWatcher {
	return st.watchStorageAttachment(s, u)
}

func (st *mockState) WatchFilesystemAttachment(mtag names.MachineTag, f names.FilesystemTag) state.NotifyWatcher {
	return st.watchFilesystemAttachment(mtag, f)
}

func (st *mockState) WatchVolumeAttachment(mtag names.MachineTag, v names.VolumeTag) state.NotifyWatcher {
	return st.watchVolumeAttachment(mtag, v)
}

func (st *mockState) EnvName() (string, error) {
	return st.envName, nil
}

func (st *mockState) AllVolumes() ([]state.Volume, error) {
	return st.allVolumes()
}

func (st *mockState) VolumeAttachments(volume names.VolumeTag) ([]state.VolumeAttachment, error) {
	return st.volumeAttachments(volume)
}

func (st *mockState) MachineVolumeAttachments(machine names.MachineTag) ([]state.VolumeAttachment, error) {
	return st.machineVolumeAttachments(machine)
}

func (st *mockState) Volume(tag names.VolumeTag) (state.Volume, error) {
	return st.volume(tag)
}

func (st *mockState) AddStorageForUnit(u names.UnitTag, name string, cons state.StorageConstraints) error {
	return st.addStorageForUnit(u, name, cons)
}

func (st *mockState) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	return st.getBlockForType(t)
}

type mockNotifyWatcher struct {
	state.NotifyWatcher
	changes chan struct{}
}

func (m *mockNotifyWatcher) Changes() <-chan struct{} {
	return m.changes
}

type mockVolume struct {
	state.Volume
	tag          names.VolumeTag
	storage      names.StorageTag
	hasNoStorage bool
}

func (m *mockVolume) StorageInstance() (names.StorageTag, error) {
	if m.hasNoStorage {
		return names.StorageTag{}, errors.NewNotAssigned(nil, "error from mock")
	}
	return m.storage, nil
}

func (m *mockVolume) VolumeTag() names.VolumeTag {
	return m.tag
}

func (m *mockVolume) Params() (state.VolumeParams, bool) {
	return state.VolumeParams{
		Pool: "loop",
		Size: 1024,
	}, true
}

func (m *mockVolume) Info() (state.VolumeInfo, error) {
	return state.VolumeInfo{}, errors.NotProvisionedf("%v", m.tag)
}

func (m *mockVolume) Status() (state.StatusInfo, error) {
	return state.StatusInfo{Status: state.StatusAttached}, nil
}

type mockFilesystem struct {
	state.Filesystem
	tag names.FilesystemTag
}

func (m *mockFilesystem) FilesystemTag() names.FilesystemTag {
	return m.tag
}

type mockFilesystemAttachment struct {
	state.FilesystemAttachment
	tag names.FilesystemTag
}

func (m *mockFilesystemAttachment) Filesystem() names.FilesystemTag {
	return m.tag
}

func (m *mockFilesystemAttachment) Info() (state.FilesystemAttachmentInfo, error) {
	return state.FilesystemAttachmentInfo{}, nil
}

type mockStorageInstance struct {
	state.StorageInstance
	kind       state.StorageKind
	owner      names.Tag
	storageTag names.Tag
}

func (m *mockStorageInstance) Kind() state.StorageKind {
	return m.kind
}

func (m *mockStorageInstance) Owner() names.Tag {
	return m.owner
}

func (m *mockStorageInstance) Tag() names.Tag {
	return m.storageTag
}

func (m *mockStorageInstance) StorageTag() names.StorageTag {
	return m.storageTag.(names.StorageTag)
}

func (m *mockStorageInstance) CharmURL() *charm.URL {
	panic("not implemented for test")
}

type mockStorageAttachment struct {
	state.StorageAttachment
	storage *mockStorageInstance
}

func (m *mockStorageAttachment) StorageInstance() names.StorageTag {
	return m.storage.Tag().(names.StorageTag)
}

func (m *mockStorageAttachment) Unit() names.UnitTag {
	return m.storage.Owner().(names.UnitTag)
}

type mockVolumeAttachment struct {
	VolumeTag  names.VolumeTag
	MachineTag names.MachineTag
}

func (va *mockVolumeAttachment) Volume() names.VolumeTag {
	return va.VolumeTag
}

func (va *mockVolumeAttachment) Machine() names.MachineTag {
	return va.MachineTag
}

func (va *mockVolumeAttachment) Life() state.Life {
	panic("not implemented for test")
}

func (va *mockVolumeAttachment) Info() (state.VolumeAttachmentInfo, error) {
	return state.VolumeAttachmentInfo{}, errors.New("not interested yet")
}

func (va *mockVolumeAttachment) Params() (state.VolumeAttachmentParams, bool) {
	panic("not implemented for test")
}

type mockBlock struct {
	state.Block
	t   state.BlockType
	msg string
}

func (b mockBlock) Type() state.BlockType {
	return b.t
}

func (b mockBlock) Message() string {
	return b.msg
}
