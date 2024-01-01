// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

const allEndpoints = ""

type machineSuite struct {
	firewallerSuite

	apiMachine *firewaller.Machine
}

var _ = gc.Suite(&machineSuite{})

func (s *machineSuite) SetUpTest(c *gc.C) {
	s.firewallerSuite.SetUpTest(c)

	var err error
	s.apiMachine, err = s.firewaller.Machine(s.machines[0].Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *machineSuite) TearDownTest(c *gc.C) {
	s.firewallerSuite.TearDownTest(c)
}

func (s *machineSuite) TestMachine(c *gc.C) {
	apiMachine42, err := s.firewaller.Machine(names.NewMachineTag("42"))
	c.Assert(err, gc.ErrorMatches, "machine 42 not found")
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
	c.Assert(apiMachine42, gc.IsNil)

	apiMachine0, err := s.firewaller.Machine(s.machines[0].Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiMachine0, gc.NotNil)
}

func (s *machineSuite) TestTag(c *gc.C) {
	c.Assert(s.apiMachine.Tag(), gc.Equals, names.NewMachineTag(s.machines[0].Id()))
}

func (s *machineSuite) TestInstanceId(c *gc.C) {
	// Add another, not provisioned machine to test
	// CodeNotProvisioned.
	newMachine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	apiNewMachine, err := s.firewaller.Machine(newMachine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	_, err = apiNewMachine.InstanceId()
	c.Assert(err, gc.ErrorMatches, "machine 3 not provisioned")
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)

	instanceId, err := s.apiMachine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceId, gc.Equals, instance.Id("i-manager"))
}

func (s *machineSuite) TestWatchUnits(c *gc.C) {
	w, err := s.apiMachine.WatchUnits()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange("wordpress/0")
	wc.AssertNoChange()

	// Change something other than the life cycle and make sure it's
	// not detected.
	err = s.machines[0].SetPassword("foo")
	c.Assert(err, gc.ErrorMatches, "password is only 3 bytes long, and is not a valid Agent password")
	wc.AssertNoChange()

	err = s.machines[0].SetPassword("foo-12345678901234567890")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Unassign unit 0 from the machine and check it's detected.
	err = s.units[0].UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("wordpress/0")
	wc.AssertNoChange()
}

func (s *machineSuite) TestIsManual(c *gc.C) {
	answer, err := s.machines[0].IsManual()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(answer, jc.IsFalse)

	m, err := s.State.AddOneMachine(state.MachineTemplate{
		Base:       state.UbuntuBase("12.10"),
		Jobs:       []state.MachineJob{state.JobHostUnits},
		InstanceId: "2",
		Nonce:      "manual:",
	})
	c.Assert(err, jc.ErrorIsNil)
	answer, err = m.IsManual()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(answer, jc.IsTrue)

}

func mustOpenPortRanges(c *gc.C, st *state.State, u *state.Unit, endpointName string, portRanges []network.PortRange) {
	unitPortRanges, err := u.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)

	for _, pr := range portRanges {
		unitPortRanges.Open(endpointName, pr)
	}

	c.Assert(st.ApplyOperation(unitPortRanges.Changes()), jc.ErrorIsNil)
}

func mustClosePortRanges(c *gc.C, st *state.State, u *state.Unit, endpointName string, portRanges []network.PortRange) {
	unitPortRanges, err := u.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)

	for _, pr := range portRanges {
		unitPortRanges.Close(endpointName, pr)
	}

	c.Assert(st.ApplyOperation(unitPortRanges.Changes()), jc.ErrorIsNil)
}

func (s *machineSuite) TestOpenedPortRanges(c *gc.C) {
	mockResponse := params.OpenMachinePortRangesResults{
		Results: []params.OpenMachinePortRangesResult{
			{
				UnitPortRanges: map[string][]params.OpenUnitPortRanges{
					"unit-mysql-0": {
						{
							Endpoint:    "server",
							SubnetCIDRs: []string{"192.168.0.0/24", "192.168.1.0/24"},
							PortRanges: []params.PortRange{
								params.FromNetworkPortRange(network.MustParsePortRange("3306/tcp")),
							},
						},
					},
					"unit-wordpress-0": {
						{
							Endpoint:    "website",
							SubnetCIDRs: []string{"192.168.0.0/24", "192.168.1.0/24"},
							PortRanges: []params.PortRange{
								params.FromNetworkPortRange(network.MustParsePortRange("80/tcp")),
							},
						},
						{
							Endpoint:    "metrics",
							SubnetCIDRs: []string{"10.0.0.0/24", "10.0.1.0/24", "192.168.0.0/24", "192.168.1.0/24"},
							PortRanges: []params.PortRange{
								params.FromNetworkPortRange(network.MustParsePortRange("1337/tcp")),
							},
						},
					},
				},
			},
		},
	}

	apiCaller := basetesting.BestVersionCaller{
		BestVersion: 6, // we need V6+ to use this API
		APICallerFunc: func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Assert(objType, gc.Equals, "Firewaller")

			// When we access the machine, the client checks that it's alive
			if request == "Life" {
				c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
				*(result.(*params.LifeResults)) = params.LifeResults{
					Results: []params.LifeResult{
						{Life: life.Alive},
					},
				}
				return nil
			}

			// This is the actual call we are testing.
			c.Assert(request, gc.Equals, "OpenedMachinePortRanges")
			c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: s.machines[0].MachineTag().String()}}})
			c.Assert(result, gc.FitsTypeOf, &params.OpenMachinePortRangesResults{})
			*(result.(*params.OpenMachinePortRangesResults)) = mockResponse
			return nil
		},
	}

	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)

	mach, err := client.Machine(s.machines[0].MachineTag())
	c.Assert(err, jc.ErrorIsNil)

	byUnitAndCIDR, byUnitAndEndpoint, err := mach.OpenedMachinePortRanges()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(byUnitAndCIDR, jc.DeepEquals, map[names.UnitTag]network.GroupedPortRanges{
		names.NewUnitTag("mysql/0"): {
			"192.168.0.0/24": []network.PortRange{
				network.MustParsePortRange("3306/tcp"),
			},
			"192.168.1.0/24": []network.PortRange{
				network.MustParsePortRange("3306/tcp"),
			},
		},
		names.NewUnitTag("wordpress/0"): {
			"10.0.0.0/24": []network.PortRange{
				network.MustParsePortRange("1337/tcp"),
			},
			"10.0.1.0/24": []network.PortRange{
				network.MustParsePortRange("1337/tcp"),
			},
			"192.168.0.0/24": []network.PortRange{
				network.MustParsePortRange("80/tcp"),
				network.MustParsePortRange("1337/tcp"),
			},
			"192.168.1.0/24": []network.PortRange{
				network.MustParsePortRange("80/tcp"),
				network.MustParsePortRange("1337/tcp"),
			},
		},
	})

	c.Assert(byUnitAndEndpoint, jc.DeepEquals, map[names.UnitTag]network.GroupedPortRanges{
		names.NewUnitTag("mysql/0"): {
			"server": []network.PortRange{
				network.MustParsePortRange("3306/tcp"),
			},
		},
		names.NewUnitTag("wordpress/0"): {
			"website": []network.PortRange{
				network.MustParsePortRange("80/tcp"),
			},
			"metrics": []network.PortRange{
				network.MustParsePortRange("1337/tcp"),
			},
		},
	})
}
