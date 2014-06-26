// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"bytes"
	"io/ioutil"

	"github.com/juju/errors"
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

var _ = gc.Suite(&managedStorageSuite{})

type managedStorageSuite struct {
	testing.BaseSuite
	gittesting.MgoSuite
	txnRunner       statetxn.Runner
	managedStorage  storage.ManagedStorage
	db              *mgo.Database
	resourceStorage storage.ResourceStorage
	collection      *mgo.Collection
}

func (s *managedStorageSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *managedStorageSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *managedStorageSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.db = s.Session.DB("juju")
	s.txnRunner = statetxn.NewRunner(txn.NewRunner(s.db.C("txns")))
	s.resourceStorage = storage.NewGridFS("storage", "test", s.Session)
	s.managedStorage = storage.NewManagedStorage(s.db, s.txnRunner, s.resourceStorage)
}

func (s *managedStorageSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *managedStorageSuite) TestResourceStoragePath(c *gc.C) {
	for _, test := range []struct {
		envUUID     string
		user        string
		path        string
		storagePath string
		error       string
	}{
		{
			envUUID:     "",
			user:        "",
			path:        "/path/to/blob",
			storagePath: "global/path/to/blob",
		}, {
			envUUID:     "env",
			user:        "",
			path:        "/path/to/blob",
			storagePath: "environs/env/path/to/blob",
		}, {
			envUUID:     "",
			user:        "user",
			path:        "/path/to/blob",
			storagePath: "users/user/path/to/blob",
		}, {
			envUUID:     "env",
			user:        "user",
			path:        "/path/to/blob",
			storagePath: "environs/env/users/user/path/to/blob",
		}, {
			envUUID: "env/123",
			user:    "user",
			path:    "/path/to/blob",
			error:   `.* cannot contain "/"`,
		}, {
			envUUID: "env",
			user:    "user/123",
			path:    "/path/to/blob",
			error:   `.* cannot contain "/"`,
		},
	} {
		result, err := storage.ResourceStoragePath(s.managedStorage, test.envUUID, test.user, test.path)
		if test.error == "" {
			c.Check(err, gc.IsNil)
			c.Check(result, gc.Equals, test.storagePath)
		} else {
			c.Check(err, gc.ErrorMatches, test.error)
		}
	}
}

type managedResourceDocStub struct {
	Path       string
	ResourceId string
}

type resourceDocStub struct {
	Path string
}

func (s *managedStorageSuite) TestPendingUpload(c *gc.C) {
	// Manually set up a scenario where there's a resource recorded
	// but the upload has not occurred.
	rc := storage.GetResourceCatalog(s.managedStorage)
	rh := &storage.ResourceHash{"foo", "bar"}
	id, _, _, err := rc.Put(rh)
	c.Assert(err, gc.IsNil)
	managedResource := storage.ManagedResource{
		EnvUUID: "env",
		User:    "user",
		Path:    "environs/env/path/to/blob",
	}
	_, err = storage.PutManagedResource(s.managedStorage, managedResource, id)
	c.Assert(err, gc.IsNil)
	_, err = s.managedStorage.GetForEnvironment("env", "/path/to/blob")
	c.Assert(err, gc.Equals, storage.ErrUploadPending)
}

func (s *managedStorageSuite) assertPut(c *gc.C, path string, blob []byte) string {
	// Put the data.
	rdr := bytes.NewReader(blob)
	err := s.managedStorage.PutForEnvironment("env", path, rdr, int64(len(blob)))
	c.Assert(err, gc.IsNil)

	// Load the managed resource record.
	var mrDoc managedResourceDocStub
	err = s.db.C("managedStoredResources").Find(bson.D{{"path", "environs/env" + path}}).One(&mrDoc)
	c.Assert(err, gc.IsNil)

	// Load the corresponding resource catalog record.
	var rd resourceDocStub
	err = s.db.C("storedResources").FindId(mrDoc.ResourceId).One(&rd)
	c.Assert(err, gc.IsNil)

	// Use the resource catalog record to load the underlying data from storage.
	r, err := s.resourceStorage.Get(rd.Path)
	c.Assert(err, gc.IsNil)
	data, err := ioutil.ReadAll(r)
	c.Assert(err, gc.IsNil)
	c.Assert(data, gc.DeepEquals, blob)
	return rd.Path
}

func (s *managedStorageSuite) assertResourceCatalogCount(c *gc.C, expected int) {
	num, err := s.db.C("storedResources").Count()
	c.Assert(err, gc.IsNil)
	c.Assert(num, gc.Equals, expected)
}

func (s *managedStorageSuite) TestPut(c *gc.C) {
	s.assertPut(c, "/path/to/blob", []byte("some resource"))
	s.assertResourceCatalogCount(c, 1)
}

func (s *managedStorageSuite) TestPutSamePathDifferentData(c *gc.C) {
	resPath := s.assertPut(c, "/path/to/blob", []byte("some resource"))
	secondResPath := s.assertPut(c, "/path/to/blob", []byte("another resource"))
	c.Assert(resPath, gc.Not(gc.Equals), secondResPath)
	s.assertResourceCatalogCount(c, 1)
}

func (s *managedStorageSuite) TestPutDifferentPathSameData(c *gc.C) {
	resPath := s.assertPut(c, "/path/to/blob", []byte("some resource"))
	secondResPath := s.assertPut(c, "/anotherpath/to/blob", []byte("some resource"))
	c.Assert(resPath, gc.Equals, secondResPath)
	s.assertResourceCatalogCount(c, 1)
}

