// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"

	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/payload"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type PayloadsSuite struct {
	ConnSuite
}

var _ = gc.Suite(&PayloadsSuite{})

func (s *PayloadsSuite) TestLookUp(c *gc.C) {
	fix := s.newFixture(c)

	result, err := fix.UnitPayloads.LookUp("returned", "ignored")
	c.Check(result, gc.Equals, "returned")
	c.Check(err, jc.ErrorIsNil)
}

func (s *PayloadsSuite) TestListPartial(c *gc.C) {
	// Note: List and ListAll are extensively tested via the Check
	// methods on payloadFixture, used throughout the suite. But
	// they don't cover this feature...
	fix, initial := s.newPayloadFixture(c)
	results, err := fix.UnitPayloads.List("whatever", initial.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2)

	missing := results[0]
	c.Check(missing.ID, gc.Equals, "whatever")
	c.Check(missing.Payload, gc.IsNil)
	c.Check(missing.NotFound, jc.IsTrue)
	c.Check(missing.Error, jc.Satisfies, errors.IsNotFound)
	c.Check(missing.Error, gc.ErrorMatches, "whatever not found")

	found := results[1]
	c.Check(found.ID, gc.Equals, initial.Name)
	c.Assert(found.Payload, gc.NotNil)
	c.Assert(*found.Payload, jc.DeepEquals, fix.FullPayload(initial))
	c.Check(found.NotFound, jc.IsFalse)
	c.Check(found.Error, jc.ErrorIsNil)
}

func (s *PayloadsSuite) TestNoPayloads(c *gc.C) {
	fix := s.newFixture(c)

	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestTrackInvalidPayload(c *gc.C) {
	// not an exhaustive test, just an indication we do Validate()
	fix := s.newFixture(c)
	pl := fix.SamplePayload("")

	err := fix.UnitPayloads.Track(pl)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `missing ID not valid`)
	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestTrackInvalidUnit(c *gc.C) {

	// Note: this is STUPID, but none of the unit-specific contexts
	// between `api/context/register.go` and here ever check that
	// the track request is correctly targeted. So we overwrite it
	// unconditionally... because register is unconditionally
	// sending a garbage unit name for some reason.

	fix := s.newFixture(c)
	expect := fix.SamplePayload("some-docker-id")
	track := expect
	track.Unit = "different/0"

	err := fix.UnitPayloads.Track(track)
	// In a sensible implementation, this would be:
	//
	//    c.Check(err, jc.Satisfies, errors.IsNotValid)
	//    c.Check(err, gc.ErrorMatches, `unexpected Unit "different/0" not valid`)
	//
	//    fix.CheckUnitPayloads(c)
	//    fix.CheckModelPayloads(c)
	//
	// ...but instead we have:
	c.Assert(err, jc.ErrorIsNil)
	fix.CheckOnePayload(c, expect)
}

func (s *PayloadsSuite) TestTrackInsertPayload(c *gc.C) {
	fix := s.newFixture(c)
	desired := fix.SamplePayload("some-docker-id")

	err := fix.UnitPayloads.Track(desired)
	c.Assert(err, jc.ErrorIsNil)
	fix.CheckOnePayload(c, desired)
}

func (s *PayloadsSuite) TestTrackUpdatePayload(c *gc.C) {
	fix, initial := s.newPayloadFixture(c)
	replacement := initial
	replacement.ID = "new-exciting-different"

	err := fix.UnitPayloads.Track(replacement)
	c.Assert(err, jc.ErrorIsNil)
	fix.CheckOnePayload(c, replacement)
}

func (s *PayloadsSuite) TestTrackMultiplePayloads(c *gc.C) {
	fix, initial := s.newPayloadFixture(c)
	additional := fix.SamplePayload("another-docker-id")
	additional.Name = "app"

	err := fix.UnitPayloads.Track(additional)
	c.Assert(err, jc.ErrorIsNil)

	full1 := fix.FullPayload(initial)
	full2 := fix.FullPayload(additional)
	fix.CheckUnitPayloads(c, full1, full2)
	fix.CheckModelPayloads(c, full1, full2)
}

func (s *PayloadsSuite) TestTrackMultipleUnits(c *gc.C) {
	fix, initial := s.newPayloadFixture(c)

	// Create a new unit to add another payload to.
	applicationName := fix.Unit.ApplicationName()
	application, err := s.State.Application(applicationName)
	c.Assert(err, jc.ErrorIsNil)
	machine2 := s.Factory.MakeMachine(c, nil)
	unit2 := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: application,
		Machine:     machine2,
	})

	// Add a payload which should be independent of the
	// UnitPayloads in the fixture.
	unit2Payloads, err := s.State.UnitPayloads(unit2)
	c.Assert(err, jc.ErrorIsNil)
	additional := initial
	additional.Unit = unit2.Name()
	err = unit2Payloads.Track(additional)
	c.Assert(err, jc.ErrorIsNil)

	// Check the independent payload only shows up in
	// the fixture's ModelPayloads, not its UnitPayloads.
	full1 := fix.FullPayload(initial)
	full2 := payload.FullPayloadInfo{
		Payload: additional,
		Machine: machine2.Id(),
	}
	fix.CheckUnitPayloads(c, full1)
	fix.CheckModelPayloads(c, full1, full2)
}

