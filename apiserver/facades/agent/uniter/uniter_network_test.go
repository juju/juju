// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8stesting "github.com/juju/juju/caas/kubernetes/provider/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/featureflag"
	"github.com/juju/juju/internal/services"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type uniterNetworkInfoSuite struct {
	uniterSuiteBase
	domainServices services.DomainServices
	mysqlCharm     state.CharmRefFull
	st             *state.State
	cmrBackend     commoncrossmodel.Backend
}

var _ = gc.Suite(&uniterNetworkInfoSuite{})

func (s *uniterNetworkInfoSuite) SetUpTest(c *gc.C) {
	c.Skip("Skip factory-based uniter tests. TODO: Re-write without factories")

	s.ControllerConfigAttrs = map[string]interface{}{
		controller.Features: featureflag.RawK8sSpec,
	}

	s.ApiServerSuite.SetUpTest(c)
	s.ApiServerSuite.SeedCAASCloud(c)

	s.domainServices = s.ControllerDomainServices(c)
	cloudService := s.domainServices.Cloud()
	err := cloudService.UpdateCloud(context.Background(), testing.DefaultCloud)
	c.Assert(err, jc.ErrorIsNil)

	cred := cloud.NewCredential(cloud.UserPassAuthType, nil)
	s.domainServices.Credential().UpdateCloudCredential(context.Background(), testing.DefaultCredentialId, cred)

	s.PatchValue(&provider.NewK8sClients, k8stesting.NoopFakeK8sClients)

	s.st = s.ControllerModel(c).State()
	networkService := s.domainServices.Network()

	spacePublic := network.SpaceInfo{
		Name: "public",
	}
	publicSpaceID, err := networkService.AddSpace(context.Background(), spacePublic)
	c.Assert(err, jc.ErrorIsNil)
	for i, cidr := range []string{"8.8.0.0/16", "240.0.0.0/12"} {
		_, err = networkService.AddSubnet(
			context.Background(),
			network.SubnetInfo{
				CIDR:              cidr,
				SpaceID:           string(publicSpaceID),
				ProviderId:        network.Id(fmt.Sprintf("subnet-0%d", i)),
				ProviderNetworkId: network.Id(fmt.Sprintf("subnet-0%d", i)),
			})
		c.Assert(err, jc.ErrorIsNil)
	}
	spaceInternal := network.SpaceInfo{
		Name: "internal",
	}
	internalSpaceID, err := networkService.AddSpace(context.Background(), spaceInternal)
	c.Assert(err, jc.ErrorIsNil)
	for i, cidr := range []string{"10.0.0.0/24"} {
		_, err = networkService.AddSubnet(
			context.Background(),
			network.SubnetInfo{
				CIDR:              cidr,
				SpaceID:           string(internalSpaceID),
				ProviderId:        network.Id(fmt.Sprintf("subnet-1%d", i)),
				ProviderNetworkId: network.Id(fmt.Sprintf("subnet-1%d", i)),
			})
		c.Assert(err, jc.ErrorIsNil)
	}
	spaceWpDefault := network.SpaceInfo{
		Name: "wp-default",
	}
	wpDefaultSpaceID, err := networkService.AddSpace(context.Background(), spaceWpDefault)
	c.Assert(err, jc.ErrorIsNil)
	for i, cidr := range []string{"100.64.0.0/16"} {
		_, err = networkService.AddSubnet(
			context.Background(),
			network.SubnetInfo{
				CIDR:              cidr,
				SpaceID:           string(wpDefaultSpaceID),
				ProviderId:        network.Id(fmt.Sprintf("subnet-2%d", i)),
				ProviderNetworkId: network.Id(fmt.Sprintf("subnet-2%d", i)),
			})
		c.Assert(err, jc.ErrorIsNil)
	}
	spaceDatabase := network.SpaceInfo{
		Name: "database",
	}
	databaseSpaceID, err := networkService.AddSpace(context.Background(), spaceDatabase)
	c.Assert(err, jc.ErrorIsNil)
	for _, cidr := range []string{"192.168.1.0/24"} {
		_, err = networkService.AddSubnet(
			context.Background(),
			network.SubnetInfo{
				CIDR:       cidr,
				SpaceID:    string(databaseSpaceID),
				ProviderId: "subnet-3",
			})
		c.Assert(err, jc.ErrorIsNil)
	}
	spaceLayerTwo := network.SpaceInfo{
		Name: "layertwo",
	}
	layerTwoSpaceID, err := networkService.AddSpace(context.Background(), spaceLayerTwo)
	c.Assert(err, jc.ErrorIsNil)

	s.st = s.ControllerModel(c).State()
	c.Assert(err, jc.ErrorIsNil)

	s.machine0 = s.addProvisionedMachineWithDevicesAndAddresses(c, 10)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	s.wpCharm = f.MakeCharm(c, &factory.CharmParams{
		Name: "wordpress-extra-bindings",
		URL:  "ch:amd64/quantal/wordpress-extra-bindings-4",
	})
	s.wordpress = f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: s.wpCharm,
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-hub",
			Platform: &state.Platform{OS: "ubuntu", Channel: "12.10/stable", Architecture: "amd64"}},
		EndpointBindings: map[string]string{
			"db":        string(internalSpaceID),  // relation name
			"admin-api": string(publicSpaceID),    // extra-binding name
			"foo-bar":   string(layerTwoSpaceID),  // extra-binding to L2
			"":          string(wpDefaultSpaceID), // explicitly specified default space
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.wordpressUnit = f.MakeUnit(c, &factory.UnitParams{
		Application: s.wordpress,
		Machine:     s.machine0,
	})

	s.machine1 = s.addProvisionedMachineWithDevicesAndAddresses(c, 20)

	s.mysqlCharm = f.MakeCharm(c, &factory.CharmParams{
		Name: "mysql",
	})
	s.mysql = f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "mysql",
		Charm: s.mysqlCharm,
		EndpointBindings: map[string]string{
			"server": string(databaseSpaceID),
		},
	})
	s.wordpressUnit = f.MakeUnit(c, &factory.UnitParams{
		Application: s.wordpress,
		Machine:     s.machine0,
	})
	s.mysqlUnit = f.MakeUnit(c, &factory.UnitParams{
		Application: s.mysql,
		Machine:     s.machine1,
	})

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.leadershipChecker = &fakeLeadershipChecker{false}
	s.setupUniterAPIForUnit(c, s.wordpressUnit)
}

