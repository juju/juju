// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/backups"
	"github.com/juju/juju/rpc/params"
)

type removeSuite struct {
	baseSuite
}

var _ = gc.Suite(&removeSuite{})

func (s *removeSuite) TestRemove(c *gc.C) {
	cleanup := backups.PatchClientFacadeCall(s.client,
		func(req string, paramsIn interface{}, resp interface{}) error {
			c.Check(req, gc.Equals, "Remove")
			c.Assert(paramsIn, gc.FitsTypeOf, params.BackupsRemoveArgs{})
			c.Assert(resp, gc.FitsTypeOf, &params.ErrorResults{})
			resp.(*params.ErrorResults).Results = []params.ErrorResult{{Error: nil}}
			return nil
		},
	)
	defer cleanup()

	result, err := s.client.Remove(s.Meta.ID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.FitsTypeOf, []params.ErrorResult{})
}

func (s *removeSuite) TestRemoveErrorExpectedResult(c *gc.C) {
	cleanup := backups.PatchClientFacadeCall(s.client,
		func(req string, paramsIn interface{}, resp interface{}) error {
			c.Check(req, gc.Equals, "Remove")
			c.Assert(paramsIn, gc.FitsTypeOf, params.BackupsRemoveArgs{})
			c.Assert(resp, gc.FitsTypeOf, &params.ErrorResults{})
			return nil
		},
	)
	defer cleanup()

	result, err := s.client.Remove(s.Meta.ID())
	c.Assert(err, gc.ErrorMatches, "expected 1 result\\(s\\), got 0")
	c.Assert(result, gc.IsNil)
}
