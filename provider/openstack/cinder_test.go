package openstack_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/goose.v1/cinder"
	"gopkg.in/goose.v1/nova"

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
}

func init() {
	// Override attempt strategy to speed things up.
	openstack.CinderAttempt.Delay = 0
}

func (s *cinderVolumeSourceSuite) TestAttachVolumes(c *gc.C) {
	mockAdapter := &mockAdapter{
		attachVolume: func(serverId, volId, mountPoint string) (*nova.VolumeAttachment, error) {
			c.Check(volId, gc.Equals, mockVolId)
			c.Check(serverId, gc.Equals, mockServerId)
			return &nova.VolumeAttachment{
				Id:       volId,
				VolumeId: volId,
				ServerId: serverId,
				Device:   "/dev/sda",
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	results, err := volSource.AttachVolumes([]storage.VolumeAttachmentParams{{
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

func (s *cinderVolumeSourceSuite) TestCreateVolume(c *gc.C) {
	const (
		requestedSize = 2 * 1024
		providedSize  = 3 * 1024
	)

	s.PatchValue(openstack.CinderAttempt, utils.AttemptStrategy{Min: 3})

	var getVolumeCalls int
	mockAdapter := &mockAdapter{
		createVolume: func(args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
			c.Assert(args, jc.DeepEquals, cinder.CreateVolumeVolumeParams{
				Size: requestedSize / 1024,
				Name: "juju-testenv-volume-123",
			})
			return &cinder.Volume{
				ID: mockVolId,
			}, nil
		},
		getVolume: func(volumeId string) (*cinder.Volume, error) {
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
		attachVolume: func(serverId, volId, mountPoint string) (*nova.VolumeAttachment, error) {
			c.Check(volId, gc.Equals, mockVolId)
			c.Check(serverId, gc.Equals, mockServerId)
			return &nova.VolumeAttachment{
				Id:       volId,
				VolumeId: volId,
				ServerId: serverId,
				Device:   "/dev/sda",
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	results, err := volSource.CreateVolumes([]storage.VolumeParams{{
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

func (s *cinderVolumeSourceSuite) TestResourceTags(c *gc.C) {
	var created bool
	mockAdapter := &mockAdapter{
		createVolume: func(args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
			created = true
			c.Assert(args, jc.DeepEquals, cinder.CreateVolumeVolumeParams{
				Size: 1,
				Name: "juju-testenv-volume-123",
				Metadata: map[string]string{
					"ResourceTag1": "Value1",
					"ResourceTag2": "Value2",
				},
			})
			return &cinder.Volume{ID: mockVolId}, nil
		},
		getVolume: func(volumeId string) (*cinder.Volume, error) {
			return &cinder.Volume{
				ID:     volumeId,
				Size:   1,
				Status: "available",
			}, nil
		},
		attachVolume: func(serverId, volId, mountPoint string) (*nova.VolumeAttachment, error) {
			return &nova.VolumeAttachment{
				Id:       volId,
				VolumeId: volId,
				ServerId: serverId,
				Device:   "/dev/sda",
			}, nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	_, err := volSource.CreateVolumes([]storage.VolumeParams{{
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

func (s *cinderVolumeSourceSuite) TestListVolumes(c *gc.C) {
	mockAdapter := &mockAdapter{
		getVolumesDetail: func() ([]cinder.Volume, error) {
			return []cinder.Volume{{
				ID: "volume-1",
			}, {
				ID: "volume-2",
				Metadata: map[string]string{
					tags.JujuEnv: "something-else",
				},
			}, {
				ID: "volume-3",
				Metadata: map[string]string{
					tags.JujuEnv: testing.EnvironmentTag.Id(),
				},
			}}, nil
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	volumeIds, err := volSource.ListVolumes()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(volumeIds, jc.DeepEquals, []string{"volume-3"})
}

func (s *cinderVolumeSourceSuite) TestDescribeVolumes(c *gc.C) {
	mockAdapter := &mockAdapter{
		getVolumesDetail: func() ([]cinder.Volume, error) {
			return []cinder.Volume{{
				ID:   mockVolId,
				Size: mockVolSize / 1024,
			}}, nil
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	volumes, err := volSource.DescribeVolumes([]string{mockVolId})
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
	errs, err := volSource.DestroyVolumes([]string{mockVolId})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, jc.DeepEquals, []error{nil})
	mockAdapter.CheckCalls(c, []gitjujutesting.StubCall{
		{"GetVolume", []interface{}{mockVolId}},
		{"DeleteVolume", []interface{}{mockVolId}},
	})
}

func (s *cinderVolumeSourceSuite) TestDestroyVolumesAttached(c *gc.C) {
	statuses := []string{"in-use", "detaching", "available"}

	mockAdapter := &mockAdapter{
		getVolume: func(volId string) (*cinder.Volume, error) {
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
	errs, err := volSource.DestroyVolumes([]string{mockVolId})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.HasLen, 1)
	c.Assert(errs[0], jc.ErrorIsNil)
	c.Assert(statuses, gc.HasLen, 0)
	mockAdapter.CheckCalls(c, []gitjujutesting.StubCall{{
		"GetVolume", []interface{}{mockVolId},
	}, {
		"GetVolume", []interface{}{mockVolId},
	}, {
		"GetVolume", []interface{}{mockVolId},
	}, {
		"DeleteVolume", []interface{}{mockVolId},
	}})
}

func (s *cinderVolumeSourceSuite) TestDetachVolumes(c *gc.C) {
	const mockServerId2 = mockServerId + "2"

	var numListCalls, numDetachCalls int
	mockAdapter := &mockAdapter{
		listVolumeAttachments: func(serverId string) ([]nova.VolumeAttachment, error) {
			numListCalls++
			if serverId == mockServerId2 {
				// no attachments
				return nil, nil
			}
			c.Check(serverId, gc.Equals, mockServerId)
			return []nova.VolumeAttachment{{
				Id:       mockVolId,
				VolumeId: mockVolId,
				ServerId: mockServerId,
				Device:   "/dev/sda",
			}}, nil
		},
		detachVolume: func(serverId, volId string) error {
			numDetachCalls++
			c.Check(serverId, gc.Equals, mockServerId)
			c.Check(volId, gc.Equals, mockVolId)
			return nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	errs, err := volSource.DetachVolumes([]storage.VolumeAttachmentParams{{
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
	// DetachVolume should only be called for existing attachments.
	mockAdapter.CheckCalls(c, []gitjujutesting.StubCall{{
		"ListVolumeAttachments", []interface{}{mockServerId},
	}, {
		"DetachVolume", []interface{}{mockServerId, mockVolId},
	}, {
		"ListVolumeAttachments", []interface{}{mockServerId2},
	}})
}

func (s *cinderVolumeSourceSuite) TestCreateVolumeCleanupDestroys(c *gc.C) {
	var numCreateCalls, numDestroyCalls, numGetCalls int
	mockAdapter := &mockAdapter{
		createVolume: func(args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
			numCreateCalls++
			if numCreateCalls == 3 {
				return nil, errors.New("no volume for you")
			}
			return &cinder.Volume{
				ID:     fmt.Sprint(numCreateCalls),
				Status: "",
			}, nil
		},
		deleteVolume: func(volId string) error {
			numDestroyCalls++
			c.Assert(volId, gc.Equals, "2")
			return errors.New("destroy fails")
		},
		getVolume: func(volumeId string) (*cinder.Volume, error) {
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
	results, err := volSource.CreateVolumes(volumeParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 3)
	c.Assert(results[0].Error, jc.ErrorIsNil)
	c.Assert(results[1].Error, gc.ErrorMatches, "waiting for volume to be provisioned: getting volume: no volume details for you")
	c.Assert(results[2].Error, gc.ErrorMatches, "no volume for you")
	c.Assert(numCreateCalls, gc.Equals, 3)
	c.Assert(numGetCalls, gc.Equals, 2)
	c.Assert(numDestroyCalls, gc.Equals, 1)
}

type mockAdapter struct {
	gitjujutesting.Stub
	getVolume             func(string) (*cinder.Volume, error)
	getVolumesDetail      func() ([]cinder.Volume, error)
	deleteVolume          func(string) error
	createVolume          func(cinder.CreateVolumeVolumeParams) (*cinder.Volume, error)
	attachVolume          func(string, string, string) (*nova.VolumeAttachment, error)
	volumeStatusNotifier  func(string, string, int, time.Duration) <-chan error
	detachVolume          func(string, string) error
	listVolumeAttachments func(string) ([]nova.VolumeAttachment, error)
}

func (ma *mockAdapter) GetVolume(volumeId string) (*cinder.Volume, error) {
	ma.MethodCall(ma, "GetVolume", volumeId)
	if ma.getVolume != nil {
		return ma.getVolume(volumeId)
	}
	return &cinder.Volume{
		ID:     volumeId,
		Status: "available",
	}, nil
}

func (ma *mockAdapter) GetVolumesDetail() ([]cinder.Volume, error) {
	ma.MethodCall(ma, "GetVolumesDetail")
	if ma.getVolumesDetail != nil {
		return ma.getVolumesDetail()
	}
	return nil, nil
}

func (ma *mockAdapter) DeleteVolume(volId string) error {
	ma.MethodCall(ma, "DeleteVolume", volId)
	if ma.deleteVolume != nil {
		return ma.deleteVolume(volId)
	}
	return nil
}

func (ma *mockAdapter) CreateVolume(args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
	ma.MethodCall(ma, "CreateVolume", args)
	if ma.createVolume != nil {
		return ma.createVolume(args)
	}
	return nil, errors.NotImplementedf("CreateVolume")
}

func (ma *mockAdapter) AttachVolume(serverId, volumeId, mountPoint string) (*nova.VolumeAttachment, error) {
	ma.MethodCall(ma, "AttachVolume", serverId, volumeId, mountPoint)
	if ma.attachVolume != nil {
		return ma.attachVolume(serverId, volumeId, mountPoint)
	}
	return nil, errors.NotImplementedf("AttachVolume")
}

func (ma *mockAdapter) DetachVolume(serverId, attachmentId string) error {
	ma.MethodCall(ma, "DetachVolume", serverId, attachmentId)
	if ma.detachVolume != nil {
		return ma.detachVolume(serverId, attachmentId)
	}
	return nil
}

func (ma *mockAdapter) ListVolumeAttachments(serverId string) ([]nova.VolumeAttachment, error) {
	ma.MethodCall(ma, "ListVolumeAttachments", serverId)
	if ma.listVolumeAttachments != nil {
		return ma.listVolumeAttachments(serverId)
	}
	return nil, nil
}
