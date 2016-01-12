// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/provisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// containerSuite has methods useful to tests working with containers. Notably
// around testing PrepareContainerInterfaceInfo and ReleaseContainerAddresses.
type containerSuite struct {
	provisionerSuite

	provAPI *provisioner.ProvisionerAPI
}

func (s *containerSuite) SetUpTest(c *gc.C) {
	s.setUpTest(c, false)
	// Reset any "broken" dummy provider methods.
	s.breakEnvironMethods(c)
}

func (s *containerSuite) newCustomAPI(c *gc.C, hostInstId instance.Id, addContainer, provisionContainer bool) *state.Machine {
	anAuthorizer := s.authorizer
	anAuthorizer.EnvironManager = false
	anAuthorizer.Tag = s.machines[0].Tag()
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(aProvisioner, gc.NotNil)
	s.provAPI = aProvisioner

	if hostInstId != "" {
		err = s.machines[0].SetProvisioned(hostInstId, "fake_nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
	}

	if !addContainer {
		return nil
	}
	container, err := s.State.AddMachineInsideMachine(
		state.MachineTemplate{
			Series: "quantal",
			Jobs:   []state.MachineJob{state.JobHostUnits},
		},
		s.machines[0].Id(),
		instance.LXC,
	)
	c.Assert(err, jc.ErrorIsNil)
	if provisionContainer {
		password, err := utils.RandomPassword()
		c.Assert(err, jc.ErrorIsNil)
		err = container.SetPassword(password)
		c.Assert(err, jc.ErrorIsNil)
		err = container.SetProvisioned("foo", "fake_nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
	}
	return container
}

func (s *containerSuite) makeArgs(machines ...*state.Machine) params.Entities {
	args := params.Entities{Entities: make([]params.Entity, len(machines))}
	for i, m := range machines {
		args.Entities[i].Tag = m.Tag().String()
	}
	return args
}

func (s *containerSuite) breakEnvironMethods(c *gc.C, methods ...string) {
	s.AssertConfigParameterUpdated(c, "broken", strings.Join(methods, " "))
}

// prepareSuite contains only tests around
// PrepareContainerInterfaceInfo method.
type prepareSuite struct {
	containerSuite
}

var _ = gc.Suite(&prepareSuite{})

func (s *prepareSuite) newAPI(c *gc.C, provisionHost, addContainer bool) *state.Machine {
	var hostInstId instance.Id
	if provisionHost {
		hostInstId = "i-host"
	}
	return s.newCustomAPI(c, hostInstId, addContainer, false)
}

func (s *prepareSuite) makeErrors(errors ...*params.Error) *params.MachineNetworkConfigResults {
	results := &params.MachineNetworkConfigResults{
		Results: make([]params.MachineNetworkConfigResult, len(errors)),
	}
	for i, err := range errors {
		results.Results[i].Error = err
	}
	return results
}

func (s *prepareSuite) makeResults(cfgs ...[]params.NetworkConfig) *params.MachineNetworkConfigResults {
	results := &params.MachineNetworkConfigResults{
		Results: make([]params.MachineNetworkConfigResult, len(cfgs)),
	}
	for i, cfg := range cfgs {
		results.Results[i].Config = cfg
	}
	return results
}

func (s *prepareSuite) assertCall(c *gc.C, args params.Entities, expectResults *params.MachineNetworkConfigResults, expectErr string) (error, []loggo.TestLogValues) {

	// Capture the logs for later inspection.
	logger := loggo.GetLogger("juju.apiserver.provisioner")
	defer logger.SetLogLevel(logger.LogLevel())
	logger.SetLogLevel(loggo.TRACE)
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("test", &tw, loggo.TRACE), gc.IsNil)
	defer loggo.RemoveWriter("test")

	results, err := s.provAPI.PrepareContainerInterfaceInfo(args)
	c.Logf("PrepareContainerInterfaceInfo returned: err=%v, results=%v", err, results)
	c.Assert(results.Results, gc.HasLen, len(args.Entities))
	if expectErr == "" {
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(expectResults, gc.NotNil)
		c.Assert(results.Results, gc.HasLen, len(expectResults.Results))
		// Check for any "regex:" prefixes first. Then replace
		// addresses in expected with the actual ones, so we can use
		// jc.DeepEquals on the whole result below.
		// Also check MAC addresses are valid, but as they're randomly
		// generated we can't test specific values.
		for i, expect := range expectResults.Results {
			cfg := results.Results[i].Config
			c.Assert(cfg, gc.HasLen, len(expect.Config))
			for j, expCfg := range expect.Config {
				if strings.HasPrefix(expCfg.Address, "regex:") {
					rex := strings.TrimPrefix(expCfg.Address, "regex:")
					c.Assert(cfg[j].Address, gc.Matches, rex)
					expectResults.Results[i].Config[j].Address = cfg[j].Address
				}
				macAddress := cfg[j].MACAddress
				c.Assert(macAddress[:8], gc.Equals, provisioner.MACAddressTemplate[:8])
				remainder := strings.Replace(macAddress[8:], ":", "", 3)
				c.Assert(remainder, gc.HasLen, 6)
				_, err = hex.DecodeString(remainder)
				c.Assert(err, jc.ErrorIsNil)
				expectResults.Results[i].Config[j].MACAddress = macAddress
			}
		}

		c.Assert(results, jc.DeepEquals, *expectResults)
	} else {
		c.Assert(err, gc.ErrorMatches, expectErr)
		if len(args.Entities) > 0 {
			result := results.Results[0]
			// Not using jc.ErrorIsNil below because
			// (*params.Error)(nil) does not satisfy the error
			// interface.
			c.Assert(result.Error, gc.IsNil)
			c.Assert(result.Config, gc.IsNil)
		}
	}
	return err, tw.Log()
}

func (s *prepareSuite) TestErrorWitnNoFeatureFlag(c *gc.C) {
	s.SetFeatureFlags() // clear the flags.
	container := s.newAPI(c, true, true)
	args := s.makeArgs(container)
	s.assertCall(c, args, &params.MachineNetworkConfigResults{},
		`address allocation not supported`,
	)
}

func (s *prepareSuite) TestErrorWithNonProvisionedHost(c *gc.C) {
	container := s.newAPI(c, false, true)
	args := s.makeArgs(container)
	s.assertCall(c, args, nil,
		`cannot allocate addresses: host machine "0" not provisioned`,
	)
}

func (s *prepareSuite) TestErrorWithProvisionedContainer(c *gc.C) {
	container := s.newAPI(c, true, true)
	err := container.SetProvisioned("i-foo", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	args := s.makeArgs(container)
	s.assertCall(c, args, s.makeErrors(
		apiservertesting.ServerError(
			`container "0/lxc/0" already provisioned as "i-foo"`,
		),
	), "")
}

func (s *prepareSuite) TestErrorWithHostInsteadOfContainer(c *gc.C) {
	s.newAPI(c, true, false)
	args := s.makeArgs(s.machines[0])
	s.assertCall(c, args, s.makeErrors(
		apiservertesting.ServerError(
			`cannot allocate address for "machine-0": not a container`,
		),
	), "")
}

func (s *prepareSuite) TestErrorsWithDifferentHosts(c *gc.C) {
	s.newAPI(c, true, false)
	args := s.makeArgs(s.machines[1], s.machines[2])
	s.assertCall(c, args, s.makeErrors(
		apiservertesting.ErrUnauthorized,
		apiservertesting.ErrUnauthorized,
	), "")
}

func (s *prepareSuite) TestErrorsWithContainersOnDifferentHost(c *gc.C) {
	s.newAPI(c, true, false)
	var containers []*state.Machine
	for i := 0; i < 2; i++ {
		container, err := s.State.AddMachineInsideMachine(
			state.MachineTemplate{
				Series: "quantal",
				Jobs:   []state.MachineJob{state.JobHostUnits},
			},
			s.machines[1].Id(),
			instance.LXC,
		)
		c.Assert(err, jc.ErrorIsNil)
		containers = append(containers, container)
	}
	args := s.makeArgs(containers...)
	s.assertCall(c, args, s.makeErrors(
		apiservertesting.ErrUnauthorized,
		apiservertesting.ErrUnauthorized,
	), "")
}

func (s *prepareSuite) TestErrorsWithNonMachineOrInvalidTags(c *gc.C) {
	s.newAPI(c, true, false)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-wordpress-0"},
		{Tag: "service-wordpress"},
		{Tag: "network-foo"},
		{Tag: "anything-invalid"},
		{Tag: "42"},
		{Tag: "machine-42"},
		{Tag: ""},
	}}

	s.assertCall(c, args, s.makeErrors(
		apiservertesting.ServerError(
			`"unit-wordpress-0" is not a valid machine tag`),
		apiservertesting.ServerError(
			`"service-wordpress" is not a valid machine tag`),
		apiservertesting.ServerError(
			`"network-foo" is not a valid machine tag`),
		apiservertesting.ServerError(
			`"anything-invalid" is not a valid tag`),
		apiservertesting.ServerError(
			`"42" is not a valid tag`),
		apiservertesting.ErrUnauthorized,
		apiservertesting.ServerError(
			`"" is not a valid tag`),
	), "")
}

