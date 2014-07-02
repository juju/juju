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
	s.collection = db.C("storedResources")
	s.txnRunner = statetxn.NewRunner(txn.NewRunner(db.C("resourceTxns")))
	s.rCatalog = storage.NewResourceCatalog(s.collection, s.txnRunner)
}

func (s *resourceCatalogSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *resourceCatalogSuite) assertPut(c *gc.C, expectedNew bool, md5hash, sha256hash string) (
	id, path string,
) {
	rh := &storage.ResourceHash{md5hash, sha256hash}
	id, path, isNew, err := s.rCatalog.Put(rh, 200)
	c.Assert(err, gc.IsNil)
	c.Assert(isNew, gc.Equals, expectedNew)
	c.Assert(id, gc.Not(gc.Equals), "")
	c.Assert(path, gc.Not(gc.Equals), "")
	s.assertGetPending(c, id)
	return id, path
}

func (s *resourceCatalogSuite) assertGetPending(c *gc.C, id string) {
	r, err := s.rCatalog.Get(id)
	c.Assert(err, gc.Equals, storage.ErrUploadPending)
	c.Assert(r, gc.IsNil)
}

func (s *resourceCatalogSuite) asserGetUploaded(c *gc.C, id string, hash *storage.ResourceHash, length int64) {
	r, err := s.rCatalog.Get(id)
	c.Assert(err, gc.IsNil)
	c.Assert(r.ResourceHash, gc.DeepEquals, *hash)
	c.Assert(r.Length, gc.Equals, length)
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
	id, _ := s.assertPut(c, true, "md5foo", "sha256foo")
	s.assertRefCount(c, id, 1)
}

func (s *resourceCatalogSuite) TestPutLengthMismatch(c *gc.C) {
	id, _ := s.assertPut(c, true, "md5foo", "sha256foo")
	rh := &storage.ResourceHash{"md5foo", "sha256foo"}
	_, _, _, err := s.rCatalog.Put(rh, 100)
	c.Assert(err, gc.ErrorMatches, "length mismatch in resource document 200 != 100")
	s.assertRefCount(c, id, 1)
}

func (s *resourceCatalogSuite) TestPutSameHashesIncRefCount(c *gc.C) {
	id, _ := s.assertPut(c, true, "md5foo", "sha256foo")
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
	id, path, isNew, err := s.rCatalog.Put(rh, 100)
	c.Assert(err, gc.IsNil)
	c.Assert(isNew, jc.IsTrue)
	c.Assert(path, gc.Not(gc.Equals), "")
	s.assertGetPending(c, id)
}

func (s *resourceCatalogSuite) TestFindNonExistent(c *gc.C) {
	rh := &storage.ResourceHash{
		MD5Hash:    "md5foo",
		SHA256Hash: "sha256foo",
	}
	_, err := s.rCatalog.Find(rh)
	c.Assert(err, gc.ErrorMatches, `resource with md5=.*, sha256=.* not found`)
}

func (s *resourceCatalogSuite) TestFind(c *gc.C) {
	rh := &storage.ResourceHash{
		MD5Hash:    "md5foo",
		SHA256Hash: "sha256foo",
	}
	id, path, isNew, err := s.rCatalog.Put(rh, 100)
	c.Assert(err, gc.IsNil)
	c.Assert(isNew, jc.IsTrue)
	c.Assert(path, gc.Not(gc.Equals), "")
	s.rCatalog.UploadComplete(id)
	c.Assert(err, gc.IsNil)
	foundId, err := s.rCatalog.Find(rh)
	c.Assert(err, gc.IsNil)
	c.Assert(foundId, gc.Equals, id)
}

func (s *resourceCatalogSuite) TestUploadComplete(c *gc.C) {
	rh := &storage.ResourceHash{
		MD5Hash:    "md5foo",
		SHA256Hash: "sha256foo",
	}
	id, _, _, err := s.rCatalog.Put(rh, 100)
	c.Assert(err, gc.IsNil)
	s.assertGetPending(c, id)
	s.rCatalog.UploadComplete(id)
	c.Assert(err, gc.IsNil)
	s.asserGetUploaded(c, id, rh, 100)
	// A second call works just fine.
	s.rCatalog.UploadComplete(id)
	c.Assert(err, gc.IsNil)
	s.asserGetUploaded(c, id, rh, 100)
}

