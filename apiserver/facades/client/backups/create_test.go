// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"context"

	"github.com/juju/mgo/v3"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/backups"
	"github.com/juju/juju/rpc/params"
)

func (s *backupsSuite) TestCreateOkay(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.PatchValue(backups.WaitUntilReady,
		func(*mgo.Session, int) error { return nil },
	)
	s.setBackups(s.meta, "")
	var args params.BackupsCreateArgs
	result, err := s.api.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	expected := params.CreateResult(s.meta, "test-filename")

	c.Check(result, gc.DeepEquals, expected)
}

func (s *backupsSuite) TestCreateNotes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.PatchValue(backups.WaitUntilReady,
		func(*mgo.Session, int) error { return nil },
	)
	s.meta.Notes = "this backup is important"
	s.setBackups(s.meta, "")
	args := params.BackupsCreateArgs{
		Notes: "this backup is important",
	}

	result, err := s.api.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	expected := params.CreateResult(s.meta, "test-filename")
	expected.Notes = "this backup is important"

	c.Check(result, gc.DeepEquals, expected)
}

func (s *backupsSuite) TestCreateError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.setBackups(nil, "failed!")
	s.PatchValue(backups.WaitUntilReady,
		func(*mgo.Session, int) error { return nil },
	)
	var args params.BackupsCreateArgs
	_, err := s.api.Create(context.Background(), args)

	c.Logf("%v", err)
	c.Check(err, gc.ErrorMatches, "failed!")
}

func (s *backupsSuite) TestCreateController(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.PatchValue(backups.WaitUntilReady,
		func(*mgo.Session, int) error { return nil },
	)
	s.meta.Controller.UUID = "controller-uuid"
	s.meta.Controller.MachineID = "11"
	s.meta.Controller.MachineInstanceID = "instance-12"
	s.meta.Controller.HANodes = int64(3)
	s.setBackups(s.meta, "")

	result, err := s.api.Create(context.Background(), params.BackupsCreateArgs{})
	c.Assert(err, jc.ErrorIsNil)
	expected := params.CreateResult(s.meta, "test-filename")
	c.Check(result, gc.DeepEquals, expected)
}
