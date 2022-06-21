// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

type serviceLocatorsSuite struct {
	ConnSuite

	suspendedRel *state.Relation
	activeRel    *state.Relation
}

var _ = gc.Suite(&serviceLocatorsSuite{})

func (s *serviceLocatorsSuite) TestAddServiceLocator(c *gc.C) {
	oc, err := s.State.ServiceLocators().AddServiceLocator(state.AddServiceLocatorParams{
		ServiceLocatorUUID: "test-service-locator-uuid",
		Name:               "test-locator",
		Type:               "l4-service",
		UnitId:             17,
		Params:             map[string]interface{}{"ip-address": "1.1.1.1"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(oc.Id(), gc.Equals, "test-service-locator-uuid")
	c.Assert(oc.Name(), gc.Equals, "test-locator")
	c.Assert(oc.Type(), gc.Equals, "l4-service")
	c.Assert(oc.UnitId(), gc.Equals, 17)
	c.Assert(oc.Params(), gc.Equals, map[string]interface{}{"ip-address": "1.1.1.1"})

	all, err := s.State.ServiceLocators().AllServiceLocators()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 2)
	c.Assert(oc.Id(), gc.Equals, "test-service-locator-uuid")
	c.Assert(oc.Name(), gc.Equals, "test-locator")
	c.Assert(oc.Type(), gc.Equals, "l4-service")
	c.Assert(oc.UnitId(), gc.Equals, 17)
	c.Assert(oc.Params(), gc.Equals, map[string]interface{}{"ip-address": "1.1.1.1"})
}
