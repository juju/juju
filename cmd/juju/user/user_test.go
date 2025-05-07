// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type BaseSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store *jujuclient.MemStore
}

func (s *BaseSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "testing"
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{
		APIEndpoints:   []string{"0.1.2.3:12345"},
		CACert:         testing.CACert,
		ControllerUUID: testing.ControllerTag.Id(),
	}
	s.store.Models["testing"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"adam/test": {ModelUUID: testing.ModelTag.Id(), ModelType: model.IAAS},
		},
	}
	s.store.Accounts["testing"] = jujuclient.AccountDetails{
		User:     "current-user",
		Password: "old-password",
	}
}

func (s *BaseSuite) setPassword(c *tc.C, controller, pass string) {
	details, ok := s.store.Accounts[controller]
	c.Assert(ok, jc.IsTrue)
	details.Password = pass
	s.store.Accounts[controller] = details
}

func (s *BaseSuite) assertStorePassword(c *tc.C, user, pass, access string) {
	details, err := s.store.AccountDetails("testing")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.User, tc.Equals, user)
	c.Assert(details.Password, tc.Equals, pass)
	c.Assert(details.LastKnownAccess, tc.Equals, access)
}
