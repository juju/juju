package openstack_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/goose.v1/cinder"
	"gopkg.in/goose.v1/nova"

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
	attachments, err := volSource.AttachVolumes([]storage.VolumeAttachmentParams{{
		Volume:   mockVolumeTag,
		VolumeId: mockVolId,
		AttachmentParams: storage.AttachmentParams{
			Provider:   openstack.CinderProviderType,
			Machine:    mockMachineTag,
			InstanceId: instance.Id(mockServerId),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(attachments, jc.DeepEquals, []storage.VolumeAttachment{{
		Volume:     mockVolumeTag,
		Machine:    mockMachineTag,
		DeviceName: "sda",
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
				Name: "volume-123",
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
	volumes, attachments, err := volSource.CreateVolumes([]storage.VolumeParams{{
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
	c.Check(volumes, jc.DeepEquals, []storage.Volume{{
		VolumeId:   mockVolId,
		Tag:        mockVolumeTag,
		Size:       providedSize,
		Persistent: true,
	}})
	c.Check(attachments, jc.DeepEquals, []storage.VolumeAttachment{{
		Volume:     mockVolumeTag,
		Machine:    mockMachineTag,
		DeviceName: "sda",
	}})

	// should have been 3 calls to GetVolume: twice initially
	// to wait until the volume became available, and then
	// again to check if it was available before attaching.
	c.Check(getVolumeCalls, gc.Equals, 3)
}

func (s *cinderVolumeSourceSuite) TestDescribeVolumes(c *gc.C) {
	mockAdapter := &mockAdapter{
		getVolumesSimple: func() ([]cinder.Volume, error) {
			return []cinder.Volume{{
				ID:   mockVolId,
				Size: mockVolSize / 1024,
			}}, nil
		},
	}
	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	volumes, err := volSource.DescribeVolumes([]string{mockVolId})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(volumes, jc.DeepEquals, []storage.Volume{{
		VolumeId:   mockVolId,
		Size:       mockVolSize,
		Persistent: true,
	}})
}

func (s *cinderVolumeSourceSuite) TestDestroyVolumes(c *gc.C) {
	var numCalls int
	mockAdapter := &mockAdapter{
		deleteVolume: func(volId string) error {
			numCalls++
			c.Check(volId, gc.Equals, mockVolId)
			return nil
		},
	}

	volSource := openstack.NewCinderVolumeSource(mockAdapter)
	errs := volSource.DestroyVolumes([]string{mockVolId})
	c.Assert(numCalls, gc.Equals, 1)
	c.Assert(errs, jc.DeepEquals, []error{nil})
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
	err := volSource.DetachVolumes([]storage.VolumeAttachmentParams{{
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
	c.Assert(numListCalls, gc.Equals, 2)
	// DetachVolume should only be called for existing attachments.
	c.Assert(numDetachCalls, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cinderVolumeSourceSuite) TestCreateVolumeCleanupDestroys(c *gc.C) {
	var numCreateCalls, numAttachCalls, numDetachCalls, numDestroyCalls int
	mockAdapter := &mockAdapter{
		createVolume: func(args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
			numCreateCalls++
			if numCreateCalls == 3 {
				return nil, errors.New("no volume for you")
			}
			return &cinder.Volume{
				ID:   fmt.Sprint(numCreateCalls),
				Size: mockVolSize / 1024,
			}, nil
		},
		attachVolume: func(serverId, volId, mountPoint string) (*nova.VolumeAttachment, error) {
			numAttachCalls++
			if numAttachCalls == 2 {
				return nil, errors.New("no attach for you")
			}
			return &nova.VolumeAttachment{
				Id:       volId,
				VolumeId: volId,
				ServerId: serverId,
				Device:   "/dev/sda" + volId,
			}, nil
		},
		detachVolume: func(serverId, volId string) error {
			numDetachCalls++
			return errors.New("detach fails")
		},
		deleteVolume: func(volId string) error {
			numDestroyCalls++
			return errors.New("destroy fails")
		},
		listVolumeAttachments: func(serverId string) ([]nova.VolumeAttachment, error) {
			return []nova.VolumeAttachment{{
				Id:       "4",
				VolumeId: "4",
				ServerId: serverId,
				Device:   "/dev/sda",
			}}, nil
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
	volumes, attachments, err := volSource.CreateVolumes(volumeParams)
	c.Assert(err, gc.ErrorMatches, "no volume for you")
	c.Assert(volumes, gc.IsNil)
	c.Assert(attachments, gc.IsNil)
	c.Assert(numCreateCalls, gc.Equals, 3)
	c.Assert(numDestroyCalls, gc.Equals, 2)

	// Second time around, the create calls should all succeed
	// but the second attach should fail. This will cause the
	// volumes to be detached and destroyed. One of the detachments
	// fails, so we should only see two destroy calls.
	_, _, err = volSource.CreateVolumes(volumeParams)
	c.Assert(err, gc.ErrorMatches, "no attach for you")
	c.Assert(numCreateCalls, gc.Equals, 6)
	c.Assert(numAttachCalls, gc.Equals, 2)
	c.Assert(numDetachCalls, gc.Equals, 1)
	c.Assert(numDestroyCalls, gc.Equals, 4)
}

type mockAdapter struct {
	getVolume             func(string) (*cinder.Volume, error)
	getVolumesSimple      func() ([]cinder.Volume, error)
	deleteVolume          func(string) error
	createVolume          func(cinder.CreateVolumeVolumeParams) (*cinder.Volume, error)
	attachVolume          func(string, string, string) (*nova.VolumeAttachment, error)
	volumeStatusNotifier  func(string, string, int, time.Duration) <-chan error
	detachVolume          func(string, string) error
	listVolumeAttachments func(string) ([]nova.VolumeAttachment, error)
}

func (ma *mockAdapter) GetVolume(volumeId string) (*cinder.Volume, error) {
	if ma.getVolume != nil {
		return ma.getVolume(volumeId)
	}
	return &cinder.Volume{
		ID:     volumeId,
		Status: "available",
	}, nil
}

func (ma *mockAdapter) GetVolumesSimple() ([]cinder.Volume, error) {
	if ma.getVolumesSimple != nil {
		return ma.getVolumesSimple()
	}
	return nil, nil
}

func (ma *mockAdapter) DeleteVolume(volId string) error {
	if ma.deleteVolume != nil {
		return ma.deleteVolume(volId)
	}
	return nil
}

func (ma *mockAdapter) CreateVolume(args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
	if ma.createVolume != nil {
		return ma.createVolume(args)
	}
	return nil, errors.NotImplementedf("CreateVolume")
}

func (ma *mockAdapter) AttachVolume(serverId, volumeId, mountPoint string) (*nova.VolumeAttachment, error) {
	if ma.attachVolume != nil {
		return ma.attachVolume(serverId, volumeId, mountPoint)
	}
	return nil, errors.NotImplementedf("AttachVolume")
}

func (ma *mockAdapter) DetachVolume(serverId, attachmentId string) error {
	if ma.detachVolume != nil {
		return ma.detachVolume(serverId, attachmentId)
	}
	return nil
}

func (ma *mockAdapter) ListVolumeAttachments(serverId string) ([]nova.VolumeAttachment, error) {
	if ma.listVolumeAttachments != nil {
		return ma.listVolumeAttachments(serverId)
	}
	return nil, nil
}
