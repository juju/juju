// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourcestorage_test

import (
	"crypto/sha512"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/blobstore"
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/txn"
	txntesting "github.com/juju/txn/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/charmresources"
	"github.com/juju/juju/state/resourcestorage"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&ResourceSuite{})

// Note: using bson.Now instead of time.Now because the timed stored
// only has millisecond precision, which is what bson.Now returns.
// We don't care apart from when we're checking the values in tests.
var hammerTime = bson.Now()

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type ResourceSuite struct {
	testing.BaseSuite
	mongo              *gitjujutesting.MgoInstance
	session            *mgo.Session
	storage            charmresources.ResourceManager
	metadataCollection *mgo.Collection

	getTimeFunc func() time.Time
}

func (s *ResourceSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mongo = &gitjujutesting.MgoInstance{}
	s.mongo.Start(nil)

	s.getTimeFunc = func() time.Time {
		return hammerTime
	}

	var err error
	s.session, err = s.mongo.Dial()
	c.Assert(err, jc.ErrorIsNil)
	s.storage = resourcestorage.NewResourceManagerInternal(
		s.session, "my-uuid",
		nil,
		nil,
		s.getTimeFunc,
	)
	s.metadataCollection = resourcestorage.MetadataCollection(s.storage)
}

func (s *ResourceSuite) TearDownTest(c *gc.C) {
	s.session.Close()
	s.mongo.DestroyWithLog()
	s.BaseSuite.TearDownTest(c)
}

func (s *ResourceSuite) TestResourcePut(c *gc.C) {
	s.testResourcePut(c, "some-resource")
}

func (s *ResourceSuite) TestResourcePutReplaces(c *gc.C) {
	s.testResourcePut(c, "abc")
	s.testResourcePut(c, "defghi")
}

func (s *ResourceSuite) testResourcePut(c *gc.C, content string) {
	metaIn := charmresources.Resource{
		Path: "/blob/s/trusty/tahr.gz",
	}
	metaOut, err := s.storage.ResourcePut(metaIn, strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)

	hash := sha512.New384()
	_, err = io.WriteString(hash, content)
	c.Assert(err, jc.ErrorIsNil)

	metaIn.Size = int64(len(content))
	metaIn.SHA384Hash = fmt.Sprintf("%x", hash.Sum(nil))
	metaIn.Created = hammerTime
	c.Assert(metaOut, jc.DeepEquals, metaIn)
	s.assertResource(c, metaOut, content)
}

func (s *ResourceSuite) TestResourceGet(c *gc.C) {
	_, err := s.storage.ResourceGet("/blob/not-there")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `.* resource metadata not found`)

	insertedMetadata := charmresources.Resource{
		Path:       "/blob/s/trusty/tahr.gz",
		Size:       4,
		SHA384Hash: "whatever",
		Created:    bson.Now(),
	}
	s.addMetadataDoc(c, "blob-path", insertedMetadata)
	_, err = s.storage.ResourceGet(insertedMetadata.Path)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `resource at path "environs/my-uuid/blob-path" not found`)

	managedStorage := resourcestorage.ManagedStorage(s.storage, s.session)
	err = managedStorage.PutForEnvironment("my-uuid", "blob-path", strings.NewReader("blah"), -1)
	c.Assert(err, jc.ErrorIsNil)

	r, err := s.storage.ResourceGet(insertedMetadata.Path)
	c.Assert(err, jc.ErrorIsNil)
	defer r[0].Close()
	c.Assert(r[0].Resource, jc.DeepEquals, insertedMetadata)

	data, err := ioutil.ReadAll(r[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "blah")
}

