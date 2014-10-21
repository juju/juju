// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "gopkg.in/check.v1"
)

type RestoreSuite struct{}

var _ = gc.Suite(&RestoreSuite{})

func (s *RestoreSuite) TestSetsPrepareRestore(c *gc.C) {
	r := restoreContext{UnknownRestoreStatus, nil}
	c.Check(r.restorePreparing(), gc.Equals, false)
	c.Check(r.restoreRunning(), gc.Equals, false)
	err := r.PrepareRestore()
	c.Assert(err, gc.IsNil)
	c.Assert(r.restorePreparing(), gc.Equals, true)
	c.Assert(r.restoreRunning(), gc.Equals, false)
	err = r.PrepareRestore()
	c.Assert(err, gc.ErrorMatches, "already in restore mode")
}

func (s *RestoreSuite) TestSetsRestoreInProgress(c *gc.C) {
	r := restoreContext{UnknownRestoreStatus, nil}
	c.Check(r.restorePreparing(), gc.Equals, false)
	c.Check(r.restoreRunning(), gc.Equals, false)
	err := r.PrepareRestore()
	c.Assert(err, gc.IsNil)
	c.Assert(r.restorePreparing(), gc.Equals, true)
	err = r.BeginRestore()
	c.Assert(err, gc.IsNil)
	c.Assert(r.restoreRunning(), gc.Equals, true)
	err = r.BeginRestore()
	c.Assert(err, gc.ErrorMatches, "already restoring")
}

func (s *RestoreSuite) TestRestoreRequiresPrepare(c *gc.C) {
	r := restoreContext{UnknownRestoreStatus, nil}
	c.Check(r.restorePreparing(), gc.Equals, false)
	c.Check(r.restoreRunning(), gc.Equals, false)
	err := r.BeginRestore()
	c.Assert(err, gc.ErrorMatches, "not in restore mode, cannot begin restoration")
	c.Assert(r.restoreRunning(), gc.Equals, false)
}
