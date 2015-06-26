// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type storageSuite struct {
	baseStorageSuite
}

var _ = gc.Suite(&storageSuite{})

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

func (s *storageSuite) TestStorageListFilesystem(c *gc.C) {
	found, err := s.api.List()
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		allStorageInstancesCall,
		storageInstanceAttachmentsCall,
		unitAssignedMachineCall,
		storageInstanceCall,
		storageInstanceFilesystemCall,
		storageInstanceFilesystemAttachmentCall,
	}
	s.assertCalls(c, expectedCalls)

	c.Assert(found.Results, gc.HasLen, 1)
	wantedDetails := s.createTestStorageInfo()
	wantedDetails.UnitTag = s.unitTag.String()
	s.assertInstanceInfoError(c, found.Results[0], wantedDetails, "")
}

func (s *storageSuite) TestStorageListVolume(c *gc.C) {
	s.storageInstance.kind = state.StorageKindBlock
	found, err := s.api.List()
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		allStorageInstancesCall,
		storageInstanceVolumeCall,
		storageInstanceAttachmentsCall,
		unitAssignedMachineCall,
		storageInstanceCall,
		storageInstanceVolumeCall,
	}
	s.assertCalls(c, expectedCalls)

	c.Assert(found.Results, gc.HasLen, 1)
	wantedDetails := s.createTestStorageInfo()
	wantedDetails.Kind = params.StorageKindBlock
	wantedDetails.UnitTag = s.unitTag.String()
	s.assertInstanceInfoError(c, found.Results[0], wantedDetails, "")
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
		storageInstanceAttachmentsCall,
	}
	s.assertCalls(c, expectedCalls)
	c.Assert(found.Results, gc.HasLen, 1)
	expectedErr := "list test error"
	wanted := s.createTestStorageInfoWithError("", expectedErr)
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
