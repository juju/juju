// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"strconv"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

const (
	contentionErr = ".*: state changing too quickly; try again soon"
)

type UnitSuite struct {
	ConnSuite
	charm   *state.Charm
	service *state.Service
	unit    *state.Unit
}

var _ = gc.Suite(&UnitSuite{})

func (s *UnitSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "wordpress")
	var err error
	s.service = s.AddTestingService(c, "wordpress", s.charm)
	c.Assert(err, jc.ErrorIsNil)
	s.unit, err = s.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.unit.Series(), gc.Equals, "quantal")
}

func (s *UnitSuite) TestUnitNotFound(c *gc.C) {
	_, err := s.State.Unit("subway/0")
	c.Assert(err, gc.ErrorMatches, `unit "subway/0" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *UnitSuite) TestService(c *gc.C) {
	svc, err := s.unit.Service()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(svc.Name(), gc.Equals, s.unit.ServiceName())
}

func (s *UnitSuite) TestConfigSettingsNeedCharmURLSet(c *gc.C) {
	_, err := s.unit.ConfigSettings()
	c.Assert(err, gc.ErrorMatches, "unit charm not set")
}

func (s *UnitSuite) TestConfigSettingsIncludeDefaults(c *gc.C) {
	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, jc.ErrorIsNil)
	settings, err := s.unit.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})
}

func (s *UnitSuite) TestConfigSettingsReflectService(c *gc.C) {
	err := s.service.UpdateConfigSettings(charm.Settings{"blog-title": "no title"})
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, jc.ErrorIsNil)
	settings, err := s.unit.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "no title"})

	err = s.service.UpdateConfigSettings(charm.Settings{"blog-title": "ironic title"})
	c.Assert(err, jc.ErrorIsNil)
	settings, err = s.unit.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "ironic title"})
}

func (s *UnitSuite) TestConfigSettingsReflectCharm(c *gc.C) {
	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, jc.ErrorIsNil)
	newCharm := s.AddConfigCharm(c, "wordpress", "options: {}", 123)
	err = s.service.SetCharm(newCharm, false)
	c.Assert(err, jc.ErrorIsNil)

	// Settings still reflect charm set on unit.
	settings, err := s.unit.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})

	// When the unit has the new charm set, it'll see the new config.
	err = s.unit.SetCharmURL(newCharm.URL())
	c.Assert(err, jc.ErrorIsNil)
	settings, err = s.unit.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{})
}

func (s *UnitSuite) TestWatchConfigSettingsNeedsCharmURL(c *gc.C) {
	_, err := s.unit.WatchConfigSettings()
	c.Assert(err, gc.ErrorMatches, "unit charm not set")
}

func (s *UnitSuite) TestWatchConfigSettings(c *gc.C) {
	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, jc.ErrorIsNil)
	w, err := s.unit.WatchConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Update config a couple of times, check a single event.
	err = s.service.UpdateConfigSettings(charm.Settings{
		"blog-title": "superhero paparazzi",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.service.UpdateConfigSettings(charm.Settings{
		"blog-title": "sauceror central",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Non-change is not reported.
	err = s.service.UpdateConfigSettings(charm.Settings{
		"blog-title": "sauceror central",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Change service's charm; nothing detected.
	newCharm := s.AddConfigCharm(c, "wordpress", floatConfig, 123)
	err = s.service.SetCharm(newCharm, false)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Change service config for new charm; nothing detected.
	err = s.service.UpdateConfigSettings(charm.Settings{
		"key": 42.0,
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// NOTE: if we were to change the unit to use the new charm, we'd see
	// another event, because the originally-watched document will become
	// unreferenced and be removed. But I'm not testing that behaviour
	// because it's not very helpful and subject to change.
}

func (s *UnitSuite) addSubordinateUnit(c *gc.C) *state.Unit {
	subCharm := s.AddTestingCharm(c, "logging")
	s.AddTestingService(c, "logging", subCharm)
	eps, err := s.State.InferEndpoints("wordpress", "logging")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	subUnit, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)
	return subUnit
}

func (s *UnitSuite) setAssignedMachineAddresses(c *gc.C, u *state.Unit) {
	err := u.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	mid, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(mid)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("i-exist", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProviderAddresses(network.Address{
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
		Value: "private.address.example.com",
	}, network.Address{
		Type:  network.IPv4Address,
		Scope: network.ScopePublic,
		Value: "public.address.example.com",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestPublicAddressSubordinate(c *gc.C) {
	subUnit := s.addSubordinateUnit(c)
	_, ok := subUnit.PublicAddress()
	c.Assert(ok, jc.IsFalse)

	s.setAssignedMachineAddresses(c, s.unit)
	address, ok := subUnit.PublicAddress()
	c.Assert(ok, jc.IsTrue)
	c.Assert(address, gc.Equals, "public.address.example.com")
}

func (s *UnitSuite) TestPublicAddress(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	address, ok := s.unit.PublicAddress()
	c.Check(address, gc.Equals, "")
	c.Assert(ok, jc.IsFalse)

	public := network.NewScopedAddress("8.8.8.8", network.ScopePublic)
	private := network.NewScopedAddress("127.0.0.1", network.ScopeCloudLocal)

	err = machine.SetProviderAddresses(public, private)
	c.Assert(err, jc.ErrorIsNil)

	address, ok = s.unit.PublicAddress()
	c.Check(address, gc.Equals, "8.8.8.8")
	c.Assert(ok, jc.IsTrue)
}

func (s *UnitSuite) TestPublicAddressMachineAddresses(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	publicProvider := network.NewScopedAddress("8.8.8.8", network.ScopePublic)
	privateProvider := network.NewScopedAddress("127.0.0.1", network.ScopeCloudLocal)
	privateMachine := network.NewAddress("127.0.0.2")

	err = machine.SetProviderAddresses(privateProvider)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetMachineAddresses(privateMachine)
	c.Assert(err, jc.ErrorIsNil)
	address, ok := s.unit.PublicAddress()
	c.Check(address, gc.Equals, "127.0.0.1")
	c.Assert(ok, jc.IsTrue)

	err = machine.SetProviderAddresses(publicProvider, privateProvider)
	c.Assert(err, jc.ErrorIsNil)
	address, ok = s.unit.PublicAddress()
	c.Check(address, gc.Equals, "8.8.8.8")
	c.Assert(ok, jc.IsTrue)
}

func (s *UnitSuite) TestPrivateAddressSubordinate(c *gc.C) {
	subUnit := s.addSubordinateUnit(c)
	_, ok := subUnit.PrivateAddress()
	c.Assert(ok, jc.IsFalse)

	s.setAssignedMachineAddresses(c, s.unit)
	address, ok := subUnit.PrivateAddress()
	c.Assert(ok, jc.IsTrue)
	c.Assert(address, gc.Equals, "private.address.example.com")
}

func (s *UnitSuite) TestPrivateAddress(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	address, ok := s.unit.PrivateAddress()
	c.Check(address, gc.Equals, "")
	c.Assert(ok, jc.IsFalse)

	public := network.NewScopedAddress("8.8.8.8", network.ScopePublic)
	private := network.NewScopedAddress("127.0.0.1", network.ScopeCloudLocal)

	err = machine.SetProviderAddresses(public, private)
	c.Assert(err, jc.ErrorIsNil)

	address, ok = s.unit.PrivateAddress()
	c.Check(address, gc.Equals, "127.0.0.1")
	c.Assert(ok, jc.IsTrue)
}

type destroyMachineTestCase struct {
	target    *state.Unit
	host      *state.Machine
	desc      string
	flipHook  []jujutxn.TestHook
	destroyed bool
}

func (s *UnitSuite) destroyMachineTestCases(c *gc.C) []destroyMachineTestCase {
	var result []destroyMachineTestCase
	var err error

	{
		tc := destroyMachineTestCase{desc: "standalone principal", destroyed: true}
		tc.host, err = s.State.AddMachine("quantal", state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		tc.target, err = s.service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(tc.target.AssignToMachine(tc.host), gc.IsNil)
		result = append(result, tc)
	}
	{
		tc := destroyMachineTestCase{desc: "co-located principals", destroyed: false}
		tc.host, err = s.State.AddMachine("quantal", state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		tc.target, err = s.service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(tc.target.AssignToMachine(tc.host), gc.IsNil)
		colocated, err := s.service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(colocated.AssignToMachine(tc.host), gc.IsNil)

		result = append(result, tc)
	}
	{
		tc := destroyMachineTestCase{desc: "host has container", destroyed: false}
		tc.host, err = s.State.AddMachine("quantal", state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		_, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
			Series: "quantal",
			Jobs:   []state.MachineJob{state.JobHostUnits},
		}, tc.host.Id(), instance.LXC)
		c.Assert(err, jc.ErrorIsNil)
		tc.target, err = s.service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(tc.target.AssignToMachine(tc.host), gc.IsNil)

		result = append(result, tc)
	}
	{
		tc := destroyMachineTestCase{desc: "host has vote", destroyed: false}
		tc.host, err = s.State.AddMachine("quantal", state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(tc.host.SetHasVote(true), gc.IsNil)
		tc.target, err = s.service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(tc.target.AssignToMachine(tc.host), gc.IsNil)

		result = append(result, tc)
	}
	{
		tc := destroyMachineTestCase{desc: "unassigned unit", destroyed: true}
		tc.host, err = s.State.AddMachine("quantal", state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		tc.target, err = s.service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(tc.target.AssignToMachine(tc.host), gc.IsNil)
		result = append(result, tc)
	}

	return result
}

func (s *UnitSuite) TestRemoveUnitMachineFastForwardDestroy(c *gc.C) {
	for _, tc := range s.destroyMachineTestCases(c) {
		c.Log(tc.desc)
		c.Assert(tc.target.Destroy(), gc.IsNil)
		if tc.destroyed {
			assertLife(c, tc.host, state.Dying)
			c.Assert(tc.host.EnsureDead(), gc.IsNil)
		} else {
			assertLife(c, tc.host, state.Alive)
			c.Assert(tc.host.Destroy(), gc.NotNil)
		}
	}
}

func (s *UnitSuite) TestRemoveUnitMachineNoFastForwardDestroy(c *gc.C) {
	for _, tc := range s.destroyMachineTestCases(c) {
		c.Log(tc.desc)
		preventUnitDestroyRemove(c, tc.target)
		c.Assert(tc.target.Destroy(), gc.IsNil)
		c.Assert(tc.target.EnsureDead(), gc.IsNil)
		assertLife(c, tc.host, state.Alive)
		c.Assert(tc.target.Remove(), gc.IsNil)
		if tc.destroyed {
			assertLife(c, tc.host, state.Dying)
		} else {
			assertLife(c, tc.host, state.Alive)
			c.Assert(tc.host.Destroy(), gc.NotNil)
		}
	}
}

func (s *UnitSuite) setMachineVote(c *gc.C, id string, hasVote bool) {
	m, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.SetHasVote(hasVote), gc.IsNil)
}

func (s *UnitSuite) TestRemoveUnitMachineThrashed(c *gc.C) {
	host, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	target, err := s.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.AssignToMachine(host), gc.IsNil)
	flip := jujutxn.TestHook{
		Before: func() {
			s.setMachineVote(c, host.Id(), true)
		},
	}
	flop := jujutxn.TestHook{
		Before: func() {
			s.setMachineVote(c, host.Id(), false)
		},
	}
	// You'll need to adjust the flip-flops to match the number of transaction
	// retries.
	defer state.SetTestHooks(c, s.State, flip, flop, flip).Check()

	c.Assert(target.Destroy(), gc.ErrorMatches, "state changing too quickly; try again soon")
}

func (s *UnitSuite) TestRemoveUnitMachineRetryVoter(c *gc.C) {
	host, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	target, err := s.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.AssignToMachine(host), gc.IsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		s.setMachineVote(c, host.Id(), true)
	}, nil).Check()

	c.Assert(target.Destroy(), gc.IsNil)
	assertLife(c, host, state.Alive)
}

func (s *UnitSuite) TestRemoveUnitMachineRetryNoVoter(c *gc.C) {
	host, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	target, err := s.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.AssignToMachine(host), gc.IsNil)
	c.Assert(host.SetHasVote(true), gc.IsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		s.setMachineVote(c, host.Id(), false)
	}, nil).Check()

	c.Assert(target.Destroy(), gc.IsNil)
	assertLife(c, host, state.Dying)
}

func (s *UnitSuite) TestRemoveUnitMachineRetryContainer(c *gc.C) {
	host, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	target, err := s.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.AssignToMachine(host), gc.IsNil)
	defer state.SetTestHooks(c, s.State, jujutxn.TestHook{
		Before: func() {
			machine, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
				Series: "quantal",
				Jobs:   []state.MachineJob{state.JobHostUnits},
			}, host.Id(), instance.LXC)
			c.Assert(err, jc.ErrorIsNil)
			assertLife(c, machine, state.Alive)

			// test-setup verification for the disqualifying machine.
			hostHandle, err := s.State.Machine(host.Id())
			c.Assert(err, jc.ErrorIsNil)
			containers, err := hostHandle.Containers()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(containers, gc.HasLen, 1)
		},
	}).Check()

	c.Assert(target.Destroy(), gc.IsNil)
	assertLife(c, host, state.Alive)
}

func (s *UnitSuite) TestRemoveUnitMachineRetryOrCond(c *gc.C) {
	host, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	target, err := s.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.AssignToMachine(host), gc.IsNil)

	// This unit will be colocated in the transaction hook to cause a retry.
	colocated, err := s.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(host.SetHasVote(true), gc.IsNil)

	defer state.SetTestHooks(c, s.State, jujutxn.TestHook{
		Before: func() {
			hostHandle, err := s.State.Machine(host.Id())
			c.Assert(err, jc.ErrorIsNil)

			// Original assertion preventing host removal is no longer valid
			c.Assert(hostHandle.SetHasVote(false), gc.IsNil)

			// But now the host gets a colocated unit, a different condition preventing removal
			c.Assert(colocated.AssignToMachine(hostHandle), gc.IsNil)
		},
	}).Check()

	c.Assert(target.Destroy(), gc.IsNil)
	assertLife(c, host, state.Alive)
}

func (s *UnitSuite) TestRefresh(c *gc.C) {
	unit1, err := s.State.Unit(s.unit.Name())
	c.Assert(err, jc.ErrorIsNil)

	err = s.unit.SetPassword("arble-farble-dying-yarble")
	c.Assert(err, jc.ErrorIsNil)
	valid := unit1.PasswordValid("arble-farble-dying-yarble")
	c.Assert(valid, jc.IsFalse)
	err = unit1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	valid = unit1.PasswordValid("arble-farble-dying-yarble")
	c.Assert(valid, jc.IsTrue)

	err = unit1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit1.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = unit1.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *UnitSuite) TestSetCharmURLSuccess(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	curl, ok := s.unit.CharmURL()
	c.Assert(ok, jc.IsFalse)
	c.Assert(curl, gc.IsNil)

	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, jc.ErrorIsNil)

	curl, ok = s.unit.CharmURL()
	c.Assert(ok, jc.IsTrue)
	c.Assert(curl, gc.DeepEquals, s.charm.URL())
}

func (s *UnitSuite) TestSetCharmURLFailures(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	curl, ok := s.unit.CharmURL()
	c.Assert(ok, jc.IsFalse)
	c.Assert(curl, gc.IsNil)

	err := s.unit.SetCharmURL(nil)
	c.Assert(err, gc.ErrorMatches, "cannot set nil charm url")

	err = s.unit.SetCharmURL(charm.MustParseURL("cs:missing/one-1"))
	c.Assert(err, gc.ErrorMatches, `unknown charm url "cs:missing/one-1"`)

	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, gc.Equals, state.ErrDead)
}

func (s *UnitSuite) TestSetCharmURLWithRemovedUnit(c *gc.C) {
	err := s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.unit)

	err = s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, gc.Equals, state.ErrDead)
}

func (s *UnitSuite) TestSetCharmURLWithDyingUnit(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	err := s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.unit, state.Dying)

	err = s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, jc.ErrorIsNil)

	curl, ok := s.unit.CharmURL()
	c.Assert(ok, jc.IsTrue)
	c.Assert(curl, gc.DeepEquals, s.charm.URL())
}

func (s *UnitSuite) TestSetCharmURLRetriesWithDeadUnit(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.unit.Destroy()
		c.Assert(err, jc.ErrorIsNil)
		err = s.unit.EnsureDead()
		c.Assert(err, jc.ErrorIsNil)
		assertLife(c, s.unit, state.Dead)
	}).Check()

	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, gc.Equals, state.ErrDead)
}

func (s *UnitSuite) TestSetCharmURLRetriesWithDifferentURL(c *gc.C) {
	sch := s.AddConfigCharm(c, "wordpress", emptyConfig, 2)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				// Set a different charm to force a retry: first on
				// the service, so the settings are created, then on
				// the unit.
				err := s.service.SetCharm(sch, false)
				c.Assert(err, jc.ErrorIsNil)
				err = s.unit.SetCharmURL(sch.URL())
				c.Assert(err, jc.ErrorIsNil)
			},
			After: func() {
				// Set back the same charm on the service, so the
				// settings refcount is correct..
				err := s.service.SetCharm(s.charm, false)
				c.Assert(err, jc.ErrorIsNil)
			},
		},
		jujutxn.TestHook{
			Before: nil, // Ensure there will be a retry.
			After: func() {
				// Verify it worked after the second attempt.
				err := s.unit.Refresh()
				c.Assert(err, jc.ErrorIsNil)
				currentURL, hasURL := s.unit.CharmURL()
				c.Assert(currentURL, jc.DeepEquals, s.charm.URL())
				c.Assert(hasURL, jc.IsTrue)
			},
		},
	).Check()

	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestDestroySetStatusRetry(c *gc.C) {
	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.SetAgentStatus(state.StatusIdle, "", nil)
		c.Assert(err, jc.ErrorIsNil)
	}, func() {
		assertLife(c, s.unit, state.Dying)
	}).Check()

	err := s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestDestroySetCharmRetry(c *gc.C) {
	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.SetCharmURL(s.charm.URL())
		c.Assert(err, jc.ErrorIsNil)
	}, func() {
		assertRemoved(c, s.unit)
	}).Check()

	err := s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestDestroyChangeCharmRetry(c *gc.C) {
	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, jc.ErrorIsNil)
	newCharm := s.AddConfigCharm(c, "mysql", "options: {}", 99)
	err = s.service.SetCharm(newCharm, false)
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.SetCharmURL(newCharm.URL())
		c.Assert(err, jc.ErrorIsNil)
	}, func() {
		assertRemoved(c, s.unit)
	}).Check()

	err = s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestDestroyAssignRetry(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.AssignToMachine(machine)
		c.Assert(err, jc.ErrorIsNil)
	}, func() {
		assertRemoved(c, s.unit)
		// Also check the unit ref was properly removed from the machine doc --
		// if it weren't, we wouldn't be able to make the machine Dead.
		err := machine.EnsureDead()
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err = s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestDestroyUnassignRetry(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.UnassignFromMachine()
		c.Assert(err, jc.ErrorIsNil)
	}, func() {
		assertRemoved(c, s.unit)
	}).Check()

	err = s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestShortCircuitDestroyUnit(c *gc.C) {
	// A unit that has not set any status is removed directly.
	err := s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.unit.Life(), gc.Equals, state.Dying)
	assertRemoved(c, s.unit)
}

func (s *UnitSuite) TestCannotShortCircuitDestroyWithSubordinates(c *gc.C) {
	// A unit with subordinates is just set to Dying.
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.unit.Life(), gc.Equals, state.Dying)
	assertLife(c, s.unit, state.Dying)
}

func (s *UnitSuite) TestCannotShortCircuitDestroyWithAgentStatus(c *gc.C) {
	for i, test := range []struct {
		status state.Status
		info   string
	}{{
		state.StatusExecuting, "blah",
	}, {
		state.StatusIdle, "blah",
	}, {
		state.StatusFailed, "blah",
	}, {
		state.StatusRebooting, "blah",
	}} {
		c.Logf("test %d: %s", i, test.status)
		unit, err := s.service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = unit.SetAgentStatus(test.status, test.info, nil)
		c.Assert(err, jc.ErrorIsNil)
		err = unit.Destroy()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(unit.Life(), gc.Equals, state.Dying)
		assertLife(c, unit, state.Dying)
	}
}

func (s *UnitSuite) TestShortCircuitDestroyWithProvisionedMachine(c *gc.C) {
	// A unit assigned to a provisioned machine is still removed directly so
	// long as it has not set status.
	err := s.unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	mid, err := s.unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(mid)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("i-malive", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.unit.Life(), gc.Equals, state.Dying)
	assertRemoved(c, s.unit)
}

func assertLife(c *gc.C, entity state.Living, life state.Life) {
	c.Assert(entity.Refresh(), gc.IsNil)
	c.Assert(entity.Life(), gc.Equals, life)
}

func assertRemoved(c *gc.C, entity state.Living) {
	err := entity.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = entity.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	if entity, ok := entity.(state.AgentLiving); ok {
		err = entity.EnsureDead()
		c.Assert(err, jc.ErrorIsNil)
		err = entity.Remove()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *UnitSuite) TestTag(c *gc.C) {
	c.Assert(s.unit.Tag().String(), gc.Equals, "unit-wordpress-0")
}

func (s *UnitSuite) TestSetPassword(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.Unit(s.unit.Name())
	})
}

func (s *UnitSuite) TestSetAgentCompatPassword(c *gc.C) {
	e, err := s.State.Unit(s.unit.Name())
	c.Assert(err, jc.ErrorIsNil)
	testSetAgentCompatPassword(c, e)
}

func (s *UnitSuite) TestUnitSetAgentPresence(c *gc.C) {
	alive, err := s.unit.AgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, jc.IsFalse)

	pinger, err := s.unit.SetAgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pinger, gc.NotNil)
	defer pinger.Stop()

	s.State.StartSync()
	alive, err = s.unit.AgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, jc.IsTrue)
}

func (s *UnitSuite) TestUnitWaitAgentPresence(c *gc.C) {
	alive, err := s.unit.AgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, jc.IsFalse)

	err = s.unit.WaitAgentPresence(coretesting.ShortWait)
	c.Assert(err, gc.ErrorMatches, `waiting for agent of unit "wordpress/0": still not alive after timeout`)

	pinger, err := s.unit.SetAgentPresence()
	c.Assert(err, jc.ErrorIsNil)

	s.State.StartSync()
	err = s.unit.WaitAgentPresence(coretesting.LongWait)
	c.Assert(err, jc.ErrorIsNil)

	alive, err = s.unit.AgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, jc.IsTrue)

	err = pinger.Kill()
	c.Assert(err, jc.ErrorIsNil)

	s.State.StartSync()

	alive, err = s.unit.AgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, jc.IsFalse)
}

func (s *UnitSuite) TestResolve(c *gc.C) {
	err := s.unit.Resolve(false)
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" is not in an error state`)
	err = s.unit.Resolve(true)
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" is not in an error state`)

	err = s.unit.SetAgentStatus(state.StatusError, "gaaah", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Resolve(false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Resolve(true)
	c.Assert(err, gc.ErrorMatches, `cannot set resolved mode for unit "wordpress/0": already resolved`)
	c.Assert(s.unit.Resolved(), gc.Equals, state.ResolvedNoHooks)

	err = s.unit.ClearResolved()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Resolve(true)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Resolve(false)
	c.Assert(err, gc.ErrorMatches, `cannot set resolved mode for unit "wordpress/0": already resolved`)
	c.Assert(s.unit.Resolved(), gc.Equals, state.ResolvedRetryHooks)
}

func (s *UnitSuite) TestGetSetClearResolved(c *gc.C) {
	mode := s.unit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedNone)

	err := s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, gc.ErrorMatches, `cannot set resolved mode for unit "wordpress/0": already resolved`)

	mode = s.unit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedNoHooks)
	err = s.unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	mode = s.unit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedNoHooks)

	err = s.unit.ClearResolved()
	c.Assert(err, jc.ErrorIsNil)
	mode = s.unit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedNone)
	err = s.unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	mode = s.unit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedNone)
	err = s.unit.ClearResolved()
	c.Assert(err, jc.ErrorIsNil)

	err = s.unit.SetResolved(state.ResolvedNone)
	c.Assert(err, gc.ErrorMatches, `cannot set resolved mode for unit "wordpress/0": invalid error resolution mode: ""`)
	err = s.unit.SetResolved(state.ResolvedMode("foo"))
	c.Assert(err, gc.ErrorMatches, `cannot set resolved mode for unit "wordpress/0": invalid error resolution mode: "foo"`)
}

