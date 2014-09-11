// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/testing"
)

type restoreRootSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&restoreRootSuite{})

// TODO(perrito666): Uncomment when Restore lands.
//func (r *restoreRootSuite) TestFindAllowedMethodWhenPreparing(c *gc.C) {
//	root := apiserver.TestingAboutToRestoreRoot(nil)
//
//	caller, err := root.FindMethod("Client", 0, "Restore")
//
//	c.Assert(err, gc.IsNil)
//	c.Assert(caller, gc.NotNil)
//}

// TODO(perrito666): Uncomment when Restore lands.
//func (r *restoreRootSuite) TestNothingAllowedMethodWhenPreparing(c *gc.C) {
//	root := apiserver.TestingRestoreInProgressRoot(nil)
//
//	caller, err := root.FindMethod("Client", 0, "Restore")
//
//	c.Assert(err, gc.IsNil)
//	c.Assert(caller, gc.NotNil)
//}

func (r *restoreRootSuite) TestFindDisallowedMethodWhenPreparing(c *gc.C) {
	root := apiserver.TestingAboutToRestoreRoot(nil)

	caller, err := root.FindMethod("Client", 0, "ServiceDeploy")

	c.Assert(err, gc.ErrorMatches, "juju restore is in progress - Juju functionality is limited to avoid data loss")
	c.Assert(caller, gc.IsNil)
}

func (r *restoreRootSuite) TestFindDisallowedMethodWhenRestoring(c *gc.C) {
	root := apiserver.TestingRestoreInProgressRoot(nil)

	caller, err := root.FindMethod("Client", 0, "ServiceDeploy")

	c.Assert(err, gc.ErrorMatches, "juju restore is in progress - Juju api is off to prevent data loss")
	c.Assert(caller, gc.IsNil)
}
