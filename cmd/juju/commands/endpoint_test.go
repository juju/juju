// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
)

type EndpointSuite struct {
	testing.JujuConnSuite

	restoreTimeouts func()
}

var _ = gc.Suite(&EndpointSuite{})

func (s *EndpointSuite) SetUpSuite(c *gc.C) {
	// Use very short attempt strategies when getting instance addresses.
	s.restoreTimeouts = envtesting.PatchAttemptStrategies()
	s.JujuConnSuite.SetUpSuite(c)
}

func (s *EndpointSuite) TearDownSuite(c *gc.C) {
	s.JujuConnSuite.TearDownSuite(c)
	s.restoreTimeouts()
}

func (s *EndpointSuite) TestNoEndpoints(c *gc.C) {
	// Reset all addresses.
	s.setCachedAPIAddresses(c)
	s.setServerAPIAddresses(c)
	s.assertCachedAddresses(c)

	stdout, stderr, err := s.runCommand(c)
	c.Assert(err, gc.ErrorMatches, "no API endpoints available")
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	s.assertCachedAddresses(c)
}

func (s *EndpointSuite) TestCachedAddressesUsedIfAvailable(c *gc.C) {
	addresses := network.NewHostPorts(1234,
		"10.0.0.1:1234",
		"[2001:db8::1]:1234",
		"0.1.2.3:1234",
		"[fc00::1]:1234",
	)
	// Set the cached addresses.
	s.setCachedAPIAddresses(c, addresses...)
	// Clear instance/state addresses to ensure we can't connect to
	// the API server.
	s.setServerAPIAddresses(c)

	testRun := func(i int, envPreferIPv6, bootPreferIPv6 bool) {
		c.Logf(
			"\ntest %d: prefer-ipv6 environ=%v, bootstrap=%v",
			i, envPreferIPv6, bootPreferIPv6,
		)
		s.setPreferIPv6EnvironConfig(c, envPreferIPv6)
		s.setPreferIPv6BootstrapConfig(c, bootPreferIPv6)

		// Without arguments, verify the first cached address is returned.
		s.runAndCheckOutput(c, "smart", expectOutput(addresses[0]))
		s.assertCachedAddresses(c, addresses...)

		// With --all, ensure all are returned.
		s.runAndCheckOutput(c, "smart", expectOutput(addresses...), "--all")
		s.assertCachedAddresses(c, addresses...)
	}

	// Ensure regardless of the prefer-ipv6 value we have the same
	// result.
	for i, envPreferIPv6 := range []bool{true, false} {
		for j, bootPreferIPv6 := range []bool{true, false} {
			testRun(i+j, envPreferIPv6, bootPreferIPv6)
		}
	}
}

func (s *EndpointSuite) TestRefresh(c *gc.C) {
	testRun := func(i int, address network.HostPort, explicitRefresh bool) {
		c.Logf("\ntest %d: address=%q, explicitRefresh=%v", i, address, explicitRefresh)

		// Cache the address.
		s.setCachedAPIAddresses(c, address)
		s.assertCachedAddresses(c, address)
		// Clear instance/state addresses to ensure only the cached
		// one will be used.
		s.setServerAPIAddresses(c)

		// Ensure we get and cache the first address (i.e. no changes)
		if explicitRefresh {
			s.runAndCheckOutput(c, "smart", expectOutput(address), "--refresh")
		} else {
			s.runAndCheckOutput(c, "smart", expectOutput(address))
		}
		s.assertCachedAddresses(c, address)
	}

	// Test both IPv4 and IPv6 endpoints separately, first with
	// implicit refresh, then explicit.
	for i, explicitRefresh := range []bool{true, false} {
		for j, addr := range s.addressesWithAPIPort(c, "localhost", "::1") {
			testRun(i+j, addr, explicitRefresh)
		}
	}
}

