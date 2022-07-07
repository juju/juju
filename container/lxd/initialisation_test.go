// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build linux
// +build linux

package lxd

import (
	"errors"
	"fmt"
	"net"

	"github.com/golang/mock/gomock"
	"github.com/juju/packaging/v2/commands"
	"github.com/juju/packaging/v2/manager"
	"github.com/juju/proxy"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	lxd "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxd/mocks"
	lxdtesting "github.com/juju/juju/container/lxd/testing"
	coretesting "github.com/juju/juju/testing"
)

type initialiserTestSuite struct {
	coretesting.BaseSuite
	testing.PatchExecHelper
}

// patchDF100GB ensures that df always returns 100GB.
func (s *initialiserTestSuite) patchDF100GB() {
	df100 := func(path string) (uint64, error) {
		return 100 * 1024 * 1024 * 1024, nil
	}
	s.PatchValue(&df, df100)
}

type InitialiserSuite struct {
	initialiserTestSuite
	calledCmds []string
}

var _ = gc.Suite(&InitialiserSuite{})

const lxdSnapChannel = "latest/stable"

func (s *InitialiserSuite) SetUpTest(c *gc.C) {
	coretesting.SkipLXDNotSupported(c)
	s.initialiserTestSuite.SetUpTest(c)
	s.calledCmds = []string{}
	s.PatchValue(&manager.RunCommandWithRetry, getMockRunCommandWithRetry(&s.calledCmds))

	nonRandomizedOctetRange := func() []int {
		// chosen by fair dice roll
		// guaranteed to be random :)
		// intentionally not random to allow for deterministic tests
		return []int{4, 5, 6, 7, 8}
	}
	s.PatchValue(&randomizedOctetRange, nonRandomizedOctetRange)
	// Fake the lxc executable for all the tests.
	testing.PatchExecutableAsEchoArgs(c, s, "lxc")
	testing.PatchExecutableAsEchoArgs(c, s, "lxd")
}

// getMockRunCommandWithRetry is a helper function which returns a function
// with an identical signature to manager.RunCommandWithRetry which saves each
// command it receives in a slice and always returns no output, error code 0
// and a nil error.
func getMockRunCommandWithRetry(calledCmds *[]string) func(string, manager.Retryable, manager.RetryPolicy) (string, int, error) {
	return func(cmd string, _ manager.Retryable, _ manager.RetryPolicy) (string, int, error) {
		*calledCmds = append(*calledCmds, cmd)
		return "", 0, nil
	}
}

func (s *initialiserTestSuite) containerInitialiser(svr lxd.ContainerServer, lxdIsRunning bool) *containerInitialiser {
	result := NewContainerInitialiser(lxdSnapChannel).(*containerInitialiser)
	result.configureLxdBridge = func() error { return nil }
	result.configureLxdProxies = func(proxy.Settings, func() (bool, error), func() (*Server, error)) error { return nil }
	result.newLocalServer = func() (*Server, error) { return NewServer(svr) }
	result.isRunningLocally = func() (bool, error) {
		return lxdIsRunning, nil
	}
	return result
}

func (s *InitialiserSuite) TestSnapInstalledNoAptInstall(c *gc.C) {
	PatchLXDViaSnap(s, true)
	PatchHostSeries(s, "cosmic")

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mgr := mocks.NewMockSnapManager(ctrl)
	mgr.EXPECT().InstalledChannel("lxd").Return("latest/stable")
	PatchGetSnapManager(s, mgr)

	err := s.containerInitialiser(nil, true).Initialise()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.calledCmds, gc.DeepEquals, []string{})
}

func (s *InitialiserSuite) TestSnapChannelMismatch(c *gc.C) {
	PatchLXDViaSnap(s, true)
	PatchHostSeries(s, "focal")

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mgr := mocks.NewMockSnapManager(ctrl)
	gomock.InOrder(
		mgr.EXPECT().InstalledChannel("lxd").Return("3.2/stable"),
		mgr.EXPECT().ChangeChannel("lxd", lxdSnapChannel),
	)
	PatchGetSnapManager(s, mgr)

	err := s.containerInitialiser(nil, true).Initialise()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InitialiserSuite) TestSnapChannelPrefixMatch(c *gc.C) {
	PatchLXDViaSnap(s, true)
	PatchHostSeries(s, "focal")

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mgr := mocks.NewMockSnapManager(ctrl)
	gomock.InOrder(
		// The channel for the installed lxd snap also includes the
		// branch for the focal release. The "track/risk" prefix is
		// the same however so the container manager should not attempt
		// to change the channel.
		mgr.EXPECT().InstalledChannel("lxd").Return("latest/stable/ubuntu-20.04"),
	)
	PatchGetSnapManager(s, mgr)

	err := s.containerInitialiser(nil, true).Initialise()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InitialiserSuite) TestNoSeriesPackages(c *gc.C) {
	PatchLXDViaSnap(s, false)

	// Here we want to test for any other series whilst avoiding the
	// possibility of hitting a cloud archive-requiring release.
	// As such, we simply pass an empty series.
	PatchHostSeries(s, "")

	paccmder, err := commands.NewPackageCommander("xenial")
	c.Assert(err, jc.ErrorIsNil)

	err = s.containerInitialiser(nil, true).Initialise()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.calledCmds, gc.DeepEquals, []string{
		paccmder.InstallCmd("lxd"),
	})
}