func (s *UnitSuite) TestOpenedPorts(c *gc.C) {
	// Verify ports can be opened and closed only when the unit has
	// assigned machine.
	err := s.unit.OpenPort("tcp", 10)
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotAssigned)
	err = s.unit.OpenPorts("tcp", 10, 20)
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotAssigned)
	err = s.unit.ClosePort("tcp", 10)
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotAssigned)
	err = s.unit.ClosePorts("tcp", 10, 20)
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotAssigned)
	open, err := s.unit.OpenedPorts()
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotAssigned)
	c.Assert(open, gc.HasLen, 0)

	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	// Verify no open ports before activity.
	open, err = s.unit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(open, gc.HasLen, 0)

	// Now open and close ports and ranges and check.

	err = s.unit.OpenPort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.OpenPorts("udp", 100, 200)
	c.Assert(err, jc.ErrorIsNil)
	open, err = s.unit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(open, gc.DeepEquals, []network.PortRange{
		{80, 80, "tcp"},
		{100, 200, "udp"},
	})

	err = s.unit.OpenPort("udp", 53)
	c.Assert(err, jc.ErrorIsNil)
	open, err = s.unit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(open, gc.DeepEquals, []network.PortRange{
		{80, 80, "tcp"},
		{53, 53, "udp"},
		{100, 200, "udp"},
	})

	err = s.unit.OpenPorts("tcp", 53, 55)
	c.Assert(err, jc.ErrorIsNil)
	open, err = s.unit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(open, gc.DeepEquals, []network.PortRange{
		{53, 55, "tcp"},
		{80, 80, "tcp"},
		{53, 53, "udp"},
		{100, 200, "udp"},
	})

	err = s.unit.OpenPort("tcp", 443)
	c.Assert(err, jc.ErrorIsNil)
	open, err = s.unit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(open, gc.DeepEquals, []network.PortRange{
		{53, 55, "tcp"},
		{80, 80, "tcp"},
		{443, 443, "tcp"},
		{53, 53, "udp"},
		{100, 200, "udp"},
	})

	err = s.unit.ClosePort("tcp", 80)
	c.Assert(err, jc.ErrorIsNil)
	open, err = s.unit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(open, gc.DeepEquals, []network.PortRange{
		{53, 55, "tcp"},
		{443, 443, "tcp"},
		{53, 53, "udp"},
		{100, 200, "udp"},
	})

	err = s.unit.ClosePorts("udp", 100, 200)
	c.Assert(err, jc.ErrorIsNil)
	open, err = s.unit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(open, gc.DeepEquals, []network.PortRange{
		{53, 55, "tcp"},
		{443, 443, "tcp"},
		{53, 53, "udp"},
	})
}

