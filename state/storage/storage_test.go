// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"io/ioutil"
	"strings"

	"github.com/juju/blobstore"
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/storage"
	"github.com/juju/juju/testing"
)

const testUUID = "9f484882-2f18-4fd2-967d-db9663db7bea"

type StorageSuite struct {
	gitjujutesting.MgoSuite
	testing.BaseSuite
	managedStorage blobstore.ManagedStorage
	storage        storage.Storage
}

var _ = gc.Suite(&StorageSuite{})

func (s *StorageSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *StorageSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *StorageSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)

	rs := blobstore.NewGridFS("blobstore", testUUID, s.Session)
	db := s.Session.DB("juju")
	s.managedStorage = blobstore.NewManagedStorage(db, rs)
	s.storage = storage.NewStorage(testUUID, s.Session)
}

func (s *StorageSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *StorageSuite) TestStorageGet(c *gc.C) {
	err := s.managedStorage.PutForEnvironment(testUUID, "abc", strings.NewReader("abc"), 3)
	c.Assert(err, jc.ErrorIsNil)

	r, length, err := s.storage.Get("abc")
	c.Assert(err, jc.ErrorIsNil)
	defer r.Close()
	c.Assert(length, gc.Equals, int64(3))

	data, err := ioutil.ReadAll(r)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "abc")
}

func (s *StorageSuite) TestStoragePut(c *gc.C) {
	err := s.storage.Put("path", strings.NewReader("abcdef"), 3)
	c.Assert(err, jc.ErrorIsNil)

	r, length, err := s.managedStorage.GetForEnvironment(testUUID, "path")
	c.Assert(err, jc.ErrorIsNil)
	defer r.Close()

	c.Assert(length, gc.Equals, int64(3))
	data, err := ioutil.ReadAll(r)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "abc")
}

func (s *StorageSuite) TestStorageRemove(c *gc.C) {
	err := s.storage.Put("path", strings.NewReader("abcdef"), 3)
	c.Assert(err, jc.ErrorIsNil)

	err = s.storage.Remove("path")
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.storage.Get("path")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	err = s.storage.Remove("path")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
