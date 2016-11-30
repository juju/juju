// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
)

type RemoteEntitiesSuite struct {
	ConnSuite
}

var _ = gc.Suite(&RemoteEntitiesSuite{})

func (s *RemoteEntitiesSuite) assertExportLocalEntity(c *gc.C, entity names.Tag) string {
	re := s.State.RemoteEntities()
	token, err := re.ExportLocalEntity(entity)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(token, gc.Not(gc.Equals), "")
	return token
}

func (s *RemoteEntitiesSuite) TestExportLocalEntity(c *gc.C) {
	entity := names.NewApplicationTag("mysql")
	token := s.assertExportLocalEntity(c, entity)

	anotherState, err := s.State.ForModel(s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	defer anotherState.Close()

	re := anotherState.RemoteEntities()
	expected, err := re.GetToken(s.State.ModelTag(), entity)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(token, gc.Equals, expected)
}

func (s *RemoteEntitiesSuite) TestExportLocalEntityTwice(c *gc.C) {
	entity := names.NewApplicationTag("mysql")
	expected := s.assertExportLocalEntity(c, entity)
	re := s.State.RemoteEntities()
	token, err := re.ExportLocalEntity(entity)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
	c.Assert(token, gc.Equals, expected)
}

func (s *RemoteEntitiesSuite) TestGetRemoteEntity(c *gc.C) {
	entity := names.NewApplicationTag("mysql")
	token := s.assertExportLocalEntity(c, entity)

	anotherState, err := s.State.ForModel(s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	defer anotherState.Close()

	re := anotherState.RemoteEntities()
	expected, err := re.GetRemoteEntity(s.State.ModelTag(), token)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity, gc.Equals, expected)
}

func (s *RemoteEntitiesSuite) TestRemoveRemoteEntity(c *gc.C) {
	entity := names.NewApplicationTag("mysql")
	token := s.assertExportLocalEntity(c, entity)

	anotherState, err := s.State.ForModel(s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	defer anotherState.Close()

	re := anotherState.RemoteEntities()
	err = re.RemoveRemoteEntity(s.State.ModelTag(), entity)
	c.Assert(err, jc.ErrorIsNil)
	re = s.State.RemoteEntities()
	_, err = re.GetRemoteEntity(s.State.ModelTag(), token)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *RemoteEntitiesSuite) TestImportRemoteEntity(c *gc.C) {
	re := s.State.RemoteEntities()
	entity := names.NewApplicationTag("mysql")
	token := utils.MustNewUUID().String()
	err := re.ImportRemoteEntity(s.State.ModelTag(), entity, token)
	c.Assert(err, jc.ErrorIsNil)

	anotherState, err := s.State.ForModel(s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	defer anotherState.Close()

	re = anotherState.RemoteEntities()
	expected, err := re.GetRemoteEntity(s.State.ModelTag(), token)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity, gc.Equals, expected)
}
