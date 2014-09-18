// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"strings"

	"github.com/juju/errors"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/utils"
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
	sources, err := environs.ImageMetadataSources(env)
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		"http://cloud-images.ubuntu.com/releases/",
	})
}

func (s *URLsSuite) TestImageMetadataURLs(c *gc.C) {
	env := s.env(c, "config-image-metadata-url", "")
	sources, err := environs.ImageMetadataSources(env)
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		"config-image-metadata-url/", "http://cloud-images.ubuntu.com/releases/",
	})
}

func (s *URLsSuite) TestImageMetadataURLsRegisteredFuncs(c *gc.C) {
	environs.RegisterImageDataSourceFunc("id0", func(environs.Environ) (simplestreams.DataSource, error) {
		return simplestreams.NewURLDataSource("id0", "betwixt/releases", utils.NoVerifySSLHostnames), nil
	})
	environs.RegisterImageDataSourceFunc("id1", func(environs.Environ) (simplestreams.DataSource, error) {
		return simplestreams.NewURLDataSource("id1", "yoink", utils.NoVerifySSLHostnames), nil
	})
	// overwrite the one previously registered against id1
	environs.RegisterImageDataSourceFunc("id1", func(environs.Environ) (simplestreams.DataSource, error) {
		return nil, errors.NewNotSupported(nil, "oyvey")
	})
	defer environs.UnregisterImageDataSourceFunc("id0")
	defer environs.UnregisterImageDataSourceFunc("id1")

	env := s.env(c, "config-image-metadata-url", "")
	sources, err := environs.ImageMetadataSources(env)
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		"config-image-metadata-url/",
		"betwixt/releases/",
		"http://cloud-images.ubuntu.com/releases/",
	})
}

func (s *URLsSuite) TestImageMetadataURLsRegisteredFuncsError(c *gc.C) {
	environs.RegisterImageDataSourceFunc("id0", func(environs.Environ) (simplestreams.DataSource, error) {
		return nil, errors.New("oyvey!")
	})
	defer environs.UnregisterImageDataSourceFunc("id0")

	env := s.env(c, "config-image-metadata-url", "")
	_, err := environs.ImageMetadataSources(env)
	c.Assert(err, gc.ErrorMatches, "oyvey!")
}

func (s *URLsSuite) TestImageMetadataURLsNonReleaseStream(c *gc.C) {
	env := s.env(c, "", "daily")
	sources, err := environs.ImageMetadataSources(env)
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		"http://cloud-images.ubuntu.com/daily/",
	})
}