func (s *uniterNetworkInfoSuite) addProvisionedMachineWithDevicesAndAddresses(c *gc.C, addrSuffix int) *state.Machine {
	machine, err := s.st.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetInstanceInfo("i-am", "", "fake_nonce", nil, nil, nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	devicesArgs, devicesAddrs := s.makeMachineDevicesAndAddressesArgs(addrSuffix)
	c.Assert(machine.SetLinkLayerDevices(devicesArgs...), jc.ErrorIsNil)
	c.Assert(machine.SetDevicesAddresses(devicesAddrs...), jc.ErrorIsNil)

	machineAddrs, err := machine.AllDeviceAddresses()
	c.Assert(err, jc.ErrorIsNil)

	netAddrs := make([]network.SpaceAddress, len(machineAddrs))
	for i, addr := range machineAddrs {
		netAddrs[i] = network.NewSpaceAddress(addr.Value())
	}

	controllerDomainServices := s.ControllerDomainServices(c)
	controllerConfigService := controllerDomainServices.ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetProviderAddresses(controllerConfig, netAddrs...)
	c.Assert(err, jc.ErrorIsNil)

	return machine
}

func (s *uniterNetworkInfoSuite) makeMachineDevicesAndAddressesArgs(addrSuffix int) ([]state.LinkLayerDeviceArgs, []state.LinkLayerDeviceAddress) {
	return []state.LinkLayerDeviceArgs{{
			Name:       "eth0",
			Type:       network.EthernetDevice,
			MACAddress: fmt.Sprintf("00:11:22:33:%0.2d:50", addrSuffix),
		}, {
			Name:       "eth0.100",
			Type:       network.VLAN8021QDevice,
			ParentName: "eth0",
			MACAddress: fmt.Sprintf("00:11:22:33:%0.2d:50", addrSuffix),
		}, {
			Name:       "eth1",
			Type:       network.EthernetDevice,
			MACAddress: fmt.Sprintf("00:11:22:33:%0.2d:51", addrSuffix),
		}, {
			Name:       "eth1.100",
			Type:       network.VLAN8021QDevice,
			ParentName: "eth1",
			MACAddress: fmt.Sprintf("00:11:22:33:%0.2d:51", addrSuffix),
		}, {
			Name:       "eth2",
			Type:       network.EthernetDevice,
			MACAddress: fmt.Sprintf("00:11:22:33:%0.2d:52", addrSuffix),
		}, {
			Name:       "eth3",
			Type:       network.EthernetDevice,
			MACAddress: fmt.Sprintf("00:11:22:33:%0.2d:53", addrSuffix),
		}, {
			Name:       "eth4",
			Type:       network.EthernetDevice,
			MACAddress: fmt.Sprintf("00:11:22:33:%0.2d:54", addrSuffix),
		}, {
			Name:       "fan-1",
			Type:       network.EthernetDevice,
			MACAddress: fmt.Sprintf("00:11:22:33:%0.2d:55", addrSuffix),
		}},
		[]state.LinkLayerDeviceAddress{{
			DeviceName:   "eth0",
			ConfigMethod: network.ConfigStatic,
			CIDRAddress:  fmt.Sprintf("8.8.8.%d/16", addrSuffix),
		}, {
			DeviceName:   "eth0.100",
			ConfigMethod: network.ConfigStatic,
			CIDRAddress:  fmt.Sprintf("10.0.0.%d/24", addrSuffix),
		}, {
			DeviceName:   "eth1",
			ConfigMethod: network.ConfigStatic,
			CIDRAddress:  fmt.Sprintf("8.8.4.%d/16", addrSuffix),
		}, {
			DeviceName:   "eth1",
			ConfigMethod: network.ConfigStatic,
			CIDRAddress:  fmt.Sprintf("8.8.4.%d/16", addrSuffix+1),
		}, {
			DeviceName:   "eth1.100",
			ConfigMethod: network.ConfigStatic,
			CIDRAddress:  fmt.Sprintf("10.0.0.%d/24", addrSuffix+1),
		}, {
			DeviceName:   "eth2",
			ConfigMethod: network.ConfigStatic,
			CIDRAddress:  fmt.Sprintf("100.64.0.%d/16", addrSuffix),
		}, {
			DeviceName:   "eth4",
			ConfigMethod: network.ConfigStatic,
			CIDRAddress:  fmt.Sprintf("192.168.1.%d/24", addrSuffix),
		}, {
			DeviceName:   "fan-1",
			ConfigMethod: network.ConfigStatic,
			CIDRAddress:  fmt.Sprintf("240.1.1.%d/12", addrSuffix),
		}}
}

func (s *uniterNetworkInfoSuite) setupUniterAPIForUnit(c *gc.C, givenUnit *state.Unit) {
	// Create a FakeAuthorizer so we can check permissions, set up assuming the
	// given unit agent has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: givenUnit.Tag(),
	}
	s.uniter = s.newUniterAPI(c, s.st, s.authorizer)
}