func (s *PayloadsSuite) TestSetStatusInvalid(c *gc.C) {
	fix, initial := s.newPayloadFixture(c)

	err := fix.UnitPayloads.SetStatus(initial.Name, "twirling")
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err.Error(), gc.Equals, `status "twirling" not supported; expected one of ["running", "starting", "stopped", "stopping"]`)

	fix.CheckOnePayload(c, initial)
}

func (s *PayloadsSuite) TestSetStatus(c *gc.C) {
	fix, initial := s.newPayloadFixture(c)
	expect := initial
	expect.Status = "stopping"

	err := fix.UnitPayloads.SetStatus(initial.Name, "stopping")
	c.Assert(err, jc.ErrorIsNil)

	fix.CheckOnePayload(c, expect)
}

func (s *PayloadsSuite) TestUntrackMissing(c *gc.C) {
	fix := s.newFixture(c)

	err := fix.UnitPayloads.Untrack("whatever")
	c.Assert(err, jc.ErrorIsNil)
	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestUntrack(c *gc.C) {
	fix, initial := s.newPayloadFixture(c)

	err := fix.UnitPayloads.Untrack(initial.Name)
	c.Assert(err, jc.ErrorIsNil)
	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestRemoveUnitUntracksPayloads(c *gc.C) {
	fix, _ := s.newPayloadFixture(c)
	additional := fix.SamplePayload("another-docker-id")
	additional.Name = "app"
	err := fix.UnitPayloads.Track(additional)
	c.Assert(err, jc.ErrorIsNil)

	err = fix.Unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestTrackRaceDyingUnit(c *gc.C) {
	fix := s.newFixture(c)
	preventUnitDestroyRemove(c, fix.Unit)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.Unit.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	desired := fix.SamplePayload("this-is-fine")
	err := fix.UnitPayloads.Track(desired)
	c.Assert(err, jc.ErrorIsNil)
	fix.CheckOnePayload(c, desired)
}

func (s *PayloadsSuite) TestTrackRaceDeadUnit(c *gc.C) {
	fix := s.newFixture(c)
	preventUnitDestroyRemove(c, fix.Unit)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.Unit.Destroy()
		c.Assert(err, jc.ErrorIsNil)
		err = fix.Unit.EnsureDead()
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	desired := fix.SamplePayload("sorry-too-late")
	err := fix.UnitPayloads.Track(desired)
	c.Check(err, gc.ErrorMatches, fix.DeadUnitMessage())
	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestTrackRaceRemovedUnit(c *gc.C) {
	fix := s.newFixture(c)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.Unit.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	desired := fix.SamplePayload("sorry-too-late")
	err := fix.UnitPayloads.Track(desired)
	c.Check(err, gc.ErrorMatches, fix.DeadUnitMessage())
	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestTrackRaceTrack(c *gc.C) {
	fix := s.newFixture(c)
	desired := fix.SamplePayload("wanted")
	interloper := fix.SamplePayload("not-wanted")

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.UnitPayloads.Track(interloper)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err := fix.UnitPayloads.Track(desired)
	c.Assert(err, jc.ErrorIsNil)
	fix.CheckOnePayload(c, desired)
}

func (s *PayloadsSuite) TestTrackRaceSetStatus(c *gc.C) {
	fix, initial := s.newPayloadFixture(c)
	desired := initial
	desired.Status = "starting"

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.UnitPayloads.SetStatus(initial.Name, "stopping")
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err := fix.UnitPayloads.Track(desired)
	c.Assert(err, jc.ErrorIsNil)
	fix.CheckOnePayload(c, desired)
}

func (s *PayloadsSuite) TestTrackRaceUntrack(c *gc.C) {
	fix, initial := s.newPayloadFixture(c)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.UnitPayloads.Untrack(initial.Name)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err := fix.UnitPayloads.Track(initial)
	c.Assert(err, jc.ErrorIsNil)
	fix.CheckOnePayload(c, initial)
}

func (s *PayloadsSuite) TestSetStatusRaceTrack(c *gc.C) {
	fix, initial := s.newPayloadFixture(c)
	expect := initial
	expect.Status = "stopped"

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.UnitPayloads.Track(initial)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err := fix.UnitPayloads.SetStatus(initial.Name, "stopped")
	c.Assert(err, jc.ErrorIsNil)
	fix.CheckOnePayload(c, expect)
}

func (s *PayloadsSuite) TestSetStatusRaceUntrack(c *gc.C) {
	fix, initial := s.newPayloadFixture(c)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.UnitPayloads.Untrack(initial.Name)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err := fix.UnitPayloads.SetStatus(initial.Name, "stopped")
	c.Check(errors.Cause(err), gc.Equals, payload.ErrNotFound)
	c.Check(err, gc.ErrorMatches, "payload not found")
	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestUntrackRaceTrack(c *gc.C) {
	fix, initial := s.newPayloadFixture(c)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.UnitPayloads.Track(initial)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err := fix.UnitPayloads.Untrack(initial.Name)
	c.Assert(err, jc.ErrorIsNil)
	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestUntrackRaceSetStatus(c *gc.C) {
	fix, initial := s.newPayloadFixture(c)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.UnitPayloads.SetStatus(initial.Name, "stopping")
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err := fix.UnitPayloads.Untrack(initial.Name)
	c.Assert(err, jc.ErrorIsNil)
	fix.CheckNoPayload(c)
}

func (s *PayloadsSuite) TestUntrackRaceUntrack(c *gc.C) {
	fix, initial := s.newPayloadFixture(c)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := fix.UnitPayloads.Untrack(initial.Name)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err := fix.UnitPayloads.Untrack(initial.Name)
	c.Assert(err, jc.ErrorIsNil)
	fix.CheckNoPayload(c)
}

// -------------------------
// test helpers

type payloadsFixture struct {
	ModelPayloads state.ModelPayloads
	UnitPayloads  state.UnitPayloads
	Machine       *state.Machine
	Unit          *state.Unit
}

func (s *PayloadsSuite) newFixture(c *gc.C) payloadsFixture {
	machine := s.Factory.MakeMachine(c, nil)
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Machine: machine})
	modelPayloads, err := s.State.ModelPayloads()
	c.Assert(err, jc.ErrorIsNil)
	unitPayloads, err := s.State.UnitPayloads(unit)
	c.Assert(err, jc.ErrorIsNil)
	return payloadsFixture{
		ModelPayloads: modelPayloads,
		UnitPayloads:  unitPayloads,
		Machine:       machine,
		Unit:          unit,
	}
}

func (s *PayloadsSuite) newPayloadFixture(c *gc.C) (payloadsFixture, payload.Payload) {
	fix := s.newFixture(c)
	initial := fix.SamplePayload("some-docker-id")
	err := fix.UnitPayloads.Track(initial)
	c.Assert(err, jc.ErrorIsNil)
	return fix, initial
}

func (fix payloadsFixture) SamplePayload(id string) payload.Payload {
	return payload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "database",
			Type: "docker",
		},
		Status: payload.StateRunning,
		ID:     id,
		Unit:   fix.Unit.Name(),
	}
}

func (fix payloadsFixture) DeadUnitMessage() string {
	return fmt.Sprintf("unit %q no longer available", fix.Unit.Name())
}

func (fix payloadsFixture) FullPayload(pl payload.Payload) payload.FullPayloadInfo {
	return payload.FullPayloadInfo{
		Payload: pl,
		Machine: fix.Machine.Id(),
	}
}

func (fix payloadsFixture) CheckNoPayload(c *gc.C) {
	fix.CheckModelPayloads(c)
	fix.CheckUnitPayloads(c)
}

func (fix payloadsFixture) CheckOnePayload(c *gc.C, expect payload.Payload) {
	full := fix.FullPayload(expect)
	fix.CheckModelPayloads(c, full)
	fix.CheckUnitPayloads(c, full)
}

func (fix payloadsFixture) CheckModelPayloads(c *gc.C, expect ...payload.FullPayloadInfo) {
	actual, err := fix.ModelPayloads.ListAll()
	c.Check(err, jc.ErrorIsNil)
	sort.Sort(byPayloadInfo(actual))
	sort.Sort(byPayloadInfo(expect))
	c.Check(actual, jc.DeepEquals, expect)
}

func (fix payloadsFixture) CheckUnitPayloads(c *gc.C, expect ...payload.FullPayloadInfo) {
	actual, err := fix.UnitPayloads.List()
	c.Check(err, jc.ErrorIsNil)
	extracted := fix.extractInfos(c, actual)
	sort.Sort(byPayloadInfo(extracted))
	sort.Sort(byPayloadInfo(expect))
	c.Check(extracted, jc.DeepEquals, expect)
}

func (payloadsFixture) extractInfos(c *gc.C, results []payload.Result) []payload.FullPayloadInfo {
	fulls := make([]payload.FullPayloadInfo, 0, len(results))
	for _, result := range results {
		c.Assert(result.ID, gc.Equals, result.Payload.Name)
		c.Assert(result.Payload, gc.NotNil)
		c.Assert(result.NotFound, jc.IsFalse)
		c.Assert(result.Error, jc.ErrorIsNil)
		fulls = append(fulls, *result.Payload)
	}
	return fulls
}

type byPayloadInfo []payload.FullPayloadInfo

func (s byPayloadInfo) Len() int      { return len(s) }
func (s byPayloadInfo) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s byPayloadInfo) Less(i, j int) bool {
	if s[i].Machine != s[j].Machine {
		return s[i].Machine < s[j].Machine
	}
	if s[i].Payload.Unit != s[j].Payload.Unit {
		return s[i].Payload.Unit < s[j].Payload.Unit
	}
	return s[i].Payload.Name < s[j].Payload.Name
}

// ----------------------------------------------------------
// original functional tests

type PayloadsFunctionalSuite struct {
	ConnSuite
}

var _ = gc.Suite(&PayloadsFunctionalSuite{})

func (s *PayloadsFunctionalSuite) TestModelPayloads(c *gc.C) {
	machine := "0"
	unit := addUnit(c, s.ConnSuite, unitArgs{
		charm:       "dummy",
		application: "a-application",
		metadata:    payloadsMetaYAML,
		machine:     machine,
	})

	ust, err := s.State.UnitPayloads(unit)
	c.Assert(err, jc.ErrorIsNil)

	st, err := s.State.ModelPayloads()
	c.Assert(err, jc.ErrorIsNil)

	payloads, err := st.ListAll()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(payloads, gc.HasLen, 0)

	err = ust.Track(payload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "payloadA",
			Type: "docker",
		},
		Status: payload.StateRunning,
		ID:     "xyz",
		Unit:   "a-application/0",
	})
	c.Assert(err, jc.ErrorIsNil)

	unitPayloads, err := ust.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitPayloads, gc.HasLen, 1)

	payloads, err = st.ListAll()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(payloads, jc.DeepEquals, []payload.FullPayloadInfo{{
		Payload: payload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "payloadA",
				Type: "docker",
			},
			ID:     "xyz",
			Status: payload.StateRunning,
			Labels: []string{},
			Unit:   "a-application/0",
		},
		Machine: machine,
	}})

	id, err := ust.LookUp("payloadA", "xyz")
	c.Assert(err, jc.ErrorIsNil)

	err = ust.Untrack(id)
	c.Assert(err, jc.ErrorIsNil)

	payloads, err = st.ListAll()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(payloads, gc.HasLen, 0)
}

