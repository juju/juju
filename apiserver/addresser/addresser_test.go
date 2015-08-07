// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser_test

import (
	"errors"

	gc "gopkg.in/check.v1"

	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/apiserver/addresser"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

type AddresserSuite struct {
	coretesting.BaseSuite

	st         *mockState
	api        *addresser.AddresserAPI
	authoriser apiservertesting.FakeAuthorizer
	resources  *common.Resources
}

var _ = gc.Suite(&AddresserSuite{})

func (s *AddresserSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.AddressAllocation)

	s.authoriser = apiservertesting.FakeAuthorizer{
		EnvironManager: true,
	}
	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	s.st = newMockState()
	addresser.PatchState(s, s.st)

	var err error
	s.api, err = addresser.NewAddresserAPI(nil, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *AddresserSuite) TestEnvironConfigSuccess(c *gc.C) {
	config := coretesting.EnvironConfig(c)
	s.st.setConfig(c, config)

	result, err := s.api.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.EnvironConfigResult{
		Config: config.AllAttrs(),
	})

	s.st.stub.CheckCallNames(c, "EnvironConfig")
}

func (s *AddresserSuite) TestEnvironConfigFailure(c *gc.C) {
	s.st.stub.SetErrors(errors.New("ouch"))

	result, err := s.api.EnvironConfig()
	c.Assert(err, gc.ErrorMatches, "ouch")
	c.Assert(result, jc.DeepEquals, params.EnvironConfigResult{})

	s.st.stub.CheckCallNames(c, "EnvironConfig")
}

func (s *AddresserSuite) TestCleanupIPAddressesSuccess(c *gc.C) {
	config := testingEnvConfig(c)
	s.st.setConfig(c, config)

	dead, err := s.st.DeadIPAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(len(dead), gc.Equals, 2)

	err = s.api.CleanupIPAddresses()
	c.Assert(err, gc.IsNil)

	dead, err = s.st.DeadIPAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(len(dead), gc.Equals, 0)
}

func (s *AddresserSuite) TestWatchIPAddresses(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	s.st.addIPAddressWatcher("0.1.2.3", "0.1.2.4", "0.1.2.7")

	result, err := s.api.WatchIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.EntityWatchResult{
		EntityWatcherId: "1",
		Changes: []string{
			"ipaddress-00000000-1111-2222-3333-0123456789ab",
			"ipaddress-00000000-1111-2222-4444-0123456789ab",
			"ipaddress-00000000-1111-2222-7777-0123456789ab",
		},
		Error: nil,
	})

	// Verify the resource was registered and stop when done.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.st, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

// testingEnvConfig prepares an environment configuration using
// the dummy provider.
func testingEnvConfig(c *gc.C) *config.Config {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig())
	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.Prepare(cfg, envcmd.BootstrapContext(coretesting.Context(c)), configstore.NewMem())
	c.Assert(err, jc.ErrorIsNil)
	return env.Config()
}

// DummyAddressSuite tests the Addresser server API using
// state and dummy provider for
type DummyAddresserSuite struct {
	testing.JujuConnSuite

	api        *addresser.AddresserAPI
	authoriser apiservertesting.FakeAuthorizer
	resources  *common.Resources

	machine *state.Machine
}

var _ = gc.Suite(&DummyAddresserSuite{})

func (s *DummyAddresserSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.AddressAllocation)
	s.AssertConfigParameterUpdated(c, "broken", "")

	// Add a test machine an its IP addresses.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	s.machine = machine
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	addr := network.NewAddress("0.1.2.3")
	ipAddr, err := s.State.AddIPAddress(addr, "foobar")
	c.Assert(err, jc.ErrorIsNil)
	err = ipAddr.AllocateTo(s.machine.Id(), "wobble", "")
	c.Assert(err, jc.ErrorIsNil)

	s.State.StartSync()

	// Create API.
	s.authoriser = apiservertesting.FakeAuthorizer{
		EnvironManager: true,
	}
	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	s.api, err = addresser.NewAddresserAPI(nil, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DummyAddresserSuite) TestCleanupIPAddresses(c *gc.C) {
	// Check dead addresses and IP life first.
	dead, err := s.State.DeadIPAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(len(dead), gc.Equals, 0)
	addr, err := s.State.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Life(), gc.Equals, state.Alive)

	// Now cleanup, it has nothing to do.
	err = s.api.CleanupIPAddresses()
	c.Assert(err, gc.IsNil)

	dead, err = s.State.DeadIPAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(len(dead), gc.Equals, 0)

	// Remove machine, address will be dead then.
	err = s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Remove()
	c.Assert(err, jc.ErrorIsNil)

	dead, err = s.State.DeadIPAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(len(dead), gc.Equals, 1)

	// Final cleanup, address will be removed.
	err = s.api.CleanupIPAddresses()
	c.Assert(err, gc.IsNil)

	dead, err = s.State.DeadIPAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(len(dead), gc.Equals, 0)

}
