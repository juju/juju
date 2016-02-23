// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type ControllerSuite struct {
	store             jujuclient.ControllerStore
	controller        jujuclient.ControllerDetails
	anothercontroller jujuclient.ControllerDetails
}

var _ = gc.Suite(&ControllerSuite{})

func (s *ControllerSuite) SetUpTest(c *gc.C) {
	s.controller = jujuclient.ControllerDetails{ControllerUUID: "controller-uuid"}
	s.anothercontroller = jujuclient.ControllerDetails{ControllerUUID: "another-uuid"}
	s.store = &jujuclienttesting.MemStore{
		Controllers: map[string]jujuclient.ControllerDetails{
			"local.controller":        s.controller,
			"anothercontroller":       s.anothercontroller,
			"local.anothercontroller": jujuclient.ControllerDetails{},
		},
	}
}

func (s *ControllerSuite) TestLocalNameFound(c *gc.C) {
	name, controller, err := jujuclient.LocalControllerByName(s.store, "local.controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.DeepEquals, "local.controller")
	c.Assert(controller, gc.DeepEquals, &s.controller)
}

func (s *ControllerSuite) TestLocalNameFallback(c *gc.C) {
	name, controller, err := jujuclient.LocalControllerByName(s.store, "controller")
	c.Assert(name, gc.DeepEquals, "local.controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controller, gc.DeepEquals, &s.controller)
}

func (s *ControllerSuite) TestNonLocalController(c *gc.C) {
	name, controller, err := jujuclient.LocalControllerByName(s.store, "anothercontroller")
	c.Assert(name, gc.DeepEquals, "anothercontroller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controller, gc.DeepEquals, &s.anothercontroller)
}

func (s *ControllerSuite) TestNotFound(c *gc.C) {
	_, _, err := jujuclient.LocalControllerByName(s.store, "foo")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	// We should report on the passed in controller name.
	c.Assert(err, gc.ErrorMatches, ".* foo .*")
}