func (s *uniterNetworkInfoSuite) addRelationAndAssertInScope(c *gc.C) {
	// Add a relation between wordpress and mysql and enter scope with
	// mysqlUnit.
	rel := s.addRelation(c, "wordpress", "mysql")
	wpRelUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = wpRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, wpRelUnit, true)
}

func (s *uniterNetworkInfoSuite) TestNetworkInfoPermissions(c *gc.C) {
	s.addRelationAndAssertInScope(c)
	var tests = []struct {
		Name   string
		Arg    params.NetworkInfoParams
		Result params.NetworkInfoResults
		Error  string
	}{
		{
			"Wrong unit name",
			params.NetworkInfoParams{Unit: "unit-foo-0", Endpoints: []string{"foo"}},
			params.NetworkInfoResults{},
			"permission denied",
		},
		{
			"Invalid tag",
			params.NetworkInfoParams{Unit: "invalid", Endpoints: []string{"db-client"}},
			params.NetworkInfoResults{},
			`"invalid" is not a valid tag`,
		},
		{
			"No access to unit",
			params.NetworkInfoParams{Unit: "unit-mysql-0", Endpoints: []string{"juju-info"}},
			params.NetworkInfoResults{},
			"permission denied",
		},
		{
			"Unknown binding name",
			params.NetworkInfoParams{Unit: s.wordpressUnit.Tag().String(), Endpoints: []string{"unknown"}},
			params.NetworkInfoResults{
				Results: map[string]params.NetworkInfoResult{
					"unknown": {
						Error: &params.Error{
							Code:    params.CodeNotValid,
							Message: `undefined for unit charm: endpoint "unknown" not valid`,
						},
					},
				},
			},
			"",
		},
	}

	for _, test := range tests {
		c.Logf("Testing %s", test.Name)
		result, err := s.uniter.NetworkInfo(context.Background(), test.Arg)
		if test.Error != "" {
			c.Check(err, gc.ErrorMatches, test.Error)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Check(result, jc.DeepEquals, test.Result)
		}
	}
}

