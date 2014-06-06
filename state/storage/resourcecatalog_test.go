// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/storage"
	statetxn "github.com/juju/juju/state/txn"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&resourceCatalogSuite{})

type resourceCatalogSuite struct {
	testing.BaseSuite
	testing.MgoSuite
	rCatalog   storage.ResourceCatalog
	collection *mgo.Collection
}

func (s *resourceCatalogSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *resourceCatalogSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *resourceCatalogSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	db := s.Session.DB("juju")
	s.collection = db.C("resourceCatalog")
	txnRunner := statetxn.NewRunner(txn.NewRunner(db.C("txns")))
	s.rCatalog = storage.NewResourceCatalog(s.collection, txnRunner)
}

func (s *resourceCatalogSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *resourceCatalogSuite) assertPut(c *gc.C, md5hash, sha256hash string) string {
	rh := &storage.ResourceHash{md5hash, sha256hash}
	id, path, err := s.rCatalog.Put(rh)
	c.Assert(err, gc.IsNil)
	c.Assert(id, gc.Not(gc.Equals), "")
	c.Assert(path, gc.Not(gc.Equals), "")
	s.assertGet(c, rh, id)
	return id
}

func (s *resourceCatalogSuite) assertGet(c *gc.C, expected *storage.ResourceHash, id string) {
	r, err := s.rCatalog.Get(id)
	c.Assert(err, gc.IsNil)
	c.Assert(r.ResourceHash, gc.DeepEquals, *expected)
	c.Assert(r.Path, gc.Not(gc.Equals), "")
}

type resourceDoc struct {
	Id       bson.ObjectId `bson:"_id"`
	RefCount int64
}

func (s *resourceCatalogSuite) assertRefCount(c *gc.C, id string, expected int64) {
	var doc resourceDoc
	err := s.collection.FindId(id).One(&doc)
	c.Assert(err, gc.IsNil)
	c.Assert(doc.RefCount, gc.Equals, expected)
}

func (s *resourceCatalogSuite) TestPut(c *gc.C) {
	id := s.assertPut(c, "md5foo", "sha256foo")
	s.assertRefCount(c, id, 1)
}

func (s *resourceCatalogSuite) TestPutSameHashesIncRefCount(c *gc.C) {
	id := s.assertPut(c, "md5foo", "sha256foo")
	s.assertPut(c, "md5foo", "sha256foo")
	s.assertRefCount(c, id, 2)
}

func (s *resourceCatalogSuite) TestGetNonExistent(c *gc.C) {
	_, err := s.rCatalog.Get(bson.NewObjectId().Hex())
	c.Assert(err, gc.ErrorMatches, `resource with id ".*" not found`)
}

func (s *resourceCatalogSuite) TestGet(c *gc.C) {
	rh := &storage.ResourceHash{
		MD5Hash:    "md5foo",
		SHA256Hash: "sha256foo",
	}
	id, path, err := s.rCatalog.Put(rh)
	c.Assert(err, gc.IsNil)
	c.Assert(path, gc.Not(gc.Equals), "")
	s.assertGet(c, rh, id)
}

func (s *resourceCatalogSuite) TestRemoveOnlyRecord(c *gc.C) {
	id := s.assertPut(c, "md5foo", "sha256foo")
	err := s.rCatalog.Remove(id)
	c.Assert(err, gc.IsNil)
	_, err = s.rCatalog.Get(id)
	c.Assert(err, gc.ErrorMatches, `resource with id ".*" not found`)
}

func (s *resourceCatalogSuite) TestRemoveDecRefCount(c *gc.C) {
	id := s.assertPut(c, "md5foo", "sha256foo")
	s.assertPut(c, "md5foo", "sha256foo")
	s.assertRefCount(c, id, 2)
	err := s.rCatalog.Remove(id)
	c.Assert(err, gc.IsNil)
	s.assertRefCount(c, id, 1)
	// The record still exists.
	rh := &storage.ResourceHash{
		MD5Hash:    "md5foo",
		SHA256Hash: "sha256foo",
	}
	s.assertGet(c, rh, id)
}

func (s *resourceCatalogSuite) TestRemoveLastCopy(c *gc.C) {
	id := s.assertPut(c, "md5foo", "sha256foo")
	s.assertPut(c, "md5foo", "sha256foo")
	s.assertRefCount(c, id, 2)
	err := s.rCatalog.Remove(id)
	c.Assert(err, gc.IsNil)
	s.assertRefCount(c, id, 1)
	err = s.rCatalog.Remove(id)
	c.Assert(err, gc.IsNil)
	_, err = s.rCatalog.Get(id)
	c.Assert(err, gc.ErrorMatches, `resource with id ".*" not found`)
}

func (s *resourceCatalogSuite) TestRemoveNonExistent(c *gc.C) {
	err := s.rCatalog.Remove(bson.NewObjectId().Hex())
	c.Assert(err, gc.ErrorMatches, `resource with id ".*" not found`)
}
