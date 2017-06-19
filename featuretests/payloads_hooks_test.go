// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/payload"
	"github.com/juju/juju/state"
)

type PayloadsHookSuite struct {
	jujutesting.JujuConnSuite

	appName string
	charm   *state.Charm

	unit    *state.Unit
	apiUnit *uniter.Unit
}

func (s *PayloadsHookSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create an application.
	s.appName = "appconfig"
	s.charm = s.AddTestingCharm(c, s.appName)

	// Deploy this application.
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  s.appName,
		Charm: s.charm,
	})
	c.Assert(err, jc.ErrorIsNil)

	s.unit, err = app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	s.unit.SetCharmURL(s.charm.URL())

	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	st := s.OpenAPIAs(c, s.unit.Tag(), password)
	uniteer, err := st.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uniteer, gc.NotNil)

	// To avoid "unit is not assigned to a machine"
	err = s.State.AssignUnit(s.unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	s.apiUnit, err = uniteer.Unit(s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
}

// The primary objective of this test is to ensure
// that payload hooks are wired correctly and that
// we can do a full stack traversal.
func (s *PayloadsHookSuite) TestPayloads(c *gc.C) {
	payloadName := "kvm-guest"

	payloadToTrack := params.TrackPayloadParams{
		Class:  payloadName,
		Type:   "kvm",
		ID:     "id",
		Status: payload.StateRunning,
		Labels: []string{"tag1", "tag 2"},
	}

	expectedResult := payload.Result{
		ID: payloadName,
		Payload: &payload.FullPayloadInfo{
			Payload: payload.Payload{
				PayloadClass: charm.PayloadClass{payloadName, payloadToTrack.Type},
				Status:       payloadToTrack.Status,
				ID:           payloadToTrack.ID,
				Labels:       payloadToTrack.Labels,
				Unit:         s.unit.Name(),
			},
			Machine: "0",
		},
	}

	err := s.apiUnit.TrackPayloads([]params.TrackPayloadParams{payloadToTrack})
	c.Assert(err, jc.ErrorIsNil)
	s.assertPayloadsInState(c, []payload.Result{expectedResult})

	payloadForStatusChange := params.PayloadStatusParams{
		Class:  payloadName,
		Status: payload.StateStopped,
	}
	err = s.apiUnit.SetPayloadsStatus([]params.PayloadStatusParams{payloadForStatusChange})
	c.Assert(err, jc.ErrorIsNil)
	expectedResult.Payload.Status = payloadForStatusChange.Status
	s.assertPayloadsInState(c, []payload.Result{expectedResult})

	payloadToUntrack := params.UntrackPayloadParams{
		Class: payloadName,
	}
	err = s.apiUnit.UntrackPayloads([]params.UntrackPayloadParams{payloadToUntrack})
	c.Assert(err, jc.ErrorIsNil)
	s.assertPayloadsInState(c, []payload.Result{})
}

func (s *PayloadsHookSuite) assertPayloadsInState(c *gc.C, expected []payload.Result) {
	up, err := s.State.UnitPayloads(s.unit)
	c.Assert(err, jc.ErrorIsNil)
	all, err := up.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.DeepEquals, expected)
}
