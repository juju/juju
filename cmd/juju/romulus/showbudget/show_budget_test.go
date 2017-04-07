// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.s

package showbudget_test

import (
	"github.com/juju/errors"
	"github.com/juju/romulus/wireformat/budget"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/cmdtesting"
	"github.com/juju/juju/cmd/juju/romulus/showbudget"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&showBudgetSuite{})

type showBudgetSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	stub          *testing.Stub
	mockBudgetAPI *mockBudgetAPI
	mockAPI       *mockAPI
}

func (s *showBudgetSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
	s.mockBudgetAPI = &mockBudgetAPI{s.stub}
	s.mockAPI = &mockAPI{s.stub}
	s.PatchValue(showbudget.NewBudgetAPIClient, showbudget.BudgetAPIClientFnc(s.mockBudgetAPI))
}

func (s *showBudgetSuite) TestShowBudgetCommand(c *gc.C) {
	tests := []struct {
		about  string
		args   []string
		err    string
		budget string
		apierr string
		output string
	}{{
		about: "missing argument",
		err:   `missing arguments`,
	}, {
		about: "unknown arguments",
		args:  []string{"my-special-budget", "extra", "arguments"},
		err:   `unrecognized args: \["extra" "arguments"\]`,
	}, {
		about:  "api error",
		args:   []string{"personal"},
		apierr: "well, this is embarrassing",
		err:    "failed to retrieve the budget: well, this is embarrassing",
	}, {
		about:  "all ok",
		args:   []string{"personal"},
		budget: "personal",
		output: "" +
			"Model      \tSpent\tAllocated\t       By\tUsage\n" +
			"uuid1      \t500  \t     1200\t user.joe\t42%  \n" +
			"uuid2      \t600  \t     1000\tuser.jess\t60%  \n" +
			"uuid3      \t10   \t      100\t user.bob\t10%  \n" +
			"           \t     \t         \t         \n" +
			"Total      \t1110 \t     2300\t         \t48%  \n" +
			"Budget     \t     \t     4000\t         \n" +
			"Unallocated\t     \t     1700\t         \n",
	},
	}

	for i, test := range tests {
		c.Logf("running test %d: %v", i, test.about)
		s.mockAPI.ResetCalls()

		errs := []error{}
		if test.apierr != "" {
			errs = append(errs, errors.New(test.apierr))
		} else {
			errs = append(errs, nil)
		}
		s.mockAPI.SetErrors(errs...)

		showBudget := showbudget.NewShowBudgetCommand()

		ctx, err := cmdtesting.RunCommand(c, showBudget, test.args...)
		if test.err == "" {
			c.Assert(err, jc.ErrorIsNil)
			s.stub.CheckCalls(c, []testing.StubCall{
				{"GetBudget", []interface{}{test.budget}},
			})
			output := cmdtesting.Stdout(ctx)
			c.Assert(output, gc.Equals, test.output)
		} else {
			c.Assert(err, gc.ErrorMatches, test.err)
		}
	}
}

type mockAPI struct {
	*testing.Stub
}

func (api *mockAPI) ModelInfo(tags []names.ModelTag) ([]params.ModelInfoResult, error) {
	return nil, api.NextErr()
}

type mockBudgetAPI struct {
	*testing.Stub
}

func (api *mockBudgetAPI) GetBudget(name string) (*budget.BudgetWithAllocations, error) {
	api.AddCall("GetBudget", name)
	return &budget.BudgetWithAllocations{
		Limit: "4000",
		Total: budget.BudgetTotals{
			Allocated:   "2300",
			Unallocated: "1700",
			Available:   "1190",
			Consumed:    "1110",
			Usage:       "48%",
		},
		Allocations: []budget.Allocation{{
			Owner:    "user.joe",
			Limit:    "1200",
			Consumed: "500",
			Usage:    "42%",
			Model:    "uuid1",
		}, {
			Owner:    "user.jess",
			Limit:    "1000",
			Consumed: "600",
			Usage:    "60%",
			Model:    "uuid2",
		}, {
			Owner:    "user.bob",
			Limit:    "100",
			Consumed: "10",
			Usage:    "10%",
			Model:    "uuid3",
		}}}, api.NextErr()
}
