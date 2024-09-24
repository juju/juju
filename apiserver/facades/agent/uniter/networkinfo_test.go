// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/juju/clock"
	"github.com/juju/names/v5"
	"github.com/juju/retry"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/agent/uniter"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8stesting "github.com/juju/juju/caas/kubernetes/provider/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type ProReqRelation struct {
	rel                    *state.Relation
	papp, rapp             *state.Application
	pu0, pu1, ru0, ru1     *state.Unit
	pru0, pru1, rru0, rru1 *state.RelationUnit
}

type RemoteProReqRelation struct {
	rel                    *state.Relation
	papp                   *state.RemoteApplication
	rapp                   *state.Application
	pru0, pru1, rru0, rru1 *state.RelationUnit
	ru0, ru1               *state.Unit
}

type NetworkService interface {
	SpaceByName(ctx context.Context, name string) (*network.SpaceInfo, error)
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
	AddSpace(ctx context.Context, space network.SpaceInfo) (network.Id, error)
	AddSubnet(ctx context.Context, args network.SubnetInfo) (network.Id, error)
	Space(ctx context.Context, uuid string) (*network.SpaceInfo, error)
}

type networkInfoSuite struct {
	testing.ApiServerSuite
	networkService NetworkService
}

// modelConfigService is a convenience function to get the controller model's
// model config service inside a test.
func (s *networkInfoSuite) modelConfigService(c *gc.C) uniter.ModelConfigService {
	return s.ControllerDomainServices(c).Config()
}

var _ = gc.Suite(&networkInfoSuite{})

func (s *networkInfoSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)
	s.ApiServerSuite.SeedCAASCloud(c)

	domainServices := s.ControllerDomainServices(c)
	s.networkService = domainServices.Network()
}

func (s *networkInfoSuite) TestNetworksForRelation(c *gc.C) {
	st := s.ControllerModel(c).State()

	prr := s.newProReqRelation(c, charm.ScopeGlobal)
	err := prr.pu0.AssignToNewMachine(s.modelConfigService(c))
	c.Assert(err, jc.ErrorIsNil)
	id, err := prr.pu0.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)

	machine, err := st.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	controllerDomainServices := s.ControllerDomainServices(c)
	controllerConfigService := controllerDomainServices.ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetProviderAddresses(
		controllerConfig,
		network.NewSpaceAddress("10.2.3.4", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("4.3.2.1", network.WithScope(network.ScopePublic)),
	)
	c.Assert(err, jc.ErrorIsNil)

	netInfo := s.newNetworkInfo(c, prr.pu0.UnitTag(), nil, nil)
	boundSpace, ingress, egress, err := netInfo.NetworksForRelation("", prr.rel)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, network.AlphaSpaceId)
	c.Assert(ingress, gc.DeepEquals,
		network.SpaceAddresses{network.NewSpaceAddress("10.2.3.4", network.WithScope(network.ScopeCloudLocal))})
	c.Assert(egress, gc.DeepEquals, []string{"10.2.3.4/32"})
}

func (s *networkInfoSuite) addDevicesWithAddresses(c *gc.C, machine *state.Machine, addresses ...string) {
	for _, address := range addresses {
		name := fmt.Sprintf("e%x", rand.Int31())
		deviceArgs := state.LinkLayerDeviceArgs{
			Name: name,
			Type: network.EthernetDevice,
		}
		err := machine.SetLinkLayerDevices(deviceArgs)
		c.Assert(err, jc.ErrorIsNil)
		device, err := machine.LinkLayerDevice(name)
		c.Assert(err, jc.ErrorIsNil)

		addressesArg := state.LinkLayerDeviceAddress{
			DeviceName:   name,
			ConfigMethod: network.ConfigStatic,
			CIDRAddress:  address,
		}
		err = machine.SetDevicesAddresses(addressesArg)
		c.Assert(err, jc.ErrorIsNil)
		deviceAddresses, err := device.Addresses()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(deviceAddresses, gc.HasLen, 1)
	}
}

func (s *networkInfoSuite) TestProcessAPIRequestForBinding(c *gc.C) {
	st := s.ControllerModel(c).State()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f = f.WithModelConfigService(s.modelConfigService(c))

	bindings := map[string]string{
		"":             network.AlphaSpaceId,
		"server-admin": network.AlphaSpaceId,
	}
	app := f.MakeApplication(c, &factory.ApplicationParams{
		EndpointBindings: bindings,
	})
	_, err := s.networkService.AddSubnet(
		context.Background(),
		network.SubnetInfo{
			CIDR:              "10.2.0.0/16",
			SpaceID:           network.AlphaSpaceId,
			ProviderId:        "subnet-0",
			ProviderNetworkId: "subnet-0",
		})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.networkService.AddSubnet(
		context.Background(),
		network.SubnetInfo{
			CIDR:              "100.2.3.0/24",
			SpaceID:           network.AlphaSpaceId,
			ProviderId:        "subnet-1",
			ProviderNetworkId: "subnet-1",
		})
	c.Assert(err, jc.ErrorIsNil)

	unit, err := app.AddUnit(s.modelConfigService(c), state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.AssignToNewMachine(s.modelConfigService(c)), jc.ErrorIsNil)

	id, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := st.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	controllerDomainServices := s.ControllerDomainServices(c)
	controllerConfigService := controllerDomainServices.ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	// We need at least one address on the machine itself, because these are
	// retrieved up-front to use as a fallback when we fail to locate addresses
	// on link-layer devices.
	err = machine.SetProviderAddresses(controllerConfig, network.NewSpaceAddress("10.2.3.4/16"))
	c.Assert(err, jc.ErrorIsNil)

	s.addDevicesWithAddresses(c, machine, "10.2.3.4/16", "100.2.3.4/24")

	netInfo := s.newNetworkInfo(c, unit.UnitTag(), nil, nil)
	result, err := netInfo.ProcessAPIRequest(params.NetworkInfoParams{
		Unit:      unit.UnitTag().String(),
		Endpoints: []string{"server-admin"},
	})
	c.Assert(err, jc.ErrorIsNil)

	res := result.Results
	c.Assert(res, gc.HasLen, 1)

	binding, ok := res["server-admin"]
	c.Assert(ok, jc.IsTrue)

	ingress := binding.IngressAddresses
	c.Assert(len(ingress), jc.GreaterThan, 0)

	// Sorting should place the public address before the cloud-local one.
	c.Check(ingress[0], gc.Equals, "100.2.3.4")
}

