// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/rpc/params"
)

type serviceLocatorsSuite struct {
	ConnSuite
}

var _ = gc.Suite(&serviceLocatorsSuite{})

func (s *serviceLocatorsSuite) TestServiceLocator(c *gc.C) {
	sl, err := s.State.ServiceLocatorsState().AddServiceLocator(params.AddServiceLocatorParams{
		ServiceLocatorUUID: "test-service-locator-uuid",
		Name:               "test-locator",
		Type:               "l4-service",
		UnitId:             17,
		ConsumerUnitId:     18,
		ConsumerRelationId: 19,
		Params:             map[string]interface{}{"ip-address": "1.1.1.1"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sl.Id(), gc.Equals, "test-service-locator-uuid")
	c.Assert(sl.Name(), gc.Equals, "test-locator")
	c.Assert(sl.Type(), gc.Equals, "l4-service")
	c.Assert(sl.UnitId(), gc.Equals, 17)
	c.Assert(sl.ConsumerUnitId(), gc.Equals, 18)
	c.Assert(sl.ConsumerRelationId(), gc.Equals, 19)

	sl2, err := s.State.ServiceLocatorsState().AddServiceLocator(params.AddServiceLocatorParams{
		ServiceLocatorUUID: "test-service-locator-uuid2",
		Name:               "test-locator2",
		Type:               "l4-service",
		UnitId:             18,
		ConsumerUnitId:     19,
		ConsumerRelationId: 20,
		Params:             map[string]interface{}{"ip-address": "2.2.2.2"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sl2.Id(), gc.Equals, "test-service-locator-uuid2")
	c.Assert(sl2.Name(), gc.Equals, "test-locator2")
	c.Assert(sl2.Type(), gc.Equals, "l4-service")
	c.Assert(sl2.UnitId(), gc.Equals, 18)
	c.Assert(sl2.ConsumerUnitId(), gc.Equals, 19)
	c.Assert(sl2.ConsumerRelationId(), gc.Equals, 20)

	all, err := s.State.ServiceLocatorsState().AllServiceLocators()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 2)
	c.Assert(all[0].Id(), gc.Equals, "test-service-locator-uuid")
	c.Assert(all[0].Name(), gc.Equals, "test-locator")
	c.Assert(all[0].Type(), gc.Equals, "l4-service")
	c.Assert(all[0].UnitId(), gc.Equals, 17)
	c.Assert(all[0].ConsumerUnitId(), gc.Equals, 18)
	c.Assert(all[0].ConsumerRelationId(), gc.Equals, 19)
}
