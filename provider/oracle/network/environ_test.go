// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/response"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/instance"
	networkenv "github.com/juju/juju/network"
	"github.com/juju/juju/provider/oracle/network"
	"github.com/juju/juju/testing"
)

type environSuite struct{}

var _ = gc.Suite(&environSuite{})

type fakeNetworkingAPI struct{}

func (f fakeNetworkingAPI) AllIpNetworks(
	filter []api.Filter,
) (response.AllIpNetworks, error) {
	return response.AllIpNetworks{
		Result: []response.IpNetwork{{
			Name:                  "/Compute-acme/jack.jones@example.com/ipnet1",
			Uri:                   "https://api-z999.compute.us0.oraclecloud.com:443/network/v1/ipnetwork/Compute-acme/jack.jones@example.com/ipnet1",
			Description:           nil,
			Tags:                  nil,
			IpAddressPrefix:       "192.168.0.0/24",
			IpNetworkExchange:     nil,
			PublicNaptEnabledFlag: false,
		}},
	}, nil
}

func (f fakeNetworkingAPI) InstanceDetails(
	name string,
) (response.Instance, error) {
	return response.Instance{
		Shape:     "oc3",
		Imagelist: "/oracle/public/oel_6.4_2GB_v1",
		Name:      "/Compute-acme/jack.jones@example.com/dev-vm",
		Label:     "dev-vm",
		SSHKeys: []string{
			"/Compute-acme/jack.jones@example.com/dev-key1",
		},
	}, nil
}

func (f fakeNetworkingAPI) AllAcls(filter []api.Filter) (response.AllAcls, error) {
	return response.AllAcls{}, nil
}

func (f fakeNetworkingAPI) ComposeName(name string) string {
	return fmt.Sprintf("https://some-url.us6.com/Compute-test/%s", name)
}

func (e *environSuite) TestNewEnviron(c *gc.C) {
	env := network.NewEnviron(&fakeNetworkingAPI{})
	c.Assert(env, gc.NotNil)
}

func (e *environSuite) TestSupportSpaces(c *gc.C) {
	env := network.NewEnviron(&fakeNetworkingAPI{})
	c.Assert(env, gc.NotNil)

	ok, err := env.SupportsSpaces()
	c.Assert(err, gc.IsNil)
	c.Assert(ok, jc.IsTrue)
}

func (e *environSuite) TestSupportsSpaceDiscovery(c *gc.C) {
	env := network.NewEnviron(&fakeNetworkingAPI{})
	c.Assert(env, gc.NotNil)

	ok, err := env.SupportsSpaceDiscovery()
	c.Assert(err, gc.IsNil)
	c.Assert(ok, jc.IsTrue)
}

func (e *environSuite) TestSupportsContainerAddress(c *gc.C) {
	env := network.NewEnviron(&fakeNetworkingAPI{})
	c.Assert(env, gc.NotNil)

	ok, err := env.SupportsContainerAddresses()
	c.Assert(err, gc.NotNil)
	c.Assert(ok, jc.IsFalse)
	is := errors.IsNotSupported(err)
	c.Assert(is, jc.IsTrue)
}

func (e *environSuite) TestAllocateContainerAddress(c *gc.C) {
	env := network.NewEnviron(&fakeNetworkingAPI{})
	c.Assert(env, gc.NotNil)

	var (
		id   instance.Id
		tag  names.MachineTag
		info []networkenv.InterfaceInfo
	)

	addr, err := env.AllocateContainerAddresses(id, tag, info)
	c.Assert(err, gc.NotNil)
	c.Assert(addr, gc.IsNil)
	is := errors.IsNotSupported(err)
	c.Assert(is, jc.IsTrue)
}

func (e *environSuite) TestReleaseContainerAddresses(c *gc.C) {
	env := network.NewEnviron(&fakeNetworkingAPI{})
	c.Assert(env, gc.NotNil)

	var i []networkenv.ProviderInterfaceInfo
	err := env.ReleaseContainerAddresses(i)
	c.Assert(err, gc.NotNil)
	is := errors.IsNotSupported(err)
	c.Assert(is, jc.IsTrue)
}

func (e *environSuite) TestSubnetsWithEmptyParams(c *gc.C) {
	env := network.NewEnviron(&fakeNetworkingAPI{})
	c.Assert(env, gc.NotNil)

	info, err := env.Subnets("", nil)
	c.Assert(info, jc.DeepEquals, []networkenv.SubnetInfo{})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestSubnets(c *gc.C) {
	env := network.NewEnviron(&fakeNetworkingAPI{})
	c.Assert(env, gc.NotNil)

	cfg := fakeEnvironConfig{cfg: testing.ModelConfig(c)}
	id := cfg.Config().UUID()
	ids := []networkenv.Id{networkenv.Id(id)}
	info, err := env.Subnets(instance.Id(id), ids)
	c.Assert(info, jc.DeepEquals, []networkenv.SubnetInfo{})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestNetworkInterfacesWithEmptyParams(c *gc.C) {
	env := network.NewEnviron(&fakeNetworkingAPI{})
	c.Assert(env, gc.NotNil)

	info, err := env.NetworkInterfaces(instance.Id(""))
	c.Assert(info, jc.DeepEquals, []networkenv.InterfaceInfo{})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestNetworkInterfaces(c *gc.C) {
	env := network.NewEnviron(&fakeNetworkingAPI{})
	c.Assert(env, gc.NotNil)

	info, err := env.NetworkInterfaces(instance.Id(
		testing.ModelConfig(c).UUID(),
	))
	c.Assert(info, jc.DeepEquals, []networkenv.InterfaceInfo{})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestSpaces(c *gc.C) {
	env := network.NewEnviron(&fakeNetworkingAPI{})
	c.Assert(env, gc.NotNil)

	info, err := env.Spaces()
	c.Assert(err, gc.IsNil)
	c.Assert(info, jc.DeepEquals, []networkenv.SpaceInfo{})
}
