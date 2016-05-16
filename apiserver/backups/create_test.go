// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/apiserver/backups"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type fakeMachine struct {
}

// Series implements backups.Machine
func (_ *fakeMachine) Series() string {
	return "xenial"
}

func fakeGetMachine(_ *state.State, _ string) (backups.Machine, error) {
	return &fakeMachine{}, nil
}

func (s *backupsSuite) TestCreateOkay(c *gc.C) {
	s.PatchValue(backups.WaitUntilReady,
		func(*mgo.Session, int) error { return nil },
	)
	s.PatchValue(backups.GetMachine, fakeGetMachine)
	s.setBackups(c, s.meta, "")
	var args params.BackupsCreateArgs
	result, err := s.api.Create(args)
	c.Assert(err, jc.ErrorIsNil)
	expected := backups.ResultFromMetadata(s.meta)

	c.Check(result, gc.DeepEquals, expected)
}

func (s *backupsSuite) TestCreateNotes(c *gc.C) {
	s.PatchValue(backups.WaitUntilReady,
		func(*mgo.Session, int) error { return nil },
	)
	s.PatchValue(backups.GetMachine, fakeGetMachine)
	s.meta.Notes = "this backup is important"
	s.setBackups(c, s.meta, "")
	args := params.BackupsCreateArgs{
		Notes: "this backup is important",
	}
	result, err := s.api.Create(args)
	c.Assert(err, jc.ErrorIsNil)
	expected := backups.ResultFromMetadata(s.meta)
	expected.Notes = "this backup is important"

	c.Check(result, gc.DeepEquals, expected)
}

func (s *backupsSuite) TestCreateError(c *gc.C) {
	s.setBackups(c, nil, "failed!")
	s.PatchValue(backups.WaitUntilReady,
		func(*mgo.Session, int) error { return nil },
	)
	s.PatchValue(backups.GetMachine, fakeGetMachine)
	var args params.BackupsCreateArgs
	_, err := s.api.Create(args)

	c.Logf("%v", err)
	c.Check(err, gc.ErrorMatches, "failed!")
}
