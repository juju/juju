// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"github.com/juju/names"
	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/instance"
	coretesting "github.com/juju/juju/testing"
)

type imageSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&imageSuite{})

var lxcURLScript = `#!/bin/bash
echo -n https://cloud-images/$1-$2-$3.tar.gz`

func (s *imageSuite) SetUpTest(c *gc.C) {
	jujutesting.PatchExecutable(c, s, "ubuntu-cloudimg-query", lxcURLScript)
}

func (s *imageSuite) TestImageURL(c *gc.C) {
	apiInfo := &api.Info{
		Addrs:      []string{"host:port"},
		EnvironTag: names.NewEnvironTag("12345"),
	}
	imageURL, err := api.ImageURL(apiInfo, instance.LXC, "trusty", "amd64")
	c.Assert(err, gc.IsNil)
	c.Assert(imageURL, gc.Equals, "https://host:port/environment/12345/images/lxc/trusty/amd64/trusty-released-amd64-root.tar.gz")
}

func (s *imageSuite) TestImageDownloadURL(c *gc.C) {
	imageDownloadURL, err := api.ImageDownloadURL(instance.LXC, "trusty", "amd64")
	c.Assert(err, gc.IsNil)
	c.Assert(imageDownloadURL, gc.Equals, "https://cloud-images/trusty-released-amd64-root.tar.gz")
}
