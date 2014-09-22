// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupstorage_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/filestorage"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backupstorage"
)

type metadataStorageSuite struct {
	baseSuite
	stor filestorage.MetadataStorage
}

var _ = gc.Suite(&metadataStorageSuite{})

func (s *metadataStorageSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	s.stor = backupstorage.NewMetadataStorage(s.State)
}

func (s *metadataStorageSuite) TestSetStoredSuccess(c *gc.C) {
	original := s.metadata(c)
	id, err := s.stor.AddMetadata(original)
	c.Check(err, gc.IsNil)
	metadata, err := s.stor.Metadata(id)
	c.Assert(err, gc.IsNil)
	c.Assert(metadata.Stored(), gc.Equals, false)

	err = s.stor.SetStored(id)
	c.Check(err, gc.IsNil)

	metadata, err = s.stor.Metadata(id)
	c.Assert(err, gc.IsNil)
	c.Assert(metadata.Stored(), gc.Equals, true)
}

func (s *metadataStorageSuite) TestSetStoredNotFound(c *gc.C) {
	err := s.stor.SetStored("spam")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}