func (s *InitialiserSuite) TestInstallViaSnap(c *gc.C) {
	PatchLXDViaSnap(s, false)

	PatchHostSeries(s, "disco")

	paccmder := commands.NewSnapPackageCommander()

	err := s.containerInitialiser(nil, true).Initialise()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.calledCmds, gc.DeepEquals, []string{
		paccmder.InstallCmd("--classic --channel latest/stable lxd"),
	})
}

func (s *InitialiserSuite) TestLXDInitBionic(c *gc.C) {
	s.patchDF100GB()
	PatchHostSeries(s, "bionic")

	err := s.containerInitialiser(nil, true).Initialise()
	c.Assert(err, jc.ErrorIsNil)

	testing.AssertEchoArgs(c, "lxd", "init", "--auto")
}

func (s *InitialiserSuite) TestLXDAlreadyInitialized(c *gc.C) {
	s.patchDF100GB()
	PatchHostSeries(s, "bionic")

	ci := s.containerInitialiser(nil, true)
	ci.getExecCommand = s.PatchExecHelper.GetExecCommand(testing.PatchExecConfig{
		Stderr:   `error: You have existing containers or images. lxd init requires an empty LXD.`,
		ExitCode: 1,
	})

	// the above error should be ignored by the code that calls lxd init.
	err := ci.Initialise()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InitialiserSuite) TestInitializeSetsProxies(c *gc.C) {
	PatchHostSeries(s, "")

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := lxdtesting.NewMockContainerServer(ctrl)

	s.PatchEnvironment("http_proxy", "http://test.local/http/proxy")
	s.PatchEnvironment("https_proxy", "http://test.local/https/proxy")
	s.PatchEnvironment("no_proxy", "test.local,localhost")

	updateReq := api.ServerPut{Config: map[string]interface{}{
		"core.proxy_http":         "http://test.local/http/proxy",
		"core.proxy_https":        "http://test.local/https/proxy",
		"core.proxy_ignore_hosts": "test.local,localhost",
	}}
	gomock.InOrder(
		cSvr.EXPECT().GetServer().Return(&api.Server{}, lxdtesting.ETag, nil).Times(2),
		cSvr.EXPECT().UpdateServer(updateReq, lxdtesting.ETag).Return(nil),
	)

	ci := s.containerInitialiser(cSvr, true)
	ci.configureLxdProxies = internalConfigureLXDProxies
	err := ci.Initialise()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InitialiserSuite) TestConfigureProxiesLXDNotRunning(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := lxdtesting.NewMockContainerServer(ctrl)

	s.PatchEnvironment("http_proxy", "http://test.local/http/proxy")
	s.PatchEnvironment("https_proxy", "http://test.local/https/proxy")
	s.PatchEnvironment("no_proxy", "test.local,localhost")

	// No expected calls.
	ci := s.containerInitialiser(cSvr, false)
	err := ci.Initialise()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InitialiserSuite) TestFindAvailableSubnetWithInterfaceAddrsError(c *gc.C) {
	s.PatchValue(&interfaceAddrs, func() ([]net.Addr, error) {
		return nil, errors.New("boom!")
	})
	subnet, err := findNextAvailableIPv4Subnet()
	c.Assert(err, gc.ErrorMatches, "cannot get network interface addresses: boom!")
	c.Assert(subnet, gc.Equals, "")
}

type testFindSubnetAddr struct {
	val string
}

func (a testFindSubnetAddr) Network() string {
	return "ip+net"
}

func (a testFindSubnetAddr) String() string {
	return a.val
}

