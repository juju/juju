// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/backups"
	"github.com/juju/juju/apiserver/params"
)

type removeSuite struct {
	backupsSuite
}

var _ = gc.Suite(&removeSuite{})

func (s *removeSuite) TestRemove(c *gc.C) {
	cleanup := backups.PatchClientFacadeCall(s.client,
		func(req string, paramsIn interface{}, resp interface{}) error {
			c.Check(req, gc.Equals, "Remove")

			c.Assert(paramsIn, gc.FitsTypeOf, params.BackupsRemoveArgs{})

			c.Check(resp, gc.IsNil)
			return nil
		},
	)
	defer cleanup()

	err := s.client.Remove(s.Meta.ID())
	c.Assert(err, jc.ErrorIsNil)
}
