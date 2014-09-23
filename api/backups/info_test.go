// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/api/backups"
	"github.com/juju/juju/apiserver/params"
)

type infoSuite struct {
	baseBackupsSuite
}

var _ = gc.Suite(&infoSuite{})

func (s *infoSuite) TestInfo(c *gc.C) {
	cleanup := backups.PatchClientFacadeCall(s.client,
		func(req string, paramsIn interface{}, resp interface{}) error {
			c.Check(req, gc.Equals, "Info")

			c.Assert(paramsIn, gc.FitsTypeOf, params.BackupsInfoArgs{})
			p := paramsIn.(params.BackupsInfoArgs)
			c.Check(p.ID, gc.Equals, "spam")

			if result, ok := resp.(*params.BackupsMetadataResult); ok {
				result.UpdateFromMetadata(s.meta)
			} else {
				c.Log("wrong output structure")
				c.Fail()
			}
			return nil
		},
	)
	defer cleanup()

	result, err := s.client.Info("spam")
	c.Assert(err, gc.IsNil)

	s.checkMetadataResult(c, result, s.meta, "")
}
