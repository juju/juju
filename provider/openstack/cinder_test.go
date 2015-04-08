package openstack

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/goose.v1/cinder"
	"gopkg.in/goose.v1/client"
	goosehttp "gopkg.in/goose.v1/http"
	"gopkg.in/goose.v1/nova"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
)

const (
	mockVolId           = "0"
	mockVolSize  uint64 = 1024 * 2
	mockVolName         = "mock-volume-name"
	mockServerId        = "mock-server-id"
	mockVolJson         = `{"volume":{"id": "` + mockVolId + `", "size":1,"name":"` + mockVolName + `"}}`
)

var _ = gc.Suite(&openstackSuite{})

type openstackSuite struct{}

func (s *openstackSuite) TestVolumeSource(c *gc.C) {

	c.Skip("no longer validating configs")

	p := &OpenstackProvider{mockClientFactoryFn(nil)}

	// Check that we're validating the config passed in.
	cfg, err := storage.NewConfig("openstack", CinderProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = p.VolumeSource(nil, cfg)
	c.Check(err, gc.ErrorMatches, "requisite configuration was not set: auth-url not assigned")

	// cfg, err = NewOpenstackStorageConfig("", "", "", "", "")
	// c.Assert(err, jc.ErrorIsNil)

	// volSource, err := p.VolumeSource(nil, cfg)
	// c.Assert(err, jc.ErrorIsNil)
	// c.Check(volSource, gc.NotNil)
}

func (s *openstackSuite) TestAttachVolumes(c *gc.C) {

	numCalls := 0
	mockAdapter := &mockAdapter{
		attachVolume: func(serverId, volId, mountPoint string) (storage.VolumeAttachment, error) {
			numCalls++

			c.Check(volId, gc.Equals, mockVolId)
			c.Check(serverId, gc.Equals, mockServerId)

			return storage.VolumeAttachment{
				Volume:     names.NewVolumeTag(volId),
				Machine:    names.NewMachineTag(serverId),
				DeviceName: "device-name",
			}, nil
		},
	}

	p := &OpenstackProvider{mockClientFactoryFn(mockAdapter)}
	cfg, err := NewOpenstackStorageConfig("", "", "", "", "")
	c.Assert(err, jc.ErrorIsNil)

	volSource, err := p.VolumeSource(nil, cfg)
	c.Assert(err, jc.ErrorIsNil)

	attachments, err := volSource.AttachVolumes([]storage.VolumeAttachmentParams{{
		VolumeId: mockVolId,
		AttachmentParams: storage.AttachmentParams{
			Provider:   CinderProviderType,
			Machine:    names.NewMachineTag("mock-machine-name"),
			InstanceId: instance.Id(mockServerId),
		}},
	})
	c.Assert(numCalls, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(attachments, gc.HasLen, 1)
}

func (s *openstackSuite) TestCreateVolume(c *gc.C) {

	numCalls := 0
	mockAdapter := &mockAdapter{
		createVolume: func(size uint64, tag names.VolumeTag) (storage.Volume, error) {
			numCalls++
			return storage.Volume{
				VolumeId: mockVolId,
				Tag:      tag,
				Size:     size,
			}, nil
		},
	}

	p := &OpenstackProvider{mockClientFactoryFn(mockAdapter)}
	cfg, err := NewOpenstackStorageConfig("", "", "", "", "")
	c.Assert(err, jc.ErrorIsNil)

	volSource, err := p.VolumeSource(nil, cfg)
	c.Assert(err, jc.ErrorIsNil)

	vols, attachments, err := volSource.CreateVolumes([]storage.VolumeParams{{
		Provider: CinderProviderType,
		Tag:      names.NewVolumeTag(mockVolId),
		Size:     mockVolSize,
	}})
	c.Assert(numCalls, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(attachments, gc.HasLen, 0)
	c.Assert(vols, gc.HasLen, 1)

	c.Check(vols[0].VolumeId, gc.Equals, mockVolId)
	c.Check(vols[0].Size, gc.Equals, mockVolSize)
	c.Check(vols[0].Tag, gc.Equals, names.NewVolumeTag(mockVolId))
}

func (s *openstackSuite) TestDescribeVolumes(c *gc.C) {

	numCalls := 0
	mockAdapter := &mockAdapter{
		getVolumesSimple: func() ([]storage.Volume, error) {
			numCalls++

			return []storage.Volume{{
				VolumeId: mockVolId,
				Tag:      names.NewVolumeTag(mockVolId),
				Size:     mockVolSize,
			}}, nil
		},
	}

	p := &OpenstackProvider{mockClientFactoryFn(mockAdapter)}
	cfg, err := NewOpenstackStorageConfig("", "", "", "", "")
	c.Assert(err, jc.ErrorIsNil)

	volSource, err := p.VolumeSource(nil, cfg)
	c.Assert(err, jc.ErrorIsNil)

	blockDevices, err := volSource.DescribeVolumes([]string{mockVolId})
	c.Assert(numCalls, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockDevices, gc.HasLen, 1)

	c.Check(blockDevices[0].VolumeId, gc.Equals, mockVolId)
	c.Check(blockDevices[0].Tag, gc.Equals, names.NewVolumeTag(mockVolId))
	c.Check(blockDevices[0].Size, gc.Equals, mockVolSize)
}

func (s *openstackSuite) TestDestroyVolumes(c *gc.C) {

	numCalls := 0
	mockAdapter := &mockAdapter{
		deleteVolume: func(volId string) error {
			numCalls++

			c.Check(volId, gc.Equals, mockVolId)
			return nil
		},
	}

	p := &OpenstackProvider{mockClientFactoryFn(mockAdapter)}
	cfg, err := NewOpenstackStorageConfig("", "", "", "", "")
	c.Assert(err, jc.ErrorIsNil)

	volSource, err := p.VolumeSource(nil, cfg)
	c.Assert(err, jc.ErrorIsNil)

	errs := volSource.DestroyVolumes([]string{mockVolId})
	c.Assert(numCalls, gc.Equals, 1)
	c.Assert(errs, gc.HasLen, 1)
	c.Check(errs[0], jc.ErrorIsNil)
}

func (s *openstackSuite) TestDetachVolumes(c *gc.C) {
	c.Skip("not yet implemented")
}

func mockClientFactoryFn(adapter OpenstackAdapter) func(*config.Config) (OpenstackAdapter, error) {
	return func(*config.Config) (OpenstackAdapter, error) {
		return adapter, nil
	}
}

type mockAdapter struct {
	getVolumesSimple      func() ([]storage.Volume, error)
	deleteVolume          func(string) error
	createVolume          func(uint64, names.VolumeTag) (storage.Volume, error)
	attachVolume          func(string, string, string) (storage.VolumeAttachment, error)
	volumeStatusNotifier  func(string, string, int, time.Duration) <-chan error
	detachVolume          func(string, string) error
	listVolumeAttachments func(string) ([]storage.VolumeAttachment, error)
}

func (ma *mockAdapter) GetVolumesSimple() ([]storage.Volume, error) {
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

func (ma *mockAdapter) CreateVolume(size uint64, tag names.VolumeTag) (storage.Volume, error) {
	if ma.createVolume != nil {
		return ma.createVolume(size, tag)
	}
	return storage.Volume{}, nil
}

func (ma *mockAdapter) AttachVolume(serverId, volumeId, mountPoint string) (storage.VolumeAttachment, error) {
	if ma.attachVolume != nil {
		return ma.attachVolume(serverId, volumeId, mountPoint)
	}
	return storage.VolumeAttachment{}, nil
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
func (ma *mockAdapter) ListVolumeAttachments(serverId string) ([]storage.VolumeAttachment, error) {
	if ma.listVolumeAttachments != nil {
		return ma.listVolumeAttachments(serverId)
	}
	return nil, nil
}

var _ = gc.Suite(&gooseAdapterSuite{})

type gooseAdapterSuite struct{}

func (s *gooseAdapterSuite) TestCreateVolume(c *gc.C) {

	numCalls := 0
	cinderHandler := func(req *http.Request) (*http.Response, error) {
		numCalls++

		bodyBytes, err := ioutil.ReadAll(req.Body)
		c.Assert(err, jc.ErrorIsNil)
		body := string(bodyBytes)

		c.Check(body, gc.Equals, `{"volume":{"size":1,"name":"`+names.NewVolumeTag(mockVolId).String()+`"}}`)

		return &http.Response{
			StatusCode: 202,
			Body:       ioutil.NopCloser(bytes.NewBufferString(mockVolJson)),
		}, nil
	}

	gooseAdapter := newTestGooseAdapter(cinderHandler, nil)

	vol, err := gooseAdapter.CreateVolume(1024, names.NewVolumeTag(mockVolId))
	c.Assert(numCalls, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vol, gc.NotNil)
}

func (s *gooseAdapterSuite) TestDeleteVolume(c *gc.C) {

	numCalls := 0
	cinderHandler := func(req *http.Request) (*http.Response, error) {
		numCalls++

		c.Check(req.URL.String(), gc.Matches, ".*/volumes/"+mockVolId+"$")

		return &http.Response{
			StatusCode: 202,
			Body:       ioutil.NopCloser(bytes.NewBufferString(mockVolJson)),
		}, nil
	}

	gooseAdapter := newTestGooseAdapter(cinderHandler, nil)

	err := gooseAdapter.DeleteVolume(mockVolId)
	c.Assert(numCalls, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *gooseAdapterSuite) TestGetVolumesSimple(c *gc.C) {

	numCalls := 0
	cinderHandler := func(req *http.Request) (*http.Response, error) {
		numCalls++

		resp := `{"volumes":[{"id": "` + mockVolId + `","name": "` + mockVolName + `"}]}`

		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(resp)),
		}, nil
	}

	gooseAdapter := newTestGooseAdapter(cinderHandler, nil)

	vols, err := gooseAdapter.GetVolumesSimple()
	c.Assert(numCalls, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vols, gc.HasLen, 1)
}

func (s *gooseAdapterSuite) TestAttachVolume(c *gc.C) {

	numCalls := 0
	novaHandler := &mockNovaClient{
		sendRequest: func(method, svcType, apiCall string, reqData *goosehttp.RequestData) error {
			numCalls++

			c.Check(apiCall, gc.Equals, "servers/"+mockServerId+"/os-volume_attachments")

			attachment := reqData.RespValue.(*nova.VolumeAttachment)
			attachment.ServerId = mockServerId
			attachment.VolumeId = mockVolId

			reqData.RespValue = attachment
			return nil
		},
	}

	gooseAdapter := newTestGooseAdapter(nil, novaHandler)

	attachment, err := gooseAdapter.AttachVolume(mockServerId, mockVolId, "/dev/sda")
	c.Assert(numCalls, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachment, gc.NotNil)

	c.Check(attachment.Volume, gc.Equals, names.NewVolumeTag(mockVolId))
	c.Check(attachment.Machine, gc.Equals, names.NewMachineTag(mockServerId))
}

type mockNovaClient struct {
	sendRequest    func(string, string, string, *goosehttp.RequestData) error
	makeServiceUrl func(string, []string) (string, error)
}

func (c *mockNovaClient) SendRequest(method, svcType, apiCall string, requestData *goosehttp.RequestData) (err error) {
	if c.sendRequest != nil {
		return c.sendRequest(method, svcType, apiCall, requestData)
	}
	return nil
}

func (c *mockNovaClient) MakeServiceURL(serviceType string, parts []string) (string, error) {
	if c.makeServiceUrl != nil {
		return c.makeServiceUrl(serviceType, parts)
	}
	return "", nil
}

func newTestGooseAdapter(
	cinderHandler cinder.RequestHandlerFn,
	novaHandler client.Client,
) *gooseAdapter {
	return &gooseAdapter{
		cinder: cinder.NewClient("mock-tenant-id", cinderHandler),
		nova:   nova.New(novaHandler),
	}
}
