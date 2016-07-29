// goose/glance - Go package to interact with OpenStack Image Service (Glance) API.
// See http://docs.openstack.org/api/openstack-image-service/2.0/content/.

package glance

import (
	"fmt"
	"net/http"

	"gopkg.in/goose.v1/client"
	"gopkg.in/goose.v1/errors"
	goosehttp "gopkg.in/goose.v1/http"
)

// API URL parts.
const (
	apiImages       = "/images"
	apiImagesDetail = "/images/detail"
)

// Client provides a means to access the OpenStack Image Service.
type Client struct {
	client client.Client
}

// New creates a new Client.
func New(client client.Client) *Client {
	return &Client{client}
}

// Link describes a link to an image in OpenStack.
type Link struct {
	Href string
	Rel  string
	Type string
}

// Image describes an OpenStack image.
type Image struct {
	Id    string
	Name  string
	Links []Link
}

// ListImages lists IDs, names, and links for available images.
func (c *Client) ListImages() ([]Image, error) {
	var resp struct {
		Images []Image
	}
	requestData := goosehttp.RequestData{RespValue: &resp, ExpectedStatus: []int{http.StatusOK}}
	err := c.client.SendRequest(client.GET, "compute", apiImages, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to get list of images")
	}
	return resp.Images, nil
}

// ImageMetadata describes metadata of an image
type ImageMetadata struct {
	Architecture string
	State        string      `json:"image_state"`
	Location     string      `json:"image_location"`
	KernelId     interface{} `json:"kernel_id"`
	ProjectId    interface{} `json:"project_id"`
	RAMDiskId    interface{} `json:"ramdisk_id"`
	OwnerId      interface{} `json:"owner_id"`
}

// ImageDetail describes extended information about an image.
type ImageDetail struct {
	Id          string
	Name        string
	Created     string
	Updated     string
	Progress    int
	Status      string
	MinimumRAM  int `json:"minRam"`
	MinimumDisk int `json:"minDisk"`
	Links       []Link
	Metadata    ImageMetadata
}

// ListImageDetails lists all details for available images.
func (c *Client) ListImagesDetail() ([]ImageDetail, error) {
	var resp struct {
		Images []ImageDetail
	}
	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.GET, "compute", apiImagesDetail, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to get list of image details")
	}
	return resp.Images, nil
}

// GetImageDetail lists details of the specified image.
func (c *Client) GetImageDetail(imageId string) (*ImageDetail, error) {
	var resp struct {
		Image ImageDetail
	}
	url := fmt.Sprintf("%s/%s", apiImages, imageId)
	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.GET, "compute", url, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to get details of imageId: %s", imageId)
	}
	return &resp.Image, nil
}