func testAddresses(c *gc.C, networks ...string) ([]net.Addr, error) {
	addrs := make([]net.Addr, 0)
	for _, n := range networks {
		_, _, err := net.ParseCIDR(n)
		if err != nil {
			return nil, err
		}
		c.Assert(err, gc.IsNil)
		addrs = append(addrs, testFindSubnetAddr{n})
	}
	return addrs, nil
}

func (s *InitialiserSuite) TestFindAvailableSubnetWithNoAddresses(c *gc.C) {
	s.PatchValue(&interfaceAddrs, func() ([]net.Addr, error) {
		return testAddresses(c)
	})
	subnet, err := findNextAvailableIPv4Subnet()
	c.Assert(err, gc.IsNil)
	c.Assert(subnet, gc.Equals, "4")
}

func (s *InitialiserSuite) TestFindAvailableSubnetWithIPv6Only(c *gc.C) {
	s.PatchValue(&interfaceAddrs, func() ([]net.Addr, error) {
		return testAddresses(c, "fe80::aa8e:a275:7ae0:34af/64")
	})
	subnet, err := findNextAvailableIPv4Subnet()
	c.Assert(err, gc.IsNil)
	c.Assert(subnet, gc.Equals, "4")
}

func (s *InitialiserSuite) TestFindAvailableSubnetWithIPv4OnlyAndNo10xSubnet(c *gc.C) {
	s.PatchValue(&interfaceAddrs, func() ([]net.Addr, error) {
		return testAddresses(c, "192.168.1.64/24")
	})
	subnet, err := findNextAvailableIPv4Subnet()
	c.Assert(err, gc.IsNil)
	c.Assert(subnet, gc.Equals, "4")
}

func (s *InitialiserSuite) TestFindAvailableSubnetWithInvalidCIDR(c *gc.C) {
	s.PatchValue(&interfaceAddrs, func() ([]net.Addr, error) {
		return []net.Addr{
			testFindSubnetAddr{"10.0.0.1"},
			testFindSubnetAddr{"10.0.5.1/24"}}, nil
	})
	subnet, err := findNextAvailableIPv4Subnet()
	c.Assert(err, gc.IsNil)
	c.Assert(subnet, gc.Equals, "4")
}

func (s *InitialiserSuite) TestFindAvailableSubnetWithIPv4AndExisting10xNetwork(c *gc.C) {
	s.PatchValue(&interfaceAddrs, func() ([]net.Addr, error) {
		return testAddresses(c, "192.168.1.64/24", "10.0.0.1/24")
	})
	subnet, err := findNextAvailableIPv4Subnet()
	c.Assert(err, gc.IsNil)
	c.Assert(subnet, gc.Equals, "4")
}

func (s *InitialiserSuite) TestFindAvailableSubnetWithExisting10xNetworks(c *gc.C) {
	s.PatchValue(&interfaceAddrs, func() ([]net.Addr, error) {
		// Note that 10.0.4.0 is a /23, so that includes 10.0.4.0/24 and 10.0.5.0/24
		// And the one for 10.0.7.0/23 is also a /23 so it includes 10.0.6.0/24 as well as 10.0.7.0/24
		return testAddresses(c, "192.168.1.0/24", "10.0.4.1/23", "10.0.7.5/23",
			"::1/128", "10.0.3.1/24", "fe80::aa8e:a275:7ae0:34af/64")
	})
	subnet, err := findNextAvailableIPv4Subnet()
	c.Assert(err, gc.IsNil)
	c.Assert(subnet, gc.Equals, "8")
}

func (s *InitialiserSuite) TestFindAvailableSubnetUpperBoundInUse(c *gc.C) {
	s.PatchValue(&interfaceAddrs, func() ([]net.Addr, error) {
		return testAddresses(c, "10.0.255.1/24")
	})
	subnet, err := findNextAvailableIPv4Subnet()
	c.Assert(err, gc.IsNil)
	c.Assert(subnet, gc.Equals, "4")
}

func (s *InitialiserSuite) TestFindAvailableSubnetUpperBoundAndLowerBoundInUse(c *gc.C) {
	s.PatchValue(&interfaceAddrs, func() ([]net.Addr, error) {
		return testAddresses(c, "10.0.255.1/24", "10.0.0.1/24")
	})
	subnet, err := findNextAvailableIPv4Subnet()
	c.Assert(err, gc.IsNil)
	c.Assert(subnet, gc.Equals, "4")
}

