// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

func (s *backupsSuite) TestRemoveOkay(c *gc.C) {
	s.setBackups(c, nil, "")
	args := params.BackupsRemoveArgs{
		ID: "some-id",
	}
	err := s.api.Remove(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *backupsSuite) TestRemoveError(c *gc.C) {
	s.setBackups(c, nil, "failed!")
	args := params.BackupsRemoveArgs{
		ID: "some-id",
	}
	err := s.api.Remove(args)

	c.Check(err, gc.ErrorMatches, "failed!")
}
