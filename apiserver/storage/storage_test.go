// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/storage"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	jujustorage "github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	coretesting "github.com/juju/juju/testing"
)

type storageSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer testing.FakeAuthorizer

	api        *storage.API
	state      *mockState
	storageTag names.StorageTag
	unitTag    names.UnitTag
	machineTag names.MachineTag

	calls []string
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) SetUpTest(c *gc.C) {
	s.resources = common.NewResources()
	s.authorizer = testing.FakeAuthorizer{names.NewUserTag("testuser"), true}
	s.calls = []string{}
	s.state = s.constructState(c)
	var err error
	s.api, err = storage.CreateAPI(s.state, &mockPoolManager{}, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

type mockPoolManager struct{}

func (*mockPoolManager) Get(name string) (*jujustorage.Config, error) {
	return jujustorage.NewConfig("foo", provider.LoopProviderType, nil)
}

func (*mockPoolManager) Create(name string, providerType jujustorage.ProviderType, attrs map[string]interface{}) (*jujustorage.Config, error) {
	panic("not implemented")
}

func (*mockPoolManager) Delete(name string) error {
	panic("not implemented")
}

func (*mockPoolManager) List() ([]*jujustorage.Config, error) {
	panic("not implemented")
}

const (
	allStorageInstancesCall                 = "allStorageInstances"
	storageInstanceAttachmentsCall          = "storageInstanceAttachments"
	unitAssignedMachineCall                 = "UnitAssignedMachine"
	storageInstanceCall                     = "StorageInstance"
	storageInstanceFilesystemCall           = "StorageInstanceFilesystem"
	storageInstanceFilesystemAttachmentCall = "storageInstanceFilesystemAttachment"
	storageInstanceVolumeCall               = "storageInstanceVolume"
)

func (s *storageSuite) TestStorageListEmpty(c *gc.C) {
	s.state.allStorageInstances = func() ([]state.StorageInstance, error) {
		s.calls = append(s.calls, allStorageInstancesCall)
		return []state.StorageInstance{}, nil
	}

	found, err := s.api.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 0)
	s.assertCalls(c, []string{allStorageInstancesCall})
}

func (s *storageSuite) TestStorageList(c *gc.C) {
	_, err := s.api.List()
	c.Assert(err, jc.ErrorIsNil)

	//	expectedCalls := []string{
	//		allStorageInstancesCall,
	//		storageInstanceAttachmentsCall,
	//		unitAssignedMachineCall,
	//		storageInstanceCall,
	//		storageInstanceFilesystemCall,
	//		storageInstanceFilesystemAttachmentCall,
	//	}
	//	s.assertCalls(c, expectedCalls)
	//
	//	c.Assert(found.Results, gc.HasLen, 1)
	//	wantedDetails := s.createTestStorageInfo()
	//	wantedDetails.UnitTag = s.unitTag.String()
	//	wantedDetails.Status = "attached"
	//	s.assertInstanceInfoError(c, found.Results[0], wantedDetails, "")
}

func (s *storageSuite) TestStorageListError(c *gc.C) {
	msg := "list test error"
	s.state.allStorageInstances = func() ([]state.StorageInstance, error) {
		s.calls = append(s.calls, allStorageInstancesCall)
		return []state.StorageInstance{}, errors.Errorf(msg)
	}

	found, err := s.api.List()
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)

	expectedCalls := []string{
		allStorageInstancesCall,
	}
	s.assertCalls(c, expectedCalls)
	c.Assert(found.Results, gc.HasLen, 0)
}

func (s *storageSuite) TestStorageListInstanceError(c *gc.C) {
	msg := "list test error"
	s.state.storageInstance = func(sTag names.StorageTag) (state.StorageInstance, error) {
		s.calls = append(s.calls, storageInstanceCall)
		c.Assert(sTag, gc.DeepEquals, s.storageTag)
		return nil, errors.Errorf(msg)
	}

	found, err := s.api.List()
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		allStorageInstancesCall,
		storageInstanceVolumeCall,
		storageInstanceAttachmentsCall,
		unitAssignedMachineCall,
		storageInstanceCall,
	}
	s.assertCalls(c, expectedCalls)
	c.Assert(found.Results, gc.HasLen, 1)
	wanted := s.createTestStorageInfoWithError("",
		fmt.Sprintf("getting storage attachment info: getting storage instance: %v", msg))
	s.assertInstanceInfoError(c, found.Results[0], wanted, msg)
}

