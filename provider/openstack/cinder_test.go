// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/goose.v2/cinder"
	gooseerror "gopkg.in/goose.v2/errors"
	"gopkg.in/goose.v2/identity"
	"gopkg.in/goose.v2/nova"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/openstack"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

const (
	mockVolId    = "0"
	mockVolSize  = 1024 * 2
	mockVolName  = "123"
	mockServerId = "mock-server-id"
	mockVolJson  = `{"volume":{"id": "` + mockVolId + `", "size":1,"name":"` + mockVolName + `"}}`
)

var (
	mockVolumeTag  = names.NewVolumeTag(mockVolName)
	mockMachineTag = names.NewMachineTag("456")
)

var _ = gc.Suite(&cinderVolumeSourceSuite{})

type cinderVolumeSourceSuite struct {
	testing.BaseSuite

	callCtx              *context.CloudCallContext
	invalidateCredential bool
}

func (s *cinderVolumeSourceSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.callCtx = &context.CloudCallContext{
		InvalidateCredentialFunc: func(string) error {
			s.invalidateCredential = true
			return nil
		},
	}
}

func (s *cinderVolumeSourceSuite) TearDownTest(c *gc.C) {
	s.invalidateCredential = false
	s.BaseSuite.TearDownTest(c)
}

func init() {
	// Override attempt strategy to speed things up.
	openstack.CinderAttempt.Delay = 0
}

func toStringPtr(s string) *string {
	return &s
}

