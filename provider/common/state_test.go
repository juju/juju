// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
)

type StateSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&StateSuite{})

func (suite *StateSuite) TestGetDNSNamesAcceptsNil(c *gc.C) {
	result := common.GetDNSNames(nil)
	c.Check(result, gc.DeepEquals, []string{})
}

func (suite *StateSuite) TestGetDNSNamesReturnsNames(c *gc.C) {
	instances := []instance.Instance{
		&dnsNameFakeInstance{name: "foo"},
		&dnsNameFakeInstance{name: "bar"},
	}

	c.Check(common.GetDNSNames(instances), gc.DeepEquals, []string{"foo", "bar"})
}

func (suite *StateSuite) TestGetDNSNamesIgnoresNils(c *gc.C) {
	c.Check(common.GetDNSNames([]instance.Instance{nil, nil}), gc.DeepEquals, []string{})
}

func (suite *StateSuite) TestGetDNSNamesIgnoresInstancesWithoutNames(c *gc.C) {
	instances := []instance.Instance{&dnsNameFakeInstance{err: instance.ErrNoDNSName}}
	c.Check(common.GetDNSNames(instances), gc.DeepEquals, []string{})
}

func (suite *StateSuite) TestGetDNSNamesIgnoresInstancesWithBlankNames(c *gc.C) {
	instances := []instance.Instance{&dnsNameFakeInstance{name: ""}}
	c.Check(common.GetDNSNames(instances), gc.DeepEquals, []string{})
}

func (suite *StateSuite) TestComposeAddressesAcceptsNil(c *gc.C) {
	c.Check(common.ComposeAddresses(nil, 1433), gc.DeepEquals, []string{})
}

func (suite *StateSuite) TestComposeAddressesSuffixesAddresses(c *gc.C) {
	c.Check(
		common.ComposeAddresses([]string{"onehost", "otherhost"}, 1957),
		gc.DeepEquals,
		[]string{"onehost:1957", "otherhost:1957"})
}

func (suite *StateSuite) TestGetStateInfo(c *gc.C) {
	cert := testing.CACert
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"ca-cert":    cert,
		"state-port": 123,
		"api-port":   456,
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	hostnames := []string{"onehost", "otherhost"}

	stateInfo, apiInfo := common.GetStateInfo(cfg, hostnames)

	c.Check(stateInfo.Addrs, gc.DeepEquals, []string{"onehost:123", "otherhost:123"})
	c.Check(string(stateInfo.CACert), gc.Equals, cert)
	c.Check(apiInfo.Addrs, gc.DeepEquals, []string{"onehost:456", "otherhost:456"})
	c.Check(string(apiInfo.CACert), gc.Equals, cert)
}
