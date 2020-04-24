// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"time"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/application"
)

type goalStateSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&goalStateSuite{})

var (
	timestamp = time.Date(2200, time.November, 5, 0, 0, 0, 0, time.UTC)

	paramsBaseGoalStateStatus = params.GoalStateStatus{
		Status: "active",
		Since:  &timestamp,
	}
	apiBaseGoalStateStatus = application.GoalStateStatus{
		Status: "active",
		Since:  &timestamp,
	}
)

func (s *goalStateSuite) TestGoalStateOneUnit(c *gc.C) {
	paramsOneUnit := params.GoalStateResults{
		Results: []params.GoalStateResult{
			{Result: &params.GoalState{
				Units: params.UnitsGoalState{
					"mysql/0": paramsBaseGoalStateStatus,
				},
			},
			},
		},
	}
	apiOneUnit := application.GoalState{
		Units: application.UnitsGoalState{
			"mysql/0": apiBaseGoalStateStatus,
		},
	}
	s.testGoalState(c, paramsOneUnit, apiOneUnit)

}

func (s *goalStateSuite) TestGoalStateTwoRelatedUnits(c *gc.C) {
	paramsTwoRelatedUnits := params.GoalStateResults{
		Results: []params.GoalStateResult{
			{Result: &params.GoalState{
				Units: params.UnitsGoalState{
					"mysql/0": paramsBaseGoalStateStatus,
				},
				Relations: map[string]params.UnitsGoalState{
					"db": {
						"wordpress/0": paramsBaseGoalStateStatus,
					},
				},
			},
			},
		},
	}
	apiTwoRelatedUnits := application.GoalState{
		Units: application.UnitsGoalState{
			"mysql/0": apiBaseGoalStateStatus,
		},
		Relations: map[string]application.UnitsGoalState{
			"db": {
				"wordpress/0": apiBaseGoalStateStatus,
			},
		},
	}
	s.testGoalState(c, paramsTwoRelatedUnits, apiTwoRelatedUnits)
}

func (s *goalStateSuite) testGoalState(c *gc.C, facadeResult params.GoalStateResults, apiResult application.GoalState) {
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Uniter")
		c.Check(version, gc.Equals, 0)
		c.Check(request, gc.Equals, "GoalStates")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-mysql-0"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.GoalStateResults{})
		*(result.(*params.GoalStateResults)) = facadeResult
		called = true
		return nil
	})

	st := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	goalStateResult, err := st.GoalState()

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(goalStateResult, jc.DeepEquals, apiResult)
	c.Assert(called, jc.IsTrue)
}
