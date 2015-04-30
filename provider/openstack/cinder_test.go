package openstack_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/goose.v1/cinder"
	"gopkg.in/goose.v1/nova"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/openstack"
	"github.com/juju/juju/storage"
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

type cinderVolumeSourceSuite struct{}

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

	mockAdapter := &mockAdapter{
		createVolume: func(args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
			c.Assert(args, jc.DeepEquals, cinder.CreateVolumeVolumeParams{
				Name: "volume-123",
				Size: requestedSize / 1024,
			})
			return &cinder.Volume{
				ID:   mockVolId,
				Name: "volume-123",
				Size: providedSize / 1024,
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
}

func (s *cinderVolumeSourceSuite) TestDescribeVolumes(c *gc.C) {

	mockAdapter := &mockAdapter{
		getVolumesSimple: func() ([]cinder.Volume, error) {
			return []cinder.Volume{{
				ID:   mockVolId,
				Name: "volume-123",
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

type mockAdapter struct {
	getVolumesSimple      func() ([]cinder.Volume, error)
	deleteVolume          func(string) error
	createVolume          func(cinder.CreateVolumeVolumeParams) (*cinder.Volume, error)
	attachVolume          func(string, string, string) (*nova.VolumeAttachment, error)
	volumeStatusNotifier  func(string, string, int, time.Duration) <-chan error
	detachVolume          func(string, string) error
	listVolumeAttachments func(string) ([]nova.VolumeAttachment, error)
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

func (ma *mockAdapter) VolumeStatusNotifier(volId, status string, numAttempts int, waitDur time.Duration) <-chan error {
	if ma.volumeStatusNotifier != nil {
		return ma.volumeStatusNotifier(volId, status, numAttempts, waitDur)
	}
	emptyChan := make(chan error)
	close(emptyChan)
	return emptyChan
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

//var _ = gc.Suite(&gooseAdapterSuite{})
//
//type gooseAdapterSuite struct{}
//
//func (s *gooseAdapterSuite) TestCreateVolume(c *gc.C) {
//
//	numCalls := 0
//	cinderHandler := func(req *http.Request) (*http.Response, error) {
//		numCalls++
//
//		bodyBytes, err := ioutil.ReadAll(req.Body)
//		c.Assert(err, jc.ErrorIsNil)
//		body := string(bodyBytes)
//
//		c.Check(body, gc.Equals, `{"volume":{"size":1,"name":"`+names.NewVolumeTag(mockVolId).String()+`"}}`)
//
//		return &http.Response{
//			StatusCode: 202,
//			Body:       ioutil.NopCloser(bytes.NewBufferString(mockVolJson)),
//		}, nil
//	}
//
//	gooseAdapter := newTestGooseAdapter(cinderHandler, nil)
//
//	vol, err := gooseAdapter.CreateVolume(1024, names.NewVolumeTag(mockVolId))
//	c.Assert(numCalls, gc.Equals, 1)
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(vol, gc.NotNil)
//}
//
//func (s *gooseAdapterSuite) TestDeleteVolume(c *gc.C) {
//
//	numCalls := 0
//	cinderHandler := func(req *http.Request) (*http.Response, error) {
//		numCalls++
//
//		c.Check(req.URL.String(), gc.Matches, ".*/volumes/"+mockVolId+"$")
//
//		return &http.Response{
//			StatusCode: 202,
//			Body:       ioutil.NopCloser(bytes.NewBufferString(mockVolJson)),
//		}, nil
//	}
//
//	gooseAdapter := newTestGooseAdapter(cinderHandler, nil)
//
//	err := gooseAdapter.DeleteVolume(mockVolId)
//	c.Assert(numCalls, gc.Equals, 1)
//	c.Assert(err, jc.ErrorIsNil)
//}
//
//func (s *gooseAdapterSuite) TestGetVolumesSimple(c *gc.C) {
//
//	numCalls := 0
//	cinderHandler := func(req *http.Request) (*http.Response, error) {
//		numCalls++
//
//		resp := `{"volumes":[{"id": "` + mockVolId + `","name": "` + mockVolName + `"}]}`
//
//		return &http.Response{
//			StatusCode: 200,
//			Body:       ioutil.NopCloser(bytes.NewBufferString(resp)),
//		}, nil
//	}
//
//	gooseAdapter := newTestGooseAdapter(cinderHandler, nil)
//
//	vols, err := gooseAdapter.GetVolumesSimple()
//	c.Assert(numCalls, gc.Equals, 1)
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(vols, gc.HasLen, 1)
//}
//
//func (s *gooseAdapterSuite) TestAttachVolume(c *gc.C) {
//
//	numCalls := 0
//	novaHandler := &mockNovaClient{
//		sendRequest: func(method, svcType, apiCall string, reqData *goosehttp.RequestData) error {
//			numCalls++
//
//			c.Check(apiCall, gc.Equals, "servers/"+mockServerId+"/os-volume_attachments")
//
//			attachment := reqData.RespValue.(*nova.VolumeAttachment)
//			attachment.ServerId = mockServerId
//			attachment.VolumeId = mockVolId
//
//			reqData.RespValue = attachment
//			return nil
//		},
//	}
//
//	gooseAdapter := newTestGooseAdapter(nil, novaHandler)
//
//	attachment, err := gooseAdapter.AttachVolume(mockServerId, mockVolId, "/dev/sda")
//	c.Assert(numCalls, gc.Equals, 1)
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(attachment, gc.NotNil)
//
//	c.Check(attachment.Volume, gc.Equals, names.NewVolumeTag(mockVolId))
//	c.Check(attachment.Machine, gc.Equals, names.NewMachineTag(mockServerId))
//}
//
//type mockNovaClient struct {
//	sendRequest    func(string, string, string, *goosehttp.RequestData) error
//	makeServiceUrl func(string, []string) (string, error)
//}
//
//func (c *mockNovaClient) SendRequest(method, svcType, apiCall string, requestData *goosehttp.RequestData) (err error) {
//	if c.sendRequest != nil {
//		return c.sendRequest(method, svcType, apiCall, requestData)
//	}
//	return nil
//}
//
//func (c *mockNovaClient) MakeServiceURL(serviceType string, parts []string) (string, error) {
//	if c.makeServiceUrl != nil {
//		return c.makeServiceUrl(serviceType, parts)
//	}
//	return "", nil
//}
//
//func newTestGooseAdapter(
//	cinderHandler cinder.RequestHandlerFn,
//	novaHandler client.Client,
//) *gooseAdapter {
//	return &gooseAdapter{
//		cinder: cinder.NewClient("mock-tenant-id", cinderHandler),
//		nova:   nova.New(novaHandler),
//	}
//}