func (s *uniterNetworkInfoSuite) TestNetworkInfoForExplicitlyBoundEndpointAndDefaultSpace(c *gc.C) {
	s.addRelationAndAssertInScope(c)

	args := params.NetworkInfoParams{
		Unit:      s.wordpressUnit.Tag().String(),
		Endpoints: []string{"db", "admin-api", "db-client"},
	}
	// For the relation "wordpress:db mysql:server" we expect to see only
	// ifaces in the "internal" space, where the "db" endpoint itself
	// is bound to.
	expectedConfigWithRelationName := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:10:50",
				InterfaceName: "eth0.100",
				Addresses: []params.InterfaceAddress{
					{Address: "10.0.0.10", CIDR: "10.0.0.0/24"},
				},
			},
			{
				MACAddress:    "00:11:22:33:10:51",
				InterfaceName: "eth1.100",
				Addresses: []params.InterfaceAddress{
					{Address: "10.0.0.11", CIDR: "10.0.0.0/24"},
				},
			},
		},
		EgressSubnets:    []string{"10.0.0.10/32"},
		IngressAddresses: []string{"10.0.0.10", "10.0.0.11"},
	}
	// For the "admin-api" extra-binding we expect to see only interfaces from
	// the "public" space.
	expectedConfigWithExtraBindingName := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:10:51",
				InterfaceName: "eth1",
				Addresses: []params.InterfaceAddress{
					{Address: "8.8.4.10", CIDR: "8.8.0.0/16"},
					{Address: "8.8.4.11", CIDR: "8.8.0.0/16"},
				},
			},
			{
				MACAddress:    "00:11:22:33:10:50",
				InterfaceName: "eth0",
				Addresses: []params.InterfaceAddress{
					{Address: "8.8.8.10", CIDR: "8.8.0.0/16"},
				},
			},
			{
				MACAddress:    "00:11:22:33:10:55",
				InterfaceName: "fan-1",
				Addresses: []params.InterfaceAddress{
					{Address: "240.1.1.10", CIDR: "240.0.0.0/12"},
				},
			},
		},
		// Egress is based on the first ingress address.
		// Addresses are sorted, with fan always last.
		EgressSubnets:    []string{"8.8.4.10/32"},
		IngressAddresses: []string{"8.8.4.10", "8.8.4.11", "8.8.8.10", "240.1.1.10"},
	}

	// For the "db-client" extra-binding we expect to see interfaces from default
	// "wp-default" space
	expectedConfigWithDefaultSpace := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:10:52",
				InterfaceName: "eth2",
				Addresses: []params.InterfaceAddress{
					{Address: "100.64.0.10", CIDR: "100.64.0.0/16"},
				},
			},
		},
		EgressSubnets:    []string{"100.64.0.10/32"},
		IngressAddresses: []string{"100.64.0.10"},
	}

	result, err := s.uniter.NetworkInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, params.NetworkInfoResults{
		Results: map[string]params.NetworkInfoResult{
			"db":        expectedConfigWithRelationName,
			"admin-api": expectedConfigWithExtraBindingName,
			"db-client": expectedConfigWithDefaultSpace,
		},
	})
}

func (s *uniterNetworkInfoSuite) TestNetworkInfoL2Binding(c *gc.C) {
	c.Skip("L2 not supported yet")
	s.addRelationAndAssertInScope(c)

	args := params.NetworkInfoParams{
		Unit:      s.wordpressUnit.Tag().String(),
		Endpoints: []string{"foo-bar"},
	}

	expectedInfo := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:10:50",
				InterfaceName: "eth2",
			},
		},
	}

	result, err := s.uniter.NetworkInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, params.NetworkInfoResults{
		Results: map[string]params.NetworkInfoResult{
			"foo-bar": expectedInfo,
		},
	})
}