func (s *UnitSuite) TestOpenClosePortWhenDying(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	preventUnitDestroyRemove(c, s.unit)
	testWhenDying(c, s.unit, noErr, contentionErr, func() error {
		err := s.unit.OpenPort("tcp", 20)
		if err != nil {
			return err
		}
		err = s.unit.OpenPorts("tcp", 10, 15)
		if err != nil {
			return err
		}
		err = s.unit.Refresh()
		if err != nil {
			return err
		}
		err = s.unit.ClosePort("tcp", 20)
		if err != nil {
			return err
		}
		return s.unit.ClosePorts("tcp", 10, 15)
	})
}

func (s *UnitSuite) TestRemoveLastUnitOnMachineRemovesAllPorts(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	ports, err := machine.AllPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.HasLen, 0)

	err = s.unit.OpenPorts("tcp", 100, 200)
	c.Assert(err, jc.ErrorIsNil)

	ports, err = machine.AllPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.HasLen, 1)
	c.Assert(ports[0].PortsForUnit(s.unit.Name()), jc.DeepEquals, []state.PortRange{
		{s.unit.Name(), 100, 200, "tcp"},
	})

	// Now remove the unit and check again.
	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Because that was the only range open, the ports doc will be
	// removed as well.
	ports, err = machine.AllPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.HasLen, 0)
}

