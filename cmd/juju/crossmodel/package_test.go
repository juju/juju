// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type BaseCrossModelSuite struct {
	jujutesting.BaseSuite

	store *jujuclient.MemStore
}

func (s *BaseCrossModelSuite) SetUpTest(c *tc.C) {
	// Set up the current controller, and write just enough info
	// so we don't try to refresh
	controllerName := "test-master"
	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = controllerName
	s.store.Controllers[controllerName] = jujuclient.ControllerDetails{}
	s.store.Models[controllerName] = &jujuclient.ControllerModels{
		CurrentModel: "prod/test",
		Models: map[string]jujuclient.ModelDetails{
			"prod/model":   {ModelUUID: "model-uuid", ModelType: model.IAAS},
			"prod/test":    {ModelUUID: "test-uuid", ModelType: model.IAAS},
			"prod/test2":   {ModelUUID: "test2-uuid", ModelType: model.IAAS},
			"staging/test": {ModelUUID: "test3-uuid", ModelType: model.IAAS},
		},
	}
	s.store.Accounts[controllerName] = jujuclient.AccountDetails{
		User: "bob",
	}
}
