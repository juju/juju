// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"fmt"
	"reflect"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	containerlxd "github.com/juju/juju/container/lxd"
	lxdtesting "github.com/juju/juju/container/lxd/testing"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/lxd"
)

type environBrokerSuite struct {
	lxd.EnvironSuite

	callCtx        context.ProviderCallContext
	defaultProfile *api.Profile
}

var _ = gc.Suite(&environBrokerSuite{})

func (s *environBrokerSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.defaultProfile = &api.Profile{
		ProfilePut: api.ProfilePut{
			Devices: map[string]map[string]string{
				"eth0": {},
			},
		},
	}
}

func (s *environBrokerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.callCtx = context.NewCloudCallContext()
}

// containerSpecMatcher is a gomock matcher for testing a container spec
// with a supplied validation func.
type containerSpecMatcher struct {
	check func(spec containerlxd.ContainerSpec) bool
}

func (m containerSpecMatcher) Matches(arg interface{}) bool {
	if spec, ok := arg.(containerlxd.ContainerSpec); ok {
		return m.check(spec)
	}
	return false
}

func (m containerSpecMatcher) String() string {
	return fmt.Sprintf("%T", m.check)
}

func matchesContainerSpec(check func(spec containerlxd.ContainerSpec) bool) gomock.Matcher {
	return containerSpecMatcher{check: check}
}

