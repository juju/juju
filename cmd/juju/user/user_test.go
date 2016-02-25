// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"os"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type BaseSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	configstore configstore.Storage
	store       *jujuclienttesting.MemStore
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	memstore := configstore.NewMem()
	s.configstore = memstore
	s.PatchValue(&configstore.Default, func() (configstore.Storage, error) {
		return memstore, nil
	})
	os.Setenv(osenv.JujuModelEnvKey, "testing")
	info := memstore.CreateInfo("testing:testing")
	info.SetBootstrapConfig(map[string]interface{}{"random": "extra data"})
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses: []string{"127.0.0.1:12345"},
		Hostnames: []string{"localhost:12345"},
		CACert:    testing.CACert,
		ModelUUID: testing.ModelTag.Id(),
	})
	info.SetAPICredentials(configstore.APICredentials{
		User:     "user-test",
		Password: "old-password",
	})
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(user.ReadPassword, func() (string, error) {
		return "sekrit", nil
	})
	err = modelcmd.WriteCurrentController("testing")
	c.Assert(err, jc.ErrorIsNil)
	s.store = jujuclienttesting.NewMemStore()
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Accounts["testing"] = &jujuclient.ControllerAccounts{
		Accounts: map[string]jujuclient.AccountDetails{
			"current-user@local": {
				User:     "current-user@local",
				Password: "old-password",
			},
			"other@local": {
				User:     "other@local",
				Password: "old-password",
			},
		},
		CurrentAccount: "current-user@local",
	}
}
