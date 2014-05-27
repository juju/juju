// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/environs/imagemetadata"
	sstesting "launchpad.net/juju-core/environs/simplestreams/testing"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/testing"
	coretesting "launchpad.net/juju-core/testing"
)

type URLsSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&URLsSuite{})

func (s *URLsSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	s.BaseSuite.TearDownTest(c)
}

func (s *URLsSuite) env(c *gc.C, imageMetadataURL, stream string) environs.Environ {
	attrs := dummy.SampleConfig()
	if stream != "" {
		attrs = attrs.Merge(testing.Attrs{
			"image-stream": stream,
		})
	}
	if imageMetadataURL != "" {
		attrs = attrs.Merge(testing.Attrs{
			"image-metadata-url": imageMetadataURL,
		})
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg, testing.Context(c), configstore.NewMem())
	c.Assert(err, gc.IsNil)
	// Put a file in images since the dummy storage provider requires a
	// file to exist before the URL can be found. This is to ensure it behaves
	// the same way as MAAS.
	err = env.Storage().Put("images/dummy", strings.NewReader("dummy"), 5)
	c.Assert(err, gc.IsNil)
	return env
}

func (s *URLsSuite) TestImageMetadataURLsNoConfigURL(c *gc.C) {
	env := s.env(c, "", "")
	sources, err := imagemetadata.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	privateStorageURL, err := env.Storage().URL("images")
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		privateStorageURL, "http://cloud-images.ubuntu.com/releases/"})
}

func (s *URLsSuite) TestImageMetadataURLs(c *gc.C) {
	env := s.env(c, "config-image-metadata-url", "")
	sources, err := imagemetadata.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	privateStorageURL, err := env.Storage().URL("images")
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		"config-image-metadata-url/", privateStorageURL, "http://cloud-images.ubuntu.com/releases/"})
}

func (s *URLsSuite) TestImageMetadataURLsNonReleaseStream(c *gc.C) {
	env := s.env(c, "", "daily")
	sources, err := imagemetadata.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	privateStorageURL, err := env.Storage().URL("images")
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		privateStorageURL, "http://cloud-images.ubuntu.com/daily/"})
}

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
