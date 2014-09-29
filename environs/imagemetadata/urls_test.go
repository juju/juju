// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/imagemetadata"
	coretesting "github.com/juju/juju/testing"
)

type URLsSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&URLsSuite{})

func (s *URLsSuite) TestImageMetadataURL(c *gc.C) {
	for source, expected := range map[string]string{
		"":           "",
		"foo":        "file://foo/images",
		"/home/foo":  "file:///home/foo/images",
		"file://foo": "file://foo",
		"http://foo": "http://foo",
	} {
		URL, err := imagemetadata.ImageMetadataURL(source, "")
		c.Assert(err, gc.IsNil)
		c.Assert(URL, gc.Equals, expected)
	}
}

func (s *URLsSuite) TestImageMetadataURLOfficialSource(c *gc.C) {
	// Released streams.
	URL, err := imagemetadata.ImageMetadataURL(imagemetadata.UbuntuCloudImagesURL, "")
	c.Assert(err, gc.IsNil)
	c.Assert(URL, gc.Equals, "http://cloud-images.ubuntu.com/releases")
	URL, err = imagemetadata.ImageMetadataURL(imagemetadata.UbuntuCloudImagesURL, imagemetadata.ReleasedStream)
	c.Assert(err, gc.IsNil)
	c.Assert(URL, gc.Equals, "http://cloud-images.ubuntu.com/releases")
	// Non-released streams.
	URL, err = imagemetadata.ImageMetadataURL(imagemetadata.UbuntuCloudImagesURL, "daily")
	c.Assert(err, gc.IsNil)
	c.Assert(URL, gc.Equals, "http://cloud-images.ubuntu.com/daily")
}