func (s *networkInfoSuite) TestProcessAPIRequestBridgeWithSameIPOverNIC(c *gc.C) {
	// Add a single subnet in the alpha space.
	st := s.ControllerModel(c).State()
	_, err := s.networkService.AddSubnet(
		context.Background(),
		network.SubnetInfo{
			CIDR:              "10.2.0.0/16",
			SpaceID:           network.AlphaSpaceId,
			ProviderId:        "subnet-0",
			ProviderNetworkId: "subnet-0",
		})
	c.Assert(err, jc.ErrorIsNil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f = f.WithModelConfigService(s.modelConfigService(c))

	bindings := map[string]string{
		"":             network.AlphaSpaceId,
		"server-admin": network.AlphaSpaceId,
	}
	app := f.MakeApplication(c, &factory.ApplicationParams{
		EndpointBindings: bindings,
	})

	unit, err := app.AddUnit(s.modelConfigService(c), state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.AssignToNewMachine(s.modelConfigService(c)), jc.ErrorIsNil)

	id, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := st.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	ip := "10.2.3.4/16"

	controllerDomainServices := s.ControllerDomainServices(c)
	controllerConfigService := controllerDomainServices.ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	// We need at least one address on the machine itself, because these are
	// retrieved up-front to use as a fallback when we fail to locate addresses
	// on link-layer devices.
	err = machine.SetProviderAddresses(controllerConfig, network.NewSpaceAddress(ip))
	c.Assert(err, jc.ErrorIsNil)

	// Create a NIC and bridge, but also add the IP to the NIC to simulate
	// this data coming from the provider via the instance poller.
	s.createNICAndBridgeWithIP(c, machine, "eth0", "br-eth0", ip)
	err = machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:   "eth0",
			CIDRAddress:  ip,
			ConfigMethod: network.ConfigStatic,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	netInfo := s.newNetworkInfo(c, unit.UnitTag(), nil, nil)
	result, err := netInfo.ProcessAPIRequest(params.NetworkInfoParams{
		Unit:      unit.UnitTag().String(),
		Endpoints: []string{"server-admin"},
	})
	c.Assert(err, jc.ErrorIsNil)

	res := result.Results
	c.Assert(res, gc.HasLen, 1)

	binding, ok := res["server-admin"]
	c.Assert(ok, jc.IsTrue)

	// We should get the bridge and only the bridge for this IP.
	info := binding.Info
	c.Assert(info, gc.HasLen, 1)
	c.Check(info[0].InterfaceName, gc.Equals, "br-eth0")
}

func (s *networkInfoSuite) TestAPIRequestForRelationIAASHostNameIngressNoEgress(c *gc.C) {
	prr := s.newProReqRelation(c, charm.ScopeGlobal)
	err := prr.pu0.AssignToNewMachine(s.modelConfigService(c))
	c.Assert(err, jc.ErrorIsNil)
	id, err := prr.pu0.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.ControllerModel(c).State().Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	// The only address is a host-name, resolvable to the IP below.
	host := "host.at.somewhere"
	ip := "100.2.3.4"

	controllerDomainServices := s.ControllerDomainServices(c)
	controllerConfigService := controllerDomainServices.ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	addr := network.NewSpaceAddress(host)
	err = machine.SetProviderAddresses(controllerConfig, addr)
	c.Assert(err, jc.ErrorIsNil)

	lookup := func(addr string) ([]string, error) {
		if addr == host {
			return []string{ip}, nil
		}
		return nil, errors.New("bad horsey")
	}

	netInfo := s.newNetworkInfo(c, prr.pu0.UnitTag(), nil, lookup)

	rID := prr.rel.Id()
	result, err := netInfo.ProcessAPIRequest(params.NetworkInfoParams{
		Unit:       names.NewUnitTag(prr.pru0.UnitName()).String(),
		Endpoints:  []string{"server"},
		RelationId: &rID,
	})
	c.Assert(err, jc.ErrorIsNil)

	res := result.Results
	c.Assert(res, gc.HasLen, 1)

	binding, ok := res["server"]
	c.Assert(ok, jc.IsTrue)

	ingress := binding.IngressAddresses
	c.Assert(ingress, gc.HasLen, 1)
	c.Check(ingress[0], gc.Equals, ip)

	c.Assert(binding.Info, gc.HasLen, 1)

	addrs := binding.Info[0].Addresses
	c.Check(addrs, gc.HasLen, 1)
	c.Check(addrs[0].Hostname, gc.Equals, host)
	c.Check(addrs[0].Address, gc.Equals, ip)
}

func (s *networkInfoSuite) TestAPIRequestForRelationCAASHostNameNoIngress(c *gc.C) {
	s.PatchValue(&provider.NewK8sClients, k8stesting.NoopFakeK8sClients)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f = f.WithModelConfigService(s.modelConfigService(c))

	// For the test to run properly with part of the model in mongo and
	// part in a service domain, a model with the same uuid is required
	// in both places for the test to work. Necessary after model config
	// was move to the domain services.
	modelUUID, err := uuid.UUIDFromString(s.DefaultModelUUID.String())
	c.Assert(err, jc.ErrorIsNil)
	st := f.MakeCAASModel(c, &factory.ModelParams{UUID: &modelUUID})
	defer func() { _ = st.Close() }()

	f2, release := s.NewFactory(c, st.ModelUUID())
	defer release()
	f2 = f2.WithModelConfigService(s.modelConfigService(c))

	ch := f2.MakeCharm(c, &factory.CharmParams{Name: "mysql-k8s", Series: "focal"})
	app := f2.MakeApplication(c, &factory.ApplicationParams{Name: "mysql", Charm: ch})
	u := f2.MakeUnit(c, &factory.UnitParams{Application: app})

	// The only address is a host-name, resolvable to the IP below.
	host := "host.at.somewhere"
	ip := "100.2.3.4"

	lookup := func(addr string) ([]string, error) {
		if addr == host {
			return []string{ip}, nil
		}
		return nil, errors.New("bad horsey")
	}

	err = app.UpdateCloudService("", network.SpaceAddresses{
		network.NewSpaceAddress(host, network.WithScope(network.ScopePublic)),
	})
	c.Assert(err, jc.ErrorIsNil)

	modelConfigService := s.ControllerDomainServices(c).Config()
	// We need to instantiate this with the new CAAS model state.
	netInfo, err := uniter.NewNetworkInfoForStrategy(context.Background(), st, s.networkService, modelConfigService, u.UnitTag(), nil, lookup, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)

	result, err := netInfo.ProcessAPIRequest(params.NetworkInfoParams{
		Unit:      u.UnitTag().String(),
		Endpoints: []string{"server"},
	})
	c.Assert(err, jc.ErrorIsNil)

	res := result.Results
	c.Assert(res, gc.HasLen, 1)

	binding, ok := res["server"]
	c.Assert(ok, jc.IsTrue)

	ingress := binding.IngressAddresses
	c.Assert(ingress, gc.HasLen, 1)
	// The ingress address host name is not resolved.
	c.Check(ingress[0], gc.Equals, host)
}

func (s *networkInfoSuite) TestNetworksForRelationWithSpaces(c *gc.C) {
	space1 := s.setupSpace(c, "space-1", "1.2.0.0/16")
	space2 := s.setupSpace(c, "space-2", "2.2.0.0/16")
	space3 := s.setupSpace(c, "space-3", "10.2.0.0/16")
	_ = s.setupSpace(c, "public-4", "4.2.0.0/16")

	// We want to have all bindings set so that no actual binding is
	// really set to the default.
	bindings := map[string]string{
		"":             space3.ID,
		"server-admin": space1.ID,
		"server":       space2.ID,
	}

	st := s.ControllerModel(c).State()

	prr := s.newProReqRelationWithBindings(c, charm.ScopeGlobal, bindings, nil)
	err := prr.pu0.AssignToNewMachine(s.modelConfigService(c))
	c.Assert(err, jc.ErrorIsNil)
	id, err := prr.pu0.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)

	machine, err := st.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	controllerDomainServices := s.ControllerDomainServices(c)
	controllerConfigService := controllerDomainServices.ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	addresses := []network.SpaceAddress{
		network.NewSpaceAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("2.2.3.4", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("10.2.3.4", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("4.3.2.1", network.WithScope(network.ScopePublic)),
	}
	err = machine.SetProviderAddresses(controllerConfig, addresses...)
	c.Assert(err, jc.ErrorIsNil)

	s.addDevicesWithAddresses(c, machine, "1.2.3.4/16", "2.2.3.4/16", "10.2.3.4/16", "4.3.2.1/16")

	netInfo := s.newNetworkInfo(c, prr.pu0.UnitTag(), nil, nil)
	boundSpace, ingress, egress, err := netInfo.NetworksForRelation("", prr.rel)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, space3.ID)

	exp := network.SpaceAddresses{network.NewSpaceAddress(
		"10.2.3.4",
		network.WithScope(network.ScopeCloudLocal),
		network.WithConfigType(network.ConfigStatic),
		network.WithCIDR("10.2.0.0/16"),
	)}
	exp[0].SpaceID = "space-3"
	c.Assert(ingress, gc.DeepEquals, exp)
	c.Assert(egress, gc.DeepEquals, []string{"10.2.3.4/32"})
}

func (s *networkInfoSuite) TestNetworksForRelationRemoteRelation(c *gc.C) {
	st := s.ControllerModel(c).State()
	prr := s.newRemoteProReqRelation(c)
	err := prr.ru0.AssignToNewMachine(s.modelConfigService(c))
	c.Assert(err, jc.ErrorIsNil)
	id, err := prr.ru0.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)

	machine, err := st.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	controllerDomainServices := s.ControllerDomainServices(c)
	controllerConfigService := controllerDomainServices.ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetProviderAddresses(
		controllerConfig,
		network.NewSpaceAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("4.3.2.1", network.WithScope(network.ScopePublic)),
	)
	c.Assert(err, jc.ErrorIsNil)

	netInfo := s.newNetworkInfo(c, prr.ru0.UnitTag(), nil, nil)
	boundSpace, ingress, egress, err := netInfo.NetworksForRelation("", prr.rel)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, network.AlphaSpaceId)
	c.Assert(ingress, gc.DeepEquals,
		network.SpaceAddresses{network.NewSpaceAddress("4.3.2.1", network.WithScope(network.ScopePublic))})
	c.Assert(egress, gc.DeepEquals, []string{"4.3.2.1/32"})
}

