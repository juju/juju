// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/backups/metadata"
)

func (s *backupsSuite) TestCreateOkay(c *gc.C) {
	s.setImpl(c, s.meta, "")
	args := params.BackupsCreateArgs{}
	result, err := s.api.Create(args)
	c.Assert(err, gc.IsNil)
	expected := params.BackupsMetadataResult{}
	expected.UpdateFromMetadata(s.meta)

	c.Check(result, gc.DeepEquals, expected)
}

func (s *backupsSuite) TestCreateNotes(c *gc.C) {
	origin := metadata.NewOrigin("", "", "")
	meta := metadata.NewMetadata(*origin, "this backup is important", nil)
	s.setImpl(c, meta, "")
	args := params.BackupsCreateArgs{
		Notes: "this backup is important",
	}
	result, err := s.api.Create(args)
	c.Assert(err, gc.IsNil)
	expected := params.BackupsMetadataResult{}
	expected.UpdateFromMetadata(meta)
	expected.Notes = "this backup is important"

	c.Check(result, gc.DeepEquals, expected)
}

func (s *backupsSuite) TestCreateError(c *gc.C) {
	s.setImpl(c, nil, "failed!")
	args := params.BackupsCreateArgs{}
	_, err := s.api.Create(args)

	c.Check(err, gc.ErrorMatches, "failed!")
}