func (s *UnitSuite) TestRemoveUnitRemovesItsPortsOnly(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	otherUnit, err := s.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = otherUnit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	ports, err := machine.AllPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.HasLen, 0)

	err = s.unit.OpenPorts("tcp", 100, 200)
	c.Assert(err, jc.ErrorIsNil)
	err = otherUnit.OpenPorts("udp", 300, 400)
	c.Assert(err, jc.ErrorIsNil)

	ports, err = machine.AllPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.HasLen, 1)
	c.Assert(ports[0].PortsForUnit(s.unit.Name()), jc.DeepEquals, []state.PortRange{
		{s.unit.Name(), 100, 200, "tcp"},
	})
	c.Assert(ports[0].PortsForUnit(otherUnit.Name()), jc.DeepEquals, []state.PortRange{
		{otherUnit.Name(), 300, 400, "udp"},
	})

	// Now remove the first unit and check again.
	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Verify only otherUnit still has open ports.
	ports, err = machine.AllPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.HasLen, 1)
	c.Assert(ports[0].PortsForUnit(s.unit.Name()), gc.HasLen, 0)
	c.Assert(ports[0].PortsForUnit(otherUnit.Name()), jc.DeepEquals, []state.PortRange{
		{otherUnit.Name(), 300, 400, "udp"},
	})
}

