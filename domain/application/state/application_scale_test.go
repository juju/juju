// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationinternal "github.com/juju/juju/domain/application/internal"
	"github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
)

func (s *applicationStateSuite) TestSetCAASApplicationUnitScaleRetainsHighestOrdinals(c *tc.C) {
	appUUID := s.createCAASScalingApplication(c, "foo", life.Alive, 0)
	newUnits := func(names ...string) []applicationinternal.ScaleCAASUnitArg {
		args := make([]applicationinternal.ScaleCAASUnitArg, len(names))
		for i, name := range names {
			args[i].ExpectedName = name
			args[i].UnitUUID = tc.Must(c, unit.NewUUID).String()
			args[i].NetNodeUUID = tc.Must(c, domainnetwork.NewNetNodeUUID).String()
		}
		return args
	}
	checkLives := func(want map[string]int) {
		got, err := s.state.GetAllUnitLifeForApplication(c.Context(), appUUID)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(got, tc.DeepEquals, want)
	}
	created, err := s.state.SetCAASApplicationUnitScale(c.Context(), appUUID.String(), nil, nil, newUnits("foo/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(created, tc.DeepEquals, []string{"foo/0"})
	created, err = s.state.SetCAASApplicationUnitScale(c.Context(), appUUID.String(), []string{"foo/0"}, nil, newUnits("foo/1", "foo/2"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(created, tc.DeepEquals, []string{"foo/1", "foo/2"})
	created, err = s.state.SetCAASApplicationUnitScale(c.Context(), appUUID.String(), []string{"foo/0", "foo/1", "foo/2"}, []string{"foo/0", "foo/1"}, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(created, tc.HasLen, 0)
	checkLives(map[string]int{"foo/0": int(life.Dying), "foo/1": int(life.Dying), "foo/2": int(life.Alive)})
	created, err = s.state.SetCAASApplicationUnitScale(c.Context(), appUUID.String(), []string{"foo/2"}, nil, newUnits("foo/3"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(created, tc.DeepEquals, []string{"foo/3"})
	checkLives(map[string]int{"foo/0": int(life.Dying), "foo/1": int(life.Dying), "foo/2": int(life.Alive), "foo/3": int(life.Alive)})

	_, err = s.state.SetCAASApplicationUnitScale(c.Context(), appUUID.String(), []string{"foo/2"}, []string{"foo/2"}, nil)
	c.Assert(err, tc.ErrorIs, applicationerrors.ScalingStateInconsistent)
	_, err = s.state.SetCAASApplicationUnitScale(c.Context(), appUUID.String(), []string{"foo/2", "foo/3"}, []string{"foo/2", "foo/3"}, nil)
	c.Assert(err, tc.ErrorIsNil)
	created, err = s.state.SetCAASApplicationUnitScale(c.Context(), appUUID.String(), nil, nil, newUnits("foo/4"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(created, tc.DeepEquals, []string{"foo/4"})
}

func (s *applicationStateSuite) TestSetCAASApplicationUnitScaleRejectsSyntheticApplication(c *tc.C) {
	appUUID := s.createCAASScalingApplication(c, "foo", life.Alive, 0)
	_, err := s.DB().ExecContext(c.Context(), `
UPDATE charm SET source_id = 2, architecture_id = NULL WHERE uuid = (
    SELECT charm_uuid FROM application WHERE uuid = ?
)`, appUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.SetCAASApplicationUnitScale(
		c.Context(), appUUID.String(), nil, nil,
		[]applicationinternal.ScaleCAASUnitArg{{
			ExpectedName: "foo/0",
			AddCAASUnitArg: applicationinternal.AddCAASUnitArg{
				UnitUUID:    tc.Must(c, unit.NewUUID).String(),
				NetNodeUUID: tc.Must(c, domainnetwork.NewNetNodeUUID).String(),
			},
		}},
	)
	c.Check(err, tc.ErrorMatches, `cannot scale synthetic application "foo"`)

	lives, err := s.state.GetAllUnitLifeForApplication(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lives, tc.HasLen, 0)
}

func (s *applicationStateSuite) TestSetCAASApplicationUnitScaleRejectsDyingApplication(c *tc.C) {
	appUUID := s.createCAASScalingApplication(c, "foo", life.Dying, 0)

	_, err := s.state.SetCAASApplicationUnitScale(
		c.Context(), appUUID.String(), nil, nil,
		[]applicationinternal.ScaleCAASUnitArg{{
			ExpectedName: "foo/0",
			AddCAASUnitArg: applicationinternal.AddCAASUnitArg{
				UnitUUID:    tc.Must(c, unit.NewUUID).String(),
				NetNodeUUID: tc.Must(c, domainnetwork.NewNetNodeUUID).String(),
			},
		}},
	)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotAlive)

	lives, err := s.state.GetAllUnitLifeForApplication(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lives, tc.HasLen, 0)
}

func (s *applicationStateSuite) TestSetCAASApplicationUnitScaleRejectsStaleMembership(c *tc.C) {
	appUUID := s.createCAASScalingApplication(c, "foo", life.Alive, 0)
	newUnits := func(names ...string) []applicationinternal.ScaleCAASUnitArg {
		args := make([]applicationinternal.ScaleCAASUnitArg, len(names))
		for i, name := range names {
			args[i].ExpectedName = name
			args[i].UnitUUID = tc.Must(c, unit.NewUUID).String()
			args[i].NetNodeUUID = tc.Must(c, domainnetwork.NewNetNodeUUID).String()
		}
		return args
	}

	_, err := s.state.SetCAASApplicationUnitScale(c.Context(), appUUID.String(), nil, nil, newUnits("foo/0", "foo/1"))
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.state.SetCAASApplicationUnitScale(c.Context(), appUUID.String(), []string{"foo/0", "foo/1"}, []string{"foo/0"}, nil)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.state.SetCAASApplicationUnitScale(c.Context(), appUUID.String(), []string{"foo/1"}, nil, newUnits("foo/2"))
	c.Assert(err, tc.ErrorIsNil)

	// A plan made for {foo/0, foo/1} must not succeed against {foo/1, foo/2}.
	_, err = s.state.SetCAASApplicationUnitScale(c.Context(), appUUID.String(), []string{"foo/0", "foo/1"}, []string{"foo/0"}, nil)
	c.Assert(err, tc.ErrorIs, applicationerrors.ScalingStateInconsistent)
	lives, err := s.state.GetAllUnitLifeForApplication(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lives, tc.DeepEquals, map[string]int{
		"foo/0": int(life.Dying),
		"foo/1": int(life.Alive),
		"foo/2": int(life.Alive),
	})
}

func (s *applicationStateSuite) TestSetCAASApplicationUnitScaleRejectsNonContiguousAliveRange(c *tc.C) {
	appUUID := s.createCAASScalingApplication(c, "foo", life.Alive, 0)
	newUnit := func(name string) []applicationinternal.ScaleCAASUnitArg {
		return []applicationinternal.ScaleCAASUnitArg{{
			ExpectedName: name,
			AddCAASUnitArg: applicationinternal.AddCAASUnitArg{
				UnitUUID:    tc.Must(c, unit.NewUUID).String(),
				NetNodeUUID: tc.Must(c, domainnetwork.NewNetNodeUUID).String(),
			},
		}}
	}
	_, err := s.state.SetCAASApplicationUnitScale(c.Context(), appUUID.String(), nil, nil, newUnit("foo/0"))
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.state.SetCAASApplicationUnitScale(c.Context(), appUUID.String(), []string{"foo/0"}, nil, newUnit(""))
	c.Assert(err, tc.ErrorIs, applicationerrors.ScalingStateInconsistent)
	err = s.state.EnsureApplicationUnitSequenceAtLeast(c.Context(), "foo", 5)
	c.Assert(err, tc.ErrorIsNil)
	created, err := s.state.SetCAASApplicationUnitScale(c.Context(), appUUID.String(), []string{"foo/0"}, nil, newUnit("foo/1"))
	c.Assert(err, tc.ErrorIs, applicationerrors.ScalingStateInconsistent)
	c.Check(created, tc.HasLen, 0)
	lives, err := s.state.GetAllUnitLifeForApplication(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lives, tc.DeepEquals, map[string]int{"foo/0": int(life.Alive)})
	last, hasSequence, err := s.state.GetApplicationUnitSequence(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hasSequence, tc.IsTrue)
	c.Check(last, tc.Equals, uint64(5))

	_, err = s.state.SetCAASApplicationUnitScale(c.Context(), appUUID.String(), []string{"foo/0"}, []string{"foo/0"}, nil)
	c.Assert(err, tc.ErrorIsNil)
	created, err = s.state.SetCAASApplicationUnitScale(c.Context(), appUUID.String(), nil, nil, newUnit("foo/6"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(created, tc.DeepEquals, []string{"foo/6"})
}

func (s *applicationStateSuite) TestSetCAASApplicationUnitScaleSerializesConcurrentRequests(c *tc.C) {
	appUUID := s.createCAASScalingApplication(c, "foo", life.Alive, 0)
	makeUnit := func() []applicationinternal.ScaleCAASUnitArg {
		return []applicationinternal.ScaleCAASUnitArg{{
			ExpectedName: "foo/0",
			AddCAASUnitArg: applicationinternal.AddCAASUnitArg{
				UnitUUID:    tc.Must(c, unit.NewUUID).String(),
				NetNodeUUID: tc.Must(c, domainnetwork.NewNetNodeUUID).String(),
			},
		}}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	ctx := c.Context()
	for range 2 {
		args := makeUnit()
		go func() {
			<-start
			_, err := s.state.SetCAASApplicationUnitScale(ctx, appUUID.String(), nil, nil, args)
			results <- err
		}()
	}
	close(start)

	successes, conflicts := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
			continue
		}
		c.Check(err, tc.ErrorIs, applicationerrors.ScalingStateInconsistent)
		conflicts++
	}
	c.Check(successes, tc.Equals, 1)
	c.Check(conflicts, tc.Equals, 1)

	lives, err := s.state.GetAllUnitLifeForApplication(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lives, tc.DeepEquals, map[string]int{"foo/0": int(life.Alive)})
}

func (s *applicationStateSuite) TestChangeCAASApplicationUnitScaleRetainsHighestOrdinals(c *tc.C) {
	appUUID := s.createCAASScalingApplication(c, "foo", life.Alive, 0)
	newUnits := func(count int) []applicationinternal.AddCAASUnitArg {
		args := make([]applicationinternal.AddCAASUnitArg, count)
		for i := range args {
			args[i].UnitUUID = tc.Must(c, unit.NewUUID).String()
			args[i].NetNodeUUID = tc.Must(c, domainnetwork.NewNetNodeUUID).String()
		}
		return args
	}

	scale, created, err := s.state.ChangeCAASApplicationUnitScale(
		c.Context(), appUUID.String(), 3, newUnits(3),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(scale, tc.Equals, 3)
	c.Check(created, tc.DeepEquals, []string{"foo/0", "foo/1", "foo/2"})

	scale, created, err = s.state.ChangeCAASApplicationUnitScale(
		c.Context(), appUUID.String(), -2, nil,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(scale, tc.Equals, 1)
	c.Check(created, tc.HasLen, 0)

	scale, created, err = s.state.ChangeCAASApplicationUnitScale(
		c.Context(), appUUID.String(), 1, newUnits(1),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(scale, tc.Equals, 2)
	c.Check(created, tc.DeepEquals, []string{"foo/3"})

	lives, err := s.state.GetAllUnitLifeForApplication(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lives, tc.DeepEquals, map[string]int{
		"foo/0": int(life.Dying),
		"foo/1": int(life.Dying),
		"foo/2": int(life.Alive),
		"foo/3": int(life.Alive),
	})
}

func (s *applicationStateSuite) TestChangeCAASApplicationUnitScaleRejectsDyingApplication(c *tc.C) {
	appUUID := s.createCAASScalingApplication(c, "foo", life.Dying, 0)
	newUnits := []applicationinternal.AddCAASUnitArg{{
		UnitUUID:    tc.Must(c, unit.NewUUID).String(),
		NetNodeUUID: tc.Must(c, domainnetwork.NewNetNodeUUID).String(),
	}}

	_, _, err := s.state.ChangeCAASApplicationUnitScale(
		c.Context(), appUUID.String(), 1, newUnits,
	)
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotAlive)

	lives, err := s.state.GetAllUnitLifeForApplication(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lives, tc.HasLen, 0)
}

func (s *applicationStateSuite) TestChangeCAASApplicationUnitScaleRejectsControllerZero(c *tc.C) {
	appUUID := s.createCAASScalingApplication(c, "foo", life.Alive, 0)
	newUnits := []applicationinternal.AddCAASUnitArg{{
		UnitUUID:    tc.Must(c, unit.NewUUID).String(),
		NetNodeUUID: tc.Must(c, domainnetwork.NewNetNodeUUID).String(),
	}}
	_, _, err := s.state.ChangeCAASApplicationUnitScale(
		c.Context(), appUUID.String(), 1, newUnits,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `
INSERT INTO application_controller (application_uuid) VALUES (?)
`, appUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = s.state.ChangeCAASApplicationUnitScale(
		c.Context(), appUUID.String(), -1, nil,
	)
	c.Assert(err, tc.ErrorMatches, "cannot scale controller application to 0 units")

	lives, err := s.state.GetAllUnitLifeForApplication(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lives, tc.DeepEquals, map[string]int{"foo/0": int(life.Alive)})
}