func (s *storageSuite) TestStorageListAttachmentError(c *gc.C) {
	s.state.storageInstanceAttachments = func(tag names.StorageTag) ([]state.StorageAttachment, error) {
		s.calls = append(s.calls, storageInstanceAttachmentsCall)
		c.Assert(tag, gc.DeepEquals, s.storageTag)
		return []state.StorageAttachment{}, errors.Errorf("list test error")
	}

	found, err := s.api.List()
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		allStorageInstancesCall,
		storageInstanceVolumeCall,
		storageInstanceAttachmentsCall,
	}
	s.assertCalls(c, expectedCalls)
	c.Assert(found.Results, gc.HasLen, 1)
	expectedErr := "permission denied"
	wanted := s.createTestStorageInfoWithError(
		"unauthorized access",
		expectedErr)
	s.assertInstanceInfoError(c, found.Results[0], wanted, expectedErr)
}

func (s *storageSuite) TestStorageListMachineError(c *gc.C) {
	msg := "list test error"
	s.state.unitAssignedMachine = func(u names.UnitTag) (names.MachineTag, error) {
		s.calls = append(s.calls, unitAssignedMachineCall)
		c.Assert(u, gc.DeepEquals, s.unitTag)
		return names.MachineTag{}, errors.Errorf(msg)
	}

	found, err := s.api.List()
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		allStorageInstancesCall,
		storageInstanceVolumeCall,
		storageInstanceAttachmentsCall,
		unitAssignedMachineCall,
	}
	s.assertCalls(c, expectedCalls)
	c.Assert(found.Results, gc.HasLen, 1)
	wanted := s.createTestStorageInfoWithError("",
		fmt.Sprintf("getting unit for storage attachment: %v", msg))
	s.assertInstanceInfoError(c, found.Results[0], wanted, msg)
}

func (s *storageSuite) TestStorageListFilesystemError(c *gc.C) {
	msg := "list test error"
	s.state.storageInstanceFilesystem = func(sTag names.StorageTag) (state.Filesystem, error) {
		s.calls = append(s.calls, storageInstanceFilesystemCall)
		c.Assert(sTag, gc.DeepEquals, s.storageTag)
		return nil, errors.Errorf(msg)
	}

	found, err := s.api.List()
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		allStorageInstancesCall,
		storageInstanceVolumeCall,
		storageInstanceAttachmentsCall,
		unitAssignedMachineCall,
		storageInstanceCall,
		storageInstanceFilesystemCall,
	}
	s.assertCalls(c, expectedCalls)
	c.Assert(found.Results, gc.HasLen, 1)
	wanted := s.createTestStorageInfoWithError("",
		fmt.Sprintf("getting storage attachment info: getting filesystem: %v", msg))
	s.assertInstanceInfoError(c, found.Results[0], wanted, msg)
}

func (s *storageSuite) TestStorageListFilesystemAttachmentError(c *gc.C) {
	msg := "list test error"
	s.state.unitAssignedMachine = func(u names.UnitTag) (names.MachineTag, error) {
		s.calls = append(s.calls, unitAssignedMachineCall)
		c.Assert(u, gc.DeepEquals, s.unitTag)
		return s.machineTag, errors.Errorf(msg)
	}

	found, err := s.api.List()
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		allStorageInstancesCall,
		storageInstanceVolumeCall,
		storageInstanceAttachmentsCall,
		unitAssignedMachineCall,
	}
	s.assertCalls(c, expectedCalls)
	c.Assert(found.Results, gc.HasLen, 1)
	wanted := s.createTestStorageInfoWithError("",
		fmt.Sprintf("getting unit for storage attachment: %v", msg))
	s.assertInstanceInfoError(c, found.Results[0], wanted, msg)
}

func (s *storageSuite) createTestStorageInfoWithError(code, msg string) params.StorageInfo {
	wanted := s.createTestStorageInfo()
	wanted.Error = &params.Error{Code: code,
		Message: fmt.Sprintf("getting attachments for storage data/0: %v", msg)}
	return wanted
}

func (s *storageSuite) createTestStorageInfo() params.StorageInfo {
	return params.StorageInfo{
		params.StorageDetails{
			StorageTag: s.storageTag.String(),
			OwnerTag:   s.unitTag.String(),
			Kind:       params.StorageKindFilesystem,
			Status:     "pending",
		},
		nil,
	}
}

