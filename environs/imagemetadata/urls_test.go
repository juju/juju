// Copyright 2012, 2013, 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/imagemetadata"
	coretesting "github.com/juju/juju/testing"
)

type URLsSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&URLsSuite{})

func (s *URLsSuite) TestImageMetadataURL(c *gc.C) {
	var imageTests = []struct {
		in          string
		expected    string
		expectedErr error
	}{{
		in:          "",
		expected:    "",
		expectedErr: nil,
	}, {
		in:          "file://foo",
		expected:    "file://foo",
		expectedErr: nil,
	}, {
		in:          "http://foo",
		expected:    "http://foo",
		expectedErr: nil,
	}, {
		in:          "foo",
		expected:    "",
		expectedErr: fmt.Errorf("foo is not an absolute path"),
	}}
	imageTests = append(imageTests, imageTestsPlatformSpecific...)
	for i, t := range imageTests {
		c.Logf("Test %d:", i)

		out, err := imagemetadata.ImageMetadataURL(t.in, "")
		c.Assert(err, gc.DeepEquals, t.expectedErr)
		c.Assert(out, gc.Equals, t.expected)
	}
}

func (s *URLsSuite) TestImageMetadataURLOfficialSource(c *gc.C) {
	s.assertImageMetadataURLOfficialSource(c, imagemetadata.UbuntuCloudImagesURL)
	s.assertImageMetadataURLOfficialSource(c, imagemetadata.JujuStreamsImagesURL)
}

func (s *URLsSuite) assertImageMetadataURLOfficialSource(c *gc.C, baseURL string) {
	// Released streams.
	URL, err := imagemetadata.ImageMetadataURL(baseURL, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(URL, gc.Equals, fmt.Sprintf("%s/%s", baseURL, "releases"))
	URL, err = imagemetadata.ImageMetadataURL(baseURL, imagemetadata.ReleasedStream)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(URL, gc.Equals, fmt.Sprintf("%s/%s", baseURL, "releases"))
	// Non-released streams.
	URL, err = imagemetadata.ImageMetadataURL(baseURL, "daily")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(URL, gc.Equals, fmt.Sprintf("%s/%s", baseURL, "daily"))
}
