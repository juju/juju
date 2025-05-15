// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	"fmt"
	"net/http"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	ociCore "github.com/oracle/oci-go-sdk/v65/core"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/provider/oci"
	"github.com/juju/juju/internal/storage"
)

type storageVolumeSuite struct {
	commonSuite

	provider storage.Provider
}

var _ = tc.Suite(&storageVolumeSuite{})

func (s *storageVolumeSuite) SetUpTest(c *tc.C) {
	s.commonSuite.SetUpTest(c)

	var err error
	s.provider, err = s.env.StorageProvider(oci.OciStorageProviderType)
	c.Assert(err, tc.IsNil)
}

func (s *storageVolumeSuite) newVolumeSource(c *tc.C) storage.VolumeSource {
	cfg, err := storage.NewConfig("iscsi", oci.OciStorageProviderType,
		map[string]interface{}{
			oci.OciVolumeType: oci.IscsiPool,
		})
	c.Assert(err, tc.IsNil)
	c.Assert(cfg, tc.NotNil)

	source, err := s.provider.VolumeSource(cfg)
	c.Assert(err, tc.IsNil)
	return source
}

func (s *storageVolumeSuite) setupCreateVolumesExpectations(c *tc.C, tag names.VolumeTag, size int64) {
	name := tag.String()
	volTags := map[string]string{
		tags.JujuModel: s.env.Config().UUID(),
	}

	volume := ociCore.Volume{
		AvailabilityDomain: makeStringPointer("fakeZone1"),
		CompartmentId:      &s.testCompartment,
		Id:                 makeStringPointer("fakeVolumeId"),
		LifecycleState:     ociCore.VolumeLifecycleStateProvisioning,
		FreeformTags:       volTags,
		SizeInGBs:          &size,
	}

	requestDetails := ociCore.CreateVolumeDetails{
		AvailabilityDomain: makeStringPointer("fakeZone1"),
		CompartmentId:      &s.testCompartment,
		DisplayName:        &name,
		SizeInMBs:          &size,
		FreeformTags:       volTags,
	}

	request := ociCore.CreateVolumeRequest{
		CreateVolumeDetails: requestDetails,
	}

	response := ociCore.CreateVolumeResponse{
		RawResponse: &http.Response{
			StatusCode: 200,
		},
		Volume: volume,
	}

	volumeAvailable := volume
	volumeAvailable.LifecycleState = ociCore.VolumeLifecycleStateAvailable

	getVolumeRequest := ociCore.GetVolumeRequest{VolumeId: volumeAvailable.Id}
	getVolumeResponse := ociCore.GetVolumeResponse{
		Volume: volumeAvailable,
	}
	s.storage.EXPECT().CreateVolume(gomock.Any(), request).Return(response, nil)
	s.storage.EXPECT().GetVolume(gomock.Any(), getVolumeRequest).Return(getVolumeResponse, nil).AnyTimes()

}

func (s *storageVolumeSuite) TestCreateVolumes(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	source := s.newVolumeSource(c)
	volumeTag := names.NewVolumeTag("1")
	s.setupListInstancesExpectations(c, s.testInstanceID, ociCore.InstanceLifecycleStateRunning, 0)
	s.setupCreateVolumesExpectations(c, volumeTag, 61440)

	results, err := source.CreateVolumes(c.Context(), []storage.VolumeParams{
		{
			Size:     uint64(61440),
			Tag:      names.NewVolumeTag("1"),
			Provider: oci.OciStorageProviderType,
			Attachment: &storage.VolumeAttachmentParams{
				AttachmentParams: storage.AttachmentParams{
					InstanceId: instance.Id(s.testInstanceID),
				},
			},
		},
	})
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Assert(results[0].Error, tc.ErrorIsNil)
}

func (s *storageVolumeSuite) TestCreateVolumesInvalidSize(c *tc.C) {
	source := s.newVolumeSource(c)
	results, err := source.CreateVolumes(c.Context(), []storage.VolumeParams{
		{
			Size:     uint64(2048),
			Tag:      names.NewVolumeTag("1"),
			Provider: oci.OciStorageProviderType,
			Attachment: &storage.VolumeAttachmentParams{
				AttachmentParams: storage.AttachmentParams{
					InstanceId: instance.Id(s.testInstanceID),
				},
			},
		},
	})
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].Error, tc.ErrorMatches, "invalid volume size 2. Valid range is.*")
}