func (s *environBrokerSuite) TestStartInstanceDefaultNIC(c *gc.C) {
	// Patch the host's arch, so the broker will filter tools.
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	// Check that no custom devices were passed - vanilla cloud-init.
	check := func(spec containerlxd.ContainerSpec) bool {
		if spec.Config[containerlxd.NetworkConfigKey] != "" {
			return false
		}
		return !(len(spec.Devices) > 0)
	}

	exp := svr.EXPECT()
	gomock.InOrder(
		exp.FindImage("bionic", arch.AMD64, gomock.Any(), true, gomock.Any()).Return(containerlxd.SourcedImage{}, nil),
		exp.GetProfile("default").Return(s.defaultProfile, lxdtesting.ETag, nil),
		exp.VerifyNetworkDevice(s.defaultProfile, lxdtesting.ETag).Return(nil),
		exp.GetNICsFromProfile("default").Return(s.defaultProfile.Devices, nil),
		exp.CreateContainerFromSpec(matchesContainerSpec(check)).Return(&containerlxd.Container{}, nil),
	)

	env := s.NewEnviron(c, svr, nil)
	_, err := env.StartInstance(s.callCtx, s.GetStartInstanceArgs(c, "bionic"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environBrokerSuite) TestStartInstanceNonDefaultNIC(c *gc.C) {
	// Patch the host's arch, so the broker will filter tools.
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	nics := map[string]map[string]string{
		"eno9": {
			"name":    "eno9",
			"mtu":     "9000",
			"nictype": "bridged",
			"parent":  "lxdbr0",
			"hwaddr":  "00:00:00:00:00",
		},
	}

	// Check that the non-standard devices were passed explicitly,
	// And that we have disabled the standard network config.
	check := func(spec containerlxd.ContainerSpec) bool {
		if !reflect.DeepEqual(spec.Devices, nics) {
			return false
		}
		return spec.Config[containerlxd.NetworkConfigKey] == `network:
  config: "disabled"
`
	}

	exp := svr.EXPECT()
	gomock.InOrder(
		exp.FindImage("bionic", arch.AMD64, gomock.Any(), true, gomock.Any()).Return(containerlxd.SourcedImage{}, nil),
		exp.GetProfile("default").Return(s.defaultProfile, lxdtesting.ETag, nil),
		exp.VerifyNetworkDevice(s.defaultProfile, lxdtesting.ETag).Return(nil),
		exp.GetNICsFromProfile("default").Return(nics, nil),
		exp.CreateContainerFromSpec(matchesContainerSpec(check)).Return(&containerlxd.Container{}, nil),
	)

	env := s.NewEnviron(c, svr, nil)
	_, err := env.StartInstance(s.callCtx, s.GetStartInstanceArgs(c, "bionic"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environBrokerSuite) TestStartInstanceWithPlacementAvailable(c *gc.C) {
	// Patch the host's arch, so the broker will filter tools.
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	target := lxdtesting.NewMockContainerServer(ctrl)
	tExp := target.EXPECT()
	serverRet := &api.Server{}
	tExp.GetServer().Return(serverRet, lxdtesting.ETag, nil)

	jujuTarget, err := containerlxd.NewServer(target)
	c.Assert(err, jc.ErrorIsNil)

	image := containerlxd.SourcedImage{
		Image: &api.Image{Filename: "container-image"},
	}

	members := []api.ClusterMember{
		{
			ServerName: "node01",
			Status:     "ONLINE",
		},
		{
			ServerName: "node02",
			Status:     "ONLINE",
		},
	}

	createOp := lxdtesting.NewMockRemoteOperation(ctrl)
	createOp.EXPECT().Wait().Return(nil)
	createOp.EXPECT().GetTarget().Return(&api.Operation{StatusCode: api.Success}, nil)

	startOp := lxdtesting.NewMockOperation(ctrl)
	startOp.EXPECT().Wait().Return(nil)

	sExp := svr.EXPECT()
	gomock.InOrder(
		sExp.FindImage("bionic", arch.AMD64, gomock.Any(), true, gomock.Any()).Return(image, nil),
		sExp.GetProfile("default").Return(s.defaultProfile, lxdtesting.ETag, nil),
		sExp.VerifyNetworkDevice(s.defaultProfile, lxdtesting.ETag).Return(nil),
		sExp.GetNICsFromProfile("default").Return(s.defaultProfile.Devices, nil),
		sExp.IsClustered().Return(true),
		sExp.GetClusterMembers().Return(members, nil),
		sExp.UseTargetServer("node01").Return(jujuTarget, nil),
	)

	// CreateContainerFromSpec is tested in container/lxd.
	// we don't bother with detailed parameter assertions here.
	tExp.CreateContainerFromImage(gomock.Any(), gomock.Any(), gomock.Any()).Return(createOp, nil)
	tExp.UpdateContainerState(gomock.Any(), gomock.Any(), "").Return(startOp, nil)
	tExp.GetContainer(gomock.Any()).Return(&api.Container{}, lxdtesting.ETag, nil)

	env := s.NewEnviron(c, svr, nil)

	args := s.GetStartInstanceArgs(c, "bionic")
	args.Placement = "zone=node01"

	_, err = env.StartInstance(s.callCtx, args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environBrokerSuite) TestStartInstanceWithPlacementNotPresent(c *gc.C) {
	// Patch the host's arch, so the broker will filter tools.
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	image := containerlxd.SourcedImage{
		Image: &api.Image{Filename: "container-image"},
	}

	members := []api.ClusterMember{{
		ServerName: "node01",
		Status:     "ONLINE",
	}}

	sExp := svr.EXPECT()
	gomock.InOrder(
		sExp.FindImage("bionic", arch.AMD64, gomock.Any(), true, gomock.Any()).Return(image, nil),
		sExp.GetProfile("default").Return(s.defaultProfile, lxdtesting.ETag, nil),
		sExp.VerifyNetworkDevice(s.defaultProfile, lxdtesting.ETag).Return(nil),
		sExp.GetNICsFromProfile("default").Return(s.defaultProfile.Devices, nil),
		sExp.IsClustered().Return(true),
		sExp.GetClusterMembers().Return(members, nil),
	)

	env := s.NewEnviron(c, svr, nil)

	args := s.GetStartInstanceArgs(c, "bionic")
	args.Placement = "zone=node03"

	_, err := env.StartInstance(s.callCtx, args)
	c.Assert(err, gc.ErrorMatches, `availability zone "node03" not valid`)
}

func (s *environBrokerSuite) TestStartInstanceWithPlacementNotAvailable(c *gc.C) {
	// Patch the host's arch, so the broker will filter tools.
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	image := containerlxd.SourcedImage{
		Image: &api.Image{Filename: "container-image"},
	}

	members := []api.ClusterMember{{
		ServerName: "node01",
		Status:     "OFFLINE",
	}}

	sExp := svr.EXPECT()
	gomock.InOrder(
		sExp.FindImage("bionic", arch.AMD64, gomock.Any(), true, gomock.Any()).Return(image, nil),
		sExp.GetProfile("default").Return(s.defaultProfile, lxdtesting.ETag, nil),
		sExp.VerifyNetworkDevice(s.defaultProfile, lxdtesting.ETag).Return(nil),
		sExp.GetNICsFromProfile("default").Return(s.defaultProfile.Devices, nil),
		sExp.IsClustered().Return(true),
		sExp.GetClusterMembers().Return(members, nil),
	)

	env := s.NewEnviron(c, svr, nil)

	args := s.GetStartInstanceArgs(c, "bionic")
	args.Placement = "zone=node01"

	_, err := env.StartInstance(s.callCtx, args)
	c.Assert(err, gc.ErrorMatches, "availability zone \"node01\" is unavailable")
}

func (s *environBrokerSuite) TestStartInstanceWithPlacementBadArgument(c *gc.C) {
	// Patch the host's arch, so the broker will filter tools.
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	image := containerlxd.SourcedImage{
		Image: &api.Image{Filename: "container-image"},
	}

	sExp := svr.EXPECT()
	gomock.InOrder(
		sExp.FindImage("bionic", arch.AMD64, gomock.Any(), true, gomock.Any()).Return(image, nil),
		sExp.GetProfile("default").Return(s.defaultProfile, lxdtesting.ETag, nil),
		sExp.VerifyNetworkDevice(s.defaultProfile, lxdtesting.ETag).Return(nil),
		sExp.GetNICsFromProfile("default").Return(s.defaultProfile.Devices, nil),
	)
	env := s.NewEnviron(c, svr, nil)

	args := s.GetStartInstanceArgs(c, "bionic")
	args.Placement = "breakfast=eggs"

	_, err := env.StartInstance(s.callCtx, args)
	c.Assert(err, gc.ErrorMatches, "unknown placement directive.*")
}

func (s *environBrokerSuite) TestStartInstanceWithConstraints(c *gc.C) {
	// Patch the host's arch, so the broker will filter tools.
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	// Check that the constraints were passed through to spec.Config.
	check := func(spec containerlxd.ContainerSpec) bool {
		cfg := spec.Config
		if cfg["limits.cpu"] != "2" {
			return false
		}
		if cfg["limits.memory"] != "2048MB" {
			return false
		}
		return spec.InstanceType == "t2.micro"
	}

	exp := svr.EXPECT()
	gomock.InOrder(
		exp.FindImage("bionic", arch.AMD64, gomock.Any(), true, gomock.Any()).Return(containerlxd.SourcedImage{}, nil),
		exp.GetProfile("default").Return(s.defaultProfile, lxdtesting.ETag, nil),
		exp.VerifyNetworkDevice(s.defaultProfile, lxdtesting.ETag).Return(nil),
		exp.GetNICsFromProfile("default").Return(s.defaultProfile.Devices, nil),
		exp.CreateContainerFromSpec(matchesContainerSpec(check)).Return(&containerlxd.Container{}, nil),
	)

	args := s.GetStartInstanceArgs(c, "bionic")
	cores := uint64(2)
	mem := uint64(2048)
	it := "t2.micro"
	args.Constraints = constraints.Value{
		CpuCores:     &cores,
		Mem:          &mem,
		InstanceType: &it,
	}

	env := s.NewEnviron(c, svr, nil)
	_, err := env.StartInstance(s.callCtx, args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environBrokerSuite) TestStartInstanceNoTools(c *gc.C) {
	// Patch the host's arch, so the broker will filter tools.
	s.PatchValue(&arch.HostArch, func() string { return arch.PPC64EL })

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	env := s.NewEnviron(c, svr, nil)
	_, err := env.StartInstance(s.callCtx, s.GetStartInstanceArgs(c, "bionic"))
	c.Assert(err, gc.ErrorMatches, "no matching agent binaries available")
}

func (s *environBrokerSuite) TestStopInstances(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	svr.EXPECT().RemoveContainers([]string{"juju-f75cba-1", "juju-f75cba-2"})

	env := s.NewEnviron(c, svr, nil)
	err := env.StopInstances(s.callCtx, "juju-f75cba-1", "juju-f75cba-2", "not-in-namespace-so-ignored")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environBrokerSuite) TestImageSourcesDefault(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	sources, err := lxd.GetImageSources(s.NewEnviron(c, svr, nil))
	c.Assert(err, jc.ErrorIsNil)

	s.checkSources(c, sources, []string{
		"https://streams.canonical.com/juju/images/releases/",
		"https://cloud-images.ubuntu.com/releases/",
	})
}

func (s *environBrokerSuite) TestImageMetadataURL(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	env := s.NewEnviron(c, svr, map[string]interface{}{
		"image-metadata-url": "https://my-test.com/images/",
	})

	sources, err := lxd.GetImageSources(env)
	c.Assert(err, jc.ErrorIsNil)

	s.checkSources(c, sources, []string{
		"https://my-test.com/images/",
		"https://streams.canonical.com/juju/images/releases/",
		"https://cloud-images.ubuntu.com/releases/",
	})
}

func (s *environBrokerSuite) TestImageMetadataURLEnsuresHTTPS(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	// HTTP should be converted to HTTPS.
	env := s.NewEnviron(c, svr, map[string]interface{}{
		"image-metadata-url": "http://my-test.com/images/",
	})

	sources, err := lxd.GetImageSources(env)
	c.Assert(err, jc.ErrorIsNil)

	s.checkSources(c, sources, []string{
		"https://my-test.com/images/",
		"https://streams.canonical.com/juju/images/releases/",
		"https://cloud-images.ubuntu.com/releases/",
	})
}

func (s *environBrokerSuite) TestImageStreamReleased(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	env := s.NewEnviron(c, svr, map[string]interface{}{
		"image-stream": "released",
	})

	sources, err := lxd.GetImageSources(env)
	c.Assert(err, jc.ErrorIsNil)

	s.checkSources(c, sources, []string{
		"https://streams.canonical.com/juju/images/releases/",
		"https://cloud-images.ubuntu.com/releases/",
	})
}

func (s *environBrokerSuite) TestImageStreamDaily(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	svr := lxd.NewMockServer(ctrl)

	env := s.NewEnviron(c, svr, map[string]interface{}{
		"image-stream": "daily",
	})

	sources, err := lxd.GetImageSources(env)
	c.Assert(err, jc.ErrorIsNil)

	s.checkSources(c, sources, []string{
		"https://streams.canonical.com/juju/images/daily/",
		"https://cloud-images.ubuntu.com/daily/",
	})
}

func (s *environBrokerSuite) checkSources(c *gc.C, sources []containerlxd.ServerSpec, expectedURLs []string) {
	var sourceURLs []string
	for _, source := range sources {
		sourceURLs = append(sourceURLs, source.Host)
	}
	c.Check(sourceURLs, gc.DeepEquals, expectedURLs)
}
