// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package container_test

import (
	"fmt"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container"
	containertesting "github.com/juju/juju/container/testing"
	"github.com/juju/juju/instance"
	coretesting "github.com/juju/juju/testing"
)

type imageURLSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&imageURLSuite{})

func (s *imageURLSuite) SetUpTest(c *gc.C) {
	testing.PatchExecutable(c, s, "ubuntu-cloudimg-query", containertesting.FakeLxcURLScript)
}

func (s *imageURLSuite) TestImageURL(c *gc.C) {
	s.assertImageURLForStream(c, "released")
	s.assertImageURLForStream(c, "daily")
}

func (s *imageURLSuite) assertImageURLForStream(c *gc.C, stream string) {
	imageURLGetter := container.NewImageURLGetter(
		container.ImageURLGetterConfig{
			ServerRoot:        "host:port",
			ModelUUID:         "12345",
			CACert:            []byte("cert"),
			CloudimgBaseUrl:   "",
			Stream:            stream,
			ImageDownloadFunc: container.ImageDownloadURL,
		})
	imageURL, err := imageURLGetter.ImageURL(instance.LXC, "trusty", "amd64")
	c.Assert(err, gc.IsNil)
	c.Assert(imageURL, gc.Equals, fmt.Sprintf("https://host:port/model/12345/images/lxc/trusty/amd64/trusty-%s-amd64-root.tar.gz", stream))
	c.Assert(imageURLGetter.CACert(), gc.DeepEquals, []byte("cert"))
}

func (s *imageURLSuite) TestImageURLOtherBase(c *gc.C) {
	var calledBaseURL string
	baseURL := "other://cloud-images"
	mockFunc := func(kind instance.ContainerType, series, arch, stream, cloudimgBaseUrl string) (string, error) {
		calledBaseURL = cloudimgBaseUrl
		return "omg://wat/trusty-released-amd64-root.tar.gz", nil
	}
	imageURLGetter := container.NewImageURLGetter(
		container.ImageURLGetterConfig{
			ServerRoot:        "host:port",
			ModelUUID:         "12345",
			CACert:            []byte("cert"),
			CloudimgBaseUrl:   baseURL,
			Stream:            "released",
			ImageDownloadFunc: mockFunc,
		})
	imageURL, err := imageURLGetter.ImageURL(instance.LXC, "trusty", "amd64")
	c.Assert(err, gc.IsNil)
	c.Assert(imageURL, gc.Equals, "https://host:port/model/12345/images/lxc/trusty/amd64/trusty-released-amd64-root.tar.gz")
	c.Assert(imageURLGetter.CACert(), gc.DeepEquals, []byte("cert"))
	c.Assert(calledBaseURL, gc.Equals, baseURL)
}

func (s *imageURLSuite) TestImageDownloadURL(c *gc.C) {
	imageDownloadURL, err := container.ImageDownloadURL(instance.LXC, "trusty", "amd64", "released", "")
	c.Assert(err, gc.IsNil)
	c.Assert(imageDownloadURL, gc.Equals, "test://cloud-images/trusty-released-amd64-root.tar.gz")
}

func (s *imageURLSuite) TestImageDownloadURLOtherBase(c *gc.C) {
	imageDownloadURL, err := container.ImageDownloadURL(instance.LXC, "trusty", "amd64", "released", "other://cloud-images")
	c.Assert(err, gc.IsNil)
	c.Assert(imageDownloadURL, gc.Equals, "other://cloud-images/trusty-released-amd64-root.tar.gz")
}

func (s *imageURLSuite) TestImageDownloadURLUnsupportedContainer(c *gc.C) {
	_, err := container.ImageDownloadURL(instance.KVM, "trusty", "amd64", "released", "")
	c.Assert(err, gc.ErrorMatches, "unsupported container .*")
}
