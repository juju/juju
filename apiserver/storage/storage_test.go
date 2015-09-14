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
	wantedDetails := s.createTestStorageDetailsResult()

	c.Assert(found.Results[0].Result.Status.Since, gc.NotNil)
	found.Results[0].Result.Status.Since = nil
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
	wantedDetails := s.createTestStorageDetailsResult()
	wantedDetails.Result.Kind = params.StorageKindBlock
	wantedDetails.Result.Status.Status = params.StatusAttached
	wantedDetails.Legacy.Kind = params.StorageKindBlock
	wantedDetails.Legacy.Status = "attached"
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
		c.Assert(sTag, jc.DeepEquals, s.storageTag)
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
	wanted := s.createTestStorageDetailsResultWithError("",
		fmt.Sprintf("getting storage attachment info: getting storage instance: %v", msg))
	s.assertInstanceInfoError(c, found.Results[0], wanted, msg)
}

func (s *storageSuite) TestStorageListAttachmentError(c *gc.C) {
	s.state.storageInstanceAttachments = func(tag names.StorageTag) ([]state.StorageAttachment, error) {
		s.calls = append(s.calls, storageInstanceAttachmentsCall)
		c.Assert(tag, jc.DeepEquals, s.storageTag)
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
	wanted := s.createTestStorageDetailsResultWithError("", expectedErr)
	s.assertInstanceInfoError(c, found.Results[0], wanted, expectedErr)
}

func (s *storageSuite) TestStorageListMachineError(c *gc.C) {
	msg := "list test error"
	s.state.unitAssignedMachine = func(u names.UnitTag) (names.MachineTag, error) {
		s.calls = append(s.calls, unitAssignedMachineCall)
		c.Assert(u, jc.DeepEquals, s.unitTag)
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
	wanted := s.createTestStorageDetailsResultWithError("",
		fmt.Sprintf("getting unit for storage attachment: %v", msg))
	s.assertInstanceInfoError(c, found.Results[0], wanted, msg)
}

func (s *storageSuite) TestStorageListFilesystemError(c *gc.C) {
	msg := "list test error"
	s.state.storageInstanceFilesystem = func(sTag names.StorageTag) (state.Filesystem, error) {
		s.calls = append(s.calls, storageInstanceFilesystemCall)
		c.Assert(sTag, jc.DeepEquals, s.storageTag)
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
	wanted := s.createTestStorageDetailsResultWithError("",
		fmt.Sprintf("getting storage attachment info: getting filesystem: %v", msg))
	s.assertInstanceInfoError(c, found.Results[0], wanted, msg)
}

func (s *storageSuite) TestStorageListFilesystemAttachmentError(c *gc.C) {
	msg := "list test error"
	s.state.unitAssignedMachine = func(u names.UnitTag) (names.MachineTag, error) {
		s.calls = append(s.calls, unitAssignedMachineCall)
		c.Assert(u, jc.DeepEquals, s.unitTag)
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
	wanted := s.createTestStorageDetailsResultWithError("",
		fmt.Sprintf("getting unit for storage attachment: %v", msg))
	s.assertInstanceInfoError(c, found.Results[0], wanted, msg)
}

func (s *storageSuite) createTestStorageDetailsResultWithError(code, msg string) params.StorageDetailsResult {
	wanted := s.createTestStorageDetailsResult()
	wanted.Error = &params.Error{Code: code,
		Message: fmt.Sprintf("getting attachments for storage data/0: %v", msg)}
	return wanted
}

func (s *storageSuite) createTestStorageDetailsResult() params.StorageDetailsResult {
	return params.StorageDetailsResult{
		&params.StorageDetails{
			StorageTag: s.storageTag.String(),
			OwnerTag:   s.unitTag.String(),
			Kind:       params.StorageKindFilesystem,
			Status: params.EntityStatus{
				Status: "pending",
			},
			Attachments: map[string]params.StorageAttachmentDetails{
				s.unitTag.String(): params.StorageAttachmentDetails{
					s.storageTag.String(),
					s.unitTag.String(),
					s.machineTag.String(),
					"", // location
				},
			},
		},
		params.LegacyStorageDetails{
			StorageTag: s.storageTag.String(),
			OwnerTag:   s.unitTag.String(),
			UnitTag:    s.unitTag.String(),
			Kind:       params.StorageKindFilesystem,
			Status:     "pending",
		},
		nil,
	}
}

func (s *storageSuite) assertInstanceInfoError(c *gc.C, obtained params.StorageDetailsResult, wanted params.StorageDetailsResult, expected string) {
	if expected != "" {
		c.Assert(errors.Cause(obtained.Error), gc.ErrorMatches, fmt.Sprintf(".*%v.*", expected))
		c.Assert(obtained.Result, gc.IsNil)
		c.Assert(obtained.Legacy, jc.DeepEquals, params.LegacyStorageDetails{})
	} else {
		c.Assert(obtained.Error, gc.IsNil)
		c.Assert(obtained, jc.DeepEquals, wanted)
	}
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
		Status: params.EntityStatus{
			Status: "pending",
		},
		Attachments: map[string]params.StorageAttachmentDetails{
			s.unitTag.String(): params.StorageAttachmentDetails{
				s.storageTag.String(),
				s.unitTag.String(),
				s.machineTag.String(),
				"",
			},
		},
	}
	c.Assert(one.Result.Status.Since, gc.NotNil)
	one.Result.Status.Since = nil
	c.Assert(one.Result, jc.DeepEquals, &expected)
}

func (s *storageSuite) TestShowStorageInvalidId(c *gc.C) {
	storageTag := "foo"
	entity := params.Entity{Tag: storageTag}

	found, err := s.api.Show(params.Entities{Entities: []params.Entity{entity}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	s.assertInstanceInfoError(c, found.Results[0], params.StorageDetailsResult{}, `"foo" is not a valid tag`)
}
