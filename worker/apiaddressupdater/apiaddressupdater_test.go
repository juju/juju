// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddressupdater_test

import (
	"io/ioutil"
	"net"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	apimachiner "github.com/juju/juju/api/machiner"
	corenetwork "github.com/juju/juju/core/network"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/apiaddressupdater"
)

type APIAddressUpdaterSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&APIAddressUpdaterSuite{})

func (s *APIAddressUpdaterSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	err := s.State.SetAPIHostPorts(nil)
	c.Assert(err, jc.ErrorIsNil)
	// By default mock these to better isolate the test from the real machine.
	s.PatchValue(&network.InterfaceByNameAddrs, func(string) ([]net.Addr, error) {
		return nil, nil
	})
	s.PatchValue(&network.LXCNetDefaultConfig, "")
}

type apiAddressSetter struct {
	servers chan []corenetwork.HostPorts
	err     error
}

func (s *apiAddressSetter) SetAPIHostPorts(servers []corenetwork.HostPorts) error {
	s.servers <- servers
	return s.err
}

func (s *APIAddressUpdaterSuite) TestStartStop(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	worker, err := apiaddressupdater.NewAPIAddressUpdater(
		apiaddressupdater.Config{
			Addresser: apimachiner.NewState(st),
			Setter:    &apiAddressSetter{},
			Logger:    loggo.GetLogger("test"),
		})
	c.Assert(err, jc.ErrorIsNil)
	worker.Kill()
	c.Assert(worker.Wait(), gc.IsNil)
}

func (s *APIAddressUpdaterSuite) TestAddressInitialUpdate(c *gc.C) {
	updatedServers := []corenetwork.SpaceHostPorts{corenetwork.NewSpaceHostPorts(1234, "localhost", "127.0.0.1")}
	err := s.State.SetAPIHostPorts(updatedServers)
	c.Assert(err, jc.ErrorIsNil)

	setter := &apiAddressSetter{servers: make(chan []corenetwork.HostPorts, 1)}
	st, _ := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	updater, err := apiaddressupdater.NewAPIAddressUpdater(
		apiaddressupdater.Config{
			Addresser: apimachiner.NewState(st),
			Setter:    setter,
			Logger:    loggo.GetLogger("test"),
		})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, updater)

	expServer := corenetwork.ProviderHostPorts{
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewProviderAddress("localhost"), NetPort: 1234},
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewProviderAddress("127.0.0.1"), NetPort: 1234},
	}.HostPorts()

	// SetAPIHostPorts should be called with the initial value.
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called")
	case servers := <-setter.servers:
		c.Assert(servers, gc.DeepEquals, []corenetwork.HostPorts{expServer})
	}

	// The values are also available through the report.
	reporter, ok := updater.(worker.Reporter)
	c.Assert(ok, jc.IsTrue)
	c.Assert(reporter.Report(), jc.DeepEquals, map[string]interface{}{
		"servers": [][]string{{"localhost:1234", "127.0.0.1:1234"}},
	})

}

func (s *APIAddressUpdaterSuite) TestAddressChange(c *gc.C) {
	setter := &apiAddressSetter{servers: make(chan []corenetwork.HostPorts, 1)}
	st, _ := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	worker, err := apiaddressupdater.NewAPIAddressUpdater(
		apiaddressupdater.Config{
			Addresser: apimachiner.NewState(st),
			Setter:    setter,
			Logger:    loggo.GetLogger("test"),
		})
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()
	s.BackingState.StartSync()
	updatedServers := []corenetwork.SpaceHostPorts{
		corenetwork.NewSpaceHostPorts(1234, "localhost", "127.0.0.1"),
	}
	// SetAPIHostPorts should be called with the initial value (empty),
	// and then the updated value.
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called initially")
	case servers := <-setter.servers:
		c.Assert(servers, gc.HasLen, 0)
	}
	err = s.State.SetAPIHostPorts(updatedServers)
	c.Assert(err, jc.ErrorIsNil)
	s.BackingState.StartSync()
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called after update")
	case servers := <-setter.servers:
		expServer := corenetwork.ProviderHostPorts{
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewProviderAddress("localhost"), NetPort: 1234},
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewProviderAddress("127.0.0.1"), NetPort: 1234},
		}.HostPorts()
		c.Assert(servers, gc.DeepEquals, []corenetwork.HostPorts{expServer})
	}
}

