// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddressupdater_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	apimachiner "github.com/juju/juju/api/agent/machiner"
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

	s.PatchValue(&network.AddressesForInterfaceName, func(string) ([]string, error) {
		return nil, nil
	})
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
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("localhost").AsProviderAddress(), NetPort: 1234},
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("127.0.0.1").AsProviderAddress(), NetPort: 1234},
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
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called after update")
	case servers := <-setter.servers:
		expServer := corenetwork.ProviderHostPorts{
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("localhost").AsProviderAddress(), NetPort: 1234},
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("127.0.0.1").AsProviderAddress(), NetPort: 1234},
		}.HostPorts()
		c.Assert(servers, gc.DeepEquals, []corenetwork.HostPorts{expServer})
	}
}

func (s *APIAddressUpdaterSuite) TestAddressChangeEmpty(c *gc.C) {
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

	// SetAPIHostPorts should be called with the initial value (empty),
	// and then the updated value.
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called initially")
	case servers := <-setter.servers:
		c.Assert(servers, gc.HasLen, 0)
	}

	updatedServers := []corenetwork.SpaceHostPorts{
		corenetwork.NewSpaceHostPorts(1234, "localhost", "127.0.0.1"),
	}

	err = s.State.SetAPIHostPorts(updatedServers)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called after update")
	case servers := <-setter.servers:
		expServer := corenetwork.ProviderHostPorts{
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("localhost").AsProviderAddress(), NetPort: 1234},
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("127.0.0.1").AsProviderAddress(), NetPort: 1234},
		}.HostPorts()
		c.Assert(servers, gc.DeepEquals, []corenetwork.HostPorts{expServer})
	}

	updatedServers = []corenetwork.SpaceHostPorts{}
	err = s.State.SetAPIHostPorts(updatedServers)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called after update")
	case servers := <-setter.servers:
		expServer := corenetwork.ProviderHostPorts{
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("localhost").AsProviderAddress(), NetPort: 1234},
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("127.0.0.1").AsProviderAddress(), NetPort: 1234},
		}.HostPorts()
		c.Assert(servers, gc.DeepEquals, []corenetwork.HostPorts{expServer})
	}
}

func (s *APIAddressUpdaterSuite) TestBridgeAddressesFiltering(c *gc.C) {
	s.PatchValue(&network.AddressesForInterfaceName, func(name string) ([]string, error) {
		if name == network.DefaultLXDBridge {
			return []string{
				"10.0.4.1",
				"10.0.4.4",
			}, nil
		} else if name == network.DefaultKVMBridge {
			return []string{
				"192.168.122.1",
			}, nil
		}
		c.Fatalf("unknown bridge in testing: %v", name)
		return nil, nil
	})

	initialServers := []corenetwork.SpaceHostPorts{
		corenetwork.NewSpaceHostPorts(1234, "localhost", "127.0.0.1"),
		corenetwork.NewSpaceHostPorts(
			4321,
			"10.0.3.3",      // not filtered
			"10.0.4.1",      // filtered lxd bridge address
			"10.0.4.2",      // not filtered
			"192.168.122.1", // filtered default virbr0
		),
	}
	err := s.State.SetAPIHostPorts(initialServers)
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

	updatedServers := []corenetwork.SpaceHostPorts{
		corenetwork.NewSpaceHostPorts(1234, "localhost", "127.0.0.1"),
		corenetwork.NewSpaceHostPorts(
			4001,
			"10.0.3.3", // not filtered
		),
	}

	expServer1 := corenetwork.ProviderHostPorts{
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("localhost").AsProviderAddress(), NetPort: 1234},
		corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("127.0.0.1").AsProviderAddress(), NetPort: 1234},
	}.HostPorts()

	// SetAPIHostPorts should be called with the initial value, and
	// then the updated value, but filtering occurs in both cases.
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called initially")
	case servers := <-setter.servers:
		c.Assert(servers, gc.HasLen, 2)

		expServerInit := corenetwork.ProviderHostPorts{
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("10.0.3.3").AsProviderAddress(), NetPort: 4321},
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("10.0.4.2").AsProviderAddress(), NetPort: 4321},
		}.HostPorts()
		c.Assert(servers, jc.DeepEquals, []corenetwork.HostPorts{expServer1, expServerInit})
	}

	err = s.State.SetAPIHostPorts(updatedServers)
	c.Assert(err, gc.IsNil)
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called after update")
	case servers := <-setter.servers:
		c.Assert(servers, gc.HasLen, 2)

		expServerUpd := corenetwork.ProviderHostPorts{
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("10.0.3.3").AsProviderAddress(), NetPort: 4001},
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