func (s *InitialiserSuite) TestFindAvailableSubnetWithFull10xSubnet(c *gc.C) {
	s.PatchValue(&interfaceAddrs, func() ([]net.Addr, error) {
		addrs := make([]net.Addr, 256)
		for i := 0; i < 256; i++ {
			subnet := fmt.Sprintf("10.0.%v.1/24", i)
			addrs[i] = testFindSubnetAddr{subnet}
		}
		return addrs, nil
	})
	subnet, err := findNextAvailableIPv4Subnet()
	c.Assert(err, gc.ErrorMatches, "could not find unused subnet")
	c.Assert(subnet, gc.Equals, "")
}

type ConfigureInitialiserSuite struct {
	initialiserTestSuite
	testing.PatchExecHelper
}

var _ = gc.Suite(&ConfigureInitialiserSuite{})

func (s *ConfigureInitialiserSuite) SetUpTest(c *gc.C) {
	s.initialiserTestSuite.SetUpTest(c)
	// Fake the lxc executable for all the tests.
	testing.PatchExecutableAsEchoArgs(c, s, "lxc")
	testing.PatchExecutableAsEchoArgs(c, s, "lxd")
}

func (s *ConfigureInitialiserSuite) TestConfigureLXDBridge(c *gc.C) {
	s.patchDF100GB()
	PatchLXDViaSnap(s, true)
	PatchHostSeries(s, "bionic")

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := lxdtesting.NewMockContainerServer(ctrl)

	mgr := mocks.NewMockSnapManager(ctrl)
	mgr.EXPECT().InstalledChannel("lxd").Return("latest/stable")
	PatchGetSnapManager(s, mgr)

	// The following nic is found, so we don't create a default nic and so
	// don't update the profile with the nic.
	profile := &api.Profile{
		ProfilePut: api.ProfilePut{
			Devices: map[string]map[string]string{
				"eth0": {
					"type":    "nic",
					"nictype": "bridged",
					"parent":  "lxdbr1",
				},
			},
		},
	}
	network := &api.Network{
		Managed: false,
	}
	gomock.InOrder(
		cSvr.EXPECT().GetServer().Return(&api.Server{
			ServerUntrusted: api.ServerUntrusted{
				APIExtensions: []string{
					"network",
				},
			},
		}, lxdtesting.ETag, nil),
		cSvr.EXPECT().GetProfile(lxdDefaultProfileName).Return(profile, "", nil),
		cSvr.EXPECT().GetNetwork("lxdbr1").Return(network, "", nil),
	)

	ci := s.containerInitialiser(cSvr, true)
	ci.configureLxdBridge = ci.internalConfigureLXDBridge
	err := ci.Initialise()
	c.Assert(err, jc.ErrorIsNil)

	testing.AssertEchoArgs(c, "lxd", "init", "--auto")
}

func (s *ConfigureInitialiserSuite) TestConfigureLXDBridgeWithoutNicsCreatesANewOne(c *gc.C) {
	s.patchDF100GB()
	PatchLXDViaSnap(s, true)
	PatchHostSeries(s, "bionic")

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := lxdtesting.NewMockContainerServer(ctrl)

	mgr := mocks.NewMockSnapManager(ctrl)
	mgr.EXPECT().InstalledChannel("lxd").Return("latest/stable")
	PatchGetSnapManager(s, mgr)

	// If no nics are found in the profile, then the configureLXDBridge will
	// create a default nic for you.
	profile := &api.Profile{
		Name: lxdDefaultProfileName,
		ProfilePut: api.ProfilePut{
			Devices: map[string]map[string]string{},
		},
	}
	network := &api.Network{
		Managed: false,
	}
	updatedProfile := api.ProfilePut{
		Devices: map[string]map[string]string{
			"eth0": {
				"type":    "nic",
				"nictype": "macvlan",
				"parent":  "lxdbr0",
			},
		},
	}
	gomock.InOrder(
		cSvr.EXPECT().GetServer().Return(&api.Server{
			ServerUntrusted: api.ServerUntrusted{
				APIExtensions: []string{
					"network",
				},
			},
		}, lxdtesting.ETag, nil),
		cSvr.EXPECT().GetProfile(lxdDefaultProfileName).Return(profile, "", nil),
		cSvr.EXPECT().GetNetwork("lxdbr0").Return(network, "", nil),
		// Because no nic was found, we create the nic info and then update the
		// update profile with that nic information.
		cSvr.EXPECT().UpdateProfile(lxdDefaultProfileName, updatedProfile, gomock.Any()).Return(nil),
	)

	ci := s.containerInitialiser(cSvr, true)
	ci.configureLxdBridge = ci.internalConfigureLXDBridge
	err := ci.Initialise()
	c.Assert(err, jc.ErrorIsNil)

	testing.AssertEchoArgs(c, "lxd", "init", "--auto")
}
