// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupstorage_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/filestorage"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backups/metadata"
	"github.com/juju/juju/state/backupstorage"
)

type docStorageSuite struct {
	baseSuite
	stor filestorage.DocStorage
}

var _ = gc.Suite(&docStorageSuite{})

func (s *docStorageSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	s.stor = backupstorage.NewDocStorage(s.State)
}

func (s *docStorageSuite) TestDocFound(c *gc.C) {
	expected := s.metadata(c)
	id, err := s.stor.AddDoc(expected)
	c.Assert(err, gc.IsNil)

	meta, err := s.stor.Doc(id)
	c.Check(err, gc.IsNil)

	s.checkMetadata(c, meta, expected, id)
}

func (s *docStorageSuite) TestDocNotFound(c *gc.C) {
	_, err := s.stor.Doc("spam")
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *docStorageSuite) TestAddDocSuccess(c *gc.C) {
	expected := s.metadata(c)
	id, err := s.stor.AddDoc(expected)
	c.Check(err, gc.IsNil)

	meta, err := s.stor.Doc(id)
	c.Assert(err, gc.IsNil)

	s.checkMetadata(c, meta, expected, id)
}

func (s *docStorageSuite) TestAddDocGeneratedID(c *gc.C) {
	expected := s.metadata(c)
	expected.SetID("spam")
	id, err := s.stor.AddDoc(expected)
	c.Check(err, gc.IsNil)

	c.Check(id, gc.Not(gc.Equals), "spam")
}

func (s *docStorageSuite) TestAddDocEmpty(c *gc.C) {
	original := metadata.Metadata{}
	c.Assert(original.Timestamp(), gc.NotNil)
	_, err := s.stor.AddDoc(&original)

	c.Check(err, gc.NotNil)
}

func (s *docStorageSuite) TestAddDocAlreadyExists(c *gc.C) {
	panic("not finished")
	//expected := s.metadata(c)
	//id, err := s.stor.AddDoc(expected)
	//c.Assert(err, gc.IsNil)
	//err = s.stor.AddDocID(s.State, expected, id)

	//c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
}