func (s *UnitSuite) TestSetClearResolvedWhenNotAlive(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	err := s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.unit.Resolved(), gc.Equals, state.ResolvedNoHooks)
	err = s.unit.ClearResolved()
	c.Assert(err, jc.ErrorIsNil)

	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, gc.ErrorMatches, deadErr)
	err = s.unit.ClearResolved()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestSubordinateChangeInPrincipal(c *gc.C) {
	subCharm := s.AddTestingCharm(c, "logging")
	for i := 0; i < 2; i++ {
		// Note: subordinate units can only be created as a side effect of a
		// principal entering scope; and a given principal can only have a
		// single subordinate unit of each service.
		name := "logging" + strconv.Itoa(i)
		s.AddTestingService(c, name, subCharm)
		eps, err := s.State.InferEndpoints(name, "wordpress")
		c.Assert(err, jc.ErrorIsNil)
		rel, err := s.State.AddRelation(eps...)
		c.Assert(err, jc.ErrorIsNil)
		ru, err := rel.Unit(s.unit)
		c.Assert(err, jc.ErrorIsNil)
		err = ru.EnterScope(nil)
		c.Assert(err, jc.ErrorIsNil)
	}

	err := s.unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	subordinates := s.unit.SubordinateNames()
	c.Assert(subordinates, gc.DeepEquals, []string{"logging0/0", "logging1/0"})

	su1, err := s.State.Unit("logging1/0")
	c.Assert(err, jc.ErrorIsNil)
	err = su1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = su1.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	subordinates = s.unit.SubordinateNames()
	c.Assert(subordinates, gc.DeepEquals, []string{"logging0/0"})
}

