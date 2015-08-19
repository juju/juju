// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

type statusGetterSuite struct {
	statusBaseSuite
	getter *common.StatusGetter
}

var _ = gc.Suite(&statusGetterSuite{})

func (s *statusGetterSuite) SetUpTest(c *gc.C) {
	s.statusBaseSuite.SetUpTest(c)

	s.getter = common.NewStatusGetter(s.State, func() (common.AuthFunc, error) {
		return s.authFunc, nil
	})
}

func (s *statusGetterSuite) TestUnauthorized(c *gc.C) {
	tag := names.NewMachineTag("42")
	s.badTag = tag
	result, err := s.getter.Status(params.Entities{[]params.Entity{{
		tag.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *statusGetterSuite) TestNotATag(c *gc.C) {
	result, err := s.getter.Status(params.Entities{[]params.Entity{{
		"not a tag",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, `"not a tag" is not a valid tag`)
}

func (s *statusGetterSuite) TestNotFound(c *gc.C) {
	result, err := s.getter.Status(params.Entities{[]params.Entity{{
		names.NewMachineTag("42").String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *statusGetterSuite) TestGetMachineStatus(c *gc.C) {
	machine := s.Factory.MakeMachine(c, nil)
	result, err := s.getter.Status(params.Entities{[]params.Entity{{
		machine.Tag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	status := result.Results[0]
	c.Assert(status.Error, gc.IsNil)
	c.Assert(status.Status, gc.Equals, params.Status(state.StatusPending))
}

func (s *statusGetterSuite) TestGetUnitStatus(c *gc.C) {
	// The status has to be a valid workload status, because get status
	// on the unit returns the workload status not the agent status as it
	// does on a machine.
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Status: &state.StatusInfo{
		Status: state.StatusMaintenance,
	}})
	result, err := s.getter.Status(params.Entities{[]params.Entity{{
		unit.Tag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	status := result.Results[0]
	c.Assert(status.Error, gc.IsNil)
	c.Assert(status.Status, gc.Equals, params.Status(state.StatusMaintenance))
}

func (s *statusGetterSuite) TestGetServiceStatus(c *gc.C) {
	service := s.Factory.MakeService(c, &factory.ServiceParams{Status: &state.StatusInfo{
		Status: state.StatusMaintenance,
	}})
	result, err := s.getter.Status(params.Entities{[]params.Entity{{
		service.Tag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	status := result.Results[0]
	c.Assert(status.Error, gc.IsNil)
	c.Assert(status.Status, gc.Equals, params.Status(state.StatusMaintenance))
}

func (s *statusGetterSuite) TestBulk(c *gc.C) {
	s.badTag = names.NewMachineTag("42")
	machine := s.Factory.MakeMachine(c, nil)
	result, err := s.getter.Status(params.Entities{[]params.Entity{{
		s.badTag.String(),
	}, {
		machine.Tag().String(),
	}, {
		"bad-tag",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(result.Results[1].Error, gc.IsNil)
	c.Assert(result.Results[1].Status, gc.Equals, params.Status(state.StatusPending))
	c.Assert(result.Results[2].Error, gc.ErrorMatches, `"bad-tag" is not a valid tag`)
}

type serviceStatusGetterSuite struct {
	statusBaseSuite
	getter *common.ServiceStatusGetter
}

var _ = gc.Suite(&serviceStatusGetterSuite{})

func (s *serviceStatusGetterSuite) SetUpTest(c *gc.C) {
	s.statusBaseSuite.SetUpTest(c)

	s.getter = common.NewServiceStatusGetter(s.State, func() (common.AuthFunc, error) {
		return s.authFunc, nil
	})
}

func (s *serviceStatusGetterSuite) TestUnauthorized(c *gc.C) {
	// Machines are unauthorized since they are not units
	tag := names.NewUnitTag("foo/0")
	s.badTag = tag
	result, err := s.getter.Status(params.Entities{[]params.Entity{{
		tag.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *serviceStatusGetterSuite) TestNotATag(c *gc.C) {
	result, err := s.getter.Status(params.Entities{[]params.Entity{{
		"not a tag",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, `"not a tag" is not a valid tag`)
}

func (s *serviceStatusGetterSuite) TestNotFound(c *gc.C) {
	result, err := s.getter.Status(params.Entities{[]params.Entity{{
		names.NewUnitTag("foo/0").String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *serviceStatusGetterSuite) TestGetMachineStatus(c *gc.C) {
	machine := s.Factory.MakeMachine(c, nil)
	result, err := s.getter.Status(params.Entities{[]params.Entity{{
		machine.Tag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	// Can't call service status on a machine.
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *serviceStatusGetterSuite) TestGetServiceStatus(c *gc.C) {
	service := s.Factory.MakeService(c, &factory.ServiceParams{Status: &state.StatusInfo{
		Status: state.StatusMaintenance,
	}})
	result, err := s.getter.Status(params.Entities{[]params.Entity{{
		service.Tag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	// Can't call service status on a service.
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *serviceStatusGetterSuite) TestGetUnitStatusNotLeader(c *gc.C) {
	// If the unit isn't the leader, it can't get it.
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Status: &state.StatusInfo{
		Status: state.StatusMaintenance,
	}})
	result, err := s.getter.Status(params.Entities{[]params.Entity{{
		unit.Tag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	status := result.Results[0]
	c.Assert(status.Error, gc.ErrorMatches, ".* is not leader of .*")
}

func (s *serviceStatusGetterSuite) TestGetUnitStatusIsLeader(c *gc.C) {
	// If the unit isn't the leader, it can't get it.
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Status: &state.StatusInfo{
		Status: state.StatusMaintenance,
	}})
	service, err := unit.Service()
	c.Assert(err, jc.ErrorIsNil)
	s.State.LeadershipClaimer().ClaimLeadership(
		service.Name(),
		unit.Name(),
		time.Minute)
	result, err := s.getter.Status(params.Entities{[]params.Entity{{
		unit.Tag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error, gc.IsNil)
	c.Assert(r.Service.Error, gc.IsNil)
	c.Assert(r.Service.Status, gc.Equals, params.Status(state.StatusMaintenance))
	units := r.Units
	c.Assert(units, gc.HasLen, 1)
	status, ok := units[unit.Name()]
	c.Assert(ok, jc.IsTrue)
	c.Assert(status.Error, gc.IsNil)
	c.Assert(status.Status, gc.Equals, params.Status(state.StatusMaintenance))
}

func (s *serviceStatusGetterSuite) TestBulk(c *gc.C) {
	s.badTag = names.NewMachineTag("42")
	machine := s.Factory.MakeMachine(c, nil)
	result, err := s.getter.Status(params.Entities{[]params.Entity{{
		s.badTag.String(),
	}, {
		machine.Tag().String(),
	}, {
		"bad-tag",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(result.Results[1].Error, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(result.Results[2].Error, gc.ErrorMatches, `"bad-tag" is not a valid tag`)
}

type statusBaseSuite struct {
	testing.StateSuite
	badTag names.Tag
}

func (s *statusBaseSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.badTag = nil
}

func (s *statusBaseSuite) authFunc(tag names.Tag) bool {
	return tag != s.badTag
}