func (s *prepareSuite) fillSubnet(c *gc.C, numAllocated int) {
	// Create the 0.10.0.0/24 subnet in state and pre-allocate up to
	// numAllocated of the range. This ensures the tests will run
	// quickly, rather than retrying potentiallu until the full /24
	// range is exhausted.
	subInfo := state.SubnetInfo{
		ProviderId:        "dummy-private",
		CIDR:              "0.10.0.0/24",
		VLANTag:           0,
		AllocatableIPLow:  "0.10.0.0",
		AllocatableIPHigh: "0.10.0.10", // Intentionally use shorter range.
	}
	sub, err := s.BackingState.AddSubnet(subInfo)
	c.Assert(err, jc.ErrorIsNil)
	for i := 0; i <= numAllocated; i++ {
		addr := network.NewAddress(fmt.Sprintf("0.10.0.%d", i))
		ipaddr, err := s.BackingState.AddIPAddress(addr, sub.ID())
		c.Check(err, jc.ErrorIsNil)
		err = ipaddr.SetState(state.AddressStateAllocated)
		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *prepareSuite) TestErrorWithEnvironMethodsFailing(c *gc.C) {
	container := s.newAPI(c, true, true)
	args := s.makeArgs(container)

	s.fillSubnet(c, 10)

	// NOTE: We're testing AllocateAddress and ReleaseAddress separately.
	for i, test := range []struct {
		method   string
		err      string
		errCheck func(error) bool
	}{{
		method: "NetworkInterfaces",
		err:    "cannot allocate addresses: dummy.NetworkInterfaces is broken",
	}, {
		method: "Subnets",
		err:    "cannot allocate addresses: dummy.Subnets is broken",
	}, {
		method:   "SupportsAddressAllocation",
		err:      "cannot allocate addresses: address allocation on any available subnets is not supported",
		errCheck: errors.IsNotSupported,
	}} {
		c.Logf("test %d: broken %q", i, test.method)
		s.breakEnvironMethods(c, test.method)
		var err error
		if test.err != "" {
			err, _ = s.assertCall(c, args, nil, test.err)
		}
		if test.errCheck != nil {
			c.Check(err, jc.Satisfies, test.errCheck)
		}
	}
}

func (s *prepareSuite) TestRetryingOnAllocateAddressFailure(c *gc.C) {
	// This test verifies the retrying logic when AllocateAddress
	// and/or setAddrState return errors.

	// Pre-allocate the first 5 addresses.
	s.fillSubnet(c, 5)

	// Now break AllocateAddress so it returns an error to verify the
	// retry logic kicks in. Because it will always fail, the end
	// result will always be an address exhaustion error.
	s.breakEnvironMethods(c, "AllocateAddress")

	container := s.newAPI(c, true, true)
	args := s.makeArgs(container)

	// Record each time setAddrState is called along with the address
	// to verify the logs later.
	var addresses []string
	origSetAddrState := *provisioner.SetAddrState
	s.PatchValue(provisioner.SetAddrState, func(ip *state.IPAddress, st state.AddressState) error {
		c.Logf("setAddrState called for address %q, state %q", ip.String(), st)
		c.Assert(st, gc.Equals, state.AddressStateUnavailable)
		addresses = append(addresses, ip.Value())

		// Return an error every other call to test it's handled ok.
		if len(addresses)%2 == 0 {
			return errors.New("pow!")
		}
		return origSetAddrState(ip, st)
	})

	_, testLog := s.assertCall(c, args, s.makeErrors(apiservertesting.ServerError(
		`failed to allocate an address for "0/lxc/0": `+
			`allocatable IP addresses exhausted for subnet "0.10.0.0/24"`,
	)), "")

	// Verify the expected addresses, ignoring the order as the
	// addresses are picked at random.
	c.Assert(addresses, jc.SameContents, []string{
		"0.10.0.6",
		"0.10.0.7",
		"0.10.0.8",
		"0.10.0.9",
		"0.10.0.10",
	})

	// Now verify the logs.
	c.Assert(testLog, jc.LogMatches, jc.SimpleMessages{{
		loggo.WARNING,
		`allocating address ".+" on instance ".+" and subnet ".+" failed: ` +
			`dummy.AllocateAddress is broken \(retrying\)`,
	}, {
		loggo.TRACE,
		`setting address ".+" to "unavailable" and retrying`,
	}, {
		loggo.TRACE,
		`picked new address ".+" on subnet ".+"`,
	}, {
		loggo.WARNING,
		`allocating address ".+" on instance ".+" and subnet ".+" failed: ` +
			`dummy.AllocateAddress is broken \(retrying\)`,
	}, {
		loggo.WARNING,
		`cannot set address ".+" to "unavailable": pow! \(ignoring and retrying\)`,
	}})
}

func (s *prepareSuite) TestReleaseAndCleanupWhenAllocateAndOrSetFail(c *gc.C) {
	// This test verifies the retrying, releasing, and cleanup logic
	// when AllocateAddress succeeds, but both ReleaseAddress and
	// setAddrsTo fail, and allocateAddrTo either succeeds or fails.

	// Pre-allocate the first 5 addresses.
	s.fillSubnet(c, 5)

	// Now break ReleaseAddress to test the how it's handled during
	// the release/cleanup loop.
	s.breakEnvironMethods(c, "ReleaseAddress")

	container := s.newAPI(c, true, true)
	args := s.makeArgs(container)

	// Record each time allocateAddrTo, setAddrsTo, and setAddrState
	// are called along with the addresses to verify the logs later.
	var allocAttemptedAddrs, allocAddrsOK, setAddrs, releasedAddrs []string
	s.PatchValue(provisioner.AllocateAddrTo, func(ip *state.IPAddress, m *state.Machine, mac string) error {
		c.Logf("allocateAddrTo called for address %q, machine %q", ip.String(), m)
		c.Assert(m.Id(), gc.Equals, container.Id())
		allocAttemptedAddrs = append(allocAttemptedAddrs, ip.Value())

		// Succeed on every other call to give a chance to call
		// setAddrsTo as well.
		if len(allocAttemptedAddrs)%2 == 0 {
			allocAddrsOK = append(allocAddrsOK, ip.Value())
			return nil
		}
		return errors.New("crash!")
	})
	s.PatchValue(provisioner.SetAddrsTo, func(ip *state.IPAddress, m *state.Machine) error {
		c.Logf("setAddrsTo called for address %q, machine %q", ip.String(), m)
		c.Assert(m.Id(), gc.Equals, container.Id())
		setAddrs = append(setAddrs, ip.Value())
		return errors.New("boom!")
	})
	s.PatchValue(provisioner.SetAddrState, func(ip *state.IPAddress, st state.AddressState) error {
		c.Logf("setAddrState called for address %q, state %q", ip.String(), st)
		c.Assert(st, gc.Equals, state.AddressStateUnavailable)
		releasedAddrs = append(releasedAddrs, ip.Value())
		return nil
	})

	_, testLog := s.assertCall(c, args, s.makeErrors(apiservertesting.ServerError(
		`failed to allocate an address for "0/lxc/0": `+
			`allocatable IP addresses exhausted for subnet "0.10.0.0/24"`,
	)), "")

	// Verify the expected addresses, ignoring the order as the
	// addresses are picked at random.
	expectAddrs := []string{
		"0.10.0.6",
		"0.10.0.7",
		"0.10.0.8",
		"0.10.0.9",
		"0.10.0.10",
	}
	// Verify that for each allocated address an attempt is made to
	// assign it to the container by calling allocateAddrTo
	// (successful or not doesn't matter).
	c.Check(allocAttemptedAddrs, jc.SameContents, expectAddrs)

	// Verify that for each allocated address an attempt is made to do
	// release/cleanup by calling setAddrState(unavailable), after
	// either allocateAddrTo or setAddrsTo fails.
	c.Check(releasedAddrs, jc.SameContents, expectAddrs)

	// Verify that for every allocateAddrTo call that passed, the
	// corresponding setAddrsTo was also called.
	c.Check(allocAddrsOK, jc.SameContents, setAddrs)

	// Now verify the logs.
	c.Assert(testLog, jc.LogMatches, jc.SimpleMessages{{
		loggo.INFO,
		`allocated address "public:.+" on instance "i-host" and subnet "dummy-private"`,
	}, {
		loggo.WARNING,
		`failed to mark address ".+" as "allocated" to container ".*": crash! \(releasing and retrying\)`,
	}, {
		loggo.WARNING,
		`failed to release address ".+" on instance "i-host" and subnet ".+": ` +
			`dummy.ReleaseAddress is broken \(ignoring and retrying\)`,
	}, {
		loggo.INFO,
		`allocated address "public:.+" on instance "i-host" and subnet "dummy-private"`,
	}})
}

func (s *prepareSuite) TestReleaseAndRetryWhenSetOnlyFails(c *gc.C) {
	// This test verifies the releasing, and cleanup, as well as
	// retrying logic when AllocateAddress and allocateAddrTo succeed,
	// but then both setAddrsTo and setAddrState fail.

	// Pre-allocate the first 9 addresses, so the only address left
	// will be 0.10.0.10.
	s.fillSubnet(c, 9)

	container := s.newAPI(c, true, true)
	args := s.makeArgs(container)

	s.PatchValue(provisioner.SetAddrsTo, func(ip *state.IPAddress, m *state.Machine) error {
		c.Logf("setAddrsTo called for address %q, machine %q", ip.String(), m)
		c.Assert(m.Id(), gc.Equals, container.Id())
		c.Assert(ip.Value(), gc.Equals, "0.10.0.10")
		return errors.New("boom!")
	})
	s.PatchValue(provisioner.SetAddrState, func(ip *state.IPAddress, st state.AddressState) error {
		c.Logf("setAddrState called for address %q, state %q", ip.String(), st)
		c.Assert(st, gc.Equals, state.AddressStateUnavailable)
		c.Assert(ip.Value(), gc.Equals, "0.10.0.10")
		return errors.New("pow!")
	})

	// After failing twice, we'll successfully release the address and retry to succeed.
	_, testLog := s.assertCall(c, args, s.makeResults([]params.NetworkConfig{{
		ProviderId:       "dummy-eth0",
		ProviderSubnetId: "dummy-private",
		NetworkName:      "juju-private",
		CIDR:             "0.10.0.0/24",
		DeviceIndex:      0,
		InterfaceName:    "eth0",
		VLANTag:          0,
		Disabled:         false,
		NoAutoStart:      false,
		ConfigType:       "static",
		Address:          "0.10.0.10",
		DNSServers:       []string{"ns1.dummy", "ns2.dummy"},
		GatewayAddress:   "0.10.0.2",
		ExtraConfig:      nil,
	}}), "")

	c.Assert(testLog, jc.LogMatches, jc.SimpleMessages{{
		loggo.INFO,
		`allocated address "public:0.10.0.10" on instance "i-host" and subnet "dummy-private"`,
	}, {
		loggo.WARNING,
		`failed to mark address ".+" as "allocated" to container ".*": boom! \(releasing and retrying\)`,
	}, {
		loggo.WARNING,
		`cannot set address "public:0.10.0.10" to "unavailable": pow! \(ignoring and releasing\)`,
	}, {
		loggo.INFO,
		`address "public:0.10.0.10" released; trying to allocate new`,
	}})
}

func (s *prepareSuite) TestErrorWhenNoSubnetsAvailable(c *gc.C) {
	// The magic "i-no-subnets-" instance id prefix for the host
	// causes the dummy provider to return no results and no errors
	// from Subnets().
	container := s.newCustomAPI(c, "i-no-subnets-here", true, false)
	args := s.makeArgs(container)
	s.assertCall(c, args, nil, "cannot allocate addresses: no subnets available")
}

func (s *prepareSuite) TestErrorWithDisabledNIC(c *gc.C) {
	// The magic "i-disabled-nic-" instance id prefix for the host
	// causes the dummy provider to return a disabled NIC from
	// NetworkInterfaces(), which should not be used for the container.
	container := s.newCustomAPI(c, "i-no-subnets-here", true, false)
	args := s.makeArgs(container)
	s.assertCall(c, args, nil, "cannot allocate addresses: no subnets available")
}

func (s *prepareSuite) TestErrorWhenNoAllocatableSubnetsAvailable(c *gc.C) {
	// The magic "i-no-alloc-all" instance id for the host causes the
	// dummy provider's Subnets() method to return all subnets without
	// an allocatable range
	container := s.newCustomAPI(c, "i-no-alloc-all", true, false)
	args := s.makeArgs(container)
	err, _ := s.assertCall(c, args, nil, "cannot allocate addresses: address allocation on any available subnets is not supported")
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *prepareSuite) TestErrorWhenNoNICSAvailable(c *gc.C) {
	// The magic "i-no-nics-" instance id prefix for the host
	// causes the dummy provider to return no results and no errors
	// from NetworkInterfaces().
	container := s.newCustomAPI(c, "i-no-nics-here", true, false)
	args := s.makeArgs(container)
	s.assertCall(c, args, nil, "cannot allocate addresses: no interfaces available")
}

func (s *prepareSuite) TestErrorWithNICNoSubnetAvailable(c *gc.C) {
	// The magic "i-nic-no-subnet-" instance id prefix for the host
	// causes the dummy provider to return a nic that has no associated
	// subnet from NetworkInterfaces().
	container := s.newCustomAPI(c, "i-nic-no-subnet-here", true, false)
	args := s.makeArgs(container)
	s.assertCall(c, args, nil, "cannot allocate addresses: no subnets available")
}

func (s *prepareSuite) TestSuccessWithSingleContainer(c *gc.C) {
	container := s.newAPI(c, true, true)
	args := s.makeArgs(container)
	_, testLog := s.assertCall(c, args, s.makeResults([]params.NetworkConfig{{
		ProviderId:       "dummy-eth0",
		ProviderSubnetId: "dummy-private",
		NetworkName:      "juju-private",
		CIDR:             "0.10.0.0/24",
		DeviceIndex:      0,
		InterfaceName:    "eth0",
		VLANTag:          0,
		Disabled:         false,
		NoAutoStart:      false,
		ConfigType:       "static",
		Address:          "regex:0.10.0.[0-9]{1,3}", // we don't care about the actual value.
		DNSServers:       []string{"ns1.dummy", "ns2.dummy"},
		GatewayAddress:   "0.10.0.2",
		ExtraConfig:      nil,
	}}), "")

	c.Assert(testLog, jc.LogMatches, jc.SimpleMessages{{
		loggo.INFO,
		`allocated address ".+" on instance "i-host" and subnet "dummy-private"`,
	}, {
		loggo.INFO,
		`assigned address ".+" to container "0/lxc/0"`,
	}})
}

func (s *prepareSuite) TestSuccessWhenFirstSubnetNotAllocatable(c *gc.C) {
	// Using "i-no-alloc-0" for the host instance id will cause the
	// dummy provider to change the Subnets() results to return no
	// allocatable range for the first subnet (dummy-private), and
	// also change its ProviderId to "noalloc-private", which in turn
	// will cause SupportsAddressAllocation() to return false for it.
	// We test here that we keep looking for other allocatable
	// subnets.
	container := s.newCustomAPI(c, "i-no-alloc-0", true, false)
	args := s.makeArgs(container)
	_, testLog := s.assertCall(c, args, s.makeResults([]params.NetworkConfig{{
		ProviderId:       "dummy-eth1",
		ProviderSubnetId: "dummy-public",
		NetworkName:      "juju-public",
		CIDR:             "0.20.0.0/24",
		DeviceIndex:      1,
		InterfaceName:    "eth1",
		VLANTag:          1,
		Disabled:         false,
		NoAutoStart:      true,
		ConfigType:       "static",
		Address:          "regex:0.20.0.[0-9]{1,3}", // we don't care about the actual value.
		DNSServers:       []string{"ns1.dummy", "ns2.dummy"},
		GatewayAddress:   "0.20.0.2",
		ExtraConfig:      nil,
	}}), "")

	c.Assert(testLog, jc.LogMatches, jc.SimpleMessages{{
		loggo.TRACE,
		`ignoring subnet "noalloc-private" - no allocatable range set`,
	}, {
		loggo.INFO,
		`allocated address ".+" on instance "i-no-alloc-0" and subnet "dummy-public"`,
	}, {
		loggo.INFO,
		`assigned address ".+" to container "0/lxc/0"`,
	}})
}

// releaseSuite contains only tests around
// ReleaseContainerAddresses method.
type releaseSuite struct {
	containerSuite
}

var _ = gc.Suite(&releaseSuite{})

func (s *releaseSuite) newAPI(c *gc.C, provisionHost, addContainer bool) *state.Machine {
	var hostInstId instance.Id
	if provisionHost {
		hostInstId = "i-host"
	}
	return s.newCustomAPI(c, hostInstId, addContainer, true)
}

func (s *releaseSuite) makeErrors(errors ...*params.Error) *params.ErrorResults {
	results := &params.ErrorResults{
		Results: make([]params.ErrorResult, len(errors)),
	}
	for i, err := range errors {
		results.Results[i].Error = err
	}
	return results
}

func (s *releaseSuite) assertCall(c *gc.C, args params.Entities, expectResults *params.ErrorResults, expectErr string) error {
	results, err := s.provAPI.ReleaseContainerAddresses(args)
	c.Logf("ReleaseContainerAddresses returned: err=%v, results=%v", err, results)
	c.Assert(results.Results, gc.HasLen, len(args.Entities))
	if expectErr == "" {
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(expectResults, gc.NotNil)
		c.Assert(results.Results, gc.HasLen, len(expectResults.Results))
		c.Assert(results, jc.DeepEquals, *expectResults)
	} else {
		c.Assert(err, gc.ErrorMatches, expectErr)
		if len(args.Entities) > 0 {
			result := results.Results[0]
			// Not using jc.ErrorIsNil below because
			// (*params.Error)(nil) does not satisfy the error
			// interface.
			c.Assert(result.Error, gc.IsNil)
		}
	}
	return err
}

func (s *releaseSuite) TestErrorWithNoFeatureFlag(c *gc.C) {
	s.SetFeatureFlags() // clear the flags.
	s.newAPI(c, true, false)
	args := s.makeArgs(s.machines[0])
	s.assertCall(c, args, &params.ErrorResults{},
		"address allocation not supported",
	)
}

func (s *releaseSuite) TestErrorWithHostInsteadOfContainer(c *gc.C) {
	s.newAPI(c, true, false)
	args := s.makeArgs(s.machines[0])
	err := s.assertCall(c, args, s.makeErrors(
		apiservertesting.ServerError(
			`cannot mark addresses for removal for "machine-0": not a container`,
		),
	), "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *releaseSuite) TestErrorsWithDifferentHosts(c *gc.C) {
	s.newAPI(c, true, false)
	args := s.makeArgs(s.machines[1], s.machines[2])
	err := s.assertCall(c, args, s.makeErrors(
		apiservertesting.ErrUnauthorized,
		apiservertesting.ErrUnauthorized,
	), "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *releaseSuite) TestErrorsWithContainersOnDifferentHost(c *gc.C) {
	s.newAPI(c, true, false)
	var containers []*state.Machine
	for i := 0; i < 2; i++ {
		container, err := s.State.AddMachineInsideMachine(
			state.MachineTemplate{
				Series: "quantal",
				Jobs:   []state.MachineJob{state.JobHostUnits},
			},
			s.machines[1].Id(),
			instance.LXC,
		)
		c.Assert(err, jc.ErrorIsNil)
		containers = append(containers, container)
	}
	args := s.makeArgs(containers...)
	err := s.assertCall(c, args, s.makeErrors(
		apiservertesting.ErrUnauthorized,
		apiservertesting.ErrUnauthorized,
	), "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *releaseSuite) TestErrorsWithNonMachineOrInvalidTags(c *gc.C) {
	s.newAPI(c, true, false)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-wordpress-0"},
		{Tag: "service-wordpress"},
		{Tag: "network-foo"},
		{Tag: "anything-invalid"},
		{Tag: "42"},
		{Tag: "machine-42"},
		{Tag: ""},
	}}

	err := s.assertCall(c, args, s.makeErrors(
		apiservertesting.ErrUnauthorized,
		apiservertesting.ErrUnauthorized,
		apiservertesting.ErrUnauthorized,
		apiservertesting.ErrUnauthorized,
		apiservertesting.ErrUnauthorized,
		apiservertesting.ErrUnauthorized,
		apiservertesting.ErrUnauthorized,
	), "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *releaseSuite) allocateAddresses(c *gc.C, containerId string, numAllocated int) {
	// Create the 0.10.0.0/24 subnet in state and pre-allocate up to
	// numAllocated of the range. It also allocates them to the specified
	// container.
	subInfo := state.SubnetInfo{
		ProviderId:        "dummy-private",
		CIDR:              "0.10.0.0/24",
		VLANTag:           0,
		AllocatableIPLow:  "0.10.0.0",
		AllocatableIPHigh: "0.10.0.10",
	}
	sub, err := s.BackingState.AddSubnet(subInfo)
	c.Assert(err, jc.ErrorIsNil)
	for i := 0; i < numAllocated; i++ {
		addr := network.NewAddress(fmt.Sprintf("0.10.0.%d", i))
		ipaddr, err := s.BackingState.AddIPAddress(addr, sub.ID())
		c.Check(err, jc.ErrorIsNil)
		err = ipaddr.AllocateTo(containerId, "", "")
		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *releaseSuite) TestSuccess(c *gc.C) {
	container := s.newAPI(c, true, true)
	args := s.makeArgs(container)

	s.allocateAddresses(c, container.Id(), 2)
	err := s.assertCall(c, args, s.makeErrors(nil), "")
	c.Assert(err, jc.ErrorIsNil)
	addresses, err := s.BackingState.AllocatedIPAddresses(container.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.HasLen, 2)
	for _, addr := range addresses {
		c.Assert(addr.Life(), gc.Equals, state.Dead)
	}
}