func (s *EndpointSuite) TestSortingAndFilteringBeforeCachingRespectsPreferIPv6(c *gc.C) {
	// Set the instance/state addresses to a mix of IPv4 and IPv6
	// addresses of all kinds.
	addresses := s.addressesWithAPIPort(c,
		// The following two are needed to actually connect to the
		// test API server.
		"127.0.0.1",
		"::1",
		// Other examples.
		"192.0.0.0",
		"2001:db8::1",
		"169.254.1.2", // link-local - will be removed.
		"fd00::1",
		"ff01::1", // link-local - will be removed.
		"fc00::1",
		"localhost",
		"0.1.2.3",
		"127.0.1.1", // removed as a duplicate.
		"::1",       // removed as a duplicate.
		"10.0.0.1",
		"8.8.8.8",
	)
	s.setServerAPIAddresses(c, addresses...)

	// Clear cached the address to force a refresh.
	s.setCachedAPIAddresses(c)
	s.assertCachedAddresses(c)
	// Set prefer-ipv6 to true first.
	s.setPreferIPv6BootstrapConfig(c, true)

	// Build the expected addresses list, after processing.
	expectAddresses := s.addressesWithAPIPort(c,
		"127.0.0.1", // This is always on top.
		"2001:db8::1",
		"0.1.2.3",
		"192.0.0.0",
		"8.8.8.8",
		"localhost",
		"fc00::1",
		"fd00::1",
		"10.0.0.1",
	)
	s.runAndCheckOutput(c, "smart", expectOutput(expectAddresses...), "--all")
	s.assertCachedAddresses(c, expectAddresses...)

	// Now run it again with prefer-ipv6: false.
	// But first reset the cached addresses..
	s.setCachedAPIAddresses(c)
	s.assertCachedAddresses(c)
	s.setPreferIPv6BootstrapConfig(c, false)

	// Rebuild the expected addresses and rebuild them so IPv4 comes
	// before IPv6.
	expectAddresses = s.addressesWithAPIPort(c,
		"127.0.0.1", // This is always on top.
		"0.1.2.3",
		"192.0.0.0",
		"8.8.8.8",
		"2001:db8::1",
		"localhost",
		"10.0.0.1",
		"fc00::1",
		"fd00::1",
	)
	s.runAndCheckOutput(c, "smart", expectOutput(expectAddresses...), "--all")
	s.assertCachedAddresses(c, expectAddresses...)
}

func (s *EndpointSuite) TestAllFormats(c *gc.C) {
	addresses := s.addressesWithAPIPort(c,
		"127.0.0.1",
		"8.8.8.8",
		"2001:db8::1",
		"::1",
		"10.0.0.1",
		"fc00::1",
	)
	s.setServerAPIAddresses(c)
	s.setCachedAPIAddresses(c, addresses...)
	s.assertCachedAddresses(c, addresses...)

	for i, test := range []struct {
		about  string
		args   []string
		format string
		output []network.HostPort
	}{{
		about:  "default format (smart), no args",
		format: "smart",
		output: addresses[0:1],
	}, {
		about:  "default format (smart), with --all",
		args:   []string{"--all"},
		format: "smart",
		output: addresses,
	}, {
		about:  "JSON format, without --all",
		args:   []string{"--format", "json"},
		format: "json",
		output: addresses[0:1],
	}, {
		about:  "JSON format, with --all",
		args:   []string{"--format", "json", "--all"},
		format: "json",
		output: addresses,
	}, {
		about:  "YAML format, without --all",
		args:   []string{"--format", "yaml"},
		format: "yaml",
		output: addresses[0:1],
	}, {
		about:  "YAML format, with --all",
		args:   []string{"--format", "yaml", "--all"},
		format: "yaml",
		output: addresses,
	}} {
		c.Logf("\ntest %d: %s", i, test.about)
		s.runAndCheckOutput(c, test.format, expectOutput(test.output...), test.args...)
	}
}

// runCommand runs the api-endpoints command with the given arguments
// and returns the output and any error.
func (s *EndpointSuite) runCommand(c *gc.C, args ...string) (string, string, error) {
	command := &EndpointCommand{}
	ctx, err := coretesting.RunCommand(c, envcmd.Wrap(command), args...)
	if err != nil {
		return "", "", err
	}
	return coretesting.Stdout(ctx), coretesting.Stderr(ctx), nil
}

// runAndCheckOutput runs api-endpoints expecting no error and
// compares the output for the given format.
func (s *EndpointSuite) runAndCheckOutput(c *gc.C, format string, output []interface{}, args ...string) {
	stdout, stderr, err := s.runCommand(c, args...)
	if !c.Check(err, jc.ErrorIsNil) {
		return
	}
	c.Check(stderr, gc.Equals, "")
	switch format {
	case "smart":
		strOutput := ""
		for _, line := range output {
			strOutput += line.(string) + "\n"
		}
		c.Check(stdout, gc.Equals, strOutput)
	case "json":
		c.Check(stdout, jc.JSONEquals, output)
	case "yaml":
		c.Check(stdout, jc.YAMLEquals, output)
	default:
		c.Fatalf("unexpected format %q", format)
	}
}

// getStoreInfo returns the current environment's EnvironInfo.
func (s *EndpointSuite) getStoreInfo(c *gc.C) configstore.EnvironInfo {
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	info, err := s.ConfigStore.ReadInfo(env.Name())
	c.Assert(err, jc.ErrorIsNil)
	return info
}

