// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/backups"
	"github.com/juju/juju/apiserver/params"
)

type listSuite struct {
	baseSuite
}

var _ = gc.Suite(&listSuite{})

func (s *listSuite) TestList(c *gc.C) {
	cleanup := backups.PatchClientFacadeCall(s.client,
		func(req string, paramsIn interface{}, resp interface{}) error {
			c.Check(req, gc.Equals, "List")

			c.Assert(paramsIn, gc.FitsTypeOf, params.BackupsListArgs{})

			if result, ok := resp.(*params.BackupsListResult); ok {
				result.List = make([]params.BackupsMetadataResult, 1)
				var resultItem params.BackupsMetadataResult
				resultItem.UpdateFromMetadata(s.Meta)
				result.List[0] = resultItem
			} else {
				c.Fatalf("wrong output structure")
			}
			return nil
		},
	)
	defer cleanup()

	result, err := s.client.List()
	c.Assert(err, gc.IsNil)

	c.Assert(result.List, gc.HasLen, 1)
	resultItem := result.List[0]
	s.checkMetadataResult(c, &resultItem, s.Meta)
}
