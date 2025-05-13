// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import "github.com/juju/tc"

func (s *connSuite) TestConnectionListMachineTypesAPI(c *tc.C) {
	_, err := s.FakeConn.ListMachineTypes("project", "a-zone")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "ListMachineTypes")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "project")
	c.Check(s.FakeConn.Calls[0].ZoneName, tc.Equals, "a-zone")
}
