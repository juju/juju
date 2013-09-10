// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/imagemetadata"
	sstesting "launchpad.net/juju-core/environs/simplestreams/testing"
	"launchpad.net/juju-core/testing"
)

type URLsSuite struct {
	home *testing.FakeHome
}

var _ = gc.Suite(&URLsSuite{})

func (s *URLsSuite) SetUpTest(c *gc.C) {
	s.home = testing.MakeEmptyFakeHome(c)
}

func (s *URLsSuite) TearDownTest(c *gc.C) {
	s.home.Restore()
}

func (s *URLsSuite) env(c *gc.C, imageMetadataURL string) environs.Environ {
	attrs := map[string]interface{}{
		"name":            "only",
		"type":            "dummy",
		"authorized-keys": "foo",
		"state-server":    true,
		"ca-cert":         testing.CACert,
		"ca-private-key":  testing.CAKey,
	}
	if imageMetadataURL != "" {
		attrs["image-metadata-url"] = imageMetadataURL
	}
	env, err := environs.NewFromAttrs(attrs)
	c.Assert(err, gc.IsNil)
	env, err = environs.Prepare(env.Config())
	c.Assert(err, gc.IsNil)
	return env
}

func (s *URLsSuite) TestImageMetadataURLsNoConfigURL(c *gc.C) {
	sources, err := imagemetadata.GetMetadataSources(s.env(c, ""))
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		"dummy-image-metadata-url/", "http://cloud-images.ubuntu.com/releases/"})
}

func (s *URLsSuite) TestImageMetadataURLs(c *gc.C) {
	sources, err := imagemetadata.GetMetadataSources(s.env(c, "config-image-metadata-url"))
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		"config-image-metadata-url/", "dummy-image-metadata-url/", "http://cloud-images.ubuntu.com/releases/"})
}
