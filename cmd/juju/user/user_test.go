// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type BaseSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store *jujuclient.MemStore
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "testing"
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{
		APIEndpoints:   []string{"0.1.2.3:12345"},
		CACert:         testing.CACert,
		ControllerUUID: testing.ControllerTag.Id(),
	}
	s.store.Accounts["testing"] = jujuclient.AccountDetails{
		User:     "current-user",
		Password: "old-password",
	}
}

func (s *BaseSuite) setPassword(c *gc.C, controller, pass string) {
	details, ok := s.store.Accounts[controller]
	c.Assert(ok, jc.IsTrue)
	details.Password = pass
	s.store.Accounts[controller] = details
}

func (s *BaseSuite) assertStorePassword(c *gc.C, user, pass, access string) {
	details, err := s.store.AccountDetails("testing")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.User, gc.Equals, user)
	c.Assert(details.Password, gc.Equals, pass)
	c.Assert(details.LastKnownAccess, gc.Equals, access)
}