func (s *storageVolumeSuite) TestCreateVolumesNilParams(c *tc.C) {
	source := s.newVolumeSource(c)
	results, err := source.CreateVolumes(c.Context(), nil)
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.HasLen, 0)
}

func (s *storageVolumeSuite) setupListVolumesExpectations(c *tc.C, size int64) map[string]ociCore.Volume {
	volTags := map[string]string{
		tags.JujuModel: s.env.Config().UUID(),
	}
	volumes := []ociCore.Volume{
		{
			AvailabilityDomain: makeStringPointer("fakeZone1"),
			CompartmentId:      &s.testCompartment,
			Id:                 makeStringPointer("fakeVolumeId"),
			LifecycleState:     ociCore.VolumeLifecycleStateAvailable,
			FreeformTags:       volTags,
			SizeInGBs:          &size,
		},
		{
			AvailabilityDomain: makeStringPointer("fakeZone1"),
			CompartmentId:      &s.testCompartment,
			Id:                 makeStringPointer("fakeVolumeId2"),
			LifecycleState:     ociCore.VolumeLifecycleStateAvailable,
			FreeformTags:       volTags,
			SizeInGBs:          &size,
		},
	}

	s.storage.EXPECT().ListVolumes(gomock.Any(), &s.testCompartment).Return(volumes, nil).AnyTimes()
	asMap := map[string]ociCore.Volume{}
	for _, vol := range volumes {
		asMap[*vol.Id] = vol
	}
	return asMap
}

func (s *storageVolumeSuite) TestListVolumes(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupListVolumesExpectations(c, 60)

	source := s.newVolumeSource(c)

	volumes, err := source.ListVolumes(c.Context())
	c.Assert(err, tc.IsNil)
	c.Assert(len(volumes), tc.Equals, 2)
	c.Assert(volumes, tc.SameContents, []string{"fakeVolumeId", "fakeVolumeId2"})
}

func (s *storageVolumeSuite) TestDescribeVolumes(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupListVolumesExpectations(c, 60)

	source := s.newVolumeSource(c)

	results, err := source.DescribeVolumes(c.Context(), []string{"fakeVolumeId"})
	c.Assert(err, tc.IsNil)
	c.Assert(len(results), tc.Equals, 1)
	c.Assert(results[0].VolumeInfo.VolumeId, tc.Equals, "fakeVolumeId")
	c.Assert(results[0].VolumeInfo.Size, tc.Equals, uint64(60*1024))
	c.Assert(results[0].VolumeInfo.Persistent, tc.Equals, true)

	results, err = source.DescribeVolumes(c.Context(), []string{"fakeVolumeId", "fakeVolumeId2"})
	c.Assert(err, tc.IsNil)
	c.Assert(len(results), tc.Equals, 2)

	results, err = source.DescribeVolumes(c.Context(), []string{"IDontExist", "fakeVolumeId2"})
	c.Assert(err, tc.IsNil)
	c.Assert(len(results), tc.Equals, 2)
	c.Assert(results[0].Error, tc.NotNil)
	c.Assert(results[1].Error, tc.IsNil)
}

func (s *storageVolumeSuite) TestValidateVolumeParams(c *tc.C) {
	source := s.newVolumeSource(c)
	params := storage.VolumeParams{
		Size:     uint64(2048),
		Tag:      names.NewVolumeTag("1"),
		Provider: oci.OciStorageProviderType,
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				InstanceId: instance.Id(s.testInstanceID),
			},
		},
	}

	err := source.ValidateVolumeParams(params)
	c.Assert(err, tc.ErrorMatches, "invalid volume size 2. Valid range is.*")

	params.Size = 61440
	err = source.ValidateVolumeParams(params)
	c.Assert(err, tc.IsNil)
}

func (s *storageVolumeSuite) setupDeleteVolumesExpectations(c *tc.C, size int64, id string) {
	volumes := s.setupListVolumesExpectations(c, size)

	request := ociCore.DeleteVolumeRequest{
		VolumeId: &id,
	}
	terminatedVol := volumes[id]
	terminatedVol.LifecycleState = ociCore.VolumeLifecycleStateTerminated
	response := ociCore.DeleteVolumeResponse{
		RawResponse: &http.Response{
			StatusCode: 200,
		},
	}
	s.storage.EXPECT().DeleteVolume(gomock.Any(), request).Return(response, nil).AnyTimes()

	getVolumeRequest := ociCore.GetVolumeRequest{VolumeId: terminatedVol.Id}
	getVolumeResponse := ociCore.GetVolumeResponse{
		Volume: terminatedVol,
	}
	s.storage.EXPECT().GetVolume(gomock.Any(), getVolumeRequest).Return(getVolumeResponse, nil).AnyTimes()
}

