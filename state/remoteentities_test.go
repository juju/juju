// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"
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

	re := s.State.RemoteEntities()
	expected, err := re.GetToken(entity)
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

	re := s.State.RemoteEntities()
	expected, err := re.GetRemoteEntity(token)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity, gc.Equals, expected)
}

func (s *RemoteEntitiesSuite) TestMacaroon(c *gc.C) {
	entity := names.NewRelationTag("mysql:db wordpress:db")
	s.assertExportLocalEntity(c, entity)

	re := s.State.RemoteEntities()
	mac, err := macaroon.New(nil, "id", "loc")
	c.Assert(err, jc.ErrorIsNil)
	err = re.SaveMacaroon(entity, mac)
	c.Assert(err, jc.ErrorIsNil)

	re = s.State.RemoteEntities()
	expected, err := re.GetMacaroon(entity)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mac, jc.DeepEquals, expected)
}

func (s *RemoteEntitiesSuite) TestRemoveRemoteEntity(c *gc.C) {
	entity := names.NewApplicationTag("mysql")
	token := s.assertExportLocalEntity(c, entity)

	re := s.State.RemoteEntities()
	err := re.RemoveRemoteEntity(entity)
	c.Assert(err, jc.ErrorIsNil)
	re = s.State.RemoteEntities()
	_, err = re.GetRemoteEntity(token)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *RemoteEntitiesSuite) TestImportRemoteEntity(c *gc.C) {
	re := s.State.RemoteEntities()
	entity := names.NewApplicationTag("mysql")
	token := utils.MustNewUUID().String()
	err := re.ImportRemoteEntity(entity, token)
	c.Assert(err, jc.ErrorIsNil)

	re = s.State.RemoteEntities()
	expected, err := re.GetRemoteEntity(token)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity, gc.Equals, expected)
}

func (s *RemoteEntitiesSuite) TestImportRemoteEntityOverwrites(c *gc.C) {
	re := s.State.RemoteEntities()
	entity := names.NewApplicationTag("mysql")
	token := utils.MustNewUUID().String()
	err := re.ImportRemoteEntity(entity, token)
	c.Assert(err, jc.ErrorIsNil)

	anotherToken := utils.MustNewUUID().String()
	err = re.ImportRemoteEntity(entity, anotherToken)
	c.Assert(err, jc.ErrorIsNil)

	re = s.State.RemoteEntities()
	_, err = re.GetRemoteEntity(token)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	expected, err := re.GetRemoteEntity(anotherToken)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity, gc.Equals, expected)
}

func (s *RemoteEntitiesSuite) TestImportRemoteEntityEmptyToken(c *gc.C) {
	re := s.State.RemoteEntities()
	entity := names.NewApplicationTag("mysql")
	err := re.ImportRemoteEntity(entity, "")
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}
