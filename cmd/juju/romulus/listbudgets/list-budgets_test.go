// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package listbudgets_test

import (
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/romulus/wireformat/budget"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/romulus/listbudgets"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&listBudgetsSuite{})

type listBudgetsSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	stub    *testing.Stub
	mockAPI *mockapi
}

func (s *listBudgetsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
	s.mockAPI = &mockapi{Stub: s.stub}
	s.PatchValue(listbudgets.NewAPIClient, listbudgets.APIClientFnc(s.mockAPI))
}

func (s *listBudgetsSuite) TestUnexpectedParameters(c *gc.C) {
	listBudgets := listbudgets.NewListBudgetsCommand()
	_, err := cmdtesting.RunCommand(c, listBudgets, "unexpected")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["unexpected"\]`)
}

func (s *listBudgetsSuite) TestAPIError(c *gc.C) {
	s.mockAPI.SetErrors(errors.New("well, this is embarrassing"))
	listBudgets := listbudgets.NewListBudgetsCommand()
	_, err := cmdtesting.RunCommand(c, listBudgets)
	c.Assert(err, gc.ErrorMatches, "failed to retrieve budgets: well, this is embarrassing")
}

func (s *listBudgetsSuite) TestListBudgetsOutput(c *gc.C) {
	s.mockAPI.result = &budget.ListBudgetsResponse{
		Budgets: budget.BudgetSummaries{
			budget.BudgetSummary{
				Owner:       "bob",
				Budget:      "personal",
				Limit:       "50",
				Allocated:   "30",
				Unallocated: "20",
				Available:   "45",
				Consumed:    "5",
			},
			budget.BudgetSummary{
				Owner:       "bob",
				Budget:      "work",
				Limit:       "200",
				Allocated:   "100",
				Unallocated: "100",
				Available:   "150",
				Consumed:    "50",
			},
			budget.BudgetSummary{
				Owner:       "bob",
				Budget:      "team",
				Limit:       "50",
				Allocated:   "10",
				Unallocated: "40",
				Available:   "40",
				Consumed:    "10",
			},
		},
		Total: budget.BudgetTotals{
			Limit:       "300",
			Allocated:   "140",
			Available:   "235",
			Unallocated: "160",
			Consumed:    "65",
		},
		Credit: "400",
	}
	// Expected command output. Make sure budgets are sorted alphabetically.
	expected := "" +
		"BUDGET       \tMONTHLY\tALLOCATED\tAVAILABLE\tSPENT\n" +
		"personal     \t     50\t       30\t       45\t    5\n" +
		"team         \t     50\t       10\t       40\t   10\n" +
		"work         \t    200\t      100\t      150\t   50\n" +
		"TOTAL        \t    300\t      140\t      235\t   65\n" +
		"             \t       \t         \t         \t     \n" +
		"Credit limit:\t    400\t         \t         \t     \n"

	listBudgets := listbudgets.NewListBudgetsCommand()

	ctx, err := cmdtesting.RunCommand(c, listBudgets)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, expected)
	s.mockAPI.CheckCallNames(c, "ListBudgets")
}

func (s *listBudgetsSuite) TestListBudgetsOutputNoBudgets(c *gc.C) {
	s.mockAPI.result = &budget.ListBudgetsResponse{
		Budgets: budget.BudgetSummaries{},
		Total: budget.BudgetTotals{
			Limit:       "0",
			Allocated:   "0",
			Available:   "0",
			Unallocated: "0",
			Consumed:    "0",
		},
		Credit: "0",
	}
	expected := "" +
		"BUDGET       \tMONTHLY\tALLOCATED\tAVAILABLE\tSPENT\n" +
		"TOTAL        \t      0\t        0\t        0\t    0\n" +
		"             \t       \t         \t         \t     \n" +
		"Credit limit:\t      0\t         \t         \t     \n"

	listBudgets := listbudgets.NewListBudgetsCommand()

	ctx, err := cmdtesting.RunCommand(c, listBudgets)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, expected)
	s.mockAPI.CheckCallNames(c, "ListBudgets")
}

func (s *listBudgetsSuite) TestListBudgetsNoOutput(c *gc.C) {
	listBudgets := listbudgets.NewListBudgetsCommand()

	ctx, err := cmdtesting.RunCommand(c, listBudgets)
	c.Assert(err, gc.ErrorMatches, `no budget information available`)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, ``)
	s.mockAPI.CheckCallNames(c, "ListBudgets")
}

type mockapi struct {
	*testing.Stub
	result *budget.ListBudgetsResponse
}

func (api *mockapi) ListBudgets() (*budget.ListBudgetsResponse, error) {
	api.AddCall("ListBudgets")
	if err := api.NextErr(); err != nil {
		return nil, err
	}
	return api.result, nil
}
