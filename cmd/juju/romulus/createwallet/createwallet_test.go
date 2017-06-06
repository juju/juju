// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package createwallet_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/romulus/createwallet"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&createWalletSuite{})

type createWalletSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	stub    *testing.Stub
	mockAPI *mockapi
}

func (s *createWalletSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
	s.mockAPI = newMockAPI(s.stub)
	s.PatchValue(createwallet.NewAPIClient, createwallet.APIClientFnc(s.mockAPI))
}

func (s *createWalletSuite) TestCreateWallet(c *gc.C) {
	s.mockAPI.resp = "name wallet set to 5"
	ctx, err := s.runCommand(c, "name", "5")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, "name wallet set to 5\n")
	s.mockAPI.CheckCall(c, 0, "CreateWallet", "name", "5")
}

func (s *createWalletSuite) TestCreateWalletAPIError(c *gc.C) {
	s.mockAPI.SetErrors(errors.New("something failed"))
	_, err := s.runCommand(c, "name", "5")
	c.Assert(err, gc.ErrorMatches, "failed to create the wallet: something failed")
	s.mockAPI.CheckCall(c, 0, "CreateWallet", "name", "5")
}

func (s *createWalletSuite) TestCreateWalletErrors(c *gc.C) {
	tests := []struct {
		about         string
		args          []string
		expectedError string
	}{
		{
			about:         "test value needs to be a number",
			args:          []string{"name", "badvalue"},
			expectedError: "wallet value needs to be a whole number",
		},
		{
			about:         "value is missing",
			args:          []string{"name"},
			expectedError: "name and value required",
		},
		{
			about:         "no args",
			args:          []string{},
			expectedError: "name and value required",
		},
	}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)
		if test.expectedError != "" {
			s.mockAPI.SetErrors(errors.New(test.expectedError))
		}
		_, err := s.runCommand(c, test.args...)
		c.Assert(err, gc.ErrorMatches, test.expectedError)
		s.mockAPI.CheckNoCalls(c)
	}
}

func (s *createWalletSuite) runCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := createwallet.NewCreateWalletCommand()
	cmd.SetClientStore(newMockStore())
	return cmdtesting.RunCommand(c, cmd, args...)
}

func newMockAPI(s *testing.Stub) *mockapi {
	return &mockapi{Stub: s}
}

type mockapi struct {
	*testing.Stub
	resp string
}

func (api *mockapi) CreateWallet(name, value string) (string, error) {
	api.MethodCall(api, "CreateWallet", name, value)
	return api.resp, api.NextErr()
}

func newMockStore() *jujuclient.MemStore {
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"0.1.2.3:1234"},
	}
	return store
}
