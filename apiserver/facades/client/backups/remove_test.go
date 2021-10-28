// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/params"
)

func (s *backupsSuite) TestRemoveOkay(c *gc.C) {
	s.setBackups(c, nil, "")
	args := params.BackupsRemoveArgs{
		IDs: []string{"some-id"},
	}
	results, err := s.api.Remove(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
}

func (s *backupsSuite) TestRemoveError(c *gc.C) {
	s.setBackups(c, nil, "failed!")
	args := params.BackupsRemoveArgs{
		IDs: []string{"some-id"},
	}
	results, err := s.api.Remove(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, gc.ErrorMatches, "failed!")
}