func (s *ResourceSuite) TestResourcePutRemovesExisting(c *gc.C) {
	// Add a metadata doc and a blob at a known path, then
	// call ResourcePut and ensure the original blob is removed.
	originalMetadata := charmresources.Resource{
		Path:       "/blob/s/trusty/tahr.gz",
		Size:       4,
		SHA384Hash: "original-hash",
		Created:    bson.Now(),
	}
	s.addMetadataDoc(c, "blob-path", originalMetadata)

	managedStorage := resourcestorage.ManagedStorage(s.storage, s.session)
	err := managedStorage.PutForEnvironment("my-uuid", "blob-path", strings.NewReader("blah"), -1)
	c.Assert(err, jc.ErrorIsNil)

	updatedMetadata := charmresources.Resource{
		Path: "/blob/s/trusty/tahr.gz",
		Size: 5,
	}
	result, err := s.storage.ResourcePut(updatedMetadata, strings.NewReader("xyzzy"))
	c.Assert(err, jc.ErrorIsNil)

	// old blob should be gone
	_, _, err = managedStorage.GetForEnvironment("my-uuid", "blob-path")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// new data should be in its place
	s.assertResource(c, result, "xyzzy")
}

func (s *ResourceSuite) TestResourcePutRemovesExistingRemoveFails(c *gc.C) {
	s.storage = resourcestorage.NewResourceManagerInternal(
		s.session, "my-uuid",
		resourcestorage.RemoveFailsManagedStorage,
		nil,
		s.getTimeFunc,
	)

	// Add a metadata doc and a blob at a known path, then
	// call ResourcePut and ensure that ResourcePut attempts
	// to remove the original blob, but does not return an
	// error if it fails.
	s.addMetadataDoc(c, "blob-path", charmresources.Resource{
		Path: "/blob/s/trusty/tahr.gz",
	})
	managedStorage := resourcestorage.ManagedStorage(s.storage, s.session)
	err := managedStorage.PutForEnvironment("my-uuid", "blob-path", strings.NewReader("blah"), -1)
	c.Assert(err, jc.ErrorIsNil)

	metaIn := charmresources.Resource{
		Path: "/blob/s/trusty/tahr.gz",
	}
	metaOut, err := s.storage.ResourcePut(metaIn, strings.NewReader("xyzzy"))
	c.Assert(err, jc.ErrorIsNil)

	// old blob should still be there
	r, _, err := managedStorage.GetForEnvironment("my-uuid", "blob-path")
	c.Assert(err, jc.ErrorIsNil)
	r.Close()

	s.assertResource(c, metaOut, "xyzzy")
}

type errorTransactionRunner struct {
	txn.Runner
}

func (errorTransactionRunner) Run(transactions txn.TransactionSource) error {
	return errors.New("Run fails")
}

func getErrorTransactionRunner(db *mgo.Database) txn.Runner {
	runner := txn.NewRunner(txn.RunnerParams{Database: db})
	return errorTransactionRunner{runner}
}

type recordingManagedStorage struct {
	gitjujutesting.Stub
	blobstore.ManagedStorage
}

func (r *recordingManagedStorage) GetForEnvironment(envUUID, path string) (io.ReadCloser, int64, error) {
	r.Stub.MethodCall(r, "GetForEnvironment", envUUID, path)
	return r.ManagedStorage.GetForEnvironment(envUUID, path)
}

func (r *recordingManagedStorage) PutForEnvironment(envUUID, path string, rdr io.Reader, length int64) error {
	r.Stub.MethodCall(r, "PutForEnvironment", envUUID, path, rdr, length)
	return r.ManagedStorage.PutForEnvironment(envUUID, path, rdr, length)
}

func (r *recordingManagedStorage) PutForEnvironmentAndCheckHash(envUUID, path string, rdr io.Reader, length int64, hash string) error {
	r.Stub.MethodCall(r, "PutForEnvironmentAndCheckHash", envUUID, path, rdr, length, hash)
	return r.ManagedStorage.PutForEnvironmentAndCheckHash(envUUID, path, rdr, length, hash)
}

func (r *recordingManagedStorage) RemoveForEnvironment(envUUID, path string) error {
	r.Stub.MethodCall(r, "RemoveForEnvironment", envUUID, path)
	return r.ManagedStorage.RemoveForEnvironment(envUUID, path)
}

func (r *recordingManagedStorage) getBlobPath(c *gc.C, resourcePath string) string {
	c.Assert(r, gc.NotNil)
	putCall := r.Calls()[0]
	blobPath := putCall.Args[1].(string)
	c.Assert(strings.HasPrefix(blobPath, "resources"+resourcePath+":"), jc.IsTrue)
	return blobPath
}

