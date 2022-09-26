// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"os"

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

func (s *AccountsFileSuite) TestWriteFile(c *gc.C) {
	writeTestAccountsFile(c)
	data, err := os.ReadFile(osenv.JujuXDGDataHomePath("accounts.yaml"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, testAccountsYAML[1:])
}

func (s *AccountsFileSuite) TestReadNoFile(c *gc.C) {
	accounts, err := jujuclient.ReadAccountsFile("nowhere.yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(accounts, gc.IsNil)
}

func (s *AccountsFileSuite) TestReadEmptyFile(c *gc.C) {
	err := os.WriteFile(osenv.JujuXDGDataHomePath("accounts.yaml"), []byte(""), 0600)
	c.Assert(err, jc.ErrorIsNil)
	accounts, err := jujuclient.ReadAccountsFile(jujuclient.JujuAccountsPath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(accounts, gc.HasLen, 0)
}

func (s *AccountsFileSuite) TestMigrateLegacyLocal(c *gc.C) {
	err := os.WriteFile(jujuclient.JujuAccountsPath(), []byte(testLegacyAccountsYAML), 0644)
	c.Assert(err, jc.ErrorIsNil)

	accounts, err := jujuclient.ReadAccountsFile(jujuclient.JujuAccountsPath())
	c.Assert(err, jc.ErrorIsNil)

	migratedData, err := os.ReadFile(jujuclient.JujuAccountsPath())
	c.Assert(err, jc.ErrorIsNil)
	migratedAccounts, err := jujuclient.ParseAccounts(migratedData)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(migratedData), jc.DeepEquals, testAccountsYAML[1:])
	c.Assert(migratedAccounts, jc.DeepEquals, accounts)
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
