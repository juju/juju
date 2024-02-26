// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

func (s *connSuite) TestConnectionListMachineTypesAPI(c *gc.C) {
	_, err := s.FakeConn.ListMachineTypes("project", "a-zone")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "ListMachineTypes")
	c.Check(s.FakeConn.Calls[0].ProjectID, gc.Equals, "project")
	c.Check(s.FakeConn.Calls[0].ZoneName, gc.Equals, "a-zone")
}