func (s *networkInfoSuite) TestNetworksForRelationRemoteRelationNoPublicAddr(c *gc.C) {
	st := s.ControllerModel(c).State()

	prr := s.newRemoteProReqRelation(c)
	err := prr.ru0.AssignToNewMachine(s.modelConfigService(c))
	c.Assert(err, jc.ErrorIsNil)
	id, err := prr.ru0.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)

	machine, err := st.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	controllerDomainServices := s.ControllerDomainServices(c)
	controllerConfigService := controllerDomainServices.ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetProviderAddresses(
		controllerConfig,
		network.NewSpaceAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal)),
	)
	c.Assert(err, jc.ErrorIsNil)

	netInfo := s.newNetworkInfo(c, prr.ru0.UnitTag(), nil, nil)
	boundSpace, ingress, egress, err := netInfo.NetworksForRelation("", prr.rel)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, network.AlphaSpaceId)
	c.Assert(ingress, gc.DeepEquals,
		network.SpaceAddresses{network.NewSpaceAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal))})
	c.Assert(egress, gc.DeepEquals, []string{"1.2.3.4/32"})
}

func (s *networkInfoSuite) TestNetworksForRelationRemoteRelationDelayedPublicAddress(c *gc.C) {
	st := s.ControllerModel(c).State()

	prr := s.newRemoteProReqRelation(c)
	err := prr.ru0.AssignToNewMachine(s.modelConfigService(c))
	c.Assert(err, jc.ErrorIsNil)
	id, err := prr.ru0.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)

	machine, err := st.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	controllerDomainServices := s.ControllerDomainServices(c)
	controllerConfigService := controllerDomainServices.ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	retryFactory := func() retry.CallArgs {
		return retry.CallArgs{
			Clock:       clock.WallClock,
			Delay:       1 * time.Millisecond,
			MaxDuration: coretesting.ShortWait,
			NotifyFunc: func(lastError error, attempt int) {
				// Set the address after one failed retrieval attempt.
				if attempt == 1 {
					err := machine.SetProviderAddresses(
						controllerConfig,
						network.NewSpaceAddress("4.3.2.1", network.WithScope(network.ScopePublic)),
					)
					c.Assert(err, jc.ErrorIsNil)
				}
			},
		}
	}

	netInfo := s.newNetworkInfo(c, prr.ru0.UnitTag(), retryFactory, nil)
	boundSpace, ingress, egress, err := netInfo.NetworksForRelation("", prr.rel)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, network.AlphaSpaceId)
	c.Assert(ingress, gc.DeepEquals,
		network.SpaceAddresses{network.NewSpaceAddress("4.3.2.1", network.WithScope(network.ScopePublic))})
	c.Assert(egress, gc.DeepEquals, []string{"4.3.2.1/32"})
}

