// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package listwallets_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/romulus/wireformat/budget"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/romulus/listwallets"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&listWalletsSuite{})

type listWalletsSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	stub    *testing.Stub
	mockAPI *mockapi
}

func (s *listWalletsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
	s.mockAPI = &mockapi{Stub: s.stub}
	s.PatchValue(listwallets.NewAPIClient, listwallets.APIClientFnc(s.mockAPI))
}

func (s *listWalletsSuite) TestUnexpectedParameters(c *gc.C) {
	_, err := s.runCommand(c, "unexpected")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["unexpected"\]`)
}

func (s *listWalletsSuite) TestAPIError(c *gc.C) {
	s.mockAPI.SetErrors(errors.New("well, this is embarrassing"))
	_, err := s.runCommand(c)
	c.Assert(err, gc.ErrorMatches, "failed to retrieve wallets: well, this is embarrassing")
}

func (s *listWalletsSuite) TestListWalletsOutput(c *gc.C) {
	s.mockAPI.result = &budget.ListWalletsResponse{
		Wallets: budget.WalletSummaries{
			budget.WalletSummary{
				Owner:       "bob",
				Wallet:      "personal",
				Limit:       "50",
				Budgeted:    "30",
				Unallocated: "20",
				Available:   "45",
				Consumed:    "5",
				Default:     true,
			},
			budget.WalletSummary{
				Owner:       "bob",
				Wallet:      "work",
				Limit:       "200",
				Budgeted:    "100",
				Unallocated: "100",
				Available:   "150",
				Consumed:    "50",
			},
			budget.WalletSummary{
				Owner:       "bob",
				Wallet:      "team",
				Limit:       "50",
				Budgeted:    "10",
				Unallocated: "40",
				Available:   "40",
				Consumed:    "10",
			},
		},
		Total: budget.WalletTotals{
			Limit:       "300",
			Budgeted:    "140",
			Available:   "235",
			Unallocated: "160",
			Consumed:    "65",
		},
		Credit: "400",
	}
	// Expected command output. Make sure wallets are sorted alphabetically.
	expected := "" +
		"Wallet       \tMonthly\tBudgeted\tAvailable\tSpent\n" +
		"personal*    \t     50\t      30\t       45\t    5\n" +
		"team         \t     50\t      10\t       40\t   10\n" +
		"work         \t    200\t     100\t      150\t   50\n" +
		"Total        \t    300\t     140\t      235\t   65\n" +
		"             \t       \t        \t         \t     \n" +
		"Credit limit:\t    400\t        \t         \t     \n"

	ctx, err := s.runCommand(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, expected)
	s.mockAPI.CheckCallNames(c, "ListWallets")
}

func (s *listWalletsSuite) TestListWalletsOutputNoWallets(c *gc.C) {
	s.mockAPI.result = &budget.ListWalletsResponse{
		Wallets: budget.WalletSummaries{},
		Total: budget.WalletTotals{
			Limit:       "0",
			Budgeted:    "0",
			Available:   "0",
			Unallocated: "0",
			Consumed:    "0",
		},
		Credit: "0",
	}
	expected := "" +
		"Wallet       \tMonthly\tBudgeted\tAvailable\tSpent\n" +
		"Total        \t      0\t       0\t        0\t    0\n" +
		"             \t       \t        \t         \t     \n" +
		"Credit limit:\t      0\t        \t         \t     \n"

	ctx, err := s.runCommand(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, expected)
	s.mockAPI.CheckCallNames(c, "ListWallets")
}

func (s *listWalletsSuite) TestListWalletsNoOutput(c *gc.C) {
	ctx, err := s.runCommand(c)
	c.Assert(err, gc.ErrorMatches, `no wallet information available`)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, ``)
	s.mockAPI.CheckCallNames(c, "ListWallets")
}

func (s *listWalletsSuite) runCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := listwallets.NewListWalletsCommand()
	cmd.SetClientStore(newMockStore())
	return cmdtesting.RunCommand(c, cmd, args...)
}

func newMockStore() *jujuclient.MemStore {
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"0.1.2.3:1234"},
	}
	return store
}

type mockapi struct {
	*testing.Stub
	result *budget.ListWalletsResponse
}

func (api *mockapi) ListWallets() (*budget.ListWalletsResponse, error) {
	api.AddCall("ListWallets")
	if err := api.NextErr(); err != nil {
		return nil, err
	}
	return api.result, nil
}
