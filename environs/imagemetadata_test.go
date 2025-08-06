// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type ImageMetadataSuite struct {
	testing.BaseSuite
}

func TestImageMetadataSuite(t *stdtesting.T) {
	tc.Run(t, &ImageMetadataSuite{})
}

func (s *ImageMetadataSuite) env(c *tc.C, imageMetadataURL, stream string, defaultsDisabled bool) environs.Environ {
	attrs := testing.FakeConfig()
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
	if defaultsDisabled {
		attrs = attrs.Merge(testing.Attrs{
			"image-metadata-defaults-disabled": true,
		})
	}
	env, err := bootstrap.PrepareController(
		false,
		envtesting.BootstrapContext(c.Context(), c),
		jujuclient.NewMemStore(),
		bootstrap.PrepareParams{
			ControllerConfig: testing.FakeControllerConfig(),
			ControllerName:   attrs["name"].(string),
			ModelConfig:      attrs,
			Cloud:            testing.FakeCloudSpec(),
			AdminSecret:      "admin-secret",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	return env.(environs.Environ)
}

func (s *ImageMetadataSuite) TestImageMetadataURLsNoConfigURL(c *tc.C) {
	env := s.env(c, "", "", false)
	sources, err := environs.ImageMetadataSources(env, sstesting.TestDataSourceFactory())
	c.Assert(err, tc.ErrorIsNil)
	sstesting.AssertExpectedSources(c, sources, []sstesting.SourceDetails{
		{"http://cloud-images.ubuntu.com/releases/", imagemetadata.SimplestreamsImagesPublicKey, true},
	})
}

func (s *ImageMetadataSuite) TestImageMetadataURLs(c *tc.C) {
	env := s.env(c, "config-image-metadata-url", "", false)
	sources, err := environs.ImageMetadataSources(env, sstesting.TestDataSourceFactory())
	c.Assert(err, tc.ErrorIsNil)
	sstesting.AssertExpectedSources(c, sources, []sstesting.SourceDetails{
		{"config-image-metadata-url/", "", false},
		{"http://cloud-images.ubuntu.com/releases/", imagemetadata.SimplestreamsImagesPublicKey, true},
	})
}

func (s *ImageMetadataSuite) TestImageMetadataURLsNoDefaults(c *tc.C) {
	env := s.env(c, "https://custom.meta.data/", "", true)
	sources, err := environs.ImageMetadataSources(env, sstesting.TestDataSourceFactory())
	c.Assert(err, tc.ErrorIsNil)
	sstesting.AssertExpectedSources(c, sources, []sstesting.SourceDetails{
		{"https://custom.meta.data/", "", false},
	})
}

func (s *ImageMetadataSuite) TestImageMetadataURLsNoDefaultsNoConfigURL(c *tc.C) {
	env := s.env(c, "", "", true)
	sources, err := environs.ImageMetadataSources(env, sstesting.TestDataSourceFactory())
	c.Assert(err, tc.ErrorIsNil)
	sstesting.AssertExpectedSources(c, sources, []sstesting.SourceDetails{})
}

func (s *ImageMetadataSuite) TestImageMetadataURLsRegisteredFuncs(c *tc.C) {
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

	env := s.env(c, "config-image-metadata-url", "", false)
	sources, err := environs.ImageMetadataSources(env, sstesting.TestDataSourceFactory())
	c.Assert(err, tc.ErrorIsNil)
	sstesting.AssertExpectedSources(c, sources, []sstesting.SourceDetails{
		{"config-image-metadata-url/", "", false},
		{"foobar/", "", false},
		{"betwixt/releases/", "", false},
		{"http://cloud-images.ubuntu.com/releases/", imagemetadata.SimplestreamsImagesPublicKey, true},
	})
}

func (s *ImageMetadataSuite) TestImageMetadataURLsRegisteredFuncsNoDefaultsNoConfigURL(c *tc.C) {
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

	env := s.env(c, "", "", true)
	sources, err := environs.ImageMetadataSources(env, sstesting.TestDataSourceFactory())
	c.Assert(err, tc.ErrorIsNil)
	sstesting.AssertExpectedSources(c, sources, []sstesting.SourceDetails{
		{"foobar/", "", false},
		{"betwixt/releases/", "", false},
	})
}

func (s *ImageMetadataSuite) TestImageMetadataURLsRegisteredFuncsError(c *tc.C) {
	environs.RegisterImageDataSourceFunc("id0", func(environs.Environ) (simplestreams.DataSource, error) {
		return nil, errors.New("oyvey!")
	})
	defer environs.UnregisterImageDataSourceFunc("id0")

	env := s.env(c, "config-image-metadata-url", "", false)
	_, err := environs.ImageMetadataSources(env, sstesting.TestDataSourceFactory())
	c.Assert(err, tc.ErrorMatches, "oyvey!")
}

func (s *ImageMetadataSuite) TestImageMetadataURLsNonReleaseStream(c *tc.C) {
	env := s.env(c, "", "daily", false)
	sources, err := environs.ImageMetadataSources(env, sstesting.TestDataSourceFactory())
	c.Assert(err, tc.ErrorIsNil)
	sstesting.AssertExpectedSources(c, sources, []sstesting.SourceDetails{
		{"http://cloud-images.ubuntu.com/daily/", imagemetadata.SimplestreamsImagesPublicKey, true},
	})
}