func (s *networkInfoSuite) TestNetworksForRelationRemoteRelationDelayedPrivateAddress(c *gc.C) {
	st := s.ControllerModel(c).State()

	prr := s.newRemoteProReqRelation(c)
	err := prr.ru0.AssignToNewMachine(s.modelConfigService(c))
	c.Assert(err, jc.ErrorIsNil)
	id, err := prr.ru0.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)

	machine, err := st.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	controllerDomainServices := s.ControllerDomainServices(c)
	controllerConfigService := controllerDomainServices.ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	// The first attempt is for the public address.
	// The retry we supply for this fails quickly.
	// The second is for the private address fallback.
	var publicAddrSentinel bool
	retryFactory := func() retry.CallArgs {
		if !publicAddrSentinel {
			publicAddrSentinel = true

			return retry.CallArgs{
				Clock:       clock.WallClock,
				Delay:       1 * time.Millisecond,
				MaxDuration: 1 * time.Millisecond,
			}
		}

		return retry.CallArgs{
			Clock:       clock.WallClock,
			Delay:       1 * time.Millisecond,
			MaxDuration: coretesting.ShortWait,
			NotifyFunc: func(lastError error, attempt int) {
				// Set the private address after one failed retrieval attempt.
				if attempt == 1 {
					err := machine.SetProviderAddresses(
						controllerConfig,
						network.NewSpaceAddress("4.3.2.1", network.WithScope(network.ScopeCloudLocal)),
					)
					c.Assert(err, jc.ErrorIsNil)
				}
			},
		}
	}

	netInfo := s.newNetworkInfo(c, prr.ru0.UnitTag(), retryFactory, nil)
	boundSpace, ingress, egress, err := netInfo.NetworksForRelation("", prr.rel)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, network.AlphaSpaceId)
	c.Assert(ingress, gc.DeepEquals,
		network.SpaceAddresses{network.NewSpaceAddress("4.3.2.1", network.WithScope(network.ScopeCloudLocal))})
	c.Assert(egress, gc.DeepEquals, []string{"4.3.2.1/32"})
}

