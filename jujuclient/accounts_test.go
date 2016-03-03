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

func (s *AccountsSuite) TestAccountByNameNoFile(c *gc.C) {
	err := os.Remove(jujuclient.JujuAccountsPath())
	c.Assert(err, jc.ErrorIsNil)
	details, err := s.store.AccountByName("not-found", "admin@local")
	c.Assert(err, gc.ErrorMatches, "controller not-found not found")
	c.Assert(details, gc.IsNil)
}

func (s *AccountsSuite) TestAccountByNameControllerNotFound(c *gc.C) {
	details, err := s.store.AccountByName("not-found", "admin@local")
	c.Assert(err, gc.ErrorMatches, "controller not-found not found")
	c.Assert(details, gc.IsNil)
}

func (s *AccountsSuite) TestAccountByNameAccountNotFound(c *gc.C) {
	details, err := s.store.AccountByName("kontroll", "admin@nowhere")
	c.Assert(err, gc.ErrorMatches, "account kontroll:admin@nowhere not found")
	c.Assert(details, gc.IsNil)
}

func (s *AccountsSuite) TestAccountByName(c *gc.C) {
	details, err := s.store.AccountByName("kontroll", "admin@local")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details, gc.NotNil)
	c.Assert(*details, jc.DeepEquals, testControllerAccounts["kontroll"].Accounts["admin@local"])
}

func (s *AccountsSuite) TestAllAccountsNoFile(c *gc.C) {
	err := os.Remove(jujuclient.JujuAccountsPath())
	c.Assert(err, jc.ErrorIsNil)
	accounts, err := s.store.AllAccounts("not-found")
	c.Assert(err, gc.ErrorMatches, "accounts for controller not-found not found")
	c.Assert(accounts, gc.HasLen, 0)
}

func (s *AccountsSuite) TestAllAccounts(c *gc.C) {
	accounts, err := s.store.AllAccounts("kontroll")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(accounts, jc.DeepEquals, testControllerAccounts["kontroll"].Accounts)
}

func (s *AccountsSuite) TestCurrentAccount(c *gc.C) {
	current, err := s.store.CurrentAccount("kontroll")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(current, gc.Equals, "admin@local")
}

func (s *AccountsSuite) TestCurrentAccountNotSet(c *gc.C) {
	_, err := s.store.CurrentAccount("ctrl")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *AccountsSuite) TestCurrentAccountControllerNotFound(c *gc.C) {
	_, err := s.store.CurrentAccount("not-found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *AccountsSuite) TestSetCurrentAccountControllerNotFound(c *gc.C) {
	err := s.store.SetCurrentAccount("not-found", "admin@local")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *AccountsSuite) TestSetCurrentAccountAccountNotFound(c *gc.C) {
	err := s.store.SetCurrentAccount("kontroll", "admin@nowhere")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *AccountsSuite) TestSetCurrentAccount(c *gc.C) {
	err := s.store.SetCurrentAccount("kontroll", "admin@local")
	c.Assert(err, jc.ErrorIsNil)
	accounts, err := jujuclient.ReadAccountsFile(jujuclient.JujuAccountsPath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(accounts["kontroll"].CurrentAccount, gc.Equals, "admin@local")
}

func (s *AccountsSuite) TestUpdateAccountNewController(c *gc.C) {
	testAccountDetails := jujuclient.AccountDetails{User: "admin@local"}
	err := s.store.UpdateAccount("new-controller", "admin@local", testAccountDetails)
	c.Assert(err, jc.ErrorIsNil)
	accounts, err := s.store.AllAccounts("new-controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(accounts, jc.DeepEquals, map[string]jujuclient.AccountDetails{
		"admin@local": testAccountDetails,
	})
}

func (s *AccountsSuite) TestUpdateAccountExistingControllerNewAccount(c *gc.C) {
	testAccountDetails := jujuclient.AccountDetails{User: "bob@environs"}
	err := s.store.UpdateAccount("kontroll", "bob@environs", testAccountDetails)
	c.Assert(err, jc.ErrorIsNil)
	accounts, err := s.store.AllAccounts("kontroll")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(accounts, jc.DeepEquals, map[string]jujuclient.AccountDetails{
		"admin@local":  kontrollAdminAccountDetails,
		"bob@local":    kontrollBobLocalAccountDetails,
		"bob@remote":   kontrollBobRemoteAccountDetails,
		"bob@environs": testAccountDetails,
	})
}

func (s *AccountsSuite) TestUpdateAccountOverwrites(c *gc.C) {
	testAccountDetails := jujuclient.AccountDetails{
		User:     "admin@local",
		Password: "fnord",
	}
	for i := 0; i < 2; i++ {
		// Twice so we exercise the code path of updating with
		// identical details.
		err := s.store.UpdateAccount("kontroll", "admin@local", testAccountDetails)
		c.Assert(err, jc.ErrorIsNil)
		details, err := s.store.AccountByName("kontroll", "admin@local")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(*details, jc.DeepEquals, testAccountDetails)
	}
}

func (s *AccountsSuite) TestRemoveAccountNoFile(c *gc.C) {
	err := os.Remove(jujuclient.JujuAccountsPath())
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.RemoveAccount("not-found", "admin@nowhere")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *AccountsSuite) TestRemoveAccountControllerNotFound(c *gc.C) {
	err := s.store.RemoveAccount("not-found", "admin@nowhere")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *AccountsSuite) TestRemoveAccountNotFound(c *gc.C) {
	err := s.store.RemoveAccount("kontroll", "admin@nowhere")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *AccountsSuite) TestRemoveAccount(c *gc.C) {
	err := s.store.RemoveAccount("kontroll", "admin@local")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.store.AccountByName("kontroll", "admin@local")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *AccountsSuite) TestRemoveControllerRemovesaccounts(c *gc.C) {
	store := jujuclient.NewFileClientStore()
	err := store.RemoveController("kontroll")
	c.Assert(err, jc.ErrorIsNil)

	accounts, err := jujuclient.ReadAccountsFile(jujuclient.JujuAccountsPath())
	c.Assert(err, jc.ErrorIsNil)
	_, ok := accounts["kontroll"]
	c.Assert(ok, jc.IsFalse) // kontroll accounts are removed
}

func (s *AccountsSuite) accountDetails(c *gc.C, controller, account string) jujuclient.AccountDetails {
	details, err := s.store.AccountByName(controller, account)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details, gc.IsNil)
	return *details
}
