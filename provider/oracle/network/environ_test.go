// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	jujunetwork "github.com/juju/juju/network"
	"github.com/juju/juju/provider/oracle"
	"github.com/juju/juju/provider/oracle/network"
	oracletesting "github.com/juju/juju/provider/oracle/testing"
	"github.com/juju/juju/testing"
)

type environSuite struct {
	env    *oracle.OracleEnviron
	netEnv *network.Environ

	callCtx context.ProviderCallContext
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
	f.callCtx = context.NewCloudCallContext()
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

func (f fakeNetworkingAPI) IpAssociationDetails(name string) (resp response.IpAssociation, err error) {
	return response.IpAssociation{
		Ip: "73.37.0.1",
	}, nil
}

func (f fakeNetworkingAPI) ComposeName(name string) string {
	return fmt.Sprintf("https://some-url.us6.com/%s", name)
}

func (e *environSuite) TestSupportSpaces(c *gc.C) {
	ok, err := e.netEnv.SupportsSpaces(e.callCtx)
	c.Assert(err, gc.IsNil)
	c.Assert(ok, jc.IsTrue)
}

func (e *environSuite) TestSupportsSpaceDiscovery(c *gc.C) {
	ok, err := e.netEnv.SupportsSpaceDiscovery(e.callCtx)
	c.Assert(err, gc.IsNil)
	c.Assert(ok, jc.IsTrue)
}

func (e *environSuite) TestSupportsContainerAddress(c *gc.C) {
	ok, err := e.netEnv.SupportsContainerAddresses(e.callCtx)
	c.Assert(err, gc.NotNil)
	c.Assert(ok, jc.IsFalse)
	is := errors.IsNotSupported(err)
	c.Assert(is, jc.IsTrue)
}

func (e *environSuite) TestAllocateContainerAddress(c *gc.C) {
	var (
		id   instance.Id
		tag  names.MachineTag
		info []corenetwork.InterfaceInfo
	)

	addr, err := e.netEnv.AllocateContainerAddresses(e.callCtx, id, tag, info)
	c.Assert(err, gc.NotNil)
	c.Assert(addr, gc.IsNil)
	is := errors.IsNotSupported(err)
	c.Assert(is, jc.IsTrue)
}

func (e *environSuite) TestReleaseContainerAddresses(c *gc.C) {
	var i []jujunetwork.ProviderInterfaceInfo
	err := e.netEnv.ReleaseContainerAddresses(e.callCtx, i)
	c.Assert(err, gc.NotNil)
	is := errors.IsNotSupported(err)
	c.Assert(is, jc.IsTrue)
}

func (e *environSuite) TestSubnetsWithEmptyParams(c *gc.C) {
	info, err := e.netEnv.Subnets(e.callCtx, "", nil)
	c.Assert(info, jc.DeepEquals, []corenetwork.SubnetInfo{})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestSubnets(c *gc.C) {
	ids := []corenetwork.Id{corenetwork.Id("0")}
	info, err := e.netEnv.Subnets(e.callCtx, instance.Id("0"), ids)
	c.Assert(info, jc.DeepEquals, []corenetwork.SubnetInfo{})
	c.Assert(err, gc.IsNil)
}

func (e *environSuite) TestNetworkInterfacesWithEmptyParams(c *gc.C) {
	envAPI := oracletesting.DefaultEnvironAPI
	origNet := envAPI.FakeInstance.All.Result[0].Networking
	origRespMap := envAPI.FakeInstance.All.Result[0].Attributes.Network
	defer func() {
		envAPI.FakeInstance.All.Result[0].Networking = origNet
		envAPI.FakeInstance.All.Result[0].Attributes.Network = origRespMap
	}()
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

	infoList, err := netEnv.NetworkInterfaces(e.callCtx, []instance.Id{instance.Id("0")})
	c.Assert(err, gc.IsNil)
	c.Assert(infoList, gc.HasLen, 1)
	c.Assert(infoList[0], jc.DeepEquals, []corenetwork.InterfaceInfo{})
}

func (e *environSuite) TestNetworkInterfacesWithIPAssociation(c *gc.C) {
	envAPI := oracletesting.DefaultEnvironAPI
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

	infoList, err := netEnv.NetworkInterfaces(e.callCtx, []instance.Id{instance.Id("0")})
	c.Assert(err, gc.IsNil)
	c.Assert(infoList, gc.HasLen, 1)
	instInfoList := infoList[0]
	c.Assert(instInfoList, gc.HasLen, 1)
	c.Assert(instInfoList[0].Addresses, jc.DeepEquals, corenetwork.ProviderAddresses{corenetwork.NewScopedProviderAddress("10.31.5.106", corenetwork.ScopeCloudLocal)})
	c.Assert(instInfoList[0].ShadowAddresses, jc.DeepEquals, corenetwork.ProviderAddresses{corenetwork.NewScopedProviderAddress("73.37.0.1", corenetwork.ScopePublic)})
}

func (e *environSuite) TestSpaces(c *gc.C) {
	info, err := e.netEnv.Spaces(e.callCtx)
	c.Assert(err, gc.IsNil)
	c.Assert(info, jc.DeepEquals, []corenetwork.SpaceInfo{})
}
