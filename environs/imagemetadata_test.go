// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	stdcontext "context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
)

type ImageMetadataSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&ImageMetadataSuite{})

func (s *ImageMetadataSuite) env(c *gc.C, imageMetadataURL, stream string) environs.Environ {
	attrs := coretesting.FakeConfig()
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
	env, err := bootstrap.PrepareController(
		false,
		envtesting.BootstrapContext(stdcontext.Background(), c),
		jujuclient.NewMemStore(),
		bootstrap.PrepareParams{
			ControllerConfig: testing.FakeControllerConfig(),
			ControllerName:   attrs["name"].(string),
			ModelConfig:      attrs,
			Cloud:            coretesting.FakeCloudSpec(),
			AdminSecret:      "admin-secret",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	return env.(environs.Environ)
}

func (s *ImageMetadataSuite) TestImageMetadataURLsNoConfigURL(c *gc.C) {
	env := s.env(c, "", "")
	sources, err := environs.ImageMetadataSources(env, sstesting.TestDataSourceFactory())
	c.Assert(err, jc.ErrorIsNil)
	sstesting.AssertExpectedSources(c, sources, []sstesting.SourceDetails{
		{"http://cloud-images.ubuntu.com/releases/", imagemetadata.SimplestreamsImagesPublicKey, true},
	})
}

func (s *ImageMetadataSuite) TestImageMetadataURLs(c *gc.C) {
	env := s.env(c, "config-image-metadata-url", "")
	sources, err := environs.ImageMetadataSources(env, sstesting.TestDataSourceFactory())
	c.Assert(err, jc.ErrorIsNil)
	sstesting.AssertExpectedSources(c, sources, []sstesting.SourceDetails{
		{"config-image-metadata-url/", "", false},
		{"http://cloud-images.ubuntu.com/releases/", imagemetadata.SimplestreamsImagesPublicKey, true},
	})
}

func (s *ImageMetadataSuite) TestImageMetadataURLsRegisteredFuncs(c *gc.C) {
	factory := sstesting.TestDataSourceFactory()
	environs.RegisterImageDataSourceFunc("id0", func(environs.Environ) (simplestreams.DataSource, error) {
		return factory.NewDataSource(simplestreams.Config{
			Description:          "id0",
			BaseURL:              "betwixt/releases",
			HostnameVerification: false,
			Priority:             simplestreams.DEFAULT_CLOUD_DATA}), nil
	})
	environs.RegisterImageDataSourceFunc("id1", func(environs.Environ) (simplestreams.DataSource, error) {
		return factory.NewDataSource(simplestreams.Config{
			Description:          "id1",
			BaseURL:              "yoink",
			HostnameVerification: false,
			Priority:             simplestreams.SPECIFIC_CLOUD_DATA}), nil
	})
	// overwrite the one previously registered against id1
	environs.RegisterImageDataSourceFunc("id1", func(environs.Environ) (simplestreams.DataSource, error) {
		return nil, errors.NewNotSupported(nil, "oyvey")
	})
	environs.RegisterUserImageDataSourceFunc("id2", func(environs.Environ) (simplestreams.DataSource, error) {
		return factory.NewDataSource(simplestreams.Config{
			Description:          "id2",
			BaseURL:              "foobar",
			HostnameVerification: false,
			Priority:             simplestreams.CUSTOM_CLOUD_DATA}), nil
	})
	defer environs.UnregisterImageDataSourceFunc("id0")
	defer environs.UnregisterImageDataSourceFunc("id1")
	defer environs.UnregisterImageDataSourceFunc("id2")

	env := s.env(c, "config-image-metadata-url", "")
	sources, err := environs.ImageMetadataSources(env, sstesting.TestDataSourceFactory())
	c.Assert(err, jc.ErrorIsNil)
	sstesting.AssertExpectedSources(c, sources, []sstesting.SourceDetails{
		{"config-image-metadata-url/", "", false},
		{"foobar/", "", false},
		{"betwixt/releases/", "", false},
		{"http://cloud-images.ubuntu.com/releases/", imagemetadata.SimplestreamsImagesPublicKey, true},
	})
}

func (s *ImageMetadataSuite) TestImageMetadataURLsRegisteredFuncsError(c *gc.C) {
	environs.RegisterImageDataSourceFunc("id0", func(environs.Environ) (simplestreams.DataSource, error) {
		return nil, errors.New("oyvey!")
	})
	defer environs.UnregisterImageDataSourceFunc("id0")

	env := s.env(c, "config-image-metadata-url", "")
	_, err := environs.ImageMetadataSources(env, sstesting.TestDataSourceFactory())
	c.Assert(err, gc.ErrorMatches, "oyvey!")
}

func (s *ImageMetadataSuite) TestImageMetadataURLsNonReleaseStream(c *gc.C) {
	env := s.env(c, "", "daily")
	sources, err := environs.ImageMetadataSources(env, sstesting.TestDataSourceFactory())
	c.Assert(err, jc.ErrorIsNil)
	sstesting.AssertExpectedSources(c, sources, []sstesting.SourceDetails{
		{"http://cloud-images.ubuntu.com/daily/", imagemetadata.SimplestreamsImagesPublicKey, true},
	})
}
