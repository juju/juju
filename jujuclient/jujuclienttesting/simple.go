// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclienttesting

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
)

// MinimalStore returns a simple store that can be used
// with CLI commands under test.
func MinimalStore() *jujuclient.MemStore {
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "arthur"
	store.Controllers["arthur"] = jujuclient.ControllerDetails{}
	store.Models["arthur"] = &jujuclient.ControllerModels{
		CurrentModel: "king/sword",
		Models: map[string]jujuclient.ModelDetails{"king/sword": {
			ModelType: model.IAAS,
		}},
	}
	store.Accounts["arthur"] = jujuclient.AccountDetails{
		User: "king",
	}
	return store
}

// SetupMinimalFileStore creates a minimal file backed Juju
// ClientStore in the current XDG Juju directory.
func SetupMinimalFileStore(c *tc.C) {
	store := MinimalStore()
	err := jujuclient.WriteControllersFile(&jujuclient.Controllers{
		Controllers:       store.Controllers,
		CurrentController: store.CurrentControllerName,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = jujuclient.WriteModelsFile(store.Models)
	c.Assert(err, tc.ErrorIsNil)
	err = jujuclient.WriteAccountsFile(store.Accounts)
	c.Assert(err, tc.ErrorIsNil)
}