func (s *storageSuite) constructState(c *gc.C) *mockState {
	s.unitTag = names.NewUnitTag("mysql/0")
	s.storageTag = names.NewStorageTag("data/0")

	mockInstance := &mockStorageInstance{
		kind:       state.StorageKindFilesystem,
		owner:      s.unitTag,
		storageTag: s.storageTag,
	}

	storageInstanceAttachment := &mockStorageAttachment{storage: mockInstance}

	s.machineTag = names.NewMachineTag("66")
	filesystemTag := names.NewFilesystemTag("104")
	volumeTag := names.NewVolumeTag("22")
	filesystem := &mockFilesystem{tag: filesystemTag}
	filesystemAttachment := &mockFilesystemAttachment{}
	volume := &mockVolume{tag: volumeTag}

	return &mockState{
		allStorageInstances: func() ([]state.StorageInstance, error) {
			s.calls = append(s.calls, allStorageInstancesCall)
			return []state.StorageInstance{mockInstance}, nil
		},
		storageInstance: func(sTag names.StorageTag) (state.StorageInstance, error) {
			s.calls = append(s.calls, storageInstanceCall)
			c.Assert(sTag, gc.DeepEquals, s.storageTag)
			return mockInstance, nil
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
			return volume, nil
		},
		unitAssignedMachine: func(u names.UnitTag) (names.MachineTag, error) {
			s.calls = append(s.calls, unitAssignedMachineCall)
			c.Assert(u, gc.DeepEquals, s.unitTag)
			return s.machineTag, nil
		},
	}
}

func (s *storageSuite) assertCalls(c *gc.C, expectedCalls []string) {
	c.Assert(s.calls, gc.HasLen, len(expectedCalls))
	c.Assert(s.calls, jc.SameContents, expectedCalls)
}

func (s *storageSuite) assertInstanceInfoError(c *gc.C, obtained params.StorageInfo, wanted params.StorageInfo, expected string) {
	if expected != "" {
		c.Assert(errors.Cause(obtained.Error), gc.ErrorMatches, fmt.Sprintf(".*%v.*", expected))
	} else {
		c.Assert(obtained.Error, gc.IsNil)
	}
	c.Assert(obtained, gc.DeepEquals, wanted)
}

func (s *storageSuite) TestShowStorageEmpty(c *gc.C) {
	found, err := s.api.Show(params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	// Nothing should have matched the filter :D
	c.Assert(found.Results, gc.HasLen, 0)
}

func (s *storageSuite) TestShowStorageNoFilter(c *gc.C) {
	found, err := s.api.Show(params.Entities{Entities: []params.Entity{}})
	c.Assert(err, jc.ErrorIsNil)
	// Nothing should have matched the filter :D
	c.Assert(found.Results, gc.HasLen, 0)
}

func (s *storageSuite) TestShowStorage(c *gc.C) {
	entity := params.Entity{Tag: s.storageTag.String()}

	found, err := s.api.Show(params.Entities{Entities: []params.Entity{entity}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)

	one := found.Results[0]
	c.Assert(one.Error, gc.IsNil)

	expected := params.StorageDetails{
		StorageTag: s.storageTag.String(),
		OwnerTag:   s.unitTag.String(),
		Kind:       params.StorageKindFilesystem,
		UnitTag:    s.unitTag.String(),
		Status:     "pending",
	}
	c.Assert(one.Result, gc.DeepEquals, expected)
}

func (s *storageSuite) TestShowStorageInvalidId(c *gc.C) {
	storageTag := "foo"
	entity := params.Entity{Tag: storageTag}

	found, err := s.api.Show(params.Entities{Entities: []params.Entity{entity}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)

	instance := found.Results[0]
	c.Assert(instance.Error, gc.ErrorMatches, `"foo" is not a valid tag`)

	expected := params.StorageDetails{Kind: params.StorageKindUnknown}
	c.Assert(instance.Result, gc.DeepEquals, expected)
}

type mockState struct {
	storageInstance     func(names.StorageTag) (state.StorageInstance, error)
	allStorageInstances func() ([]state.StorageInstance, error)

	storageInstanceAttachments func(names.StorageTag) ([]state.StorageAttachment, error)

	unitAssignedMachine func(u names.UnitTag) (names.MachineTag, error)

	storageInstanceVolume           func(names.StorageTag) (state.Volume, error)
	storageInstanceVolumeAttachment func(names.MachineTag, names.VolumeTag) (state.VolumeAttachment, error)

	storageInstanceFilesystem           func(names.StorageTag) (state.Filesystem, error)
	storageInstanceFilesystemAttachment func(m names.MachineTag, f names.FilesystemTag) (state.FilesystemAttachment, error)

	watchFilesystemAttachment func(names.MachineTag, names.FilesystemTag) state.NotifyWatcher
	watchVolumeAttachment     func(names.MachineTag, names.VolumeTag) state.NotifyWatcher
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

func (st *mockState) WatchFilesystemAttachment(mtag names.MachineTag, f names.FilesystemTag) state.NotifyWatcher {
	return st.watchFilesystemAttachment(mtag, f)
}

func (st *mockState) WatchVolumeAttachment(mtag names.MachineTag, v names.VolumeTag) state.NotifyWatcher {
	return st.watchVolumeAttachment(mtag, v)
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
	tag names.VolumeTag
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
