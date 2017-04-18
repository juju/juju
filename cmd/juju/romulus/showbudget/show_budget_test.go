// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.s

package showwallet_test

import (
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/romulus/wireformat/wallet"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/romulus/showwallet"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&showWalletSuite{})

type showWalletSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	stub          *testing.Stub
	mockWalletAPI *mockWalletAPI
	mockAPI       *mockAPI
}

func (s *showWalletSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
	s.mockWalletAPI = &mockWalletAPI{s.stub}
	s.mockAPI = &mockAPI{s.stub}
	s.PatchValue(showwallet.NewWalletAPIClient, showwallet.WalletAPIClientFnc(s.mockWalletAPI))
	s.PatchValue(showwallet.NewJujuclientStore, newMockClientStore)
}

func (s *showWalletSuite) TestShowWalletCommand(c *gc.C) {
	tests := []struct {
		about  string
		args   []string
		err    string
		wallet string
		apierr string
		output string
	}{{
		about: "missing argument",
		err:   `missing arguments`,
	}, {
		about: "unknown arguments",
		args:  []string{"my-special-wallet", "extra", "arguments"},
		err:   `unrecognized args: \["extra" "arguments"\]`,
	}, {
		about:  "api error",
		args:   []string{"personal"},
		apierr: "well, this is embarrassing",
		err:    "failed to retrieve the wallet: well, this is embarrassing",
	}, {
		about:  "all ok",
		args:   []string{"personal"},
		wallet: "personal",
		output: "" +
			"Model      \tSpent\t Budgeted\t       By\tUsage\n" +
			"c:m1       \t500  \t     1200\t user.joe\t42%  \n" +
			"c:m2       \t600  \t     1000\tuser.jess\t60%  \n" +
			"c:m3       \t10   \t      100\t user.bob\t10%  \n" +
			"uuid4      \t10   \t      100\t user.bob\t10%  \n" +
			"           \t     \t         \t         \n" +
			"Total      \t1120 \t     2400\t         \t47%  \n" +
			"Wallet     \t     \t     4000\t         \n" +
			"Unallocated\t     \t     1600\t         \n",
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

		showWallet := showwallet.NewShowWalletCommand()

		ctx, err := cmdtesting.RunCommand(c, showWallet, test.args...)
		if test.err == "" {
			c.Assert(err, jc.ErrorIsNil)
			s.stub.CheckCalls(c, []testing.StubCall{
				{"GetWallet", []interface{}{test.wallet}},
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

type mockWalletAPI struct {
	*testing.Stub
}

func (api *mockWalletAPI) GetWallet(name string) (*wallet.WalletWithBudgets, error) {
	api.AddCall("GetWallet", name)
	return &wallet.WalletWithBudgets{
		Limit: "4000",
		Total: wallet.WalletTotals{
			Budgeted:    "2400",
			Unallocated: "1600",
			Available:   "1180",
			Consumed:    "1120",
			Usage:       "47%",
		},
		Budgets: []wallet.Budget{{
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
		}, {
			Owner:    "user.bob",
			Limit:    "100",
			Consumed: "10",
			Usage:    "10%",
			Model:    "uuid4",
		}}}, api.NextErr()
}

type mockClientStore struct {
	jujuclient.ClientStore
}

func newMockClientStore() jujuclient.ClientStore {
	return &mockClientStore{}
}

func (s *mockClientStore) AllControllers() (map[string]jujuclient.ControllerDetails, error) {
	n := 3
	return map[string]jujuclient.ControllerDetails{
		"c": jujuclient.ControllerDetails{
			ModelCount: &n,
		},
	}, nil
}

func (s *mockClientStore) AllModels(controllerName string) (map[string]jujuclient.ModelDetails, error) {
	return map[string]jujuclient.ModelDetails{
		"m1": jujuclient.ModelDetails{ModelUUID: "uuid1"},
		"m2": jujuclient.ModelDetails{ModelUUID: "uuid2"},
		"m3": jujuclient.ModelDetails{ModelUUID: "uuid3"},
	}, nil
}
