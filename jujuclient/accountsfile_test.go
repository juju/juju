// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"os"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
)

type AccountsFileSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

func TestAccountsFileSuite(t *stdtesting.T) {
	tc.Run(t, &AccountsFileSuite{})
}

const testLegacyAccountsYAML = `
controllers:
  ctrl:
    user: admin@local
    password: hunter2
    last-known-access: superuser
  kontroll:
    user: bob@remote
`

const testAccountsYAML = `
controllers:
  ctrl:
    user: admin
    password: hunter2
    last-known-access: superuser
  kontroll:
    user: bob@remote
`

var testControllerAccounts = map[string]jujuclient.AccountDetails{
	"ctrl":     ctrlAdminAccountDetails,
	"kontroll": kontrollBobRemoteAccountDetails,
}

var (
	ctrlAdminAccountDetails = jujuclient.AccountDetails{
		User:            "admin",
		Password:        "hunter2",
		LastKnownAccess: "superuser",
	}
	kontrollBobRemoteAccountDetails = jujuclient.AccountDetails{
		User: "bob@remote",
	}
)

func (s *AccountsFileSuite) TestWriteFile(c *tc.C) {
	writeTestAccountsFile(c)
	data, err := os.ReadFile(osenv.JujuXDGDataHomePath("accounts.yaml"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, testAccountsYAML[1:])
}

func (s *AccountsFileSuite) TestReadNoFile(c *tc.C) {
	accounts, err := jujuclient.ReadAccountsFile("nowhere.yaml")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(accounts, tc.IsNil)
}

func (s *AccountsFileSuite) TestReadEmptyFile(c *tc.C) {
	err := os.WriteFile(osenv.JujuXDGDataHomePath("accounts.yaml"), []byte(""), 0600)
	c.Assert(err, tc.ErrorIsNil)
	accounts, err := jujuclient.ReadAccountsFile(jujuclient.JujuAccountsPath())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(accounts, tc.HasLen, 0)
}

func (s *AccountsFileSuite) TestMigrateLegacyLocal(c *tc.C) {
	err := os.WriteFile(jujuclient.JujuAccountsPath(), []byte(testLegacyAccountsYAML), 0644)
	c.Assert(err, tc.ErrorIsNil)

	accounts, err := jujuclient.ReadAccountsFile(jujuclient.JujuAccountsPath())
	c.Assert(err, tc.ErrorIsNil)

	migratedData, err := os.ReadFile(jujuclient.JujuAccountsPath())
	c.Assert(err, tc.ErrorIsNil)
	migratedAccounts, err := jujuclient.ParseAccounts(migratedData)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(migratedData), tc.DeepEquals, testAccountsYAML[1:])
	c.Assert(migratedAccounts, tc.DeepEquals, accounts)
}

func writeTestAccountsFile(c *tc.C) {
	err := jujuclient.WriteAccountsFile(testControllerAccounts)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *AccountsFileSuite) TestParseAccounts(c *tc.C) {
	accounts, err := jujuclient.ParseAccounts([]byte(testAccountsYAML))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(accounts, tc.DeepEquals, testControllerAccounts)
}

func (s *AccountsFileSuite) TestParseAccountMetadataError(c *tc.C) {
	accounts, err := jujuclient.ParseAccounts([]byte("fail me now"))
	c.Assert(err, tc.ErrorMatches,
		"cannot unmarshal accounts: yaml: unmarshal errors:"+
			"\n  line 1: cannot unmarshal !!str `fail me...` into "+
			"jujuclient.accountsCollection",
	)
	c.Assert(accounts, tc.IsNil)
}