func (s *networkInfoSuite) TestNetworksForRelationCAASModel(c *gc.C) {
	s.PatchValue(&provider.NewK8sClients, k8stesting.NoopFakeK8sClients)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f = f.WithModelConfigService(s.modelConfigService(c))

	// For the test to run properly with part of the model in mongo and
	// part in a service domain, a model with the same uuid is required
	// in both places for the test to work. Necessary after model config
	// was move to the domain services.
	modelUUID, err := uuid.UUIDFromString(s.DefaultModelUUID.String())
	c.Assert(err, jc.ErrorIsNil)
	st := f.MakeCAASModel(c, &factory.ModelParams{UUID: &modelUUID})
	defer func() { _ = st.Close() }()

	f2, release := s.NewFactory(c, st.ModelUUID())
	defer release()
	f2 = f2.WithModelConfigService(s.modelConfigService(c))

	gitlabch := f2.MakeCharm(c, &factory.CharmParams{Name: "gitlab-k8s", Series: "focal"})
	mysqlch := f2.MakeCharm(c, &factory.CharmParams{Name: "mysql-k8s", Series: "focal"})
	gitlab := f2.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: gitlabch})
	mysql := f2.MakeApplication(c, &factory.ApplicationParams{Name: "mysql", Charm: mysqlch})

	prr := newProReqRelationForApps(c, st, s.modelConfigService(c), mysql, gitlab)

	modelConfigService := s.ControllerDomainServices(c).Config()
	// We need to instantiate this with the new CAAS model state.
	netInfo, err := uniter.NewNetworkInfoForStrategy(context.Background(), st, s.networkService, modelConfigService, prr.pu0.UnitTag(), nil, nil, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)

	// First no address.
	boundSpace, ingress, egress, err := netInfo.NetworksForRelation("", prr.rel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(boundSpace, gc.Equals, network.AlphaSpaceId)
	c.Assert(ingress, gc.HasLen, 0)
	c.Assert(egress, gc.HasLen, 0)

	// Add an application address.
	err = mysql.UpdateCloudService("", network.SpaceAddresses{
		network.NewSpaceAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal)),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = prr.pu0.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	// We need a new instance here, because unit addresses
	// are populated in the constructor.
	netInfo, err = uniter.NewNetworkInfoForStrategy(context.Background(), st, s.networkService, modelConfigService, prr.pu0.UnitTag(), nil, nil, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)
	boundSpace, ingress, egress, err = netInfo.NetworksForRelation("", prr.rel)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, network.AlphaSpaceId)
	c.Assert(ingress, gc.DeepEquals,
		network.SpaceAddresses{network.NewSpaceAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal))})
	c.Assert(egress, gc.DeepEquals, []string{"1.2.3.4/32"})
}

func (s *networkInfoSuite) TestNetworksForRelationCAASModelInvalidBinding(c *gc.C) {

	s.PatchValue(&provider.NewK8sClients, k8stesting.NoopFakeK8sClients)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f = f.WithModelConfigService(s.modelConfigService(c))

	// For the test to run properly with part of the model in mongo and
	// part in a service domain, a model with the same uuid is required
	// in both places for the test to work. Necessary after model config
	// was move to the domain services.
	modelUUID, err := uuid.UUIDFromString(s.DefaultModelUUID.String())
	c.Assert(err, jc.ErrorIsNil)
	st := f.MakeCAASModel(c, &factory.ModelParams{UUID: &modelUUID})
	defer func() { _ = st.Close() }()

	f2, release := s.NewFactory(c, st.ModelUUID())
	defer release()
	f2 = f2.WithModelConfigService(s.modelConfigService(c))

	gitLabCh := f2.MakeCharm(c, &factory.CharmParams{Name: "gitlab-k8s", Series: "focal"})
	mySqlCh := f2.MakeCharm(c, &factory.CharmParams{Name: "mysql-k8s", Series: "focal"})
	gitLab := f2.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: gitLabCh})
	mySql := f2.MakeApplication(c, &factory.ApplicationParams{Name: "mysql", Charm: mySqlCh})

	prr := newProReqRelationForApps(c, st, s.modelConfigService(c), mySql, gitLab)

	modelConfigService := s.ControllerDomainServices(c).Config()
	// We need to instantiate this with the new CAAS model state.
	netInfo, err := uniter.NewNetworkInfoForStrategy(context.Background(), st, s.networkService, modelConfigService, prr.pu0.UnitTag(), nil, nil, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)

	_, _, _, err = netInfo.NetworksForRelation("unknown", prr.rel)
	c.Assert(err, gc.ErrorMatches, `undefined for unit charm: endpoint "unknown" not valid`)
}

