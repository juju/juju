// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type BaseCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store *jujuclienttesting.MemStore
}

var _ = gc.Suite(&BaseCommandSuite{})

func (s *BaseCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	//s.PatchEnvironment("JUJU_CLI_VERSION", "")

	s.store = jujuclienttesting.NewMemStore()
	s.store.CurrentControllerName = "foo"
	s.store.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"testing.invalid:1234"},
	}
	s.store.Accounts["foo"] = &jujuclient.ControllerAccounts{
		Accounts: map[string]jujuclient.AccountDetails{
			"bar@local": {User: "bar@local", Password: "hunter2"},
		},
		CurrentAccount: "bar@local",
	}
}

func (s *BaseCommandSuite) TestLoginExpiry(c *gc.C) {
	apiOpen := func(*api.Info, api.DialOpts) (api.Connection, error) {
		return nil, &params.Error{Code: params.CodeLoginExpired, Message: "meep"}
	}
	var cmd modelcmd.JujuCommandBase
	cmd.SetAPIOpen(apiOpen)
	conn, err := cmd.NewAPIRoot(s.store, "foo", "bar@local", "")
	c.Assert(conn, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `login expired

Your login for the "foo" controller has expired.
To log back in, run the following command:

    juju login bar
`)
}
