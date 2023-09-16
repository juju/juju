// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"os"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type AccountsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store jujuclient.AccountStore
}

var _ = gc.Suite(&AccountsSuite{})

func (s *AccountsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = jujuclient.NewFileClientStore()
	writeTestAccountsFile(c)
}

func (s *AccountsSuite) TestAccountDetailsNoFile(c *gc.C) {
	err := os.Remove(jujuclient.JujuAccountsPath())
	c.Assert(err, jc.ErrorIsNil)
	details, err := s.store.AccountDetails("not-found")
	c.Assert(err, gc.ErrorMatches, "account details for controller not-found not found")
	c.Assert(details, gc.IsNil)
}

func (s *AccountsSuite) TestAccountDetailsControllerNotFound(c *gc.C) {
	details, err := s.store.AccountDetails("not-found")
	c.Assert(err, gc.ErrorMatches, "account details for controller not-found not found")
	c.Assert(details, gc.IsNil)
}

func (s *AccountsSuite) TestAccountDetails(c *gc.C) {
	details, err := s.store.AccountDetails("kontroll")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details, gc.NotNil)
	c.Assert(*details, jc.DeepEquals, kontrollBobRemoteAccountDetails)
}

func (s *AccountsSuite) TestUpdateAccountIgnoresEmptyAccess(c *gc.C) {
	testAccountDetails := jujuclient.AccountDetails{
		User:     "admin",
		Password: "fnord",
	}
	err := s.store.UpdateAccount("ctrl", testAccountDetails)
	c.Assert(err, jc.ErrorIsNil)
	details, err := s.store.AccountDetails("ctrl")
	c.Assert(err, jc.ErrorIsNil)
	testAccountDetails.LastKnownAccess = ctrlAdminAccountDetails.LastKnownAccess
	c.Assert(testAccountDetails.LastKnownAccess, gc.Equals, "superuser")
	c.Assert(*details, jc.DeepEquals, testAccountDetails)
}

func (s *AccountsSuite) TestUpdateAccountNewController(c *gc.C) {
	testAccountDetails := jujuclient.AccountDetails{User: "admin"}
	err := s.store.UpdateAccount("new-controller", testAccountDetails)
	c.Assert(err, jc.ErrorIsNil)
	details, err := s.store.AccountDetails("new-controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*details, jc.DeepEquals, testAccountDetails)
}

func (s *AccountsSuite) TestUpdateAccountOverwrites(c *gc.C) {
	testAccountDetails := jujuclient.AccountDetails{
		User:            "admin",
		Password:        "fnord",
		LastKnownAccess: "add-model",
	}
	for i := 0; i < 2; i++ {
		// Twice so we exercise the code path of updating with
		// identical details.
		err := s.store.UpdateAccount("kontroll", testAccountDetails)
		c.Assert(err, jc.ErrorIsNil)
		details, err := s.store.AccountDetails("kontroll")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(*details, jc.DeepEquals, testAccountDetails)
	}
}

func (s *AccountsSuite) TestRemoveAccountNoFile(c *gc.C) {
	err := os.Remove(jujuclient.JujuAccountsPath())
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.RemoveAccount("not-found")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *AccountsSuite) TestRemoveAccountControllerNotFound(c *gc.C) {
	err := s.store.RemoveAccount("not-found")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *AccountsSuite) TestRemoveAccount(c *gc.C) {
	err := s.store.RemoveAccount("kontroll")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.store.AccountDetails("kontroll")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *AccountsSuite) TestRemoveControllerRemovesaccounts(c *gc.C) {
	store := jujuclient.NewFileClientStore()
	err := store.AddController("kontroll", jujuclient.ControllerDetails{
		ControllerUUID: "abc",
		CACert:         "woop",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = store.RemoveController("kontroll")
	c.Assert(err, jc.ErrorIsNil)

	accounts, err := jujuclient.ReadAccountsFile(jujuclient.JujuAccountsPath())
	c.Assert(err, jc.ErrorIsNil)
	_, ok := accounts["kontroll"]
	c.Assert(ok, jc.IsFalse) // kontroll accounts are removed
}
