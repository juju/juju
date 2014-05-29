// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"errors"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/testing"
)

type StateSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&StateSuite{})

func (suite *StateSuite) TestGetAddressesAcceptsNil(c *gc.C) {
	result := common.GetAddresses(nil)
	c.Check(result, gc.DeepEquals, []string{})
}

func (suite *StateSuite) TestGetAddressesReturnsNames(c *gc.C) {
	instances := []instance.Instance{
		&mockInstance{addresses: instance.NewAddresses("foo")},
		&mockInstance{addresses: instance.NewAddresses("bar")},
	}
	c.Check(common.GetAddresses(instances), gc.DeepEquals, []string{"foo", "bar"})
}

func (suite *StateSuite) TestGetAddressesIgnoresNils(c *gc.C) {
	instances := []instance.Instance{
		nil,
		&mockInstance{addresses: instance.NewAddresses("foo")},
		nil,
		&mockInstance{addresses: instance.NewAddresses("bar")},
		nil,
	}
	c.Check(common.GetAddresses(instances), gc.DeepEquals, []string{"foo", "bar"})
	c.Check(common.GetAddresses(nil), gc.DeepEquals, []string{})
}

func (suite *StateSuite) TestGetAddressesIgnoresInstancesWithoutAddrs(c *gc.C) {
	instances := []instance.Instance{&mockInstance{}}
	c.Check(common.GetAddresses(instances), gc.DeepEquals, []string{})
}

func (suite *StateSuite) TestGetAddressesIgnoresAddressesErrors(c *gc.C) {
	instances := []instance.Instance{
		&mockInstance{
			addresses:    instance.NewAddresses("one"),
			addressesErr: errors.New("ignored"),
		},
		&mockInstance{addresses: instance.NewAddresses("two", "three")},
	}
	c.Check(common.GetAddresses(instances), gc.DeepEquals, []string{"two", "three"})
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