func (s *uniterNetworkInfoSuite) TestNetworkInfoForImplicitlyBoundEndpoint(c *gc.C) {
	// Since wordpressUnit has explicit binding for "db", switch the API to
	// mysqlUnit and check "mysql:server" uses the machine preferred private
	// address.
	s.setupUniterAPIForUnit(c, s.mysqlUnit)
	rel := s.addRelation(c, "mysql", "wordpress")
	mysqlRelUnit, err := rel.Unit(s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, mysqlRelUnit, true)

	args := params.NetworkInfoParams{
		Unit:      s.mysqlUnit.Tag().String(),
		Endpoints: []string{"server"},
	}

	expectedInfo := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:20:54",
				InterfaceName: "eth4",
				Addresses: []params.InterfaceAddress{
					{Address: "192.168.1.20", CIDR: "192.168.1.0/24"},
				},
			},
		},
		EgressSubnets:    []string{"192.168.1.20/32"},
		IngressAddresses: []string{"192.168.1.20"},
	}

	result, err := s.uniter.NetworkInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, params.NetworkInfoResults{
		Results: map[string]params.NetworkInfoResult{
			"server": expectedInfo,
		},
	})
}

func (s *uniterNetworkInfoSuite) TestNetworkInfoUsesRelationAddressNonDefaultBinding(c *gc.C) {
	// If a network info call is made in the context of a relation, and the
	// endpoint of that relation is bound to the non default space, we
	// provide the ingress addresses as those belonging to the space.
	s.setupUniterAPIForUnit(c, s.mysqlUnit)
	_, err := s.cmrBackend.AddRemoteApplication(commoncrossmodel.AddRemoteApplicationParams{
		SourceModel: coretesting.ModelTag,
		Name:        "wordpress-remote",
		Endpoints:   []charm.Relation{{Name: "db", Interface: "mysql", Role: "requirer"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	rel := s.addRelation(c, "mysql", "wordpress-remote")
	mysqlRelUnit, err := rel.Unit(s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, mysqlRelUnit, true)

	// Relation specific egress subnets override model config.
	err = s.ControllerDomainServices(c).Config().UpdateModelConfig(context.Background(), map[string]interface{}{config.EgressSubnets: "10.0.0.0/8"}, nil)
	c.Assert(err, jc.ErrorIsNil)
	relEgress := state.NewRelationEgressNetworks(s.st)
	_, err = relEgress.Save(rel.Tag().Id(), false, []string{"192.168.1.0/24"})
	c.Assert(err, jc.ErrorIsNil)

	relId := rel.Id()
	args := params.NetworkInfoParams{
		Unit:       s.mysqlUnit.Tag().String(),
		Endpoints:  []string{"server"},
		RelationId: &relId,
	}

	expectedInfo := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:20:54",
				InterfaceName: "eth4",
				Addresses: []params.InterfaceAddress{
					{Address: "192.168.1.20", CIDR: "192.168.1.0/24"},
				},
			},
		},
		EgressSubnets:    []string{"192.168.1.0/24"},
		IngressAddresses: []string{"192.168.1.20"},
	}

	result, err := s.uniter.NetworkInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, params.NetworkInfoResults{
		Results: map[string]params.NetworkInfoResult{
			"server": expectedInfo,
		},
	})
}