func (s *storageVolumeSuite) TestDestroyVolumes(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupDeleteVolumesExpectations(c, 60, "fakeVolumeId")

	source := s.newVolumeSource(c)

	results, err := source.DestroyVolumes(c.Context(), []string{"fakeVolumeId"})
	c.Assert(err, tc.IsNil)
	c.Assert(len(results), tc.Equals, 1)
	c.Assert(results[0], tc.IsNil)

	results, err = source.DestroyVolumes(c.Context(), []string{"bogusId"})
	c.Assert(err, tc.IsNil)
	c.Assert(len(results), tc.Equals, 1)
	c.Assert(results[0], tc.ErrorMatches, "no such volume.*")
}

func (s *storageVolumeSuite) setupUpdateVolumesExpectations(c *tc.C, id string) {
	volumes := s.setupListVolumesExpectations(c, 60)
	vol := volumes[id]
	volTags := map[string]string{
		tags.JujuModel: "",
	}

	requestDetails := ociCore.UpdateVolumeDetails{
		FreeformTags: volTags,
	}
	request := ociCore.UpdateVolumeRequest{
		UpdateVolumeDetails: requestDetails,
		VolumeId:            vol.Id,
	}
	s.storage.EXPECT().UpdateVolume(gomock.Any(), request).Return(ociCore.UpdateVolumeResponse{}, nil).AnyTimes()
}

func (s *storageVolumeSuite) TestReleaseVolumes(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	s.setupUpdateVolumesExpectations(c, "fakeVolumeId")
	source := s.newVolumeSource(c)

	results, err := source.ReleaseVolumes(c.Context(), []string{"fakeVolumeId"})
	c.Assert(err, tc.IsNil)
	c.Assert(len(results), tc.Equals, 1)
	c.Assert(results[0], tc.IsNil)

	results, err = source.ReleaseVolumes(c.Context(), []string{"IAmNotHereWhatIsHereIsntHereJustThereButWithoutTheT"})
	c.Assert(err, tc.IsNil)
	c.Assert(len(results), tc.Equals, 1)
	c.Assert(results[0], tc.ErrorMatches, "no such volume.*")
}

func (s *storageVolumeSuite) setupGetInstanceExpectations(c *tc.C, instance string, state ociCore.InstanceLifecycleStateEnum) {
	requestMachine1, responseMachine1 := makeGetInstanceRequestResponse(
		ociCore.Instance{
			AvailabilityDomain: makeStringPointer("fakeZone1"),
			CompartmentId:      &s.testCompartment,
			Id:                 makeStringPointer(instance),
			LifecycleState:     state,
			Region:             makeStringPointer("us-phoenix-1"),
			Shape:              makeStringPointer("VM.Standard1.1"),
			DisplayName:        makeStringPointer("fakeName"),
			FreeformTags:       s.tags,
		},
	)
	s.compute.EXPECT().GetInstance(gomock.Any(), requestMachine1).Return(
		responseMachine1, nil).AnyTimes()
}

func (s *storageVolumeSuite) makeListVolumeAttachmentExpectations(c *tc.C, instance string, volumeId string, returnEmpty bool, times int) {
	port := 3260
	response := []ociCore.VolumeAttachment{}

	if returnEmpty == false {
		response = []ociCore.VolumeAttachment{
			ociCore.IScsiVolumeAttachment{
				AvailabilityDomain: makeStringPointer("fakeZone1"),
				InstanceId:         &instance,
				CompartmentId:      &s.testCompartment,
				Iqn:                makeStringPointer("bogus"),
				Id:                 makeStringPointer("fakeVolumeAttachment1"),
				VolumeId:           &volumeId,
				Ipv4:               makeStringPointer("192.168.1.1"),
				Port:               &port,
				DisplayName:        makeStringPointer("fakeVolumeAttachment"),
				ChapSecret:         makeStringPointer("superSecretPassword"),
				ChapUsername:       makeStringPointer("JohnDoe"),
				LifecycleState:     ociCore.VolumeAttachmentLifecycleStateAttached,
			},
		}
	}
	expect := s.compute.EXPECT().ListVolumeAttachments(gomock.Any(), &s.testCompartment, &instance).Return(response, nil)
	if times == 0 {
		expect.AnyTimes()
	} else {
		expect.Times(times)
	}
}

