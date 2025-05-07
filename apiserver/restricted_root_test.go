// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc"
)

type restrictedRootSuite struct {
	testing.BaseSuite
	root rpc.Root
}

var _ = tc.Suite(&restrictedRootSuite{})

func (r *restrictedRootSuite) SetUpTest(c *tc.C) {
	r.BaseSuite.SetUpTest(c)
	r.root = apiserver.TestingRestrictedRoot(func(facade, method string) error {
		return nil
	})
}

func (r *restrictedRootSuite) TestAllowedMethod(c *tc.C) {
	caller, err := r.root.FindMethod("Client", 8, "FullStatus")
	c.Check(err, jc.ErrorIsNil)
	c.Check(caller, tc.NotNil)
}

func (r *restrictedRootSuite) TestDisallowedMethod(c *tc.C) {
	r.root = apiserver.TestingRestrictedRoot(func(facade, method string) error {
		if facade == "Client" && method == "FullStatus" {
			return errors.New("blam")
		}
		return nil
	})
	caller, err := r.root.FindMethod("Client", 8, "FullStatus")
	c.Assert(err, tc.ErrorMatches, "blam")
	c.Assert(caller, tc.IsNil)
}

func (r *restrictedRootSuite) TestMethodNonExistentVersion(c *tc.C) {
	caller, err := r.root.FindMethod("Client", 99999999, "WatchAll")
	c.Assert(err, tc.ErrorMatches, `unknown version .+`)
	c.Assert(caller, tc.IsNil)
}

func (r *restrictedRootSuite) TestNonExistentFacade(c *tc.C) {
	caller, err := r.root.FindMethod("SomeFacade", 0, "Method")
	c.Assert(err, tc.ErrorMatches, `unknown facade type "SomeFacade"`)
	c.Assert(caller, tc.IsNil)
}

func (r *restrictedRootSuite) TestNonExistentMethod(c *tc.C) {
	caller, err := r.root.FindMethod("Client", 8, "Bar")
	c.Assert(err, tc.ErrorMatches, `unknown method "Bar" at version 8 for facade type "Client"`)
	c.Assert(caller, tc.IsNil)
}