func (s *APIAddressUpdaterSuite) TestBridgeAddressesFiltering(c *gc.C) {
	lxcFakeNetConfig := filepath.Join(c.MkDir(), "lxc-net")
	netConf := []byte(`
  # comments ignored
LXC_BR= ignored
LXC_ADDR = "fooo"
LXC_BRIDGE="foobar" # detected
anything else ignored
LXC_BRIDGE="ignored"`[1:])
	err := ioutil.WriteFile(lxcFakeNetConfig, netConf, 0644)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(&network.InterfaceByNameAddrs, func(name string) ([]net.Addr, error) {
		if name == "foobar" {
			// The addresses on the LXC bridge
			return []net.Addr{
				&net.IPAddr{IP: net.IPv4(10, 0, 3, 1)},
				&net.IPAddr{IP: net.IPv4(10, 0, 3, 4)},
			}, nil
		} else if name == network.DefaultLXDBridge {
			// The addresses on the LXD bridge
			return []net.Addr{
				&net.IPAddr{IP: net.IPv4(10, 0, 4, 1)},
				&net.IPAddr{IP: net.IPv4(10, 0, 4, 4)},
			}, nil
		} else if name == network.DefaultKVMBridge {
			return []net.Addr{
				&net.IPAddr{IP: net.IPv4(192, 168, 122, 1)},
			}, nil
		}
		c.Fatalf("unknown bridge in testing: %v", name)
		return nil, nil
	})
	s.PatchValue(&network.LXCNetDefaultConfig, lxcFakeNetConfig)

	initialServers := []corenetwork.SpaceHostPorts{
		corenetwork.NewSpaceHostPorts(1234, "localhost", "127.0.0.1"),
		corenetwork.NewSpaceHostPorts(
			4321,
			"10.0.3.1",      // filtered
			"10.0.3.3",      // not filtered (not a lxc bridge address)
			"10.0.4.1",      // filtered lxd bridge address
			"10.0.4.2",      // not filtered
			"192.168.122.1", // filtered default virbr0
		),
		corenetwork.NewSpaceHostPorts(4242, "10.0.3.4"), // filtered
	}
	err = s.State.SetAPIHostPorts(initialServers)
	c.Assert(err, jc.ErrorIsNil)

	setter := &apiAddressSetter{servers: make(chan []corenetwork.HostPorts, 1)}
	st, _ := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	w, err := apiaddressupdater.NewAPIAddressUpdater(
		apiaddressupdater.Config{
			Addresser: apimachiner.NewState(st),
			Setter:    setter,
			Logger:    loggo.GetLogger("test"),
		})
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(w.Wait(), gc.IsNil) }()
	defer w.Kill()
	s.BackingState.StartSync()

	updatedServers := []corenetwork.SpaceHostPorts{
		corenetwork.NewSpaceHostPorts(1234, "localhost", "127.0.0.1"),
		corenetwork.NewSpaceHostPorts(
			4001,
			"10.0.3.1", // filtered
			"10.0.3.3", // not filtered (not a lxc bridge address)
		),
		corenetwork.NewSpaceHostPorts(4200, "10.0.3.4"), // filtered
		corenetwork.NewSpaceHostPorts(4200, "10.0.4.1"), // filtered
	}

	expServer1 := corenetwork.ProviderHostPorts{
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewProviderAddress("localhost"), NetPort: 1234},
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewProviderAddress("127.0.0.1"), NetPort: 1234},
	}.HostPorts()

	// SetAPIHostPorts should be called with the initial value, and
	// then the updated value, but filtering occurs in both cases.
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called initially")
	case servers := <-setter.servers:
		c.Assert(servers, gc.HasLen, 2)

		expServerInit := corenetwork.ProviderHostPorts{
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewProviderAddress("10.0.3.3"), NetPort: 4321},
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewProviderAddress("10.0.4.2"), NetPort: 4321},
		}.HostPorts()
		c.Assert(servers, jc.DeepEquals, []corenetwork.HostPorts{expServer1, expServerInit})
	}

	err = s.State.SetAPIHostPorts(updatedServers)
	c.Assert(err, gc.IsNil)
	s.BackingState.StartSync()
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called after update")
	case servers := <-setter.servers:
		c.Assert(servers, gc.HasLen, 2)

		expServerUpd := corenetwork.ProviderHostPorts{
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewProviderAddress("10.0.3.3"), NetPort: 4001},
		}.HostPorts()
		c.Assert(servers, jc.DeepEquals, []corenetwork.HostPorts{expServer1, expServerUpd})
	}
}

type ValidateSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ValidateSuite{})

func (*ValidateSuite) TestValid(c *gc.C) {
	err := validConfig().Validate()
	c.Check(err, jc.ErrorIsNil)
}

func (*ValidateSuite) TestMissingAddresser(c *gc.C) {
	config := validConfig()
	config.Addresser = nil
	checkNotValid(c, config, "nil Addresser not valid")
}

func (*ValidateSuite) TestMissingSetter(c *gc.C) {
	config := validConfig()
	config.Setter = nil
	checkNotValid(c, config, "nil Setter not valid")
}

func (*ValidateSuite) TestMissingLogger(c *gc.C) {
	config := validConfig()
	config.Logger = nil
	checkNotValid(c, config, "nil Logger not valid")
}

func validConfig() apiaddressupdater.Config {
	return apiaddressupdater.Config{
		Addresser: struct{ apiaddressupdater.APIAddresser }{},
		Setter: struct {
			apiaddressupdater.APIAddressSetter
		}{},
		Logger: loggo.GetLogger("test"),
	}
}

func checkNotValid(c *gc.C, config apiaddressupdater.Config, expect string) {
	check := func(err error) {
		c.Check(err, gc.ErrorMatches, expect)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	}

	err := config.Validate()
	check(err)

	worker, err := apiaddressupdater.NewAPIAddressUpdater(config)
	c.Check(worker, gc.IsNil)
	check(err)
}