func (s *ResourceSuite) TestResourcePutRemovesBlobOnFailure(c *gc.C) {
	var rms *recordingManagedStorage
	getManagedStorage := func(db *mgo.Database, rs blobstore.ResourceStorage) blobstore.ManagedStorage {
		rms = &recordingManagedStorage{
			ManagedStorage: blobstore.NewManagedStorage(db, rs),
		}
		return rms
	}

	s.storage = resourcestorage.NewResourceManagerInternal(
		s.session, "my-uuid",
		getManagedStorage,
		getErrorTransactionRunner,
		s.getTimeFunc,
	)

	metaIn := charmresources.Resource{
		Path: "/blob/badness",
	}
	_, err := s.storage.ResourcePut(metaIn, strings.NewReader("xyzzy"))
	c.Assert(err, gc.ErrorMatches, "cannot store resource metadata: Run fails")

	blobPath := rms.getBlobPath(c, metaIn.Path)
	managedStorage := resourcestorage.ManagedStorage(s.storage, s.session)
	_, _, err = managedStorage.GetForEnvironment("my-uuid", blobPath)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ResourceSuite) TestResourcePutRemovesBlobOnFailureRemoveFails(c *gc.C) {
	var rms *recordingManagedStorage
	getManagedStorage := func(db *mgo.Database, rs blobstore.ResourceStorage) blobstore.ManagedStorage {
		rms = &recordingManagedStorage{
			ManagedStorage: resourcestorage.RemoveFailsManagedStorage(db, rs),
		}
		return rms
	}
	s.storage = resourcestorage.NewResourceManagerInternal(
		s.session, "my-uuid",
		getManagedStorage,
		getErrorTransactionRunner,
		s.getTimeFunc,
	)

	meta := charmresources.Resource{Path: "/blob/badness"}
	_, err := s.storage.ResourcePut(meta, strings.NewReader("xyzzy"))
	c.Assert(err, gc.ErrorMatches, "cannot store resource metadata: Run fails")

	// blob should still be there, because the removal failed.
	blobPath := rms.getBlobPath(c, meta.Path)
	managedStorage := resourcestorage.ManagedStorage(s.storage, s.session)
	r, _, err := managedStorage.GetForEnvironment("my-uuid", blobPath)
	c.Assert(err, jc.ErrorIsNil)
	r.Close()
}