func (s *UnitSuite) TestDeathWithSubordinates(c *gc.C) {
	// Check that units can become dead when they've never had subordinates.
	u, err := s.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// Create a new unit and add a subordinate.
	u, err = s.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, jc.ErrorIsNil)
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(u)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Check the unit cannot become Dead, but can become Dying...
	err = u.EnsureDead()
	c.Assert(err, gc.Equals, state.ErrUnitHasSubordinates)
	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// ...and that it still can't become Dead now it's Dying.
	err = u.EnsureDead()
	c.Assert(err, gc.Equals, state.ErrUnitHasSubordinates)

	// Make the subordinate Dead and check the principal still cannot be removed.
	sub, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)
	err = sub.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.EnsureDead()
	c.Assert(err, gc.Equals, state.ErrUnitHasSubordinates)

	// remove the subordinate and check the principal can finally become Dead.
	err = sub.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestPrincipalName(c *gc.C) {
	subCharm := s.AddTestingCharm(c, "logging")
	s.AddTestingService(c, "logging", subCharm)
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	subordinates := s.unit.SubordinateNames()
	c.Assert(subordinates, gc.DeepEquals, []string{"logging/0"})

	su, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)
	principal, valid := su.PrincipalName()
	c.Assert(valid, jc.IsTrue)
	c.Assert(principal, gc.Equals, s.unit.Name())

	// Calling PrincipalName on a principal unit yields "", false.
	principal, valid = s.unit.PrincipalName()
	c.Assert(valid, jc.IsFalse)
	c.Assert(principal, gc.Equals, "")
}

