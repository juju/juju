// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/addresser"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
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

func (s *AddresserSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	environs.RegisterProvider("mock", mockEnvironProvider{})
}

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

func (s *AddresserSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	s.BaseSuite.TearDownTest(c)
}

func (s *AddresserSuite) TestCanDeallocateAddressesEnabled(c *gc.C) {
	config := testingEnvConfig(c)
	s.st.setConfig(c, config)

	result := s.api.CanDeallocateAddresses()
	c.Assert(result, jc.DeepEquals, params.BoolResult{
		Error:  nil,
		Result: true,
	})
}

func (s *AddresserSuite) TestCanDeallocateAddressesDisabled(c *gc.C) {
	config := testingEnvConfig(c)
	s.st.setConfig(c, config)
	s.SetFeatureFlags()

	result := s.api.CanDeallocateAddresses()
	c.Assert(result, jc.DeepEquals, params.BoolResult{
		Error:  nil,
		Result: false,
	})
}

func (s *AddresserSuite) TestCanDeallocateAddressesConfigGetFailure(c *gc.C) {
	config := testingEnvConfig(c)
	s.st.setConfig(c, config)

	s.st.stub.SetErrors(errors.New("ouch"))

	result := s.api.CanDeallocateAddresses()
	c.Assert(result.Error, gc.ErrorMatches, "getting environment config: ouch")
	c.Assert(result.Result, jc.IsFalse)
}

func (s *AddresserSuite) TestCanDeallocateAddressesEnvironmentNewFailure(c *gc.C) {
	config := nonexTestingEnvConfig(c)
	s.st.setConfig(c, config)

	result := s.api.CanDeallocateAddresses()
	c.Assert(result.Error, gc.ErrorMatches, `validating environment config: no registered provider for "nonex"`)
	c.Assert(result.Result, jc.IsFalse)
}

func (s *AddresserSuite) TestCanDeallocateAddressesNotSupportedFailure(c *gc.C) {
	config := mockTestingEnvConfig(c)
	s.st.setConfig(c, config)

	result := s.api.CanDeallocateAddresses()
	c.Assert(result, jc.DeepEquals, params.BoolResult{
		Error:  nil,
		Result: false,
	})
}

func (s *AddresserSuite) TestCleanupIPAddressesSuccess(c *gc.C) {
	config := testingEnvConfig(c)
	s.st.setConfig(c, config)

	dead, err := s.st.DeadIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dead, gc.HasLen, 2)

	apiErr := s.api.CleanupIPAddresses()
	c.Assert(apiErr, jc.DeepEquals, params.ErrorResult{})

	dead, err = s.st.DeadIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dead, gc.HasLen, 0)
}

func (s *AddresserSuite) TestReleaseAddress(c *gc.C) {
	config := testingEnvConfig(c)
	s.st.setConfig(c, config)

	// Cleanup initial dead IP addresses.
	dead, err := s.st.DeadIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dead, gc.HasLen, 2)

	apiErr := s.api.CleanupIPAddresses()
	c.Assert(apiErr, jc.DeepEquals, params.ErrorResult{})

	dead, err = s.st.DeadIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dead, gc.HasLen, 0)

	// Prepare tests.
	called := 0
	s.PatchValue(addresser.NetEnvReleaseAddress, func(env environs.NetworkingEnviron,
		instId instance.Id, subnetId network.Id, addr network.Address, macAddress string) error {
		called++
		c.Assert(instId, gc.Equals, instance.Id("a3"))
		c.Assert(subnetId, gc.Equals, network.Id("a"))
		c.Assert(addr, gc.Equals, network.NewAddress("0.1.2.3"))
		c.Assert(macAddress, gc.Equals, "fff3")
		return nil
	})

	// Set address 0.1.2.3 to dead.
	s.st.setDead(c, "0.1.2.3")

	dead, err = s.st.DeadIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dead, gc.HasLen, 1)

	apiErr = s.api.CleanupIPAddresses()
	c.Assert(apiErr, jc.DeepEquals, params.ErrorResult{})
	c.Assert(called, gc.Equals, 1)

	dead, err = s.st.DeadIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dead, gc.HasLen, 0)
}

func (s *AddresserSuite) TestCleanupIPAddressesConfigGetFailure(c *gc.C) {
	config := testingEnvConfig(c)
	s.st.setConfig(c, config)

	dead, err := s.st.DeadIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dead, gc.HasLen, 2)

	s.st.stub.SetErrors(errors.New("ouch"))

	// First action is getting the environment configuration,
	// so the injected error is returned here.
	apiErr := s.api.CleanupIPAddresses()
	c.Assert(apiErr.Error, gc.ErrorMatches, "getting environment config: ouch")

	// Still has two dead addresses.
	dead, err = s.st.DeadIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dead, gc.HasLen, 2)
}

func (s *AddresserSuite) TestCleanupIPAddressesEnvironmentNewFailure(c *gc.C) {
	config := nonexTestingEnvConfig(c)
	s.st.setConfig(c, config)

	dead, err := s.st.DeadIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dead, gc.HasLen, 2)

	// Validation of configuration fails due to illegal provider.
	apiErr := s.api.CleanupIPAddresses()
	c.Assert(apiErr.Error, gc.ErrorMatches, `validating environment config: no registered provider for "nonex"`)

	// Still has two dead addresses.
	dead, err = s.st.DeadIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dead, gc.HasLen, 2)
}

func (s *AddresserSuite) TestCleanupIPAddressesNotSupportedFailure(c *gc.C) {
	config := mockTestingEnvConfig(c)
	s.st.setConfig(c, config)

	dead, err := s.st.DeadIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dead, gc.HasLen, 2)

	// The tideland environment does not support networking.
	apiErr := s.api.CleanupIPAddresses()
	c.Assert(apiErr.Error, gc.ErrorMatches, "IP address deallocation not supported")

	// Still has two dead addresses.
	dead, err = s.st.DeadIPAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dead, gc.HasLen, 2)
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

// nonexTestingEnvConfig prepares an environment configuration using
// a non-existent provider.
func nonexTestingEnvConfig(c *gc.C) *config.Config {
	attrs := dummy.SampleConfig().Merge(coretesting.Attrs{
		"type": "nonex",
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

// mockTestingEnvConfig prepares an environment configuration using
// the mock provider which does not support networking.
func mockTestingEnvConfig(c *gc.C) *config.Config {
	cfg, err := config.New(config.NoDefaults, mockConfig())
	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.Prepare(cfg, envcmd.BootstrapContext(coretesting.Context(c)), configstore.NewMem())
	c.Assert(err, jc.ErrorIsNil)
	return env.Config()
}
