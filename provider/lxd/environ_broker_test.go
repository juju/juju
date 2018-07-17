// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	gc "gopkg.in/check.v1"

	containerlxd "github.com/juju/juju/container/lxd"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/lxd"
)

type environBrokerSuite struct {
	lxd.BaseSuite

	callCtx context.ProviderCallContext
}

var _ = gc.Suite(&environBrokerSuite{})

func (s *environBrokerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.callCtx = context.NewCloudCallContext()
}

func (s *environBrokerSuite) TestStartInstanceDefaultNIC(c *gc.C) {
	s.Client.Container = s.Container
	s.Client.Profile.Devices = map[string]map[string]string{
		"eth0": {},
	}

	// Patch the host's arch, so the broker will filter tools.
	s.PatchValue(&arch.HostArch, func() string { return arch.ARM64 })

	result, err := s.Env.StartInstance(s.callCtx, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Instance, gc.DeepEquals, s.Instance)
	c.Check(result.Hardware, gc.DeepEquals, s.HWC)
	c.Assert(s.StartInstArgs.InstanceConfig.AgentVersion().Arch, gc.Equals, arch.ARM64)

	s.Stub.CheckCallNames(c, "FindImage", "GetNICsFromProfile", "CreateContainerFromSpec")
	s.Stub.CheckCall(c, 0, "FindImage", "trusty", "arm64")

	// Check that no custom devices were passed - vanilla cloud-init.
	spec := s.Stub.Calls()[2].Args[0].(containerlxd.ContainerSpec)
	c.Check(spec.Devices, gc.IsNil)
	c.Check(spec.Config[containerlxd.NetworkConfigKey], gc.Equals, "")
}

func (s *environBrokerSuite) TestStartInstanceNonDefaultNIC(c *gc.C) {
	s.Client.Container = s.Container
	s.Client.Profile.Devices = map[string]map[string]string{
		"eno9": {
			"name":    "eno9",
			"mtu":     "9000",
			"nictype": "bridged",
			"parent":  "lxdbr0",
		},
	}

	// Patch the host's arch, so the broker will filter tools.
	s.PatchValue(&arch.HostArch, func() string { return arch.ARM64 })

	result, err := s.Env.StartInstance(s.callCtx, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Instance, gc.DeepEquals, s.Instance)
	c.Check(result.Hardware, gc.DeepEquals, s.HWC)
	c.Assert(s.StartInstArgs.InstanceConfig.AgentVersion().Arch, gc.Equals, arch.ARM64)

	s.Stub.CheckCallNames(c, "FindImage", "GetNICsFromProfile", "CreateContainerFromSpec")
	s.Stub.CheckCall(c, 0, "FindImage", "trusty", "arm64")

	// Check that the non-standard devices were passed explicitly.
	spec := s.Stub.Calls()[2].Args[0].(containerlxd.ContainerSpec)
	c.Check(spec.Devices, gc.HasLen, 1)
	c.Check(spec.Devices["eno9"]["name"], gc.Equals, "eno9")
	c.Check(spec.Config[containerlxd.NetworkConfigKey], gc.Equals, `network:
  config: "disabled"
`)
}

func (s *environBrokerSuite) TestStartInstanceNoTools(c *gc.C) {
	s.Client.Container = s.Container

	// Patch the host's arch, so the broker will filter tools.
	s.PatchValue(&arch.HostArch, func() string { return arch.PPC64EL })

	_, err := s.Env.StartInstance(s.callCtx, s.StartInstArgs)
	c.Assert(err, gc.ErrorMatches, "no matching agent binaries available")
}

func (s *environBrokerSuite) TestStopInstances(c *gc.C) {
	err := s.Env.StopInstances(s.callCtx, s.Instance.Id())
	c.Assert(err, jc.ErrorIsNil)

	// TODO (manadart 2018-06-25) This call has no IDs as arguments,
	// because of filtering by the env namespace prefix.
	// These tests will all be rewritten.
	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "RemoveContainers",
		Args: []interface{}{
			[]string{},
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

func (s *environBrokerSuite) checkSourcesFromStream(c *gc.C, stream string, expectedURLs []string) {
	if stream != "" {
		s.UpdateConfig(c, map[string]interface{}{"image-stream": stream})
	}
	s.checkSources(c, expectedURLs)
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
