// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package container_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container"
	containertesting "github.com/juju/juju/container/testing"
	"github.com/juju/juju/instance"
)

type imageURLSuite struct {
	testing.CleanupSuite
}

var _ = gc.Suite(&imageURLSuite{})

func (s *imageURLSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	testing.PatchExecutable(c, s, "ubuntu-cloudimg-query", containertesting.FakeLxcURLScript)
}

func (s *imageURLSuite) TestImageURL(c *gc.C) {
	imageURLGetter := container.NewImageURLGetter(
		container.ImageURLGetterConfig{
			"host:port", "12345", []byte("cert"), "",
			container.ImageDownloadURL,
		})
	imageURL, err := imageURLGetter.ImageURL(instance.LXC, "trusty", "amd64")
	c.Assert(err, gc.IsNil)
	c.Assert(imageURL, gc.Equals, "https://host:port/model/12345/images/lxc/trusty/amd64/trusty-released-amd64-root.tar.gz")
	c.Assert(imageURLGetter.CACert(), gc.DeepEquals, []byte("cert"))
}

func (s *imageURLSuite) TestImageURLOtherBase(c *gc.C) {
	var calledBaseURL string
	baseURL := "other://cloud-images"
	mockFunc := func(kind instance.ContainerType, series, arch, cloudimgBaseUrl string) (string, error) {
		calledBaseURL = cloudimgBaseUrl
		return "omg://wat/trusty-released-amd64-root.tar.gz", nil
	}
	imageURLGetter := container.NewImageURLGetter(
		container.ImageURLGetterConfig{
			"host:port", "12345", []byte("cert"), baseURL, mockFunc,
		})
	imageURL, err := imageURLGetter.ImageURL(instance.LXC, "trusty", "amd64")
	c.Assert(err, gc.IsNil)
	c.Assert(imageURL, gc.Equals, "https://host:port/model/12345/images/lxc/trusty/amd64/trusty-released-amd64-root.tar.gz")
	c.Assert(imageURLGetter.CACert(), gc.DeepEquals, []byte("cert"))
	c.Assert(calledBaseURL, gc.Equals, baseURL)
}

func (s *imageURLSuite) TestImageDownloadURL(c *gc.C) {
	imageDownloadURL, err := container.ImageDownloadURL(instance.LXC, "trusty", "amd64", "")
	c.Assert(err, gc.IsNil)
	c.Assert(imageDownloadURL, gc.Equals, "test://cloud-images/trusty-released-amd64-root.tar.gz")
}

func (s *imageURLSuite) TestImageDownloadURLOtherBase(c *gc.C) {
	imageDownloadURL, err := container.ImageDownloadURL(instance.LXC, "trusty", "amd64", "other://cloud-images")
	c.Assert(err, gc.IsNil)
	c.Assert(imageDownloadURL, gc.Equals, "other://cloud-images/trusty-released-amd64-root.tar.gz")
}

func (s *imageURLSuite) TestImageDownloadURLUnsupportedContainer(c *gc.C) {
	_, err := container.ImageDownloadURL(instance.KVM, "trusty", "amd64", "")
	c.Assert(err, gc.ErrorMatches, "unsupported container .*")
}
