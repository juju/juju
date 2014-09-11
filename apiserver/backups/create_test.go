// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/params"
)

func (s *backupsSuite) TestCreateOkay(c *gc.C) {
	s.setBackups(c, s.meta, "")
	var args params.BackupsCreateArgs
	result, err := s.api.Create(args)
	c.Assert(err, gc.IsNil)
	var expected params.BackupsMetadataResult
	expected.UpdateFromMetadata(s.meta)

	c.Check(result, gc.DeepEquals, expected)
}

func (s *backupsSuite) TestCreateNotes(c *gc.C) {
	meta := s.newMeta("this backup is important")
	s.setBackups(c, meta, "")
	args := params.BackupsCreateArgs{
		Notes: "this backup is important",
	}
	result, err := s.api.Create(args)
	c.Assert(err, gc.IsNil)
	var expected params.BackupsMetadataResult
	expected.UpdateFromMetadata(meta)
	expected.Notes = "this backup is important"

	c.Check(result, gc.DeepEquals, expected)
}

func (s *backupsSuite) TestCreateError(c *gc.C) {
	s.setBackups(c, nil, "failed!")
	var args params.BackupsCreateArgs
	_, err := s.api.Create(args)

	c.Check(err, gc.ErrorMatches, "failed!")
}