func (s *uniterNetworkInfoSuite) TestNetworkInfoUsesRelationAddressDefaultBinding(c *gc.C) {
	// If a network info call is made in the context of a relation, and the
	// endpoint of that relation is not bound, or bound to the default space, we
	// provide the ingress address relevant to the relation: public for CMR.
	_, err := s.cmrBackend.AddRemoteApplication(commoncrossmodel.AddRemoteApplicationParams{
		SourceModel: coretesting.ModelTag,
		Name:        "wordpress-remote",
		Endpoints:   []charm.Relation{{Name: "db", Interface: "mysql", Role: "requirer"}},
	})
	c.Assert(err, jc.ErrorIsNil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	// Recreate mysql app without endpoint binding.
	s.mysql = f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "mysql-default",
		Charm: s.mysqlCharm,
	})
	s.mysqlUnit = f.MakeUnit(c, &factory.UnitParams{
		Application: s.mysql,
		Machine:     s.machine1,
	})
	s.setupUniterAPIForUnit(c, s.mysqlUnit)

	rel := s.addRelation(c, "mysql-default", "wordpress-remote")
	mysqlRelUnit, err := rel.Unit(s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, mysqlRelUnit, true)

	// Relation specific egress subnets override model config.
	err = s.ControllerDomainServices(c).Config().UpdateModelConfig(context.Background(), map[string]interface{}{config.EgressSubnets: "10.0.0.0/8"}, nil)
	c.Assert(err, jc.ErrorIsNil)
	relEgress := state.NewRelationEgressNetworks(s.st)
	_, err = relEgress.Save(rel.Tag().Id(), false, []string{"192.168.1.0/24"})
	c.Assert(err, jc.ErrorIsNil)

	relId := rel.Id()
	args := params.NetworkInfoParams{
		Unit:       s.mysqlUnit.Tag().String(),
		Endpoints:  []string{"server"},
		RelationId: &relId,
	}

	// Since it is a remote relation, the expected ingress address is set to the
	// machine's public address.
	expectedIngressAddress, err := s.machine1.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)

	expectedInfo := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:20:50",
				InterfaceName: "eth0.100",
				Addresses: []params.InterfaceAddress{
					{Address: "10.0.0.20", CIDR: "10.0.0.0/24"},
				},
			},
		},
		EgressSubnets:    []string{"192.168.1.0/24"},
		IngressAddresses: []string{expectedIngressAddress.Value},
	}

	result, err := s.uniter.NetworkInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, params.NetworkInfoResults{
		Results: map[string]params.NetworkInfoResult{
			"server": expectedInfo,
		},
	})
}

func (s *uniterNetworkInfoSuite) TestUpdateNetworkInfo(c *gc.C) {
	s.addRelationAndAssertInScope(c)

	// Clear network settings from all relation units
	relList, err := s.wordpressUnit.RelationsJoined()
	c.Assert(err, gc.IsNil)
	for _, rel := range relList {
		relUnit, err := rel.Unit(s.wordpressUnit)
		c.Assert(err, gc.IsNil)
		relSettings, err := relUnit.Settings()
		c.Assert(err, gc.IsNil)
		relSettings.Delete("private-address")
		relSettings.Delete("ingress-address")
		relSettings.Delete("egress-subnets")
		_, err = relSettings.Write()
		c.Assert(err, gc.IsNil)
	}

	// Making an UpdateNetworkInfo call should re-generate them for us.
	args := params.Entities{
		Entities: []params.Entity{
			{
				Tag: s.wordpressUnit.Tag().String(),
			},
		},
	}

	res, err := s.uniter.UpdateNetworkInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.OneError(), gc.IsNil)

	// Validate settings
	for _, rel := range relList {
		relUnit, err := rel.Unit(s.wordpressUnit)
		c.Assert(err, gc.IsNil)
		relSettings, err := relUnit.Settings()
		c.Assert(err, gc.IsNil)
		relMap := relSettings.Map()
		c.Assert(relMap["private-address"], gc.Equals, "10.0.0.10")
		c.Assert(relMap["ingress-address"], gc.Equals, "10.0.0.10")
		c.Assert(relMap["egress-subnets"], gc.Equals, "10.0.0.10/32")
	}
}