func (s *networkInfoSuite) TestNetworksForRelationCAASModelCrossModelNoPrivate(c *gc.C) {

	s.PatchValue(&provider.NewK8sClients, k8stesting.NoopFakeK8sClients)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f = f.WithModelConfigService(s.modelConfigService(c))

	// For the test to run properly with part of the model in mongo and
	// part in a service domain, a model with the same uuid is required
	// in both places for the test to work. Necessary after model config
	// was move to the domain services.
	modelUUID, err := uuid.UUIDFromString(s.DefaultModelUUID.String())
	c.Assert(err, jc.ErrorIsNil)
	st := f.MakeCAASModel(c, &factory.ModelParams{UUID: &modelUUID})
	defer func() { _ = st.Close() }()

	f2, release := s.NewFactory(c, st.ModelUUID())
	defer release()
	f2 = f2.WithModelConfigService(s.modelConfigService(c))

	gitLabCh := f2.MakeCharm(c, &factory.CharmParams{Name: "gitlab-k8s", Series: "focal"})
	gitLab := f2.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: gitLabCh})

	// Add a local-machine address.
	// Adding it to the service instead of the container is OK here,
	// as we are interested in the return from unit.AllAddresses().
	// It simulates the same thing.
	// This should never be returned as an ingress address.
	err = gitLab.UpdateCloudService("", network.SpaceAddresses{
		network.NewSpaceAddress("1.2.3.4", network.WithScope(network.ScopeMachineLocal)),
	})
	c.Assert(err, jc.ErrorIsNil)

	papp, err := st.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql",
		SourceModel: coretesting.ModelTag,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "server",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}}})
	c.Assert(err, jc.ErrorIsNil)

	eps, err := st.InferEndpoints("mysql", "gitlab")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	prr := &RemoteProReqRelation{rel: rel, papp: papp, rapp: gitLab}
	prr.pru0 = addRemoteRU(c, rel, "mysql/0")
	prr.pru1 = addRemoteRU(c, rel, "mysql/1")
	prr.ru0, prr.rru0 = addRU(c, s.modelConfigService(c), gitLab, rel, nil)
	prr.ru1, prr.rru1 = addRU(c, s.modelConfigService(c), gitLab, rel, nil)

	// Add a container address.
	// These are scoped as local-machine and are fallen back to for CAAS by
	// unit.PrivateAddress when scope matching returns nothing.
	addr := "1.2.3.4"
	err = st.ApplyOperation(prr.ru0.UpdateOperation(state.UnitUpdateProperties{Address: &addr}))
	c.Assert(err, jc.ErrorIsNil)
	err = prr.ru0.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	retryFactory := func() retry.CallArgs {
		return retry.CallArgs{
			Clock:       clock.WallClock,
			Delay:       1 * time.Millisecond,
			MaxDuration: 1 * time.Millisecond,
		}
	}

	modelConfigService := s.ControllerDomainServices(c).Config()
	netInfo, err := uniter.NewNetworkInfoForStrategy(context.Background(), st, s.networkService, modelConfigService, prr.ru0.UnitTag(), retryFactory, nil, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)

	// At this point we only have a container (local-machine) address.
	// We expect no return when asking to poll for the public address.
	boundSpace, ingress, egress, err := netInfo.NetworksForRelation("", prr.rel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(boundSpace, gc.Equals, network.AlphaSpaceId)
	c.Assert(ingress, gc.HasLen, 0)
	c.Assert(egress, gc.HasLen, 0)

	// Now set a public address. This is a suitable ingress address.
	err = gitLab.UpdateCloudService("", network.SpaceAddresses{
		network.NewSpaceAddress("2.3.4.5", network.WithScope(network.ScopePublic)),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = prr.ru0.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	// We need a new instance here, because unit addresses
	// are populated in the constructor.
	netInfo, err = uniter.NewNetworkInfoForStrategy(context.Background(), st, s.networkService, modelConfigService, prr.ru0.UnitTag(), retryFactory, nil, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)
	boundSpace, ingress, egress, err = netInfo.NetworksForRelation("", prr.rel)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, network.AlphaSpaceId)
	c.Assert(ingress, gc.DeepEquals,
		network.SpaceAddresses{network.NewSpaceAddress("2.3.4.5", network.WithScope(network.ScopePublic))})
	c.Assert(egress, gc.DeepEquals, []string{"2.3.4.5/32"})
}

func (s *networkInfoSuite) TestMachineNetworkInfos(c *gc.C) {
	spaceDefault := s.setupSpace(c, "default", "10.0.0.0/24")
	spacePublic := s.setupSpace(c, "public", "10.10.0.0/24")
	_ = s.setupSpace(c, "private", "10.20.0.0/24")

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f = f.WithModelConfigService(s.modelConfigService(c))

	bindings := map[string]string{
		"":             spaceDefault.ID,
		"server-admin": spacePublic.ID,
	}
	app := f.MakeApplication(c, &factory.ApplicationParams{
		EndpointBindings: bindings,
	})

	unit, err := app.AddUnit(s.modelConfigService(c), state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	st := s.ControllerModel(c).State()
	machine, err := st.AddOneMachine(
		s.modelConfigService(c),
		state.MachineTemplate{
			Base: state.UbuntuBase("12.10"),
			Jobs: []state.MachineJob{state.JobHostUnits},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = unit.AssignToMachine(s.modelConfigService(c), machine)
	c.Assert(err, jc.ErrorIsNil)

	s.createNICAndBridgeWithIP(c, machine, "eth0", "br-eth0", "10.0.0.20/24")
	s.createNICWithIP(c, machine, network.EthernetDevice, "eth1", "10.10.0.20/24")
	s.createNICWithIP(c, machine, network.EthernetDevice, "eth2", "10.20.0.20/24")

	controllerDomainServices := s.ControllerDomainServices(c)
	controllerConfigService := controllerDomainServices.ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetMachineAddresses(
		controllerConfig,
		network.NewSpaceAddress("10.0.0.20", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("10.10.0.20", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("10.10.0.30", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("10.20.0.20", network.WithScope(network.ScopeCloudLocal)),
	)
	c.Assert(err, jc.ErrorIsNil)

	ni := s.newNetworkInfo(c, unit.UnitTag(), nil, nil)
	netInfo := ni.(*uniter.NetworkInfoIAAS)

	res, err := netInfo.MachineNetworkInfos()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 3)

	resDefault, ok := res[spaceDefault.ID]
	c.Assert(ok, jc.IsTrue)
	c.Assert(resDefault, gc.HasLen, 1)
	c.Check(resDefault[0].DeviceName(), gc.Equals, "br-eth0")
	c.Check(resDefault[0].Host(), gc.Equals, "10.0.0.20")
	c.Check(resDefault[0].AddressCIDR(), gc.Equals, "10.0.0.0/24")

	resPublic, ok := res[spacePublic.ID]
	c.Assert(ok, jc.IsTrue)
	c.Assert(resPublic, gc.HasLen, 1)
	c.Check(resPublic[0].DeviceName(), gc.Equals, "eth1")
	c.Check(resPublic[0].Host(), gc.Equals, "10.10.0.20")
	c.Check(resPublic[0].AddressCIDR(), gc.Equals, "10.10.0.0/24")

	// The implicit juju-info endpoint is bound to alpha.
	// With no NICs in this space, we pick the NIC that matches the machine's
	// local-cloud address, even though it is actually in the private space.
	resEmpty, ok := res[network.AlphaSpaceId]
	c.Assert(ok, jc.IsTrue)
	c.Assert(resEmpty, gc.HasLen, 1)
	c.Check(resEmpty[0].DeviceName(), gc.Equals, "eth2")
	c.Check(resEmpty[0].Host(), gc.Equals, "10.20.0.20")
	c.Check(resEmpty[0].AddressCIDR(), gc.Equals, "10.20.0.0/24")
}

// TODO (manadart 2020-02-21): This test can be removed after universal subnet
// discovery is implemented.
func (s *networkInfoSuite) TestMachineNetworkInfosAlphaNoSubnets(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f = f.WithModelConfigService(s.modelConfigService(c))

	app := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})

	unit, err := app.AddUnit(s.modelConfigService(c), state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	st := s.ControllerModel(c).State()
	machine, err := st.AddOneMachine(
		s.modelConfigService(c),
		state.MachineTemplate{
			Base: state.UbuntuBase("12.10"),
			Jobs: []state.MachineJob{state.JobHostUnits},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = unit.AssignToMachine(s.modelConfigService(c), machine)
	c.Assert(err, jc.ErrorIsNil)

	s.createNICAndBridgeWithIP(c, machine, "eth0", "br-eth0", "10.0.0.20/24")
	s.createNICWithIP(c, machine, network.EthernetDevice, "eth1", "10.10.0.20/24")
	s.createNICWithIP(c, machine, network.EthernetDevice, "eth2", "10.20.0.20/24")

	controllerDomainServices := s.ControllerDomainServices(c)
	controllerConfigService := controllerDomainServices.ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetMachineAddresses(
		controllerConfig,
		network.NewSpaceAddress("10.0.0.20", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("10.10.0.20", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("10.10.0.30", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("10.20.0.20", network.WithScope(network.ScopeCloudLocal)),
	)
	c.Assert(err, jc.ErrorIsNil)

	ni := s.newNetworkInfo(c, unit.UnitTag(), nil, nil)
	netInfo := ni.(*uniter.NetworkInfoIAAS)

	res, err := netInfo.MachineNetworkInfos()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 1)

	resEmpty, ok := res[network.AlphaSpaceId]
	c.Assert(ok, jc.IsTrue)
	c.Assert(resEmpty, gc.HasLen, 1)
	c.Check(resEmpty[0].DeviceName(), gc.Equals, "eth2")
	c.Check(resEmpty[0].Host(), gc.Equals, "10.20.0.20")
	c.Check(resEmpty[0].AddressCIDR(), gc.Equals, "10.20.0.0/24")
}

func (s *networkInfoSuite) setupSpace(c *gc.C, spaceName, cidr string) network.SpaceInfo {
	spaceID, err := s.networkService.AddSpace(context.Background(), network.SpaceInfo{
		ID:   spaceName,
		Name: network.SpaceName(spaceName),
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.networkService.AddSubnet(
		context.Background(),
		network.SubnetInfo{
			CIDR:              cidr,
			SpaceID:           spaceID.String(),
			ProviderId:        network.Id("subnet-" + spaceName),
			ProviderNetworkId: network.Id("subnet-" + spaceName),
		})
	c.Assert(err, jc.ErrorIsNil)

	spInfo, err := s.networkService.Space(context.Background(), spaceID.String())
	c.Assert(err, jc.ErrorIsNil)
	return *spInfo
}

// createNICAndBridgeWithIP creates a network interface and a bridge on the
// machine, and assigns the requested CIDRAddress to the bridge.
func (s *networkInfoSuite) createNICAndBridgeWithIP(
	c *gc.C, machine *state.Machine, deviceName, bridgeName, cidrAddress string,
) {
	s.createNICWithIP(c, machine, network.BridgeDevice, bridgeName, cidrAddress)

	err := machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       deviceName,
			Type:       network.EthernetDevice,
			ParentName: bridgeName,
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *networkInfoSuite) createNICWithIP(
	c *gc.C, machine *state.Machine, deviceType network.LinkLayerDeviceType, deviceName, cidrAddress string,
) {
	err := machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       deviceName,
			Type:       deviceType,
			ParentName: "",
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:   deviceName,
			CIDRAddress:  cidrAddress,
			ConfigMethod: network.ConfigStatic,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *networkInfoSuite) newNetworkInfo(
	c *gc.C, tag names.UnitTag, retryFactory func() retry.CallArgs, lookupHost func(string) ([]string, error),
) uniter.NetworkInfo {
	// Allow the caller to supply nil if this is not important.
	// We fill it with an optimistic default.
	if retryFactory == nil {
		retryFactory = func() retry.CallArgs {
			return retry.CallArgs{
				Clock:       clock.WallClock,
				Delay:       1 * time.Millisecond,
				MaxDuration: 1 * time.Millisecond,
			}
		}
	}

	modelConfigService := s.ControllerDomainServices(c).Config()
	ni, err := uniter.NewNetworkInfoForStrategy(context.Background(), s.ControllerModel(c).State(), s.networkService, modelConfigService, tag, retryFactory, lookupHost, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)
	return ni
}

func (s *networkInfoSuite) newProReqRelationWithBindings(
	c *gc.C, scope charm.RelationScope, pBindings, rBindings map[string]string,
) *ProReqRelation {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f = f.WithModelConfigService(s.modelConfigService(c))

	papp := f.MakeApplication(c, &factory.ApplicationParams{
		EndpointBindings: pBindings,
	})
	var rapp *state.Application
	if scope == charm.ScopeGlobal {
		rapp = f.MakeApplication(c, &factory.ApplicationParams{
			Name:             "wordpress",
			Charm:            f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
			EndpointBindings: rBindings,
		})
	} else {
		rapp = f.MakeApplication(c, &factory.ApplicationParams{
			Name:             "logging",
			Charm:            f.MakeCharm(c, &factory.CharmParams{Name: "logging"}),
			EndpointBindings: rBindings,
		})
	}
	return newProReqRelationForApps(c, s.ControllerModel(c).State(), s.modelConfigService(c), papp, rapp)
}

func (s *networkInfoSuite) newProReqRelation(c *gc.C, scope charm.RelationScope) *ProReqRelation {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f = f.WithModelConfigService(s.modelConfigService(c))

	pApp := f.MakeApplication(c, nil)

	var rApp *state.Application
	if scope == charm.ScopeGlobal {
		rApp = f.MakeApplication(c, &factory.ApplicationParams{
			Name:  "wordpress",
			Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
		})
	} else {
		rApp = f.MakeApplication(c, &factory.ApplicationParams{
			Name:  "wordpress",
			Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
		})
	}

	return newProReqRelationForApps(c, s.ControllerModel(c).State(), s.modelConfigService(c), pApp, rApp)
}

func (s *networkInfoSuite) newRemoteProReqRelation(c *gc.C) *RemoteProReqRelation {
	st := s.ControllerModel(c).State()
	papp, err := st.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql",
		SourceModel: coretesting.ModelTag,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "server",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}}})
	c.Assert(err, jc.ErrorIsNil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f = f.WithModelConfigService(s.modelConfigService(c))

	rapp := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})

	eps, err := st.InferEndpoints("mysql", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	prr := &RemoteProReqRelation{rel: rel, papp: papp, rapp: rapp}
	prr.pru0 = addRemoteRU(c, rel, "mysql/0")
	prr.pru1 = addRemoteRU(c, rel, "mysql/1")
	prr.ru0, prr.rru0 = addRU(c, s.modelConfigService(c), rapp, rel, nil)
	prr.ru1, prr.rru1 = addRU(c, s.modelConfigService(c), rapp, rel, nil)
	return prr
}

func newProReqRelationForApps(
	c *gc.C,
	st *state.State,
	modelConfigService uniter.ModelConfigService,
	proApp, reqApp *state.Application,
) *ProReqRelation {
	eps, err := st.InferEndpoints(proApp.Name(), reqApp.Name())
	c.Assert(err, jc.ErrorIsNil)
	rel, err := st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	prr := &ProReqRelation{rel: rel, papp: proApp, rapp: reqApp}
	prr.pu0, prr.pru0 = addRU(c, modelConfigService, proApp, rel, nil)
	prr.pu1, prr.pru1 = addRU(c, modelConfigService, proApp, rel, nil)
	if eps[0].Scope == charm.ScopeGlobal {
		prr.ru0, prr.rru0 = addRU(c, modelConfigService, reqApp, rel, nil)
		prr.ru1, prr.rru1 = addRU(c, modelConfigService, reqApp, rel, nil)
	} else {
		prr.ru0, prr.rru0 = addRU(c, modelConfigService, reqApp, rel, prr.pu0)
		prr.ru1, prr.rru1 = addRU(c, modelConfigService, reqApp, rel, prr.pu1)
	}
	return prr
}

func addRU(
	c *gc.C,
	modelConfigService uniter.ModelConfigService,
	app *state.Application,
	rel *state.Relation,
	principal *state.Unit,
) (*state.Unit, *state.RelationUnit) {
	// Given the application app in the relation rel, add a unit of app and create
	// a RelationUnit with rel. If principal is supplied, app is assumed to be
	// subordinate and the unit will be created by temporarily entering the
	// relation's scope as the principal.
	var u *state.Unit
	if principal == nil {
		unit, err := app.AddUnit(modelConfigService, state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		u = unit
	} else {
		origUnits, err := app.AllUnits()
		c.Assert(err, jc.ErrorIsNil)
		pru, err := rel.Unit(principal)
		c.Assert(err, jc.ErrorIsNil)
		err = pru.EnterScope(modelConfigService, nil) // to create the subordinate
		c.Assert(err, jc.ErrorIsNil)
		err = pru.LeaveScope() // to reset to initial expected state
		c.Assert(err, jc.ErrorIsNil)
		newUnits, err := app.AllUnits()
		c.Assert(err, jc.ErrorIsNil)
		for _, unit := range newUnits {
			found := false
			for _, old := range origUnits {
				if unit.Name() == old.Name() {
					found = true
					break
				}
			}
			if !found {
				u = unit
				break
			}
		}
		c.Assert(u, gc.NotNil)
	}
	ru, err := rel.Unit(u)
	c.Assert(err, jc.ErrorIsNil)
	return u, ru
}

func addRemoteRU(c *gc.C, rel *state.Relation, unitName string) *state.RelationUnit {
	// Add a remote unit with the given name to rel.
	ru, err := rel.RemoteUnit(unitName)
	c.Assert(err, jc.ErrorIsNil)
	return ru
}