func (s *storageVolumeSuite) TestAttachVolumeWithExistingAttachment(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	volumeId := "fakeVolumeId"
	s.setupListInstancesExpectations(c, s.testInstanceID, ociCore.InstanceLifecycleStateRunning, 0)
	s.setupGetInstanceExpectations(c, s.testInstanceID, ociCore.InstanceLifecycleStateRunning)
	s.makeListVolumeAttachmentExpectations(c, s.testInstanceID, volumeId, false, 0)

	source := s.newVolumeSource(c)

	result, err := source.AttachVolumes(c.Context(), []storage.VolumeAttachmentParams{
		{
			AttachmentParams: storage.AttachmentParams{
				Provider:   oci.OciStorageProviderType,
				InstanceId: instance.Id(s.testInstanceID),
				ReadOnly:   false,
				Machine:    names.NewMachineTag("1"),
			},
			VolumeId: volumeId,
			Volume:   names.NewVolumeTag("1"),
		},
	})
	c.Assert(err, tc.IsNil)
	c.Assert(len(result), tc.Equals, 1)
	c.Assert(result[0].Error, tc.IsNil)
	planInfo := result[0].VolumeAttachment.VolumeAttachmentInfo.PlanInfo
	c.Assert(planInfo.DeviceAttributes["iqn"], tc.Equals, "bogus")
	c.Assert(planInfo.DeviceAttributes["address"], tc.Equals, "192.168.1.1")
	c.Assert(planInfo.DeviceAttributes["port"], tc.Equals, "3260")
	c.Assert(planInfo.DeviceAttributes["chap-user"], tc.Equals, "JohnDoe")
	c.Assert(planInfo.DeviceAttributes["chap-secret"], tc.Equals, "superSecretPassword")

}

func (s *storageVolumeSuite) TestAttachVolumeWithInvalidInstanceState(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	volumeId := "fakeVolumeId"
	s.setupListInstancesExpectations(c, s.testInstanceID, ociCore.InstanceLifecycleStateTerminated, 0)

	source := s.newVolumeSource(c)

	result, err := source.AttachVolumes(c.Context(), []storage.VolumeAttachmentParams{
		{
			AttachmentParams: storage.AttachmentParams{
				Provider:   oci.OciStorageProviderType,
				InstanceId: instance.Id(s.testInstanceID),
				ReadOnly:   false,
				Machine:    names.NewMachineTag("1"),
			},
			VolumeId: volumeId,
			Volume:   names.NewVolumeTag("1"),
		},
	})
	c.Assert(err, tc.IsNil)
	c.Assert(len(result), tc.Equals, 1)
	c.Assert(result[0].Error, tc.ErrorMatches, "invalid instance state for volume attachment:.*")
}

func (s *storageVolumeSuite) setupAttachNewVolumeExpectations(c *tc.C, instance, volumeId, attachmentId string) {
	useChap := true
	displayName := fmt.Sprintf("%s_%s", instance, volumeId)
	attachDetails := ociCore.AttachIScsiVolumeDetails{
		InstanceId:  &instance,
		VolumeId:    &volumeId,
		UseChap:     &useChap,
		DisplayName: &displayName,
	}
	request := ociCore.AttachVolumeRequest{
		AttachVolumeDetails: attachDetails,
	}

	attachment := s.getVolumeAttachmentTemplate(instance, volumeId, attachmentId)
	attachment.LifecycleState = ociCore.VolumeAttachmentLifecycleStateAttaching
	response := ociCore.AttachVolumeResponse{
		RawResponse: &http.Response{
			StatusCode: 200,
		},
		VolumeAttachment: attachment,
	}
	s.compute.EXPECT().AttachVolume(gomock.Any(), request).Return(response, nil)

}

