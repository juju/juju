// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	networkenv "github.com/juju/juju/network"
	"github.com/juju/juju/provider/oracle"
	"github.com/juju/juju/provider/oracle/network"
	oracletesting "github.com/juju/juju/provider/oracle/testing"
	"github.com/juju/juju/testing"
)

type environSuite struct {
	env    *oracle.OracleEnviron
	netEnv *network.Environ
}

var _ = gc.Suite(&environSuite{})

type fakeNetworkingAPI struct{}

func (f *environSuite) SetUpTest(c *gc.C) {
	var err error
	f.env, err = oracle.NewOracleEnviron(
		&oracle.EnvironProvider{},
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		oracletesting.DefaultEnvironAPI,
		&advancingClock,
	)

	c.Assert(err, gc.IsNil)
	c.Assert(f.env, gc.NotNil)

	f.netEnv = network.NewEnviron(&fakeNetworkingAPI{}, f.env)
	c.Assert(f.netEnv, gc.NotNil)
}

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
		Name:      "/Compute-acme/jack.jones@example.com/0/88e5710d-ccce-4da6-97f8-36ab7999b705",
		Label:     "0",
		SSHKeys: []string{
			"/Compute-acme/jack.jones@example.com/dev-key1",
		},
	}, nil
}

func (f fakeNetworkingAPI) AllAcls(filter []api.Filter) (response.AllAcls, error) {
	return response.AllAcls{}, nil
}

func (f fakeNetworkingAPI) ComposeName(name string) string {
	return fmt.Sprintf("https://some-url.us6.com/%s", name)
}

func (e *environSuite) TestSupportSpaces(c *gc.C) {
	ok, err := e.netEnv.SupportsSpaces()
	c.Assert(err, gc.IsNil)
	c.Assert(ok, jc.IsTrue)
}

func (e *environSuite) TestSupportsSpaceDiscovery(c *gc.C) {
	ok, err := e.netEnv.SupportsSpaceDiscovery()
	c.Assert(err, gc.IsNil)
	c.Assert(ok, jc.IsTrue)
}

func (e *environSuite) TestSupportsContainerAddress(c *gc.C) {
	ok, err := e.netEnv.SupportsContainerAddresses()
	c.Assert(err, gc.NotNil)
	c.Assert(ok, jc.IsFalse)
	is := errors.IsNotSupported(err)
	c.Assert(is, jc.IsTrue)
}

func (e *environSuite) TestAllocateContainerAddress(c *gc.C) {
	var (
		id   instance.Id
		tag  names.MachineTag
		info []networkenv.InterfaceInfo
	)

	addr, err := e.netEnv.AllocateContainerAddresses(id, tag, info)
	c.Assert(err, gc.NotNil)
	c.Assert(addr, gc.IsNil)
	is := errors.IsNotSupported(err)
	c.Assert(is, jc.IsTrue)
}

func (e *environSuite) TestReleaseContainerAddresses(c *gc.C) {
	var i []networkenv.ProviderInterfaceInfo
	err := e.netEnv.ReleaseContainerAddresses(i)
	c.Assert(err, gc.NotNil)
	is := errors.IsNotSupported(err)
	c.Assert(is, jc.IsTrue)
}

func (e *environSuite) TestSubnetsWithEmptyParams(c *gc.C) {
	info, err := e.netEnv.Subnets("", nil)
	c.Assert(info, jc.DeepEquals, []networkenv.SubnetInfo{})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestSubnets(c *gc.C) {
	ids := []networkenv.Id{networkenv.Id("0")}
	info, err := e.netEnv.Subnets(instance.Id("0"), ids)
	c.Assert(info, jc.DeepEquals, []networkenv.SubnetInfo{})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestNetworkInterfacesWithEmptyParams(c *gc.C) {
	envAPI := oracletesting.DefaultEnvironAPI
	envAPI.FakeInstance.All.Result[0].Networking = common.Networking{}
	envAPI.FakeInstance.All.Result[0].Attributes.Network = map[string]response.Network{}

	env, err := oracle.NewOracleEnviron(
		&oracle.EnvironProvider{},
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		envAPI,
		&advancingClock,
	)

	c.Assert(err, gc.IsNil)
	c.Assert(env, gc.NotNil)

	netEnv := network.NewEnviron(&fakeNetworkingAPI{}, env)
	c.Assert(netEnv, gc.NotNil)

	info, err := netEnv.NetworkInterfaces(instance.Id("0"))
	c.Assert(info, jc.DeepEquals, []networkenv.InterfaceInfo{})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestSpaces(c *gc.C) {
	info, err := e.netEnv.Spaces()
	c.Assert(err, gc.IsNil)
	c.Assert(info, jc.DeepEquals, []networkenv.SpaceInfo{})
}
