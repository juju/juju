// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"math/rand"
	"sync"
	"time"

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
	id, _, _, err := rc.Put(rh, 100)
	c.Assert(err, gc.IsNil)
	managedResource := storage.ManagedResource{
		EnvUUID: "env",
		User:    "user",
		Path:    "environs/env/path/to/blob",
	}
	_, err = storage.PutManagedResource(s.managedStorage, managedResource, id)
	c.Assert(err, gc.IsNil)
	_, _, err = s.managedStorage.GetForEnvironment("env", "/path/to/blob")
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
	r, length, err := s.managedStorage.GetForEnvironment("env", path)
	c.Assert(err, gc.IsNil)
	defer r.Close()
	data, err := ioutil.ReadAll(r)
	c.Assert(err, gc.IsNil)
	c.Assert(data, gc.DeepEquals, blob)
	c.Assert(int(length), gc.Equals, len(blob))
}

func (s *managedStorageSuite) TestGet(c *gc.C) {
	blob := []byte("some resource")
	s.assertPut(c, "/path/to/blob", blob)
	s.assertGet(c, "/path/to/blob", blob)
}

func (s *managedStorageSuite) TestGetNonExistent(c *gc.C) {
	_, _, err := s.managedStorage.GetForEnvironment("env", "/path/to/nowhere")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *managedStorageSuite) TestRemove(c *gc.C) {
	blob := []byte("some resource")
	resPath := s.assertPut(c, "/path/to/blob", blob)
	err := s.managedStorage.RemoveForEnvironment("env", "/path/to/blob")
	c.Assert(err, gc.IsNil)

	// Check the data and catalog entry really are removed.
	_, _, err = s.managedStorage.GetForEnvironment("env", "path/to/blob")
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

func (s *managedStorageSuite) TestPutRaceWhereCatalogEntryRemoved(c *gc.C) {
	blob := []byte("some resource")
	// Remove the resource catalog entry with the resourceId that we are about
	// to write to a managed resource entry.
	beforeFunc := []func(){
		nil,
		nil,
		func() {
			// Shamelessly exploit our knowledge of how ids are made.
			md5hash, sha256hash := calculateCheckSums(c, 0, int64(len(blob)), blob)
			_, _, err := storage.GetResourceCatalog(s.managedStorage).Remove(md5hash + sha256hash)
			c.Assert(err, gc.IsNil)
		},
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, beforeFunc...).Check()
	rdr := bytes.NewReader(blob)
	err := s.managedStorage.PutForEnvironment("env", "/path/to/blob", rdr, int64(len(blob)))
	c.Assert(err, gc.ErrorMatches, "unexpected deletion .*")
	s.assertResourceCatalogCount(c, 0)
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
	_, _, err = s.managedStorage.GetForEnvironment("env", "/path/to/blob")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *managedStorageSuite) TestPutRequestNotFound(c *gc.C) {
	hash := storage.ResourceHash{"md5", "sha256"}
	_, err := s.managedStorage.PutRequestForEnvironment("env", "path/to/blob", hash)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *managedStorageSuite) putTestBlob(c *gc.C, path string) (blob []byte, md5hashHex, sha256hashHex string) {
	id := bson.NewObjectId().Hex()
	blob = []byte(id)
	rdr := bytes.NewReader(blob)
	err := s.managedStorage.PutForEnvironment("env", path, rdr, int64(len(blob)))
	c.Assert(err, gc.IsNil)
	s.assertGet(c, path, blob)
	md5hashHex, sha256hashHex = calculateCheckSums(c, 0, int64(len(blob)), blob)
	return blob, md5hashHex, sha256hashHex
}

func calculateCheckSums(c *gc.C, start, length int64, blob []byte) (md5hashHex, sha256hashHex string) {
	data := blob[start : start+length]
	sha256hash := sha256.New()
	_, err := sha256hash.Write(data)
	c.Assert(err, gc.IsNil)
	md5hash := md5.New()
	_, err = md5hash.Write(data)
	c.Assert(err, gc.IsNil)
	md5hashHex = fmt.Sprintf("%x", md5hash.Sum(nil))
	sha256hashHex = fmt.Sprintf("%x", sha256hash.Sum(nil))
	return md5hashHex, sha256hashHex
}

func (s *managedStorageSuite) TestPutRequestResponseMD5Mismatch(c *gc.C) {
	blob, md5hash, sha256hash := s.putTestBlob(c, "path/to/blob")
	hash := storage.ResourceHash{md5hash, sha256hash}
	reqResp, err := s.managedStorage.PutRequestForEnvironment("env", "path/to/blob", hash)
	c.Assert(err, gc.IsNil)
	_, sha256Response := calculateCheckSums(c, reqResp.RangeStart, reqResp.RangeLength, blob)
	response := storage.NewPutResponse(reqResp.RequestId, "notmd5", sha256Response)
	err = s.managedStorage.PutResponse(response)
	c.Assert(err, gc.Equals, storage.ErrResponseMismatch)
	c.Assert(storage.RequestQueueLength(s.managedStorage), gc.Equals, 0)
}

func (s *managedStorageSuite) TestPutRequestResponseSHA256Mismatch(c *gc.C) {
	blob, md5hash, sha256hash := s.putTestBlob(c, "path/to/blob")
	hash := storage.ResourceHash{md5hash, sha256hash}
	reqResp, err := s.managedStorage.PutRequestForEnvironment("env", "path/to/blob", hash)
	c.Assert(err, gc.IsNil)
	md5Response, _ := calculateCheckSums(c, reqResp.RangeStart, reqResp.RangeLength, blob)
	response := storage.NewPutResponse(reqResp.RequestId, md5Response, "notsha256")
	err = s.managedStorage.PutResponse(response)
	c.Assert(err, gc.Equals, storage.ErrResponseMismatch)
	c.Assert(storage.RequestQueueLength(s.managedStorage), gc.Equals, 0)
}

func (s *managedStorageSuite) assertPutRequestSingle(c *gc.C, blob []byte, resourceCount int) {
	if blob == nil {
		id := bson.NewObjectId().Hex()
		blob = []byte(id)
	}
	rdr := bytes.NewReader(blob)
	err := s.managedStorage.PutForEnvironment("env", "path/to/blob", rdr, int64(len(blob)))
	c.Assert(err, gc.IsNil)
	md5hash, sha256hash := calculateCheckSums(c, 0, int64(len(blob)), blob)
	hash := storage.ResourceHash{md5hash, sha256hash}
	reqResp, err := s.managedStorage.PutRequestForEnvironment("env", "path/to/blob", hash)
	c.Assert(err, gc.IsNil)
	md5Response, sha256Response := calculateCheckSums(c, reqResp.RangeStart, reqResp.RangeLength, blob)
	response := storage.NewPutResponse(reqResp.RequestId, md5Response, sha256Response)
	err = s.managedStorage.PutResponse(response)
	c.Assert(err, gc.IsNil)
	s.assertGet(c, "path/to/blob", blob)
	s.assertResourceCatalogCount(c, resourceCount)
}

func (s *managedStorageSuite) TestPutRequestSingle(c *gc.C) {
	s.PatchValue(storage.RequestExpiry, 5*time.Millisecond)
	s.assertPutRequestSingle(c, nil, 1)
	c.Assert(storage.RequestQueueLength(s.managedStorage), gc.Equals, 0)
	// Wait for request timer to trigger.
	time.Sleep(7 * time.Millisecond)
	c.Assert(storage.RequestQueueLength(s.managedStorage), gc.Equals, 0)
}

func (s *managedStorageSuite) TestPutRequestLarge(c *gc.C) {
	s.PatchValue(storage.RequestExpiry, 5*time.Millisecond)
	// Use a blob size of 4096 which is greater than max range of put response range length.
	blob := make([]byte, 4096)
	for i := 0; i < 4096; i++ {
		blob[i] = byte(rand.Intn(255))
	}
	s.assertPutRequestSingle(c, blob, 1)
	c.Assert(storage.RequestQueueLength(s.managedStorage), gc.Equals, 0)
	// Wait for request timer to trigger.
	time.Sleep(7 * time.Millisecond)
	c.Assert(storage.RequestQueueLength(s.managedStorage), gc.Equals, 0)
}

func (s *managedStorageSuite) TestPutRequestMultiSequential(c *gc.C) {
	s.PatchValue(storage.RequestExpiry, 5*time.Millisecond)
	s.assertPutRequestSingle(c, nil, 1)
	c.Assert(storage.RequestQueueLength(s.managedStorage), gc.Equals, 0)
	// Wait for request timer to trigger.
	time.Sleep(7 * time.Millisecond)
	c.Assert(storage.RequestQueueLength(s.managedStorage), gc.Equals, 0)
	s.assertPutRequestSingle(c, nil, 1)
	c.Assert(storage.RequestQueueLength(s.managedStorage), gc.Equals, 0)
	// Wait for request timer to trigger.
	time.Sleep(7 * time.Millisecond)
	c.Assert(storage.RequestQueueLength(s.managedStorage), gc.Equals, 0)
}

func (s *managedStorageSuite) checkPutResponse(c *gc.C, index int, wg *sync.WaitGroup,
	requestId int64, md5hash, sha256hash string, blob []byte) {

	// After a random time, respond to a previously queued put request and check the result.
	go func() {
		delay := rand.Intn(3)
		time.Sleep(time.Duration(delay) * time.Millisecond)
		expectError := index == 2
		var response storage.PutResponse
		if expectError {
			response = storage.NewPutResponse(requestId, "bad", sha256hash)
		} else {
			response = storage.NewPutResponse(requestId, md5hash, sha256hash)
		}
		err := s.managedStorage.PutResponse(response)
		if expectError {
			c.Check(err, gc.NotNil)
		} else {
			c.Check(err, gc.IsNil)
			if err == nil {
				r, length, err := s.managedStorage.GetForEnvironment("env", fmt.Sprintf("path/to/blob%d", index))
				c.Check(err, gc.IsNil)
				if err == nil {
					data, err := ioutil.ReadAll(r)
					c.Check(err, gc.IsNil)
					c.Check(data, gc.DeepEquals, blob)
					c.Check(int(length), gc.DeepEquals, len(blob))
				}
			}
		}
		wg.Done()
	}()
}

func (s *managedStorageSuite) queuePutRequests(c *gc.C, done chan struct{}) {
	var wg sync.WaitGroup
	// One request is allowed to expire so set up wait group for 1 less than number of requests.
	wg.Add(9)
	go func() {
		for i := 0; i < 10; i++ {
			blobPath := fmt.Sprintf("path/to/blob%d", i)
			blob, md5hash, sha256hash := s.putTestBlob(c, blobPath)
			hash := storage.ResourceHash{md5hash, sha256hash}
			reqResp, err := s.managedStorage.PutRequestForEnvironment("env", "path/to/blob", hash)
			c.Assert(err, gc.IsNil)
			// Let one request timeout
			if i == 3 {
				continue
			}
			md5Response, sha256Response := calculateCheckSums(c, reqResp.RangeStart, reqResp.RangeLength, blob)
			s.checkPutResponse(c, i, &wg, reqResp.RequestId, md5Response, sha256Response, blob)
		}
		wg.Wait()
		close(done)
	}()
}

func (s *managedStorageSuite) TestPutRequestMultiRandom(c *gc.C) {
	s.PatchValue(storage.RequestExpiry, 100*time.Millisecond)
	done := make(chan struct{})
	s.queuePutRequests(c, done)
	select {
	case <-done:
		c.Logf("all done")
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for put requests to be processed")
	}
	len := storage.RequestQueueLength(s.managedStorage)
	if len > 1 {
		c.Fatal("request queue length too long")
	}
	// Wait for the final request to expire if it hasn't already.
	time.Sleep(100 * time.Millisecond)
	c.Assert(storage.RequestQueueLength(s.managedStorage), gc.Equals, 0)
}

func (s *managedStorageSuite) TestPutRequestExpired(c *gc.C) {
	s.PatchValue(storage.RequestExpiry, 5*time.Millisecond)
	blob, md5hash, sha256hash := s.putTestBlob(c, "path/to/blob")
	hash := storage.ResourceHash{md5hash, sha256hash}
	reqResp, err := s.managedStorage.PutRequestForEnvironment("env", "path/to/blob", hash)
	c.Assert(err, gc.IsNil)
	md5Response, sha256Response := calculateCheckSums(c, reqResp.RangeStart, reqResp.RangeLength, blob)
	time.Sleep(7 * time.Millisecond)
	response := storage.NewPutResponse(reqResp.RequestId, md5Response, sha256Response)
	err = s.managedStorage.PutResponse(response)
	c.Assert(err, gc.Equals, storage.ErrRequestExpired)
	c.Assert(storage.RequestQueueLength(s.managedStorage), gc.Equals, 0)
}

func (s *managedStorageSuite) TestPutRequestExpiredMulti(c *gc.C) {
	s.PatchValue(storage.RequestExpiry, 5*time.Millisecond)
	blob, md5hash, sha256hash := s.putTestBlob(c, "path/to/blob")
	hash := storage.ResourceHash{md5hash, sha256hash}
	reqResp, err := s.managedStorage.PutRequestForEnvironment("env", "path/to/blob", hash)
	c.Assert(err, gc.IsNil)
	md5Response, sha256Response := calculateCheckSums(c, reqResp.RangeStart, reqResp.RangeLength, blob)
	reqResp2, err := s.managedStorage.PutRequestForEnvironment("env", "path/to/blob2", hash)
	c.Assert(err, gc.IsNil)
	md5Response2, sha256Response2 := calculateCheckSums(c, reqResp.RangeStart, reqResp.RangeLength, blob)
	time.Sleep(7 * time.Millisecond)
	c.Assert(storage.RequestQueueLength(s.managedStorage), gc.Equals, 0)
	response := storage.NewPutResponse(reqResp.RequestId, md5Response, sha256Response)
	response2 := storage.NewPutResponse(reqResp2.RequestId, md5Response2, sha256Response2)
	err = s.managedStorage.PutResponse(response)
	c.Assert(err, gc.Equals, storage.ErrRequestExpired)
	err = s.managedStorage.PutResponse(response2)
	c.Assert(err, gc.Equals, storage.ErrRequestExpired)
}

func (s *managedStorageSuite) TestPutRequestDeleted(c *gc.C) {
	blob, md5hash, sha256hash := s.putTestBlob(c, "path/to/blob")
	hash := storage.ResourceHash{md5hash, sha256hash}
	reqResp, err := s.managedStorage.PutRequestForEnvironment("env", "path/to/blob", hash)
	c.Assert(err, gc.IsNil)
	err = s.managedStorage.RemoveForEnvironment("env", "path/to/blob")
	c.Assert(err, gc.IsNil)

	md5Response, sha256Response := calculateCheckSums(c, reqResp.RangeStart, reqResp.RangeLength, blob)
	response := storage.NewPutResponse(reqResp.RequestId, md5Response, sha256Response)
	err = s.managedStorage.PutResponse(response)
	c.Assert(err, gc.Equals, storage.ErrResourceDeleted)
}