func (s *storageVolumeSuite) getVolumeAttachmentTemplate(instance, volume, attachment string) ociCore.IScsiVolumeAttachment {
	port := 3260
	return ociCore.IScsiVolumeAttachment{
		AvailabilityDomain: makeStringPointer("fakeZone1"),
		InstanceId:         &instance,
		CompartmentId:      &s.testCompartment,
		Iqn:                makeStringPointer("bogus"),
		Id:                 &attachment,
		VolumeId:           &volume,
		Ipv4:               makeStringPointer("192.168.1.1"),
		Port:               &port,
		DisplayName:        makeStringPointer("fakeVolumeAttachment"),
		ChapSecret:         makeStringPointer("superSecretPassword"),
		ChapUsername:       makeStringPointer("JohnDoe"),
		LifecycleState:     ociCore.VolumeAttachmentLifecycleStateAttaching,
	}
}

func (s *storageVolumeSuite) setupGetVolumeAttachmentExpectations(
	c *tc.C, instance, volumeId, attachmentId string, state ociCore.VolumeAttachmentLifecycleStateEnum) {
	request := ociCore.GetVolumeAttachmentRequest{
		VolumeAttachmentId: &attachmentId,
	}
	attachment := s.getVolumeAttachmentTemplate(instance, volumeId, attachmentId)
	attachment.LifecycleState = state
	response := ociCore.GetVolumeAttachmentResponse{
		VolumeAttachment: attachment,
	}
	s.compute.EXPECT().GetVolumeAttachment(gomock.Any(), request).Return(response, nil).AnyTimes()
}

func (s *storageVolumeSuite) TestAttachVolume(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	volumeId := "fakeVolumeId"
	attachId := "fakeVolumeAttachmentId"
	s.setupListInstancesExpectations(c, s.testInstanceID, ociCore.InstanceLifecycleStateRunning, 0)
	s.setupGetInstanceExpectations(c, s.testInstanceID, ociCore.InstanceLifecycleStateRunning)
	s.makeListVolumeAttachmentExpectations(c, s.testInstanceID, volumeId, true, 1)
	s.setupAttachNewVolumeExpectations(c, s.testInstanceID, volumeId, attachId)
	s.setupGetVolumeAttachmentExpectations(
		c, s.testInstanceID, volumeId, attachId,
		ociCore.VolumeAttachmentLifecycleStateAttached)

	source := s.newVolumeSource(c)

	result, err := source.AttachVolumes(c.Context(), []storage.VolumeAttachmentParams{
		{
			AttachmentParams: storage.AttachmentParams{
				Provider:   oci.OciStorageProviderType,
				InstanceId: instance.Id(s.testInstanceID),
				ReadOnly:   false,
				Machine:    names.NewMachineTag("1"),
			},
			VolumeId: volumeId,
			Volume:   names.NewVolumeTag("1"),
		},
	})
	c.Assert(err, tc.IsNil)
	c.Assert(len(result), tc.Equals, 1)
	c.Assert(result[0].Error, tc.IsNil)
}

func (s *storageVolumeSuite) setupDetachVolumesExpectations(c *tc.C, attachmentId string) {
	request := ociCore.DetachVolumeRequest{
		VolumeAttachmentId: &attachmentId,
	}
	response := ociCore.DetachVolumeResponse{
		RawResponse: &http.Response{
			StatusCode: 200,
		},
	}
	s.compute.EXPECT().DetachVolume(gomock.Any(), request).Return(response, nil).AnyTimes()
}

func (s *storageVolumeSuite) TestDetachVolume(c *tc.C) {
	ctrl := s.patchEnv(c)
	defer ctrl.Finish()

	volumeId := "fakeVolumeId"
	attachId := "fakeVolumeAttachment1"
	s.setupListInstancesExpectations(c, s.testInstanceID, ociCore.InstanceLifecycleStateRunning, 0)
	s.makeListVolumeAttachmentExpectations(c, s.testInstanceID, volumeId, false, 1)
	s.setupDetachVolumesExpectations(c, attachId)
	s.setupGetVolumeAttachmentExpectations(
		c, s.testInstanceID, volumeId, attachId,
		ociCore.VolumeAttachmentLifecycleStateDetached)

	source := s.newVolumeSource(c)

	result, err := source.DetachVolumes(c.Context(), []storage.VolumeAttachmentParams{
		{
			AttachmentParams: storage.AttachmentParams{
				Provider:   oci.OciStorageProviderType,
				InstanceId: instance.Id(s.testInstanceID),
				ReadOnly:   false,
				Machine:    names.NewMachineTag("1"),
			},
			VolumeId: volumeId,
			Volume:   names.NewVolumeTag("1"),
		},
	})

	c.Assert(err, tc.IsNil)
	c.Assert(len(result), tc.Equals, 1)
}
