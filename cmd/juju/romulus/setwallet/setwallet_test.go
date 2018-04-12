// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package setwallet_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/romulus/setwallet"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&setWalletSuite{})

type setWalletSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	stub    *testing.Stub
	mockAPI *mockapi
}

func (s *setWalletSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
	s.mockAPI = newMockAPI(s.stub)
	s.PatchValue(setwallet.NewAPIClient, setwallet.APIClientFnc(s.mockAPI))
}

func (s *setWalletSuite) TestSetWallet(c *gc.C) {
	s.mockAPI.resp = "name wallet set to 5"
	ctx, err := s.runCommand(c, "name", "5")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, "name wallet set to 5\n")
	s.mockAPI.CheckCall(c, 0, "SetWallet", "name", "5")
}

func (s *setWalletSuite) TestSetWalletAPIError(c *gc.C) {
	s.stub.SetErrors(errors.New("something failed"))

	_, err := s.runCommand(c, "name", "5")
	c.Assert(err, gc.ErrorMatches, "failed to set the wallet: something failed")
	s.mockAPI.CheckCall(c, 0, "SetWallet", "name", "5")
}

func (s *setWalletSuite) runCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := setwallet.NewSetWalletCommand()
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

func (s *setWalletSuite) TestSetWalletErrors(c *gc.C) {
	tests := []struct {
		about         string
		args          []string
		expectedError string
	}{
		{
			about:         "value needs to be a number",
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
		s.stub.SetErrors(errors.New(test.expectedError))
		defer s.mockAPI.ResetCalls()
		_, err := s.runCommand(c, test.args...)
		c.Assert(err, gc.ErrorMatches, test.expectedError)
		s.mockAPI.CheckNoCalls(c)
	}
}

func newMockAPI(s *testing.Stub) *mockapi {
	return &mockapi{Stub: s}
}

type mockapi struct {
	*testing.Stub
	resp string
}

func (api *mockapi) SetWallet(name, value string) (string, error) {
	api.MethodCall(api, "SetWallet", name, value)
	return api.resp, api.NextErr()
}
