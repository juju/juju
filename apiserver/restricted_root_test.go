// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/testing"
)

type restrictedRootSuite struct {
	testing.BaseSuite
	root rpc.Root
}

var _ = gc.Suite(&restrictedRootSuite{})

func (r *restrictedRootSuite) SetUpTest(c *gc.C) {
	r.BaseSuite.SetUpTest(c)
	r.root = apiserver.TestingRestrictedRoot(func(facade, method string) error {
		if facade == "Client" && method == "FullStatus" {
			return errors.New("blam")
		}
		return nil
	})
}

func (r *restrictedRootSuite) TestAllowedMethod(c *gc.C) {
	caller, err := r.root.FindMethod("Client", 6, "WatchAll")
	c.Check(err, jc.ErrorIsNil)
	c.Check(caller, gc.NotNil)
}

func (r *restrictedRootSuite) TestDisallowedMethod(c *gc.C) {
	caller, err := r.root.FindMethod("Client", 6, "FullStatus")
	c.Assert(err, gc.ErrorMatches, "blam")
	c.Assert(caller, gc.IsNil)
}

func (r *restrictedRootSuite) TestMethodNonExistentVersion(c *gc.C) {
	caller, err := r.root.FindMethod("Client", 99999999, "WatchAll")
	c.Assert(err, gc.ErrorMatches, `unknown version .+`)
	c.Assert(caller, gc.IsNil)
}

func (r *restrictedRootSuite) TestNonExistentFacade(c *gc.C) {
	caller, err := r.root.FindMethod("SomeFacade", 0, "Method")
	c.Assert(err, gc.ErrorMatches, `unknown facade type "SomeFacade"`)
	c.Assert(caller, gc.IsNil)
}

func (r *restrictedRootSuite) TestNonExistentMethod(c *gc.C) {
	caller, err := r.root.FindMethod("Client", 6, "Bar")
	c.Assert(err, gc.ErrorMatches, `unknown method "Bar" at version 6 for facade type "Client"`)
	c.Assert(caller, gc.IsNil)
}
