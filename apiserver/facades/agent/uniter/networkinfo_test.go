// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/clock"
	"github.com/juju/names/v4"
	"github.com/juju/retry"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/agent/uniter"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
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

type networkInfoSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&networkInfoSuite{})

func (s *networkInfoSuite) TestNetworksForRelation(c *gc.C) {
	prr := s.newProReqRelation(c, charm.ScopeGlobal)
	err := prr.pu0.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	id, err := prr.pu0.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetProviderAddresses(
		network.NewScopedSpaceAddress("1.2.3.4", network.ScopeCloudLocal),
		network.NewScopedSpaceAddress("4.3.2.1", network.ScopePublic),
	)
	c.Assert(err, jc.ErrorIsNil)

	boundSpace, ingress, egress, err := s.newNetworkInfo(c, prr.pu0.UnitTag()).NetworksForRelation("", prr.rel, true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, network.AlphaSpaceId)
	c.Assert(ingress, gc.DeepEquals,
		network.SpaceAddresses{network.NewScopedSpaceAddress("1.2.3.4", network.ScopeCloudLocal)})
	c.Assert(egress, gc.DeepEquals, []string{"1.2.3.4/32"})
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
			ConfigMethod: network.StaticAddress,
			CIDRAddress:  address,
		}
		err = machine.SetDevicesAddresses(addressesArg)
		c.Assert(err, jc.ErrorIsNil)
		deviceAddresses, err := device.Addresses()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(deviceAddresses, gc.HasLen, 1)
	}
}

