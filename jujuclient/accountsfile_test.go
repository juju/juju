// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"io/ioutil"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type AccountsFileSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&AccountsFileSuite{})

const testAccountsYAML = `
controllers:
  ctrl:
    accounts:
      admin@local:
        user: admin@local
        password: hunter2
  kontroll:
    accounts:
      admin@local:
        user: admin@local
        password: hunter2
      bob@local:
        user: bob@local
        password: huntert00
      bob@remote:
        user: bob@remote
    current-account: admin@local
`

var testControllerAccounts = map[string]*jujuclient.ControllerAccounts{
	"kontroll": {
		Accounts: map[string]jujuclient.AccountDetails{
			"admin@local": kontrollAdminAccountDetails,
			"bob@local":   kontrollBobLocalAccountDetails,
			"bob@remote":  kontrollBobRemoteAccountDetails,
		},
		CurrentAccount: "admin@local",
	},
	"ctrl": {
		Accounts: map[string]jujuclient.AccountDetails{
			"admin@local": ctrlAdminAccountDetails,
		},
	},
}

var (
	kontrollAdminAccountDetails = jujuclient.AccountDetails{
		User:     "admin@local",
		Password: "hunter2",
	}
	kontrollBobLocalAccountDetails = jujuclient.AccountDetails{
		User:     "bob@local",
		Password: "huntert00",
	}
	kontrollBobRemoteAccountDetails = jujuclient.AccountDetails{
		User: "bob@remote",
	}
	ctrlAdminAccountDetails = jujuclient.AccountDetails{
		User:     "admin@local",
		Password: "hunter2",
	}
)

func (s *AccountsFileSuite) TestWriteFile(c *gc.C) {
	writeTestAccountsFile(c)
	data, err := ioutil.ReadFile(osenv.JujuXDGDataHomePath("accounts.yaml"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, testAccountsYAML[1:])
}

func (s *AccountsFileSuite) TestReadNoFile(c *gc.C) {
	accounts, err := jujuclient.ReadAccountsFile("nowhere.yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(accounts, gc.IsNil)
}

func (s *AccountsFileSuite) TestReadEmptyFile(c *gc.C) {
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("accounts.yaml"), []byte(""), 0600)
	c.Assert(err, jc.ErrorIsNil)
	accounts, err := jujuclient.ReadAccountsFile(jujuclient.JujuAccountsPath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(accounts, gc.HasLen, 0)
}

func writeTestAccountsFile(c *gc.C) {
	err := jujuclient.WriteAccountsFile(testControllerAccounts)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *AccountsFileSuite) TestParseAccounts(c *gc.C) {
	accounts, err := jujuclient.ParseAccounts([]byte(testAccountsYAML))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(accounts, jc.DeepEquals, testControllerAccounts)
}

func (s *AccountsFileSuite) TestParseAccountMetadataError(c *gc.C) {
	accounts, err := jujuclient.ParseAccounts([]byte("fail me now"))
	c.Assert(err, gc.ErrorMatches,
		"cannot unmarshal accounts: yaml: unmarshal errors:"+
			"\n  line 1: cannot unmarshal !!str `fail me...` into "+
			"jujuclient.accountsCollection",
	)
	c.Assert(accounts, gc.IsNil)
}