func (s *managedStorageSuite) TestPutSamePathDifferentDataMulti(c *gc.C) {
	resPath := s.assertPut(c, "/path/to/blob", []byte("another resource"))
	secondResPath := s.assertPut(c, "/anotherpath/to/blob", []byte("some resource"))
	c.Assert(resPath, gc.Not(gc.Equals), secondResPath)
	s.assertResourceCatalogCount(c, 2)

	thirdResPath := s.assertPut(c, "/path/to/blob", []byte("some resource"))
	c.Assert(resPath, gc.Not(gc.Equals), secondResPath)
	c.Assert(secondResPath, gc.Equals, thirdResPath)
	s.assertResourceCatalogCount(c, 1)
}

func (s *managedStorageSuite) TestPutManagedResourceFail(c *gc.C) {
	var resourcePath string
	s.PatchValue(storage.PutResourceTxn, func(
		coll *mgo.Collection, managedResource storage.ManagedResource, resourceId string) (string, []txn.Op, error) {
		rc := storage.GetResourceCatalog(s.managedStorage)
		r, err := rc.Get(resourceId)
		c.Assert(err, gc.IsNil)
		resourcePath = r.Path
		return "", nil, errors.Errorf("some error")
	})
	// Attempt to put the data.
	blob := []byte("data")
	rdr := bytes.NewReader(blob)
	err := s.managedStorage.PutForEnvironment("env", "/some/path", rdr, int64(len(blob)))
	c.Assert(err, gc.ErrorMatches, "cannot update managed resource catalog: some error")

	// Now ensure there's no blob data left behind in storage, nor a resource catalog record.
	s.assertResourceCatalogCount(c, 0)
	_, err = s.resourceStorage.Get(resourcePath)
	c.Assert(err, gc.ErrorMatches, ".*not found")
}

func (s *managedStorageSuite) assertGet(c *gc.C, path string, blob []byte) {
	r, err := s.managedStorage.GetForEnvironment("env", path)
	c.Assert(err, gc.IsNil)
	defer r.Close()
	data, err := ioutil.ReadAll(r)
	c.Assert(err, gc.IsNil)
	c.Assert(data, gc.DeepEquals, blob)
}

func (s *managedStorageSuite) TestGet(c *gc.C) {
	blob := []byte("some resource")
	s.assertPut(c, "/path/to/blob", blob)
	s.assertGet(c, "/path/to/blob", blob)
}

func (s *managedStorageSuite) TestGetNonExistent(c *gc.C) {
	_, err := s.managedStorage.GetForEnvironment("env", "/path/to/nowhere")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *managedStorageSuite) TestRemove(c *gc.C) {
	blob := []byte("some resource")
	resPath := s.assertPut(c, "/path/to/blob", blob)
	err := s.managedStorage.RemoveForEnvironment("env", "/path/to/blob")
	c.Assert(err, gc.IsNil)

	// Check the data and catalog entry really are removed.
	_, err = s.managedStorage.GetForEnvironment("env", "path/to/blob")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.resourceStorage.Get(resPath)
	c.Assert(err, gc.NotNil)

	s.assertResourceCatalogCount(c, 0)
}

func (s *managedStorageSuite) TestRemoveNonExistent(c *gc.C) {
	err := s.managedStorage.RemoveForEnvironment("env", "/path/to/nowhere")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *managedStorageSuite) TestRemoveDifferentPathKeepsData(c *gc.C) {
	blob := []byte("some resource")
	s.assertPut(c, "/path/to/blob", blob)
	s.assertPut(c, "/anotherpath/to/blob", blob)
	s.assertResourceCatalogCount(c, 1)
	err := s.managedStorage.RemoveForEnvironment("env", "/path/to/blob")
	c.Assert(err, gc.IsNil)
	s.assertGet(c, "/anotherpath/to/blob", blob)
	s.assertResourceCatalogCount(c, 1)
}

func (s *managedStorageSuite) TestPutRace(c *gc.C) {
	blob := []byte("some resource")
	beforeFunc := func() {
		s.assertPut(c, "/path/to/blob", blob)
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, beforeFunc).Check()
	anotherblob := []byte("another resource")
	s.assertPut(c, "/path/to/blob", anotherblob)
	s.assertResourceCatalogCount(c, 1)
}

func (s *managedStorageSuite) TestPutDeleteRace(c *gc.C) {
	blob := []byte("some resource")
	s.assertPut(c, "/path/to/blob", blob)
	beforeFunc := func() {
		err := s.managedStorage.RemoveForEnvironment("env", "/path/to/blob")
		c.Assert(err, gc.IsNil)
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, beforeFunc).Check()
	anotherblob := []byte("another resource")
	s.assertPut(c, "/path/to/blob", anotherblob)
	s.assertResourceCatalogCount(c, 1)
}

func (s *managedStorageSuite) TestRemoveRace(c *gc.C) {
	blob := []byte("some resource")
	s.assertPut(c, "/path/to/blob", blob)
	beforeFunc := func() {
		err := s.managedStorage.RemoveForEnvironment("env", "/path/to/blob")
		c.Assert(err, gc.IsNil)
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, beforeFunc).Check()
	err := s.managedStorage.RemoveForEnvironment("env", "/path/to/blob")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.managedStorage.GetForEnvironment("env", "/path/to/blob")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
