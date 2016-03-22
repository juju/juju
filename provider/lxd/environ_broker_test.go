// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd_test

import (
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/lxd"
)

type environBrokerSuite struct {
	lxd.BaseSuite
}

var _ = gc.Suite(&environBrokerSuite{})

func (s *environBrokerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *environBrokerSuite) TestStartInstance(c *gc.C) {
	s.Client.Inst = s.RawInstance

	result, err := s.Env.StartInstance(s.StartInstArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Instance, gc.DeepEquals, s.Instance)
	c.Check(result.Hardware, gc.DeepEquals, s.HWC)
}

func (s *environBrokerSuite) TestStopInstances(c *gc.C) {
	err := s.Env.StopInstances(s.Instance.Id())
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "RemoveInstances",
		Args: []interface{}{
			"juju-2d02eeac-9dbb-11e4-89d3-123b93f75cba-machine-",
			[]string{"spam"},
		},
	}})
}

func (s *environBrokerSuite) TestImageMetadataURL(c *gc.C) {
	s.UpdateConfig(c, map[string]interface{}{
		"image-metadata-url": "https://my-test.com/images/",
		})
	s.checkSources(c, []string{
		"https://my-test.com/images/",
		"https://streams.canonical.com/juju/images/releases/",
		"https://cloud-images.ubuntu.com/releases/",
	})
}

func (s *environBrokerSuite) TestImageMetadataURLMungesHTTP(c *gc.C) {
	// LXD requires 'https://' hosts for simplestreams data.
	// https://github.com/lxc/lxd/issues/1763
	s.UpdateConfig(c, map[string]interface{}{
		"image-metadata-url": "http://my-test.com/images/",
		})
	s.checkSources(c, []string{
		"https://my-test.com/images/",
		"https://streams.canonical.com/juju/images/releases/",
		"https://cloud-images.ubuntu.com/releases/",
	})
}

func (s *environBrokerSuite) checkSources(c *gc.C, expectedURLs []string) {
	sources, err := lxd.GetImageSources(s.Env)
	c.Assert(err, jc.ErrorIsNil)
	var sourceURLs []string
	for _, source := range sources {
		sourceURLs = append(sourceURLs, source.Host)
	}
	c.Check(sourceURLs, gc.DeepEquals, expectedURLs)
}

func (s *environBrokerSuite) checkSourcesFromStream(c *gc.C, stream string, expectedURLs []string) {
	if stream != "" {
		s.UpdateConfig(c, map[string]interface{}{"image-stream": stream})
	}
	s.checkSources(c, expectedURLs)
}

func (s *environBrokerSuite) TestImageStreamDefault(c *gc.C) {
	s.checkSourcesFromStream(c, "", []string{
		"https://streams.canonical.com/juju/images/releases/",
		"https://cloud-images.ubuntu.com/releases/",
	})
}

func (s *environBrokerSuite) TestImageStreamReleased(c *gc.C) {
	s.checkSourcesFromStream(c, "released", []string{
		"https://streams.canonical.com/juju/images/releases/",
		"https://cloud-images.ubuntu.com/releases/",
	})
}

func (s *environBrokerSuite) TestImageStreamDaily(c *gc.C) {
	s.checkSourcesFromStream(c, "daily", []string{
		"https://streams.canonical.com/juju/images/daily/",
		"https://cloud-images.ubuntu.com/daily/",
	})
}
