// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	_ "github.com/juju/testing/checkers"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/testing"
)

type restoreRootSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&restoreRootSuite{})

func (r *restoreRootSuite) TestFindAllowedMethodWhenPreparing(c *gc.C) {
	root := apiserver.TestingAboutToRestoreRoot(nil)

	// TODO(perrito666): Uncomment when Restore lands and delete
	// the following line.
	//caller, err := root.FindMethod("Backups", 0, "Restore")
	caller, err := root.FindMethod("Client", 0, "FullStatus")

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(caller, gc.NotNil)
}

func (r *restoreRootSuite) TestNothingAllowedMethodWhenPreparing(c *gc.C) {
	root := apiserver.TestingRestoreInProgressRoot(nil)

	caller, err := root.FindMethod("Client", 0, "ServiceDeploy")

	c.Assert(err, gc.ErrorMatches, "juju restore is in progress - Juju api is off to prevent data loss")
	c.Assert(caller, gc.IsNil)
}

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