func (s *PayloadsFunctionalSuite) TestUnitPayloads(c *gc.C) {
	machine := "0"
	unit := addUnit(c, s.ConnSuite, unitArgs{
		charm:       "dummy",
		application: "a-application",
		metadata:    payloadsMetaYAML,
		machine:     machine,
	})

	st, err := s.State.UnitPayloads(unit)
	c.Assert(err, jc.ErrorIsNil)

	results, err := st.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.HasLen, 0)

	pl := payload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "payloadA",
			Type: "docker",
		},
		ID:     "xyz",
		Status: payload.StateRunning,
		Unit:   "a-application/0",
	}
	err = st.Track(pl)
	c.Assert(err, jc.ErrorIsNil)

	results, err = st.List()
	c.Assert(err, jc.ErrorIsNil)
	// TODO(ericsnow) Once Track returns the new ID we can drop
	// the following two lines.
	c.Assert(results, gc.HasLen, 1)
	id := results[0].ID
	c.Check(results, jc.DeepEquals, []payload.Result{{
		ID: id,
		Payload: &payload.FullPayloadInfo{
			Payload: pl,
			Machine: machine,
		},
	}})

	lookedUpID, err := st.LookUp("payloadA", "xyz")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(lookedUpID, gc.Equals, id)

	c.Logf("using ID %q", id)
	results, err = st.List(id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, jc.DeepEquals, []payload.Result{{
		ID: id,
		Payload: &payload.FullPayloadInfo{
			Payload: pl,
			Machine: machine,
		},
	}})

	err = st.SetStatus(id, "running")
	c.Assert(err, jc.ErrorIsNil)

	results, err = st.List(id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, jc.DeepEquals, []payload.Result{{
		ID: id,
		Payload: &payload.FullPayloadInfo{
			Payload: pl,
			Machine: machine,
		},
	}})

	// Ensure existing ones are replaced.
	update := pl
	update.ID = "abc"
	err = st.Track(update)
	c.Check(err, jc.ErrorIsNil)
	results, err = st.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, jc.DeepEquals, []payload.Result{{
		ID: id,
		Payload: &payload.FullPayloadInfo{
			Payload: update,
			Machine: machine,
		},
	}})

	err = st.Untrack(id)
	c.Assert(err, jc.ErrorIsNil)

	results, err = st.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.HasLen, 0)
}

const payloadsMetaYAML = `
name: a-charm
summary: a charm...
description: a charm...
payloads:
  payloadA:
    type: docker
`

type unitArgs struct {
	charm       string
	application string
	metadata    string
	machine     string
}

func addUnit(c *gc.C, s ConnSuite, args unitArgs) *state.Unit {
	s.AddTestingCharm(c, args.charm)
	ch := s.AddMetaCharm(c, args.charm, args.metadata, 2)

	app := s.AddTestingApplication(c, args.application, ch)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// TODO(ericsnow) Explicitly: call unit.AssignToMachine(m)?
	c.Assert(args.machine, gc.Equals, "0")
	err = unit.AssignToNewMachine() // machine "0"
	c.Assert(err, jc.ErrorIsNil)

	return unit
}
