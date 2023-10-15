// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	jujutesting "github.com/juju/juju/testing"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type BaseCrossModelSuite struct {
	jujutesting.BaseSuite

	store *jujuclient.MemStore
}

func (s *BaseCrossModelSuite) SetUpTest(c *gc.C) {
	// Set up the current controller, and write just enough info
	// so we don't try to refresh
	controllerName := "test-master"
	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = controllerName
	s.store.Controllers[controllerName] = jujuclient.ControllerDetails{}
	s.store.Models[controllerName] = &jujuclient.ControllerModels{
		CurrentModel: "fred/test",
		Models: map[string]jujuclient.ModelDetails{
			"bob/test":  {ModelUUID: "test-uuid", ModelType: model.IAAS},
			"bob/prod":  {ModelUUID: "prod-uuid", ModelType: model.IAAS},
			"fred/test": {ModelUUID: "fred-uuid", ModelType: model.IAAS},
		},
	}
	s.store.Accounts[controllerName] = jujuclient.AccountDetails{
		User: "bob",
	}
}