func (s *resourceCatalogSuite) TestRemoveOnlyRecord(c *gc.C) {
	id, path := s.assertPut(c, true, "md5foo", "sha256foo")
	wasDeleted, removedPath, err := s.rCatalog.Remove(id)
	c.Assert(err, gc.IsNil)
	c.Assert(wasDeleted, jc.IsTrue)
	c.Assert(removedPath, gc.Equals, path)
	_, err = s.rCatalog.Get(id)
	c.Assert(err, gc.ErrorMatches, `resource with id ".*" not found`)
}

func (s *resourceCatalogSuite) TestRemoveDecRefCount(c *gc.C) {
	id, _ := s.assertPut(c, true, "md5foo", "sha256foo")
	s.assertPut(c, false, "md5foo", "sha256foo")
	s.assertRefCount(c, id, 2)
	wasDeleted, _, err := s.rCatalog.Remove(id)
	c.Assert(err, gc.IsNil)
	c.Assert(wasDeleted, jc.IsFalse)
	s.assertRefCount(c, id, 1)
	s.assertGetPending(c, id)
}

func (s *resourceCatalogSuite) TestRemoveLastCopy(c *gc.C) {
	id, _ := s.assertPut(c, true, "md5foo", "sha256foo")
	s.assertPut(c, false, "md5foo", "sha256foo")
	s.assertRefCount(c, id, 2)
	_, _, err := s.rCatalog.Remove(id)
	c.Assert(err, gc.IsNil)
	s.assertRefCount(c, id, 1)
	_, _, err = s.rCatalog.Remove(id)
	c.Assert(err, gc.IsNil)
	_, err = s.rCatalog.Get(id)
	c.Assert(err, gc.ErrorMatches, `resource with id ".*" not found`)
}

func (s *resourceCatalogSuite) TestRemoveNonExistent(c *gc.C) {
	_, _, err := s.rCatalog.Remove(bson.NewObjectId().Hex())
	c.Assert(err, gc.ErrorMatches, `resource with id ".*" not found`)
}

func (s *resourceCatalogSuite) TestPutNewResourceRace(c *gc.C) {
	var firstId string
	beforeFuncs := []func(){
		func() { firstId, _ = s.assertPut(c, true, "md5foo", "sha256foo") },
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, beforeFuncs...).Check()
	rh := &storage.ResourceHash{"md5foo", "sha256foo"}
	id, _, isNew, err := s.rCatalog.Put(rh, 200)
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
	c.Assert(int(r.Length), gc.Equals, 200)
}

func (s *resourceCatalogSuite) TestPutDeletedResourceRace(c *gc.C) {
	firstId, _ := s.assertPut(c, true, "md5foo", "sha256foo")
	err := s.rCatalog.UploadComplete(firstId)
	c.Assert(err, gc.IsNil)
	beforeFuncs := []func(){
		func() {
			_, _, err := s.rCatalog.Remove(firstId)
			c.Assert(err, gc.IsNil)
		},
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, beforeFuncs...).Check()
	rh := &storage.ResourceHash{"md5foo", "sha256foo"}
	id, _, isNew, err := s.rCatalog.Put(rh, 200)
	c.Assert(err, gc.IsNil)
	c.Assert(isNew, jc.IsTrue)
	c.Assert(firstId, gc.Equals, id)
	err = s.rCatalog.UploadComplete(id)
	c.Assert(err, gc.IsNil)
	r, err := s.rCatalog.Get(id)
	c.Assert(err, gc.IsNil)
	s.assertRefCount(c, id, 1)
	c.Assert(r.MD5Hash, gc.Equals, "md5foo")
	c.Assert(r.SHA256Hash, gc.Equals, "sha256foo")
	c.Assert(r.Length, gc.Equals, int64(200))
}

func (s *resourceCatalogSuite) TestDeleteResourceRace(c *gc.C) {
	id, _ := s.assertPut(c, true, "md5foo", "sha256foo")
	s.assertPut(c, false, "md5foo", "sha256foo")
	beforeFuncs := []func(){
		func() {
			_, _, err := s.rCatalog.Remove(id)
			c.Assert(err, gc.IsNil)
		},
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, beforeFuncs...).Check()
	_, _, err := s.rCatalog.Remove(id)
	c.Assert(err, gc.IsNil)
	_, err = s.rCatalog.Get(id)
	c.Assert(err, gc.ErrorMatches, `resource with id ".*" not found`)
}
