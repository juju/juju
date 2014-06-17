// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	gittesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/storage"
	statetxn "github.com/juju/juju/state/txn"
	txntesting "github.com/juju/juju/state/txn/testing"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&resourceCatalogSuite{})

type resourceCatalogSuite struct {
	testing.BaseSuite
	gittesting.MgoSuite
	txnRunner  statetxn.Runner
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
	s.txnRunner = statetxn.NewRunner(txn.NewRunner(db.C("resourceTxns")))
	s.rCatalog = storage.NewResourceCatalog(s.collection, s.txnRunner)
}

func (s *resourceCatalogSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *resourceCatalogSuite) assertPut(c *gc.C, expectedNew bool, md5hash, sha256hash string) string {
	rh := &storage.ResourceHash{md5hash, sha256hash}
	id, path, isNew, err := s.rCatalog.Put(rh)
	c.Assert(err, gc.IsNil)
	c.Assert(isNew, gc.Equals, expectedNew)
	c.Assert(id, gc.Not(gc.Equals), "")
	c.Assert(path, gc.Not(gc.Equals), "")
	s.assertGet(c, id, rh, true)
	return id
}

func (s *resourceCatalogSuite) assertGet(c *gc.C, id string, hash *storage.ResourceHash, pending bool) {
	r, err := s.rCatalog.Get(id)
	if pending {
		c.Assert(err, gc.Equals, storage.ErrUploadPending)
		c.Assert(r, gc.IsNil)
		return
	}
	c.Assert(err, gc.IsNil)
	c.Assert(r.ResourceHash, gc.DeepEquals, *hash)
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
	id := s.assertPut(c, true, "md5foo", "sha256foo")
	s.assertRefCount(c, id, 1)
}

func (s *resourceCatalogSuite) TestPutSameHashesIncRefCount(c *gc.C) {
	id := s.assertPut(c, true, "md5foo", "sha256foo")
	s.assertPut(c, false, "md5foo", "sha256foo")
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
	id, path, isNew, err := s.rCatalog.Put(rh)
	c.Assert(err, gc.IsNil)
	c.Assert(isNew, jc.IsTrue)
	c.Assert(path, gc.Not(gc.Equals), "")
	s.assertGet(c, id, rh, true)
}

func (s *resourceCatalogSuite) TestUploadComplete(c *gc.C) {
	rh := &storage.ResourceHash{
		MD5Hash:    "md5foo",
		SHA256Hash: "sha256foo",
	}
	id, _, _, err := s.rCatalog.Put(rh)
	c.Assert(err, gc.IsNil)
	s.assertGet(c, id, rh, true)
	s.rCatalog.UploadComplete(id)
	c.Assert(err, gc.IsNil)
	s.assertGet(c, id, rh, false)
	// A second call works just fine.
	s.rCatalog.UploadComplete(id)
	c.Assert(err, gc.IsNil)
	s.assertGet(c, id, rh, false)
}

func (s *resourceCatalogSuite) TestRemoveOnlyRecord(c *gc.C) {
	id := s.assertPut(c, true, "md5foo", "sha256foo")
	err := s.rCatalog.Remove(id)
	c.Assert(err, gc.IsNil)
	_, err = s.rCatalog.Get(id)
	c.Assert(err, gc.ErrorMatches, `resource with id ".*" not found`)
}

func (s *resourceCatalogSuite) TestRemoveDecRefCount(c *gc.C) {
	id := s.assertPut(c, true, "md5foo", "sha256foo")
	s.assertPut(c, false, "md5foo", "sha256foo")
	s.assertRefCount(c, id, 2)
	err := s.rCatalog.Remove(id)
	c.Assert(err, gc.IsNil)
	s.assertRefCount(c, id, 1)
	// The record still exists.
	rh := &storage.ResourceHash{
		MD5Hash:    "md5foo",
		SHA256Hash: "sha256foo",
	}
	s.assertGet(c, id, rh, true)
}

func (s *resourceCatalogSuite) TestRemoveLastCopy(c *gc.C) {
	id := s.assertPut(c, true, "md5foo", "sha256foo")
	s.assertPut(c, false, "md5foo", "sha256foo")
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

func (s *resourceCatalogSuite) TestPutNewResourceRace(c *gc.C) {
	var firstId string
	beforeFuncs := []func(){
		func() { firstId = s.assertPut(c, true, "md5foo", "sha256foo") },
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, beforeFuncs...).Check()
	rh := &storage.ResourceHash{"md5foo", "sha256foo"}
	id, _, isNew, err := s.rCatalog.Put(rh)
	c.Assert(err, gc.IsNil)
	c.Assert(id, gc.Equals, firstId)
	c.Assert(isNew, jc.IsFalse)
	err = s.rCatalog.UploadComplete(id)
	c.Assert(err, gc.IsNil)
	r, err := s.rCatalog.Get(id)
	c.Assert(err, gc.IsNil)
	s.assertRefCount(c, id, 2)
	c.Assert(r.MD5Hash, gc.Equals, "md5foo")
	c.Assert(r.SHA256Hash, gc.Equals, "sha256foo")
}

func (s *resourceCatalogSuite) TestDeleteResourceRace(c *gc.C) {
	id := s.assertPut(c, true, "md5foo", "sha256foo")
	s.assertPut(c, false, "md5foo", "sha256foo")
	beforeFuncs := []func(){
		func() {
			err := s.rCatalog.Remove(id)
			c.Assert(err, gc.IsNil)
		},
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, beforeFuncs...).Check()
	err := s.rCatalog.Remove(id)
	c.Assert(err, gc.IsNil)
	_, err = s.rCatalog.Get(id)
	c.Assert(err, gc.ErrorMatches, `resource with id ".*" not found`)
}