func (s *UnitSuite) TestRelations(c *gc.C) {
	wordpress0 := s.unit
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysql0, err := mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	assertEquals := func(actual, expect []*state.Relation) {
		c.Assert(actual, gc.HasLen, len(expect))
		for i, a := range actual {
			c.Assert(a.Id(), gc.Equals, expect[i].Id())
		}
	}
	assertRelationsJoined := func(unit *state.Unit, expect ...*state.Relation) {
		actual, err := unit.RelationsJoined()
		c.Assert(err, jc.ErrorIsNil)
		assertEquals(actual, expect)
	}
	assertRelationsInScope := func(unit *state.Unit, expect ...*state.Relation) {
		actual, err := unit.RelationsInScope()
		c.Assert(err, jc.ErrorIsNil)
		assertEquals(actual, expect)
	}
	assertRelations := func(unit *state.Unit, expect ...*state.Relation) {
		assertRelationsInScope(unit, expect...)
		assertRelationsJoined(unit, expect...)
	}
	assertRelations(wordpress0)
	assertRelations(mysql0)

	mysql0ru, err := rel.Unit(mysql0)
	c.Assert(err, jc.ErrorIsNil)
	err = mysql0ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertRelations(wordpress0)
	assertRelations(mysql0, rel)

	wordpress0ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress0ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertRelations(wordpress0, rel)
	assertRelations(mysql0, rel)

	err = mysql0ru.PrepareLeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	assertRelations(wordpress0, rel)
	assertRelationsInScope(mysql0, rel)
	assertRelationsJoined(mysql0)
}