// setPreferIPv6EnvironConfig sets the "prefer-ipv6" environment
// setting to given value.
func (s *EndpointSuite) setPreferIPv6EnvironConfig(c *gc.C, value bool) {
	// Technically, because prefer-ipv6 is an immutable setting, what
	// follows should be impossible, but the dummy provider doesn't
	// seem to validate the new config against the current (old) one
	// when calling SetConfig().
	allAttrs := s.Environ.Config().AllAttrs()
	allAttrs["prefer-ipv6"] = value
	cfg, err := config.New(config.NoDefaults, allAttrs)
	c.Assert(err, jc.ErrorIsNil)
	err = s.Environ.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)
	setValue := cfg.AllAttrs()["prefer-ipv6"].(bool)
	c.Logf("environ config prefer-ipv6 set to %v", setValue)
}

// setPreferIPv6BootstrapConfig sets the "prefer-ipv6" setting to the
// given value on the current environment's bootstrap config by
// recreating it (the only way to change bootstrap config once set).
func (s *EndpointSuite) setPreferIPv6BootstrapConfig(c *gc.C, value bool) {
	currentInfo := s.getStoreInfo(c)
	endpoint := currentInfo.APIEndpoint()
	creds := currentInfo.APICredentials()
	bootstrapConfig := currentInfo.BootstrapConfig()
	delete(bootstrapConfig, "prefer-ipv6")

	// The only way to change the bootstrap config is to recreate the
	// info.
	err := currentInfo.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	newInfo := s.ConfigStore.CreateInfo(s.Environ.Config().Name())
	newInfo.SetAPICredentials(creds)
	newInfo.SetAPIEndpoint(endpoint)
	newCfg := make(coretesting.Attrs)
	newCfg["prefer-ipv6"] = value
	newInfo.SetBootstrapConfig(newCfg.Merge(bootstrapConfig))
	err = newInfo.Write()
	c.Assert(err, jc.ErrorIsNil)
	setValue := newInfo.BootstrapConfig()["prefer-ipv6"].(bool)
	c.Logf("bootstrap config prefer-ipv6 set to %v", setValue)
}

// setCachedAPIAddresses sets the given addresses on the cached
// EnvironInfo endpoint. APIEndpoint.Hostnames are not touched,
// because the interactions between Addresses and Hostnames are
// separately tested in juju/api_test.go
func (s *EndpointSuite) setCachedAPIAddresses(c *gc.C, addresses ...network.HostPort) {
	info := s.getStoreInfo(c)
	endpoint := info.APIEndpoint()
	endpoint.Addresses = network.HostPortsToStrings(addresses)
	info.SetAPIEndpoint(endpoint)
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("cached addresses set to %v", info.APIEndpoint().Addresses)
}

// setServerAPIAddresses sets the given addresses on the dummy
// bootstrap instance and in state.
func (s *EndpointSuite) setServerAPIAddresses(c *gc.C, addresses ...network.HostPort) {
	insts, err := s.Environ.Instances([]instance.Id{dummy.BootstrapInstanceId})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SetAPIHostPorts([][]network.HostPort{addresses})
	c.Assert(err, jc.ErrorIsNil)
	dummy.SetInstanceAddresses(insts[0], network.HostsWithoutPort(addresses))
	instAddrs, err := insts[0].Addresses()
	c.Assert(err, jc.ErrorIsNil)
	stateAddrs, err := s.State.APIHostPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("instance addresses set to %v", instAddrs)
	c.Logf("state addresses set to %v", stateAddrs)
}

// addressesWithAPIPort returns the given addresses appending the test
// API server listening port to each one.
func (s *EndpointSuite) addressesWithAPIPort(c *gc.C, addresses ...string) []network.HostPort {
	apiPort := s.Environ.Config().APIPort()
	return network.NewHostPorts(apiPort, addresses...)
}

// assertCachedAddresses ensures the endpoint addresses (not
// hostnames) stored in the store match the given ones.
// APIEndpoint.Hostnames and APIEndpoint.Addresses interactions are
// separately testing in juju/api_test.go.
func (s *EndpointSuite) assertCachedAddresses(c *gc.C, addresses ...network.HostPort) {
	info := s.getStoreInfo(c)
	strAddresses := network.HostPortsToStrings(addresses)
	c.Assert(info.APIEndpoint().Addresses, jc.DeepEquals, strAddresses)
}

// expectOutput is a helper used to construct the expected ouput
// argument to runAndCheckOutput.
func expectOutput(addresses ...network.HostPort) []interface{} {
	result := make([]interface{}, len(addresses))
	for i, addr := range addresses {
		result[i] = addr.NetAddr()
	}
	return result
}