func (s *networkInfoSuite) TestNetworksForRelationWithSpaces(c *gc.C) {
	subnet1, err := s.State.AddSubnet(network.SubnetInfo{CIDR: "1.2.0.0/16"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("space-1", "pid-1", []string{subnet1.ID()}, false)
	c.Assert(err, jc.ErrorIsNil)

	subnet2, err := s.State.AddSubnet(network.SubnetInfo{CIDR: "2.2.0.0/16"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("space-2", "pid-2", []string{subnet2.ID()}, false)
	c.Assert(err, jc.ErrorIsNil)

	subnet3, err := s.State.AddSubnet(network.SubnetInfo{CIDR: "3.2.0.0/16"})
	c.Assert(err, jc.ErrorIsNil)
	space3, err := s.State.AddSpace("space-3", "pid-3", []string{subnet3.ID()}, false)
	c.Assert(err, jc.ErrorIsNil)

	subnet4, err := s.State.AddSubnet(network.SubnetInfo{CIDR: "4.3.0.0/16"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("public-4", "pid-4", []string{subnet4.ID()}, true)
	c.Assert(err, jc.ErrorIsNil)

	// We want to have all bindings set so that no actual binding is
	// really set to the default.
	bindings := map[string]string{
		"":             "space-3",
		"server-admin": "space-1",
		"server":       "space-2",
	}

	prr := s.newProReqRelationWithBindings(c, charm.ScopeGlobal, bindings, nil)
	err = prr.pu0.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	id, err := prr.pu0.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	addresses := []network.SpaceAddress{
		network.NewScopedSpaceAddress("1.2.3.4", network.ScopeCloudLocal),
		network.NewScopedSpaceAddress("2.2.3.4", network.ScopeCloudLocal),
		network.NewScopedSpaceAddress("3.2.3.4", network.ScopeCloudLocal),
		network.NewScopedSpaceAddress("4.3.2.1", network.ScopePublic),
	}
	err = machine.SetProviderAddresses(addresses...)
	c.Assert(err, jc.ErrorIsNil)

	s.addDevicesWithAddresses(c, machine, "1.2.3.4/16", "2.2.3.4/16", "3.2.3.4/16", "4.3.2.1/16")

	boundSpace, ingress, egress, err := s.newNetworkInfo(c, prr.pu0.UnitTag()).NetworksForRelation("", prr.rel, true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, space3.Id())
	c.Assert(ingress, gc.DeepEquals,
		network.SpaceAddresses{network.NewScopedSpaceAddress("3.2.3.4", network.ScopeCloudLocal)})
	c.Assert(egress, gc.DeepEquals, []string{"3.2.3.4/32"})
}

func (s *networkInfoSuite) TestNetworksForRelationRemoteRelation(c *gc.C) {
	prr := s.newRemoteProReqRelation(c)
	err := prr.ru0.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	id, err := prr.ru0.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetProviderAddresses(
		network.NewScopedSpaceAddress("1.2.3.4", network.ScopeCloudLocal),
		network.NewScopedSpaceAddress("4.3.2.1", network.ScopePublic),
	)
	c.Assert(err, jc.ErrorIsNil)

	boundSpace, ingress, egress, err := s.newNetworkInfo(c, prr.ru0.UnitTag()).NetworksForRelation("", prr.rel, true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, network.AlphaSpaceId)
	c.Assert(ingress, gc.DeepEquals,
		network.SpaceAddresses{network.NewScopedSpaceAddress("4.3.2.1", network.ScopePublic)})
	c.Assert(egress, gc.DeepEquals, []string{"4.3.2.1/32"})
}

func (s *networkInfoSuite) TestNetworksForRelationRemoteRelationNoPublicAddr(c *gc.C) {
	prr := s.newRemoteProReqRelation(c)
	err := prr.ru0.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	id, err := prr.ru0.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetProviderAddresses(
		network.NewScopedSpaceAddress("1.2.3.4", network.ScopeCloudLocal),
	)
	c.Assert(err, jc.ErrorIsNil)

	boundSpace, ingress, egress, err := s.newNetworkInfo(c, prr.ru0.UnitTag()).NetworksForRelation("", prr.rel, true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, network.AlphaSpaceId)
	c.Assert(ingress, gc.DeepEquals,
		network.SpaceAddresses{network.NewScopedSpaceAddress("1.2.3.4", network.ScopeCloudLocal)})
	c.Assert(egress, gc.DeepEquals, []string{"1.2.3.4/32"})
}

func (s *networkInfoSuite) TestNetworksForRelationRemoteRelationDelayedPublicAddress(c *gc.C) {
	prr := s.newRemoteProReqRelation(c)
	err := prr.ru0.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	id, err := prr.ru0.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	s.PatchValue(&uniter.PreferredAddressRetryArgs, func() retry.CallArgs {
		return retry.CallArgs{
			Clock:       clock.WallClock,
			Delay:       1 * time.Millisecond,
			MaxDuration: coretesting.LongWait,
			NotifyFunc: func(lastError error, attempt int) {
				// Set the address after one failed retrieval attempt.
				if attempt == 1 {
					err := machine.SetProviderAddresses(network.NewScopedSpaceAddress("4.3.2.1", network.ScopePublic))
					c.Assert(err, jc.ErrorIsNil)
				}
			},
		}
	})

	boundSpace, ingress, egress, err := s.newNetworkInfo(c, prr.ru0.UnitTag()).NetworksForRelation("", prr.rel, true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, network.AlphaSpaceId)
	c.Assert(ingress, gc.DeepEquals,
		network.SpaceAddresses{network.NewScopedSpaceAddress("4.3.2.1", network.ScopePublic)})
	c.Assert(egress, gc.DeepEquals, []string{"4.3.2.1/32"})
}

func (s *networkInfoSuite) TestNetworksForRelationCAASModel(c *gc.C) {
	st := s.Factory.MakeCAASModel(c, nil)
	defer func() { _ = st.Close() }()

	f := factory.NewFactory(st, s.StatePool)
	gitlabch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	mysqlch := f.MakeCharm(c, &factory.CharmParams{Name: "mysql", Series: "kubernetes"})
	gitlab := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: gitlabch})
	mysql := f.MakeApplication(c, &factory.ApplicationParams{Name: "mysql", Charm: mysqlch})

	prr := newProReqRelationForApps(c, st, mysql, gitlab)

	// We need to instantiate this with the new CAAS model state.
	netInfo, err := uniter.NewNetworkInfo(st, prr.pu0.UnitTag())
	c.Assert(err, jc.ErrorIsNil)

	// First no address.
	boundSpace, ingress, egress, err := netInfo.NetworksForRelation("", prr.rel, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(boundSpace, gc.Equals, network.AlphaSpaceId)
	c.Assert(ingress, gc.HasLen, 0)
	c.Assert(egress, gc.HasLen, 0)

	// Add a application address.
	err = mysql.UpdateCloudService("", network.SpaceAddresses{
		network.NewScopedSpaceAddress("1.2.3.4", network.ScopeCloudLocal),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = prr.pu0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	boundSpace, ingress, egress, err = netInfo.NetworksForRelation("", prr.rel, true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, network.AlphaSpaceId)
	c.Assert(ingress, gc.DeepEquals,
		network.SpaceAddresses{network.NewScopedSpaceAddress("1.2.3.4", network.ScopeCloudLocal)})
	c.Assert(egress, gc.DeepEquals, []string{"1.2.3.4/32"})
}

func (s *networkInfoSuite) newNetworkInfo(c *gc.C, tag names.UnitTag) *uniter.NetworkInfo {
	ni, err := uniter.NewNetworkInfo(s.State, tag)
	c.Assert(err, jc.ErrorIsNil)
	return ni
}

func (s *networkInfoSuite) newProReqRelationWithBindings(
	c *gc.C, scope charm.RelationScope, pbindings, rbindings map[string]string,
) *ProReqRelation {
	papp := s.AddTestingApplicationWithBindings(c, "mysql", s.AddTestingCharm(c, "mysql"), pbindings)
	var rapp *state.Application
	if scope == charm.ScopeGlobal {
		rapp = s.AddTestingApplicationWithBindings(c, "wordpress", s.AddTestingCharm(c, "wordpress"), rbindings)
	} else {
		rapp = s.AddTestingApplicationWithBindings(c, "logging", s.AddTestingCharm(c, "logging"), rbindings)
	}
	return newProReqRelationForApps(c, s.State, papp, rapp)
}

func (s *networkInfoSuite) newProReqRelation(c *gc.C, scope charm.RelationScope) *ProReqRelation {
	papp := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	var rapp *state.Application
	if scope == charm.ScopeGlobal {
		rapp = s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	} else {
		rapp = s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	}
	return newProReqRelationForApps(c, s.State, papp, rapp)
}

func (s *networkInfoSuite) newRemoteProReqRelation(c *gc.C) *RemoteProReqRelation {
	papp, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql",
		SourceModel: coretesting.ModelTag,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "server",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	rapp := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	prr := &RemoteProReqRelation{rel: rel, papp: papp, rapp: rapp}
	prr.pru0 = addRemoteRU(c, rel, "mysql/0")
	prr.pru1 = addRemoteRU(c, rel, "mysql/1")
	prr.ru0, prr.rru0 = addRU(c, rapp, rel, nil)
	prr.ru1, prr.rru1 = addRU(c, rapp, rel, nil)
	return prr
}

func newProReqRelationForApps(c *gc.C, st *state.State, proApp, reqApp *state.Application) *ProReqRelation {
	eps, err := st.InferEndpoints(proApp.Name(), reqApp.Name())
	c.Assert(err, jc.ErrorIsNil)
	rel, err := st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	prr := &ProReqRelation{rel: rel, papp: proApp, rapp: reqApp}
	prr.pu0, prr.pru0 = addRU(c, proApp, rel, nil)
	prr.pu1, prr.pru1 = addRU(c, proApp, rel, nil)
	if eps[0].Scope == charm.ScopeGlobal {
		prr.ru0, prr.rru0 = addRU(c, reqApp, rel, nil)
		prr.ru1, prr.rru1 = addRU(c, reqApp, rel, nil)
	} else {
		prr.ru0, prr.rru0 = addRU(c, reqApp, rel, prr.pu0)
		prr.ru1, prr.rru1 = addRU(c, reqApp, rel, prr.pu1)
	}
	return prr
}

func addRU(
	c *gc.C, app *state.Application, rel *state.Relation, principal *state.Unit,
) (*state.Unit, *state.RelationUnit) {
	// Given the application app in the relation rel, add a unit of app and create
	// a RelationUnit with rel. If principal is supplied, app is assumed to be
	// subordinate and the unit will be created by temporarily entering the
	// relation's scope as the principal.
	var u *state.Unit
	if principal == nil {
		unit, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		u = unit
	} else {
		origUnits, err := app.AllUnits()
		c.Assert(err, jc.ErrorIsNil)
		pru, err := rel.Unit(principal)
		c.Assert(err, jc.ErrorIsNil)
		err = pru.EnterScope(nil) // to create the subordinate
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