func (s *UnitSuite) TestRemove(c *gc.C) {
	err := s.unit.Remove()
	c.Assert(err, gc.ErrorMatches, `cannot remove unit "wordpress/0": unit is not dead`)
	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	units, err := s.service.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 0)
	err = s.unit.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestRemovePathological(c *gc.C) {
	// Add a relation between wordpress and mysql...
	wordpress := s.service
	wordpress0 := s.unit
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// The relation holds a reference to wordpress, but that can't keep
	// wordpress from being removed -- because the relation will be removed
	// if we destroy wordpress.
	// However, if a unit of the *other* service joins the relation, that
	// will add an additional reference and prevent the relation -- and
	// thus wordpress itself -- from being removed when its last unit is.
	mysql0, err := mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	mysql0ru, err := rel.Unit(mysql0)
	c.Assert(err, jc.ErrorIsNil)
	err = mysql0ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Destroy wordpress, and remove its last unit.
	err = wordpress.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress0.Remove()
	c.Assert(err, jc.ErrorIsNil)

	// Check this didn't kill the service or relation yet...
	err = wordpress.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	// ...but when the unit on the other side departs the relation, the
	// relation and the other service are cleaned up.
	err = mysql0ru.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *UnitSuite) TestRemovePathologicalWithBuggyUniter(c *gc.C) {
	// Add a relation between wordpress and mysql...
	wordpress := s.service
	wordpress0 := s.unit
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// The relation holds a reference to wordpress, but that can't keep
	// wordpress from being removed -- because the relation will be removed
	// if we destroy wordpress.
	// However, if a unit of the *other* service joins the relation, that
	// will add an additional reference and prevent the relation -- and
	// thus wordpress itself -- from being removed when its last unit is.
	mysql0, err := mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	mysql0ru, err := rel.Unit(mysql0)
	c.Assert(err, jc.ErrorIsNil)
	err = mysql0ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Destroy wordpress, and remove its last unit.
	err = wordpress.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress0.Remove()
	c.Assert(err, jc.ErrorIsNil)

	// Check this didn't kill the service or relation yet...
	err = wordpress.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	// ...and that when the malfunctioning unit agent on the other side
	// sets itself to dead *without* departing the relation, the unit's
	// removal causes the relation and the other service to be cleaned up.
	err = mysql0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = mysql0.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *UnitSuite) TestWatchSubordinates(c *gc.C) {
	// TODO(mjs) - ENVUUID - test with multiple environments with
	// identically named units and ensure there's no leakage.
	w := s.unit.WatchSubordinateUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a couple of subordinates, check change.
	subCharm := s.AddTestingCharm(c, "logging")
	var subUnits []*state.Unit
	for i := 0; i < 2; i++ {
		// Note: subordinate units can only be created as a side effect of a
		// principal entering scope; and a given principal can only have a
		// single subordinate unit of each service.
		name := "logging" + strconv.Itoa(i)
		subSvc := s.AddTestingService(c, name, subCharm)
		eps, err := s.State.InferEndpoints(name, "wordpress")
		c.Assert(err, jc.ErrorIsNil)
		rel, err := s.State.AddRelation(eps...)
		c.Assert(err, jc.ErrorIsNil)
		ru, err := rel.Unit(s.unit)
		c.Assert(err, jc.ErrorIsNil)
		err = ru.EnterScope(nil)
		c.Assert(err, jc.ErrorIsNil)
		units, err := subSvc.AllUnits()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(units, gc.HasLen, 1)
		subUnits = append(subUnits, units[0])
	}
	wc.AssertChange(subUnits[0].Name(), subUnits[1].Name())
	wc.AssertNoChange()

	// Set one to Dying, check change.
	err := subUnits[0].Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(subUnits[0].Name())
	wc.AssertNoChange()

	// Set both to Dead, and remove one; check change.
	err = subUnits[0].EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = subUnits[1].EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = subUnits[1].Remove()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(subUnits[0].Name(), subUnits[1].Name())
	wc.AssertNoChange()

	// Stop watcher, check closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Start a new watch, check Dead unit is reported.
	w = s.unit.WatchSubordinateUnits()
	defer testing.AssertStop(c, w)
	wc = testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange(subUnits[0].Name())
	wc.AssertNoChange()

	// Remove the leftover, check no change.
	err = subUnits[0].Remove()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *UnitSuite) TestWatchUnit(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	w := s.unit.Watch()
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Make one change (to a separate instance), check one event.
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, jc.ErrorIsNil)
	s.setAssignedMachineAddresses(c, unit)
	wc.AssertOneChange()

	// Make two changes, check one event.
	err = unit.SetPassword("arble-farble-dying-yarble")
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Stop, check closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Remove unit, start new watch, check single event.
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	w = s.unit.Watch()
	defer testing.AssertStop(c, w)
	testing.NewNotifyWatcherC(c, s.State, w).AssertOneChange()
}

func (s *UnitSuite) TestUnitAgentTools(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	testAgentTools(c, s.unit, `unit "wordpress/0"`)
}

func (s *UnitSuite) TestActionSpecs(c *gc.C) {
	basicActions := `
snapshot:
  params:
    outfile:
      type: string
`[1:]

	wordpress := s.AddTestingService(c, "wordpress-actions", s.AddActionsCharm(c, "wordpress", basicActions, 1))
	unit1, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	specs, err := unit1.ActionSpecs()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(specs, jc.DeepEquals, state.ActionSpecsByName{
		"snapshot": charm.ActionSpec{
			Description: "No description",
			Params: map[string]interface{}{
				"type":        "object",
				"title":       "snapshot",
				"description": "No description",
				"properties": map[string]interface{}{
					"outfile": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
	})
}

func (s *UnitSuite) TestUnitActionsFindsRightActions(c *gc.C) {
	// An actions.yaml which permits actions by the following names
	basicActions := `
action-a-a:
action-a-b:
action-a-c:
action-b-a:
action-b-b:
`[1:]

	// Add simple service and two units
	dummy := s.AddTestingService(c, "dummy", s.AddActionsCharm(c, "dummy", basicActions, 1))

	unit1, err := dummy.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	unit2, err := dummy.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	// Add 3 actions to first unit, and 2 to the second unit
	_, err = unit1.AddAction("action-a-a", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit1.AddAction("action-a-b", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit1.AddAction("action-a-c", nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = unit2.AddAction("action-b-a", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit2.AddAction("action-b-b", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Verify that calling Actions on unit1 returns only
	// the three actions added to unit1
	actions1, err := unit1.Actions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(actions1), gc.Equals, 3)
	for _, action := range actions1 {
		c.Assert(action.Name(), gc.Matches, "^action-a-.")
	}

	// Verify that calling Actions on unit2 returns only
	// the two actions added to unit2
	actions2, err := unit2.Actions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(actions2), gc.Equals, 2)
	for _, action := range actions2 {
		c.Assert(action.Name(), gc.Matches, "^action-b-.")
	}
}