func (s *cinderVolumeSourceSuite) TestAttachVolumes(c *gc.C) {
	mockAdapter := &mockAdapter{
		attachVolume: func(ctx context.ProviderCallContext, serverId, volId, mountPoint string) (*nova.VolumeAttachment, error) {
			c.Check(volId, gc.Equals, mockVolId)
			c.Check(serverId, gc.Equals, mockServerId)
			return &nova.VolumeAttachment{
				Id:       volId,
				VolumeId: volId,
				ServerId: serverId,
				Device:   toStringPtr("/dev/sda"),
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	results, err := volSource.AttachVolumes(s.callCtx, []storage.VolumeAttachmentParams{{
		Volume:   mockVolumeTag,
		VolumeId: mockVolId,
		AttachmentParams: storage.AttachmentParams{
			Provider:   openstack.CinderProviderType,
			Machine:    mockMachineTag,
			InstanceId: instance.Id(mockServerId),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, jc.DeepEquals, []storage.AttachVolumesResult{{
		VolumeAttachment: &storage.VolumeAttachment{
			mockVolumeTag,
			mockMachineTag,
			storage.VolumeAttachmentInfo{
				DeviceName: "sda",
			},
		},
	}})
}

func (s *cinderVolumeSourceSuite) TestAttachVolumesInvalidCredential(c *gc.C) {
	mockAdapter := &mockAdapter{
		attachVolume: func(ctx context.ProviderCallContext, serverId, volId, mountPoint string) (*nova.VolumeAttachment, error) {
			c.Check(volId, gc.Equals, mockVolId)
			c.Check(serverId, gc.Equals, mockServerId)
			return &nova.VolumeAttachment{}, gooseerror.NewUnauthorisedf(nil, "", "Unauthorised error.")
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	result, err := volSource.AttachVolumes(s.callCtx, []storage.VolumeAttachmentParams{{
		Volume:   mockVolumeTag,
		VolumeId: mockVolId,
		AttachmentParams: storage.AttachmentParams{
			Provider:   openstack.CinderProviderType,
			Machine:    mockMachineTag,
			InstanceId: instance.Id(mockServerId),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result[0].Error, gc.ErrorMatches, "Unauthorised error.")
	c.Assert(s.invalidateCredential, jc.IsTrue)
}

func (s *cinderVolumeSourceSuite) TestAttachVolumesNoDevice(c *gc.C) {
	mockAdapter := &mockAdapter{
		attachVolume: func(ctx context.ProviderCallContext, serverId, volId, mountPoint string) (*nova.VolumeAttachment, error) {
			return &nova.VolumeAttachment{
				Id:       volId,
				VolumeId: volId,
				ServerId: serverId,
				Device:   nil,
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	results, err := volSource.AttachVolumes(s.callCtx, []storage.VolumeAttachmentParams{{
		Volume:   mockVolumeTag,
		VolumeId: mockVolId,
		AttachmentParams: storage.AttachmentParams{
			Provider:   openstack.CinderProviderType,
			Machine:    mockMachineTag,
			InstanceId: instance.Id(mockServerId),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.ErrorMatches, "device not assigned to volume attachment")
	c.Assert(s.invalidateCredential, gc.Equals, false)
}

func (s *cinderVolumeSourceSuite) TestCreateVolume(c *gc.C) {
	const (
		requestedSize = 2 * 1024
		providedSize  = 3 * 1024
	)

	s.PatchValue(openstack.CinderAttempt, utils.AttemptStrategy{Min: 3})

	var getVolumeCalls int
	mockAdapter := &mockAdapter{
		createVolume: func(ctx context.ProviderCallContext, args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
			c.Assert(args, jc.DeepEquals, cinder.CreateVolumeVolumeParams{
				Size: requestedSize / 1024,
				Name: "juju-testmodel-volume-123",
			})
			return &cinder.Volume{
				ID: mockVolId,
			}, nil
		},
		getVolume: func(ctx context.ProviderCallContext, volumeId string) (*cinder.Volume, error) {
			var status string
			getVolumeCalls++
			if getVolumeCalls > 1 {
				status = "available"
			}
			return &cinder.Volume{
				ID:     volumeId,
				Size:   providedSize / 1024,
				Status: status,
			}, nil
		},
		attachVolume: func(ctx context.ProviderCallContext, serverId, volId, mountPoint string) (*nova.VolumeAttachment, error) {
			c.Check(volId, gc.Equals, mockVolId)
			c.Check(serverId, gc.Equals, mockServerId)
			return &nova.VolumeAttachment{
				Id:       volId,
				VolumeId: volId,
				ServerId: serverId,
				Device:   toStringPtr("/dev/sda"),
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	results, err := volSource.CreateVolumes(s.callCtx, []storage.VolumeParams{{
		Provider: openstack.CinderProviderType,
		Tag:      mockVolumeTag,
		Size:     requestedSize,
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   openstack.CinderProviderType,
				Machine:    mockMachineTag,
				InstanceId: instance.Id(mockServerId),
			},
		},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, jc.ErrorIsNil)

	c.Check(results[0].Volume, jc.DeepEquals, &storage.Volume{
		mockVolumeTag,
		storage.VolumeInfo{
			VolumeId:   mockVolId,
			Size:       providedSize,
			Persistent: true,
		},
	})

	// should have been 2 calls to GetVolume: twice initially
	// to wait until the volume became available.
	c.Check(getVolumeCalls, gc.Equals, 2)
}

func (s *cinderVolumeSourceSuite) TestCreateVolumeInvalidatCredential(c *gc.C) {
	const (
		requestedSize = 2 * 1024
		providedSize  = 3 * 1024
	)
	mockAdapter := &mockAdapter{
		createVolume: func(ctx context.ProviderCallContext, args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
			c.Assert(args, jc.DeepEquals, cinder.CreateVolumeVolumeParams{
				Size: requestedSize / 1024,
				Name: "juju-testmodel-volume-123",
			})
			return &cinder.Volume{}, gooseerror.NewUnauthorisedf(nil, "", "Unauthorised error.")
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	result, err := volSource.CreateVolumes(s.callCtx, []storage.VolumeParams{{
		Provider: openstack.CinderProviderType,
		Tag:      mockVolumeTag,
		Size:     requestedSize,
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   openstack.CinderProviderType,
				Machine:    mockMachineTag,
				InstanceId: instance.Id(mockServerId),
			},
		},
	}})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result[0].Error, gc.ErrorMatches, "Unauthorised error.")
	//c.Assert(s.invalidateCredential, jc.IsTrue)
}

func (s *cinderVolumeSourceSuite) TestCreateVolumeVolumeType(c *gc.C) {
	var created bool
	mockAdapter := &mockAdapter{
		createVolume: func(ctx context.ProviderCallContext, args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
			created = true
			c.Assert(args, jc.DeepEquals, cinder.CreateVolumeVolumeParams{
				Size:       1,
				Name:       "juju-testmodel-volume-123",
				VolumeType: "SSD",
			})
			return &cinder.Volume{ID: mockVolId}, nil
		},
		getVolume: func(ctx context.ProviderCallContext, volumeId string) (*cinder.Volume, error) {
			return &cinder.Volume{
				ID:     volumeId,
				Size:   1,
				Status: "available",
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	_, err := volSource.CreateVolumes(s.callCtx, []storage.VolumeParams{{
		Provider: openstack.CinderProviderType,
		Tag:      mockVolumeTag,
		Size:     1024,
		Attributes: map[string]interface{}{
			"volume-type": "SSD",
		},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(created, jc.IsTrue)
}

func (s *cinderVolumeSourceSuite) TestResourceTags(c *gc.C) {
	var created bool
	mockAdapter := &mockAdapter{
		createVolume: func(ctx context.ProviderCallContext, args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
			created = true
			c.Assert(args, jc.DeepEquals, cinder.CreateVolumeVolumeParams{
				Size: 1,
				Name: "juju-testmodel-volume-123",
				Metadata: map[string]string{
					"ResourceTag1": "Value1",
					"ResourceTag2": "Value2",
				},
			})
			return &cinder.Volume{ID: mockVolId}, nil
		},
		getVolume: func(ctx context.ProviderCallContext, volumeId string) (*cinder.Volume, error) {
			return &cinder.Volume{
				ID:     volumeId,
				Size:   1,
				Status: "available",
			}, nil
		},
		attachVolume: func(ctx context.ProviderCallContext, serverId, volId, mountPoint string) (*nova.VolumeAttachment, error) {
			return &nova.VolumeAttachment{
				Id:       volId,
				VolumeId: volId,
				ServerId: serverId,
				Device:   toStringPtr("/dev/sda"),
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	_, err := volSource.CreateVolumes(s.callCtx, []storage.VolumeParams{{
		Provider: openstack.CinderProviderType,
		Tag:      mockVolumeTag,
		Size:     1024,
		ResourceTags: map[string]string{
			"ResourceTag1": "Value1",
			"ResourceTag2": "Value2",
		},
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   openstack.CinderProviderType,
				Machine:    mockMachineTag,
				InstanceId: instance.Id(mockServerId),
			},
		},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(created, jc.IsTrue)
}

func (s *cinderVolumeSourceSuite) TestListVolumesInvalidCredential(c *gc.C) {
	mockAdapter := &mockAdapter{
		getVolumesDetail: func(ctx context.ProviderCallContext) ([]cinder.Volume, error) {
			return []cinder.Volume{{
				ID: "volume-1",
			}, {
				ID: "volume-2",
				Metadata: map[string]string{
					tags.JujuModel: "something-else",
				},
			}, {
				ID: "volume-3",
				Metadata: map[string]string{
					tags.JujuModel: testing.ModelTag.Id(),
				},
			}}, gooseerror.NewUnauthorisedf(nil, "", "Unauthorised error.")
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	_, err := volSource.ListVolumes(s.callCtx)
	c.Assert(err.Error(), gc.Equals, "Unauthorised error.")
	//c.Assert(s.invalidateCredential, jc.IsTrue)
}

func (s *cinderVolumeSourceSuite) TestListVolumes(c *gc.C) {
	mockAdapter := &mockAdapter{
		getVolumesDetail: func(ctx context.ProviderCallContext) ([]cinder.Volume, error) {
			return []cinder.Volume{{
				ID: "volume-1",
			}, {
				ID: "volume-2",
				Metadata: map[string]string{
					tags.JujuModel: "something-else",
				},
			}, {
				ID: "volume-3",
				Metadata: map[string]string{
					tags.JujuModel: testing.ModelTag.Id(),
				},
			}}, nil
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	volumeIds, err := volSource.ListVolumes(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(volumeIds, jc.DeepEquals, []string{"volume-3"})
}

func (s *cinderVolumeSourceSuite) TestDescribeVolumes(c *gc.C) {
	mockAdapter := &mockAdapter{
		getVolumesDetail: func(ctx context.ProviderCallContext) ([]cinder.Volume, error) {
			return []cinder.Volume{{
				ID:   mockVolId,
				Size: mockVolSize / 1024,
			}}, nil
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	volumes, err := volSource.DescribeVolumes(s.callCtx, []string{mockVolId})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(volumes, jc.DeepEquals, []storage.DescribeVolumesResult{{
		VolumeInfo: &storage.VolumeInfo{
			VolumeId:   mockVolId,
			Size:       mockVolSize,
			Persistent: true,
		},
	}})
}

func (s *cinderVolumeSourceSuite) TestDestroyVolumes(c *gc.C) {
	mockAdapter := &mockAdapter{}
	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	errs, err := volSource.DestroyVolumes(s.callCtx, []string{mockVolId})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, jc.DeepEquals, []error{nil})
	mockAdapter.CheckCalls(c, []gitjujutesting.StubCall{
		{"GetVolume", []interface{}{s.callCtx, mockVolId}},
		{"DeleteVolume", []interface{}{s.callCtx, mockVolId}},
	})
}

func (s *cinderVolumeSourceSuite) TestDestroyVolumesNotFound(c *gc.C) {
	mockAdapter := &mockAdapter{
		getVolume: func(ctx context.ProviderCallContext, volId string) (*cinder.Volume, error) {
			return nil, errors.NotFoundf("volume %q", volId)
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	errs, err := volSource.DestroyVolumes(s.callCtx, []string{mockVolId})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.invalidateCredential, jc.IsFalse)
	c.Assert(errs, jc.DeepEquals, []error{nil})
	mockAdapter.CheckCalls(c, []gitjujutesting.StubCall{
		{"GetVolume", []interface{}{s.callCtx, mockVolId}},
	})
}

func (s *cinderVolumeSourceSuite) TestDestroyVolumesAttached(c *gc.C) {
	statuses := []string{"in-use", "detaching", "available"}

	mockAdapter := &mockAdapter{
		getVolume: func(ctx context.ProviderCallContext, volId string) (*cinder.Volume, error) {
			c.Assert(statuses, gc.Not(gc.HasLen), 0)
			status := statuses[0]
			statuses = statuses[1:]
			return &cinder.Volume{
				ID:     volId,
				Status: status,
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	errs, err := volSource.DestroyVolumes(s.callCtx, []string{mockVolId})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.invalidateCredential, jc.IsFalse)
	c.Assert(errs, gc.HasLen, 1)
	c.Assert(errs[0], jc.ErrorIsNil)
	c.Assert(statuses, gc.HasLen, 0)
	mockAdapter.CheckCalls(c, []gitjujutesting.StubCall{{
		"GetVolume", []interface{}{s.callCtx, mockVolId},
	}, {
		"GetVolume", []interface{}{s.callCtx, mockVolId},
	}, {
		"GetVolume", []interface{}{s.callCtx, mockVolId},
	}, {
		"DeleteVolume", []interface{}{s.callCtx, mockVolId},
	}})
}

func (s *cinderVolumeSourceSuite) TestReleaseVolumes(c *gc.C) {
	mockAdapter := &mockAdapter{}
	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	errs, err := volSource.ReleaseVolumes(s.callCtx, []string{mockVolId})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, jc.DeepEquals, []error{nil})
	metadata := map[string]string{
		"juju-controller-uuid": "",
		"juju-model-uuid":      "",
	}
	mockAdapter.CheckCalls(c, []gitjujutesting.StubCall{
		{"GetVolume", []interface{}{s.callCtx, mockVolId}},
		{"SetVolumeMetadata", []interface{}{s.callCtx, mockVolId, metadata}},
	})
}

func (s *cinderVolumeSourceSuite) TestReleaseVolumesInvalidCredentials(c *gc.C) {
	statuses := []string{"in-use", "releasing", "available"}

	mockAdapter := &mockAdapter{
		getVolume: func(ctx context.ProviderCallContext, volId string) (*cinder.Volume, error) {
			c.Assert(statuses, gc.Not(gc.HasLen), 0)
			status := statuses[0]
			statuses = statuses[1:]
			return &cinder.Volume{
				ID:     volId,
				Status: status,
			}, gooseerror.NewUnauthorisedf(nil, "", "Unauthorised error.")
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	result, err := volSource.ReleaseVolumes(s.callCtx, []string{mockVolId})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result[0].Error(), gc.Equals, "cannot release volume \"0\": getting volume: Unauthorised error.")
	//c.Assert(s.invalidateCredential, jc.IsTrue)
}

func (s *cinderVolumeSourceSuite) TestReleaseVolumesAttached(c *gc.C) {
	mockAdapter := &mockAdapter{
		getVolume: func(ctx context.ProviderCallContext, volId string) (*cinder.Volume, error) {
			return &cinder.Volume{
				ID:     volId,
				Status: "in-use",
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	errs, err := volSource.ReleaseVolumes(s.callCtx, []string{mockVolId})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.HasLen, 1)
	c.Assert(errs[0], gc.ErrorMatches, `cannot release volume "0": volume still in-use`)
	mockAdapter.CheckCalls(c, []gitjujutesting.StubCall{{
		"GetVolume", []interface{}{s.callCtx, mockVolId},
	}})
}

func (s *cinderVolumeSourceSuite) TestReleaseVolumesDetaching(c *gc.C) {
	statuses := []string{"detaching", "available"}

	mockAdapter := &mockAdapter{
		getVolume: func(ctx context.ProviderCallContext, volId string) (*cinder.Volume, error) {
			c.Assert(statuses, gc.Not(gc.HasLen), 0)
			status := statuses[0]
			statuses = statuses[1:]
			return &cinder.Volume{
				ID:     volId,
				Status: status,
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	errs, err := volSource.ReleaseVolumes(s.callCtx, []string{mockVolId})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.HasLen, 1)
	c.Assert(errs[0], jc.ErrorIsNil)
	c.Assert(statuses, gc.HasLen, 0)
	mockAdapter.CheckCallNames(c, "GetVolume", "GetVolume", "SetVolumeMetadata")
}

func (s *cinderVolumeSourceSuite) TestDetachVolumes(c *gc.C) {
	const mockServerId2 = mockServerId + "2"

	var numDetachCalls int
	mockAdapter := &mockAdapter{
		detachVolume: func(ctx context.ProviderCallContext, serverId, volId string) error {
			numDetachCalls++
			if volId == "42" {
				return errors.NotFoundf("attachment")
			}
			c.Check(serverId, gc.Equals, mockServerId)
			c.Check(volId, gc.Equals, mockVolId)
			return nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	errs, err := volSource.DetachVolumes(s.callCtx, []storage.VolumeAttachmentParams{{
		Volume:   names.NewVolumeTag("123"),
		VolumeId: mockVolId,
		AttachmentParams: storage.AttachmentParams{
			Machine:    names.NewMachineTag("0"),
			InstanceId: mockServerId,
		},
	}, {
		Volume:   names.NewVolumeTag("42"),
		VolumeId: "42",
		AttachmentParams: storage.AttachmentParams{
			Machine:    names.NewMachineTag("0"),
			InstanceId: mockServerId2,
		},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, jc.DeepEquals, []error{nil, nil})
	c.Assert(s.invalidateCredential, jc.IsFalse)
	mockAdapter.CheckCalls(c, []gitjujutesting.StubCall{
		{"DetachVolume", []interface{}{s.callCtx, mockServerId, mockVolId}},
		{"DetachVolume", []interface{}{s.callCtx, mockServerId2, "42"}},
	})
}

func (s *cinderVolumeSourceSuite) TestCreateVolumeCleanupDestroys(c *gc.C) {
	var numCreateCalls, numDestroyCalls, numGetCalls int
	mockAdapter := &mockAdapter{
		createVolume: func(ctx context.ProviderCallContext, args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
			numCreateCalls++
			if numCreateCalls == 3 {
				return nil, errors.New("no volume for you")
			}
			return &cinder.Volume{
				ID:     fmt.Sprint(numCreateCalls),
				Status: "",
			}, nil
		},
		deleteVolume: func(ctx context.ProviderCallContext, volId string) error {
			numDestroyCalls++
			c.Assert(volId, gc.Equals, "2")
			return errors.New("destroy fails")
		},
		getVolume: func(ctx context.ProviderCallContext, volumeId string) (*cinder.Volume, error) {
			numGetCalls++
			if numGetCalls == 2 {
				return nil, errors.New("no volume details for you")
			}
			return &cinder.Volume{
				ID:     "4",
				Size:   mockVolSize / 1024,
				Status: "available",
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	volumeParams := []storage.VolumeParams{{
		Provider: openstack.CinderProviderType,
		Tag:      names.NewVolumeTag("0"),
		Size:     mockVolSize,
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   openstack.CinderProviderType,
				Machine:    mockMachineTag,
				InstanceId: instance.Id(mockServerId),
			},
		},
	}, {
		Provider: openstack.CinderProviderType,
		Tag:      names.NewVolumeTag("1"),
		Size:     mockVolSize,
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   openstack.CinderProviderType,
				Machine:    mockMachineTag,
				InstanceId: instance.Id(mockServerId),
			},
		},
	}, {
		Provider: openstack.CinderProviderType,
		Tag:      names.NewVolumeTag("2"),
		Size:     mockVolSize,
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   openstack.CinderProviderType,
				Machine:    mockMachineTag,
				InstanceId: instance.Id(mockServerId),
			},
		},
	}}
	results, err := volSource.CreateVolumes(s.callCtx, volumeParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.invalidateCredential, jc.IsFalse)
	c.Assert(results, gc.HasLen, 3)
	c.Assert(results[0].Error, jc.ErrorIsNil)
	c.Assert(results[1].Error, gc.ErrorMatches, "waiting for volume to be provisioned: getting volume: no volume details for you")
	c.Assert(results[2].Error, gc.ErrorMatches, "no volume for you")
	c.Assert(numCreateCalls, gc.Equals, 3)
	c.Assert(numGetCalls, gc.Equals, 2)
	c.Assert(numDestroyCalls, gc.Equals, 1)
}

func (s *cinderVolumeSourceSuite) TestImportVolume(c *gc.C) {
	mockAdapter := &mockAdapter{
		getVolume: func(ctx context.ProviderCallContext, volumeId string) (*cinder.Volume, error) {
			return &cinder.Volume{
				ID:     volumeId,
				Size:   mockVolSize / 1024,
				Status: "available",
			}, nil
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	c.Assert(volSource, gc.Implements, new(storage.VolumeImporter))

	tags := map[string]string{
		"a": "b",
		"c": "d",
	}
	info, err := volSource.(storage.VolumeImporter).ImportVolume(s.callCtx, mockVolId, tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, storage.VolumeInfo{
		VolumeId:   mockVolId,
		Size:       mockVolSize,
		Persistent: true,
	})
	mockAdapter.CheckCalls(c, []gitjujutesting.StubCall{
		{"GetVolume", []interface{}{s.callCtx, mockVolId}},
		{"SetVolumeMetadata", []interface{}{s.callCtx, mockVolId, tags}},
	})
}

func (s *cinderVolumeSourceSuite) TestImportVolumeInvalidCredential(c *gc.C) {
	mockAdapter := &mockAdapter{
		getVolume: func(ctx context.ProviderCallContext, volumeId string) (*cinder.Volume, error) {
			return &cinder.Volume{
				ID:     volumeId,
				Size:   mockVolSize / 1024,
				Status: "available",
			}, gooseerror.NewUnauthorisedf(nil, "", "Unauthorised error.")
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	c.Assert(volSource, gc.Implements, new(storage.VolumeImporter))

	tags := map[string]string{
		"a": "b",
		"c": "d",
	}
	volSource.(storage.VolumeImporter).ImportVolume(s.callCtx, mockVolId, tags)
	//c.Assert(s.invalidateCredential, jc.IsTrue)
}

func (s *cinderVolumeSourceSuite) TestImportVolumeInUse(c *gc.C) {
	mockAdapter := &mockAdapter{
		getVolume: func(ctx context.ProviderCallContext, volumeId string) (*cinder.Volume, error) {
			return &cinder.Volume{
				ID:     volumeId,
				Status: "in-use",
			}, nil
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	_, err := volSource.(storage.VolumeImporter).ImportVolume(s.callCtx, mockVolId, nil)
	c.Assert(err, gc.ErrorMatches, `cannot import volume "0" with status "in-use"`)
	mockAdapter.CheckCalls(c, []gitjujutesting.StubCall{
		{"GetVolume", []interface{}{s.callCtx, mockVolId}},
	})
}

type mockAdapter struct {
	gitjujutesting.Stub
	getVolume             func(context.ProviderCallContext, string) (*cinder.Volume, error)
	getVolumesDetail      func(context.ProviderCallContext) ([]cinder.Volume, error)
	deleteVolume          func(context.ProviderCallContext, string) error
	createVolume          func(context.ProviderCallContext, cinder.CreateVolumeVolumeParams) (*cinder.Volume, error)
	attachVolume          func(context.ProviderCallContext, string, string, string) (*nova.VolumeAttachment, error)
	volumeStatusNotifier  func(context.ProviderCallContext, string, string, int, time.Duration) <-chan error
	detachVolume          func(context.ProviderCallContext, string, string) error
	listVolumeAttachments func(context.ProviderCallContext, string) ([]nova.VolumeAttachment, error)
	setVolumeMetadata     func(context.ProviderCallContext, string, map[string]string) (map[string]string, error)
}

func (ma *mockAdapter) GetVolume(ctx context.ProviderCallContext, volumeId string) (*cinder.Volume, error) {
	ma.MethodCall(ma, "GetVolume", ctx, volumeId)
	if ma.getVolume != nil {
		return ma.getVolume(ctx, volumeId)
	}
	return &cinder.Volume{
		ID:     volumeId,
		Status: "available",
	}, nil
}

func (ma *mockAdapter) GetVolumesDetail(ctx context.ProviderCallContext) ([]cinder.Volume, error) {
	ma.MethodCall(ma, "GetVolumesDetail", ctx)
	if ma.getVolumesDetail != nil {
		return ma.getVolumesDetail(ctx)
	}
	return nil, nil
}

func (ma *mockAdapter) DeleteVolume(ctx context.ProviderCallContext, volId string) error {
	ma.MethodCall(ma, "DeleteVolume", ctx, volId)
	if ma.deleteVolume != nil {
		return ma.deleteVolume(ctx, volId)
	}
	return nil
}

func (ma *mockAdapter) CreateVolume(ctx context.ProviderCallContext, args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
	ma.MethodCall(ma, "CreateVolume", ctx, args)
	if ma.createVolume != nil {
		return ma.createVolume(ctx, args)
	}
	return nil, errors.NotImplementedf("CreateVolume")
}

func (ma *mockAdapter) AttachVolume(ctx context.ProviderCallContext, serverId, volumeId, mountPoint string) (*nova.VolumeAttachment, error) {
	ma.MethodCall(ma, "AttachVolume", ctx, serverId, volumeId, mountPoint)
	if ma.attachVolume != nil {
		return ma.attachVolume(ctx, serverId, volumeId, mountPoint)
	}
	return nil, errors.NotImplementedf("AttachVolume")
}

func (ma *mockAdapter) DetachVolume(ctx context.ProviderCallContext, serverId, attachmentId string) error {
	ma.MethodCall(ma, "DetachVolume", ctx, serverId, attachmentId)
	if ma.detachVolume != nil {
		return ma.detachVolume(ctx, serverId, attachmentId)
	}
	return nil
}

func (ma *mockAdapter) ListVolumeAttachments(ctx context.ProviderCallContext, serverId string) ([]nova.VolumeAttachment, error) {
	ma.MethodCall(ma, "ListVolumeAttachments", ctx, serverId)
	if ma.listVolumeAttachments != nil {
		return ma.listVolumeAttachments(ctx, serverId)
	}
	return nil, nil
}

func (ma *mockAdapter) SetVolumeMetadata(ctx context.ProviderCallContext, volumeId string, metadata map[string]string) (map[string]string, error) {
	ma.MethodCall(ma, "SetVolumeMetadata", ctx, volumeId, metadata)
	if ma.setVolumeMetadata != nil {
		return ma.setVolumeMetadata(ctx, volumeId, metadata)
	}
	return nil, nil
}

type testEndpointResolver struct {
	authenticated   bool
	regionEndpoints map[string]identity.ServiceURLs
}

func (r *testEndpointResolver) IsAuthenticated() bool {
	return r.authenticated
}

func (r *testEndpointResolver) Authenticate() error {
	r.authenticated = true
	return nil
}

func (r *testEndpointResolver) EndpointsForRegion(region string) identity.ServiceURLs {
	if !r.authenticated {
		return identity.ServiceURLs{}
	}
	return r.regionEndpoints[region]
}

func (s *cinderVolumeSourceSuite) TestGetVolumeEndpointVolume(c *gc.C) {
	var ctx context.ProviderCallContext
	client := &testEndpointResolver{regionEndpoints: map[string]identity.ServiceURLs{
		"west": map[string]string{"volume": "http://cinder.testing/v1"},
	}}
	url, err := openstack.GetVolumeEndpointURL(ctx, client, "west")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url.String(), gc.Equals, "http://cinder.testing/v1")
}

func (s *cinderVolumeSourceSuite) TestGetVolumeEndpointVolumeV2(c *gc.C) {
	var ctx context.ProviderCallContext
	client := &testEndpointResolver{regionEndpoints: map[string]identity.ServiceURLs{
		"west": map[string]string{"volumev2": "http://cinder.testing/v2"},
	}}
	url, err := openstack.GetVolumeEndpointURL(ctx, client, "west")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url.String(), gc.Equals, "http://cinder.testing/v2")
}

func (s *cinderVolumeSourceSuite) TestGetVolumeEndpointPreferV2(c *gc.C) {
	var ctx context.ProviderCallContext
	client := &testEndpointResolver{regionEndpoints: map[string]identity.ServiceURLs{
		"south": map[string]string{
			"volume":   "http://cinder.testing/v1",
			"volumev2": "http://cinder.testing/v2",
		},
	}}
	url, err := openstack.GetVolumeEndpointURL(ctx, client, "south")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url.String(), gc.Equals, "http://cinder.testing/v2")
}

func (s *cinderVolumeSourceSuite) TestGetVolumeEndpointMissing(c *gc.C) {
	var ctx context.ProviderCallContext
	client := &testEndpointResolver{}
	url, err := openstack.GetVolumeEndpointURL(ctx, client, "east")
	c.Assert(err, gc.ErrorMatches, `endpoint "volume" in region "east" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(url, gc.IsNil)
}

func (s *cinderVolumeSourceSuite) TestGetVolumeEndpointBadURL(c *gc.C) {
	var ctx context.ProviderCallContext
	client := &testEndpointResolver{regionEndpoints: map[string]identity.ServiceURLs{
		"north": map[string]string{"volumev2": "some %4"},
	}}
	url, err := openstack.GetVolumeEndpointURL(ctx, client, "north")
	c.Assert(err, gc.ErrorMatches, `parse some %4: .*`)
	c.Assert(url, gc.IsNil)
}
