// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type BaseSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store *jujuclienttesting.MemStore
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.store = jujuclienttesting.NewMemStore()
	s.store.CurrentControllerName = "testing"
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{
		APIEndpoints:   []string{"127.0.0.1:12345"},
		CACert:         testing.CACert,
		ControllerUUID: testing.ControllerTag.Id(),
	}
	s.store.Accounts["testing"] = jujuclient.AccountDetails{
		User:     "current-user@local",
		Password: "old-password",
	}
}

func (s *BaseSuite) assertStorePassword(c *gc.C, user, pass, access string) {
	details, err := s.store.AccountDetails("testing")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.User, gc.Equals, user)
	c.Assert(details.Password, gc.Equals, pass)
	c.Assert(details.LastKnownAccess, gc.Equals, access)
}

func (s *BaseSuite) assertStoreMacaroon(c *gc.C, user string, mac *macaroon.Macaroon) {
	details, err := s.store.AccountDetails("testing")
	c.Assert(err, jc.ErrorIsNil)
	if mac == nil {
		c.Assert(details.Macaroon, gc.Equals, "")
		return
	}
	macaroonJSON, err := mac.MarshalJSON()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.Macaroon, gc.Equals, string(macaroonJSON))
}
