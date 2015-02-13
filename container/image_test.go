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
	imageURLGetter := container.NewImageURLGetter("host:port", "12345", []byte("cert"))
	imageURL, err := imageURLGetter.ImageURL(instance.LXC, "trusty", "amd64")
	c.Assert(err, gc.IsNil)
	c.Assert(imageURL, gc.Equals, "https://host:port/environment/12345/images/lxc/trusty/amd64/trusty-released-amd64-root.tar.gz")
	c.Assert(imageURLGetter.CACert(), gc.DeepEquals, []byte("cert"))
}

func (s *imageURLSuite) TestImageDownloadURL(c *gc.C) {
	imageDownloadURL, err := container.ImageDownloadURL(instance.LXC, "trusty", "amd64")
	c.Assert(err, gc.IsNil)
	c.Assert(imageDownloadURL, gc.Equals, "test://cloud-images/trusty-released-amd64-root.tar.gz")
}

func (s *imageURLSuite) TestImageDownloadURLUnsupportedContainer(c *gc.C) {
	_, err := container.ImageDownloadURL(instance.KVM, "trusty", "amd64")
	c.Assert(err, gc.ErrorMatches, "unsupported container .*")
}