func (s *uniterNetworkInfoSuite) TestCommitHookChanges(c *gc.C) {
	// TODO (manadart 2024-11-18): This test should *never* have been added to
	// this suite. It is, as the name implies, a suite for testing NetworkInfo.
	c.Skip("Rewrite this in another suite once committing a hook is transactional")
	s.addRelationAndAssertInScope(c)

	s.leadershipChecker.isLeader = true

	// Clear network settings from all relation units
	relList, err := s.wordpressUnit.RelationsJoined()
	c.Assert(err, gc.IsNil)
	for _, rel := range relList {
		relUnit, err := rel.Unit(s.wordpressUnit)
		c.Assert(err, gc.IsNil)
		relSettings, err := relUnit.Settings()
		c.Assert(err, gc.IsNil)
		relSettings.Delete("private-address")
		relSettings.Delete("ingress-address")
		relSettings.Delete("egress-subnets")
		relSettings.Set("some", "settings")
		_, err = relSettings.Write()
		c.Assert(err, gc.IsNil)
	}

	b := apiuniter.NewCommitHookParamsBuilder(s.wordpressUnit.UnitTag())
	b.UpdateNetworkInfo()
	b.UpdateRelationUnitSettings(relList[0].Tag().String(), params.Settings{"just": "added"}, params.Settings{"app_data": "updated"})
	// Manipulate ports for one of the charm's endpoints.
	b.OpenPortRange("monitoring-port", network.MustParsePortRange("80-81/tcp"))
	b.OpenPortRange("monitoring-port", network.MustParsePortRange("7337/tcp")) // same port closed below; this should be a no-op
	b.ClosePortRange("monitoring-port", network.MustParsePortRange("7337/tcp"))
	b.UpdateCharmState(map[string]string{"charm-key": "charm-value"})
	req, _ := b.Build()

	// Add some extra args to test error handling
	req.Args = append(req.Args,
		params.CommitHookChangesArg{Tag: "not-a-unit-tag"},
		params.CommitHookChangesArg{Tag: "unit-mysql-0"}, // not accessible by current user
		params.CommitHookChangesArg{Tag: "unit-notfound-0"},
	)

	// Test-suite uses an older API version
	api, err := uniter.NewUniterAPI(context.Background(), s.facadeContext(c))
	c.Assert(err, jc.ErrorIsNil)

	result, err := api.CommitHookChanges(context.Background(), req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: &params.Error{Message: `"not-a-unit-tag" is not a valid tag`}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify expected wordpress unit state
	relUnit, err := relList[0].Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	relSettings, err := relUnit.Settings()
	c.Assert(err, jc.ErrorIsNil)
	expRelSettings := map[string]interface{}{
		// Network info injected due to the "UpdateNetworkInfo" request
		"egress-subnets":  "10.0.0.10/32",
		"ingress-address": "10.0.0.10",
		"private-address": "10.0.0.10",
		// Pre-existing setting
		"some": "settings",
		// Setting added due to update relation settings request
		"just": "added",
	}
	c.Assert(relSettings.Map(), jc.DeepEquals, expRelSettings, gc.Commentf("composed model operations did not yield expected result for unit relation settings"))

	unitUUID, err := s.domainServices.Application().GetUnitUUID(context.Background(), unit.Name(s.wordpressUnit.Tag().Id()))
	c.Assert(err, jc.ErrorIsNil)
	grp, err := s.domainServices.Port().GetUnitOpenedPorts(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(grp, jc.DeepEquals, network.GroupedPortRanges{
		"monitoring-port": {
			{Protocol: "tcp", FromPort: 80, ToPort: 81},
			// NOTE: Opening and closing the same port range at the same time
			// leads to different behaviour between DQLite and Mongo
			// TODO (jack-w-shaw): evaluate if this is worth fixing
			{Protocol: "tcp", FromPort: 7337, ToPort: 7337},
		},
	}, gc.Commentf("unit ports where not opened for the requested endpoint"))

	// unitState, err := s.wordpressUnit.State()
	// c.Assert(err, jc.ErrorIsNil)
	// charmState, _ := unitState.CharmState()
	// c.Assert(charmState, jc.DeepEquals, map[string]string{"charm-key": "charm-value"}, gc.Commentf("state doc not updated"))

	appCfg, err := relList[0].ApplicationSettings(s.wordpress.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appCfg, gc.DeepEquals, map[string]interface{}{"app_data": "updated"}, gc.Commentf("application data not updated by leader unit"))
}

func (s *uniterNetworkInfoSuite) TestCommitHookChangesWhenNotLeader(c *gc.C) {
	s.addRelationAndAssertInScope(c)

	// Make it so we're not the leader.
	s.leadershipChecker.isLeader = false

	relList, err := s.wordpressUnit.RelationsJoined()
	c.Assert(err, gc.IsNil)

	b := apiuniter.NewCommitHookParamsBuilder(s.wordpressUnit.UnitTag())
	b.UpdateRelationUnitSettings(relList[0].Tag().String(), nil, params.Settings{"can't": "touch this!"})
	req, _ := b.Build()

	// Test-suite uses an older API version
	api, err := uniter.NewUniterAPI(context.Background(), s.facadeContext(c))
	c.Assert(err, jc.ErrorIsNil)

	result, err := api.CommitHookChanges(context.Background(), req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: `checking leadership continuity: "wordpress/1" is not leader of "wordpress"`}},
		},
	})
}
