// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	_ "github.com/juju/testing/checkers"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/testing"
)

type restrictRestoreSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&restrictRestoreSuite{})

func (r *restrictRestoreSuite) TestAllowed(c *gc.C) {
	root := apiserver.TestingAboutToRestoreRoot()
	caller, err := root.FindMethod("Backups", 1, "Restore")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(caller, gc.NotNil)
}

func (r *restrictRestoreSuite) TestNotAllowed(c *gc.C) {
	root := apiserver.TestingAboutToRestoreRoot()
	caller, err := root.FindMethod("Application", 1, "Deploy")
	c.Assert(err, gc.ErrorMatches, "juju restore is in progress - functionality is limited to avoid data loss")
	c.Assert(caller, gc.IsNil)
}