func (s *ResourceSuite) TestResourcePutSame(c *gc.C) {
	metaIn := charmresources.Resource{Path: "/blob/s/trusty/tahr.gz"}
	metaOut, err := s.storage.ResourcePut(metaIn, strings.NewReader("0"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertResource(c, metaOut, "0")
	_, err = s.storage.ResourcePut(metaIn, strings.NewReader("0"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertResource(c, metaOut, "0") // nothing should have changed
}

func (s *ResourceSuite) TestResourcePutAndJustMetadataExists(c *gc.C) {
	s.addMetadataDoc(c, "blob-path", charmresources.Resource{
		Path:       "/blob/s/trusty/tahr.gz",
		Size:       4,
		SHA384Hash: "whatever",
	})
	n, err := s.metadataCollection.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1)
	s.testResourcePut(c, "abc")
	n, err = s.metadataCollection.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1)
}

func (s *ResourceSuite) TestJustMetadataFails(c *gc.C) {
	s.addMetadataDoc(c, "blob-path", charmresources.Resource{
		Path: "/blob/s/trusty/tahr.gz",
	})
	_, err := s.storage.ResourceGet("/blob/s/trusty/tahr.gz")
	c.Assert(err, gc.ErrorMatches, `resource at path "environs/my-uuid/blob-path" not found`)
}

func (s *ResourceSuite) TestResourcePutConcurrent(c *gc.C) {
	var rms *recordingManagedStorage
	getManagedStorage := func(db *mgo.Database, rs blobstore.ResourceStorage) blobstore.ManagedStorage {
		rms = &recordingManagedStorage{
			ManagedStorage: blobstore.NewManagedStorage(db, rs),
		}
		return rms
	}
	txnRunner := txn.NewRunner(txn.RunnerParams{Database: s.metadataCollection.Database})
	getTxnRunner := func(db *mgo.Database) txn.Runner {
		return txnRunner
	}
	s.storage = resourcestorage.NewResourceManagerInternal(
		s.session, "my-uuid",
		getManagedStorage,
		getTxnRunner,
		s.getTimeFunc,
	)

	metadata0 := charmresources.Resource{
		Path: "/blob/s/trusty/tahr.gz",
		// sha384sum(abc)
		SHA384Hash: "cb00753f45a35e8bb5a03d699ac65007272c32ab0eded1631a8b605a43ff5bed8086072ba1e7cc2358baeca134c825a7",
	}
	metadata1 := charmresources.Resource{
		Path: "/blob/s/trusty/tahr.gz",
		// sha384sum(def)
		SHA384Hash: "180c325cccb299e76ec6c03a5b5a7755af8ef499906dbf531f18d0ca509e4871b0805cac0f122b962d54badc6119f3cf",
	}

	var oldBlobPath string
	resourcePut := func() {
		_, err := s.storage.ResourcePut(metadata0, strings.NewReader("abc"))
		c.Assert(err, jc.ErrorIsNil)
		oldBlobPath = rms.getBlobPath(c, metadata0.Path)
		managedStorage := resourcestorage.ManagedStorage(s.storage, s.session)
		r, _, err := managedStorage.GetForEnvironment("my-uuid", oldBlobPath)
		c.Assert(err, jc.ErrorIsNil)
		r.Close()
	}
	defer txntesting.SetBeforeHooks(c, txnRunner, resourcePut).Check()

	metaOut, err := s.storage.ResourcePut(metadata1, strings.NewReader("def"))
	c.Assert(err, jc.ErrorIsNil)

	// Blob added in before-hook should be removed.
	managedStorage := resourcestorage.ManagedStorage(s.storage, s.session)
	_, _, err = managedStorage.GetForEnvironment("my-uuid", oldBlobPath)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.assertResource(c, metaOut, "def")
}

func (s *ResourceSuite) TestResourcePutExcessiveContention(c *gc.C) {
	var rms *recordingManagedStorage
	getManagedStorage := func(db *mgo.Database, rs blobstore.ResourceStorage) blobstore.ManagedStorage {
		rms = &recordingManagedStorage{
			ManagedStorage: blobstore.NewManagedStorage(db, rs),
		}
		return rms
	}
	txnRunner := txn.NewRunner(txn.RunnerParams{Database: s.metadataCollection.Database})
	getTxnRunner := func(db *mgo.Database) txn.Runner {
		return txnRunner
	}
	s.storage = resourcestorage.NewResourceManagerInternal(
		s.session, "my-uuid",
		getManagedStorage,
		getTxnRunner,
		s.getTimeFunc,
	)

	content := []string{"abc", "def", "ghi", "jkl"}
	metadata := []charmresources.Resource{{
		Path: "/blob/s/trusty/tahr.gz",
		// sha384sum(abc)
		SHA384Hash: "cb00753f45a35e8bb5a03d699ac65007272c32ab0eded1631a8b605a43ff5bed8086072ba1e7cc2358baeca134c825a7",
	}, {
		Path: "/blob/s/trusty/tahr.gz",
		// sha384sum(def)
		SHA384Hash: "180c325cccb299e76ec6c03a5b5a7755af8ef499906dbf531f18d0ca509e4871b0805cac0f122b962d54badc6119f3cf",
	}, {
		Path: "/blob/s/trusty/tahr.gz",
		// sha384sum(ghi)
		SHA384Hash: "1ad66ef0418b7e24de0bf2db0c46e700bd8a705efd781a477f5663561970f418f85a159ead0a6de87f17eba03cb7f542",
	}, {
		Path: "/blob/s/trusty/tahr.gz",
		// sha384sum(jkl)
		SHA384Hash: "264270d5871aa7f6f2a765aa512889fe095bca5335a2bdc60dc60478e85153b348ecd8dc167cf5277356e462f4ba1e97",
	}}

	i := 1
	var blobPaths []string
	metaOut := make([]charmresources.Resource, 1) // placeholder for the outer ResourcePut
	resourcePut := func() {
		if i == 1 {
			// This is for the outer ResourcePut
			blobPaths = []string{rms.getBlobPath(c, metadata[0].Path)}
		}
		meta, err := s.storage.ResourcePut(metadata[i], strings.NewReader(content[i]))
		c.Assert(err, jc.ErrorIsNil)
		blobPaths = append(blobPaths, rms.getBlobPath(c, metadata[i].Path))
		metaOut = append(metaOut, meta)
		i++
	}
	defer txntesting.SetBeforeHooks(c, txnRunner, resourcePut, resourcePut, resourcePut).Check()

	_, err := s.storage.ResourcePut(metadata[0], strings.NewReader(content[0]))
	c.Assert(err, gc.ErrorMatches, "cannot store resource metadata: state changing too quickly; try again soon")

	// There should be no blobs apart from the one added by the last before-hook.
	for i := 0; i < 3; i++ {
		managedStorage := resourcestorage.ManagedStorage(s.storage, s.session)
		_, _, err = managedStorage.GetForEnvironment("my-uuid", blobPaths[i])
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}

	s.assertResource(c, metaOut[3], content[3])
}

func (s *ResourceSuite) TestResourceDelete(c *gc.C) {
	s.addMetadataDoc(c, "blob-path", charmresources.Resource{
		Path:       "/blob/s/trusty/tahr.gz",
		SHA384Hash: "whatever",
		Size:       4,
	})
	managedStorage := resourcestorage.ManagedStorage(s.storage, s.session)
	err := managedStorage.PutForEnvironment("my-uuid", "blob-path", strings.NewReader("blah"), -1)
	c.Assert(err, jc.ErrorIsNil)

	r, err := s.storage.ResourceGet("/blob/s/trusty/tahr.gz")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r, gc.NotNil)
	r[0].Close()

	err = s.storage.ResourceDelete("/blob/s/trusty/tahr.gz")
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = managedStorage.GetForEnvironment("my-uuid", "blob-path")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.storage.ResourceGet("/blob/s/trusty/tahr.gz")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ResourceSuite) TestResourceDeleteNotFound(c *gc.C) {
	err := s.storage.ResourceDelete("/blob/not-found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ResourceSuite) addMetadataDoc(c *gc.C, blobPath string, r charmresources.Resource) {
	attrs, err := charmresources.ParseResourcePath(r.Path)
	c.Assert(err, jc.ErrorIsNil)
	doc := resourcestorage.ResourceMetadataDoc{
		Id:       fmt.Sprintf("my-uuid-%s", r.Path),
		EnvUUID:  "my-uuid",
		SHA384:   r.SHA384Hash,
		Size:     r.Size,
		Created:  r.Created,
		BlobPath: blobPath,
		Type:     attrs.Type,
		User:     attrs.User,
		Org:      attrs.Org,
		Stream:   attrs.Stream,
		Series:   attrs.Series,
		PathName: attrs.PathName,
		Revision: attrs.Revision,
	}
	err = s.metadataCollection.Insert(&doc)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ResourceSuite) assertResource(c *gc.C, meta charmresources.Resource, content string) {
	r, err := s.storage.ResourceGet(meta.Path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r, gc.HasLen, 1)
	c.Assert(r[0], gc.NotNil)
	defer r[0].Close()
	c.Assert(r[0].Resource, jc.DeepEquals, meta)

	data, err := ioutil.ReadAll(r[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, content)
}

func (s *ResourceSuite) createListResourceMetadata(c *gc.C, paths ...string) []charmresources.Resource {
	var resources []charmresources.Resource
	for _, path := range paths {
		resource := charmresources.Resource{Path: path}
		s.addMetadataDoc(c, "blob-path", resource)
		resources = append(resources, resource)
	}
	return resources
}

func (s *ResourceSuite) TestListAllResources(c *gc.C) {
	expected := s.createListResourceMetadata(c,
		"/blob/s/trusty/tahr.gz",
		"/zip/u/doo/dah",
		"/blob/org/anic/ally",
	)
	metadata, err := s.storage.ResourceList(charmresources.ResourceAttributes{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, jc.SameContents, expected)
}

func (s *ResourceSuite) TestResourceListByType(c *gc.C) {
	all := s.createListResourceMetadata(c,
		"/blob/s/trusty/tahr.gz",
		"/zip/u/doo/dah",
		"/blob/org/anic/ally",
	)
	metadata, err := s.storage.ResourceList(charmresources.ResourceAttributes{Type: "blob"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, jc.SameContents, []charmresources.Resource{all[0], all[2]})
}

func (s *ResourceSuite) TestResourceListBySeries(c *gc.C) {
	all := s.createListResourceMetadata(c,
		"/blob/s/trusty/tahr.gz",
		"/zip/u/doo/dah",
		"/blob/org/anic/ally",
	)
	metadata, err := s.storage.ResourceList(charmresources.ResourceAttributes{Series: "trusty"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, jc.SameContents, []charmresources.Resource{all[0]})
}

func (s *ResourceSuite) TestResourceListByStream(c *gc.C) {
	all := s.createListResourceMetadata(c,
		"/blob/c/released/hounds",
		"/blob/c/debug/isbuggy",
	)
	metadata, err := s.storage.ResourceList(charmresources.ResourceAttributes{Stream: "released"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, jc.SameContents, []charmresources.Resource{all[0]})
}

func (s *ResourceSuite) TestResourceListByUser(c *gc.C) {
	all := s.createListResourceMetadata(c,
		"/blob/u/sir/areanidiot",
		"/blob/u/surper/throne",
	)
	metadata, err := s.storage.ResourceList(charmresources.ResourceAttributes{User: "sir"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, jc.SameContents, []charmresources.Resource{all[0]})
}

func (s *ResourceSuite) TestResourceListByOrg(c *gc.C) {
	all := s.createListResourceMetadata(c,
		"/blob/org/anic/ally",
		"/blob/org/anis/m",
		"/blob/org/anis/t",
	)
	metadata, err := s.storage.ResourceList(charmresources.ResourceAttributes{Org: "anis"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, jc.SameContents, all[1:])
}

func (s *ResourceSuite) TestResourceListByPathName(c *gc.C) {
	all := s.createListResourceMetadata(c,
		"/blob/c/release/thingy",
		"/blob/c/debug/thingy",
		"/blob/c/debug/thingy2",
	)
	metadata, err := s.storage.ResourceList(charmresources.ResourceAttributes{PathName: "thingy"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, jc.SameContents, all[:2])
}

func (s *ResourceSuite) TestResourceListByRevision(c *gc.C) {
	all := s.createListResourceMetadata(c,
		"/blob/c/release/thingy/0.0.1-r2",
		"/blob/c/debug/thingy/0.0.1-r2",
		"/blob/c/debug/thingy2/0.0.1-r3",
	)
	metadata, err := s.storage.ResourceList(charmresources.ResourceAttributes{Revision: "0.0.1-r2"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, jc.SameContents, all[:2])
}

func (s *ResourceSuite) TestResourceListNoMatch(c *gc.C) {
	s.createListResourceMetadata(c,
		"/blob/c/released/hounds",
		"/blob/c/debug/isbuggy",
	)
	metadata, err := s.storage.ResourceList(charmresources.ResourceAttributes{Stream: "fnord"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, jc.SameContents, []charmresources.Resource{})
}

func (s *ResourceSuite) TestResourceListMultiFilter(c *gc.C) {
	all := s.createListResourceMetadata(c,
		"/blob/c/released/s/trusty/hounds",
		"/blob/c/released/s/trusty/c/kraken",
		"/blob/c/debug/s/trusty/hounds",
		"/blob/c/released/s/vivid/hounds",
		"/blob/c/debug/s/vivid/isbuggy",
	)
	metadata, err := s.storage.ResourceList(charmresources.ResourceAttributes{
		Series: "trusty", Stream: "released",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, jc.SameContents, all[:2])
}
