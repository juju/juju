// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"os"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type AccountsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store jujuclient.AccountStore
}

var _ = tc.Suite(&AccountsSuite{})

func (s *AccountsSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = jujuclient.NewFileClientStore()
	writeTestAccountsFile(c)
}

func (s *AccountsSuite) TestAccountDetailsNoFile(c *tc.C) {
	err := os.Remove(jujuclient.JujuAccountsPath())
	c.Assert(err, tc.ErrorIsNil)
	details, err := s.store.AccountDetails("not-found")
	c.Assert(err, tc.ErrorMatches, "account details for controller not-found not found")
	c.Assert(details, tc.IsNil)
}

func (s *AccountsSuite) TestAccountDetailsControllerNotFound(c *tc.C) {
	details, err := s.store.AccountDetails("not-found")
	c.Assert(err, tc.ErrorMatches, "account details for controller not-found not found")
	c.Assert(details, tc.IsNil)
}

func (s *AccountsSuite) TestAccountDetails(c *tc.C) {
	details, err := s.store.AccountDetails("kontroll")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(details, tc.NotNil)
	c.Assert(*details, tc.DeepEquals, kontrollBobRemoteAccountDetails)
}

func (s *AccountsSuite) TestUpdateAccountIgnoresEmptyAccess(c *tc.C) {
	testAccountDetails := jujuclient.AccountDetails{
		User:     "admin",
		Password: "fnord",
	}
	err := s.store.UpdateAccount("ctrl", testAccountDetails)
	c.Assert(err, tc.ErrorIsNil)
	details, err := s.store.AccountDetails("ctrl")
	c.Assert(err, tc.ErrorIsNil)
	testAccountDetails.LastKnownAccess = ctrlAdminAccountDetails.LastKnownAccess
	c.Assert(testAccountDetails.LastKnownAccess, tc.Equals, "superuser")
	c.Assert(*details, tc.DeepEquals, testAccountDetails)
}

func (s *AccountsSuite) TestUpdateAccountNewController(c *tc.C) {
	testAccountDetails := jujuclient.AccountDetails{User: "admin"}
	err := s.store.UpdateAccount("new-controller", testAccountDetails)
	c.Assert(err, tc.ErrorIsNil)
	details, err := s.store.AccountDetails("new-controller")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*details, tc.DeepEquals, testAccountDetails)
}

func (s *AccountsSuite) TestUpdateAccountOverwrites(c *tc.C) {
	testAccountDetails := jujuclient.AccountDetails{
		User:            "admin",
		Password:        "fnord",
		LastKnownAccess: "add-model",
	}
	for i := 0; i < 2; i++ {
		// Twice so we exercise the code path of updating with
		// identical details.
		err := s.store.UpdateAccount("kontroll", testAccountDetails)
		c.Assert(err, tc.ErrorIsNil)
		details, err := s.store.AccountDetails("kontroll")
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(*details, tc.DeepEquals, testAccountDetails)
	}
}

func (s *AccountsSuite) TestRemoveAccountNoFile(c *tc.C) {
	err := os.Remove(jujuclient.JujuAccountsPath())
	c.Assert(err, tc.ErrorIsNil)
	err = s.store.RemoveAccount("not-found")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *AccountsSuite) TestRemoveAccountControllerNotFound(c *tc.C) {
	err := s.store.RemoveAccount("not-found")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *AccountsSuite) TestRemoveAccount(c *tc.C) {
	err := s.store.RemoveAccount("kontroll")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.store.AccountDetails("kontroll")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *AccountsSuite) TestRemoveControllerRemovesaccounts(c *tc.C) {
	store := jujuclient.NewFileClientStore()
	err := store.AddController("kontroll", jujuclient.ControllerDetails{
		ControllerUUID: "abc",
		CACert:         "woop",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = store.RemoveController("kontroll")
	c.Assert(err, tc.ErrorIsNil)

	accounts, err := jujuclient.ReadAccountsFile(jujuclient.JujuAccountsPath())
	c.Assert(err, tc.ErrorIsNil)
	_, ok := accounts["kontroll"]
	c.Assert(ok, tc.IsFalse) // kontroll accounts are removed
}
