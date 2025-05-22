// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/application"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type goalStateSuite struct {
	coretesting.BaseSuite
}

func TestGoalStateSuite(t *stdtesting.T) {
	tc.Run(t, &goalStateSuite{})
}

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

func (s *goalStateSuite) TestGoalStateOneUnit(c *tc.C) {
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

func (s *goalStateSuite) TestGoalStateTwoRelatedUnits(c *tc.C) {
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

func (s *goalStateSuite) testGoalState(c *tc.C, facadeResult params.GoalStateResults, apiResult application.GoalState) {
	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Uniter")
		c.Check(version, tc.Equals, 0)
		c.Check(request, tc.Equals, "GoalStates")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-mysql-0"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.GoalStateResults{})
		*(result.(*params.GoalStateResults)) = facadeResult
		called = true
		return nil
	})

	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	goalStateResult, err := client.GoalState(c.Context())

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(goalStateResult, tc.DeepEquals, apiResult)
	c.Assert(called, tc.IsTrue)
}
