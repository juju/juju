// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package binarystorage_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	mgotesting "github.com/juju/mgo/v3/testing"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn/v3"
	txntesting "github.com/juju/txn/v3/testing"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/mongo"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/testing"
)

const current = "2.0.42-ubuntu-amd64"

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

type binaryStorageSuite struct {
	mgotesting.IsolatedMgoSuite

	storage            binarystorage.Storage
	managedStorage     binarystorage.ManagedStorage
	metadataCollection mongo.Collection
	txnRunner          jujutxn.Runner

	cleanUps []func(*gc.C)
}

var _ = gc.Suite(&binaryStorageSuite{})

func (s *binaryStorageSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)

	catalogue := s.Session.DB("catalogue")

	var closer func()
	s.metadataCollection, closer = mongo.CollectionFromName(catalogue, "binarymetadata")
	s.addCleanup(func(*gc.C) { closer() })

	s.managedStorage = jujutesting.NewObjectStore(c, "my-uuid", sessionShim{session: s.Session})
	s.txnRunner = jujutxn.NewRunner(jujutxn.RunnerParams{
		Database:                  catalogue,
		TransactionCollectionName: "txns",
		ChangeLogName:             "-",
		ServerSideTransactions:    true,
		MaxRetryAttempts:          3,
	})
	s.storage = binarystorage.New(s.managedStorage, s.metadataCollection, s.txnRunner)
}

func (s *binaryStorageSuite) addCleanup(f func(*gc.C)) {
	s.cleanUps = append(s.cleanUps, f)
}

func (s *binaryStorageSuite) TearDownTest(c *gc.C) {
	for _, f := range s.cleanUps {
		// Ensure to close sessions before IsolatedMgoSuite.TearDownTest here.
		f(c)
	}

	s.storage = nil
	s.managedStorage = nil
	s.metadataCollection = nil
	s.txnRunner = nil
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *binaryStorageSuite) TestAdd(c *gc.C) {
	s.testAdd(c, "some-binary")
}

func (s *binaryStorageSuite) TestAddReplaces(c *gc.C) {
	s.testAdd(c, "abc")
	s.testAdd(c, "def")
}

func (s *binaryStorageSuite) testAdd(c *gc.C, content string) {
	r := bytes.NewReader([]byte(content))
	addedMetadata := binarystorage.Metadata{
		Version: current,
		Size:    int64(len(content)),
		SHA256:  "hash(" + content + ")",
	}
	err := s.storage.Add(context.Background(), r, addedMetadata)
	c.Assert(err, jc.ErrorIsNil)

	metadata, rc, err := s.storage.Open(context.Background(), current)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r, gc.NotNil)
	defer rc.Close()
	c.Assert(metadata, gc.Equals, addedMetadata)

	data, err := io.ReadAll(rc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, content)
}

func bumpVersion(v string) string {
	vers := version.MustParseBinary(v)
	vers.Build++
	return vers.String()
}

func (s *binaryStorageSuite) TestAllMetadata(c *gc.C) {
	metadata, err := s.storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 0)

	s.addMetadataDoc(c, current, 3, "hash(abc)", "path")
	metadata, err = s.storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 1)
	expected := []binarystorage.Metadata{{
		Version: current,
		Size:    3,
		SHA256:  "hash(abc)",
	}}
	c.Assert(metadata, jc.SameContents, expected)

	alias := bumpVersion(current)
	s.addMetadataDoc(c, alias, 3, "hash(abc)", "path")

	metadata, err = s.storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 2)
	expected = append(expected, binarystorage.Metadata{
		Version: alias,
		Size:    3,
		SHA256:  "hash(abc)",
	})
	c.Assert(metadata, jc.SameContents, expected)
}

func (s *binaryStorageSuite) TestMetadata(c *gc.C) {
	_, err := s.storage.Metadata(current)
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	s.addMetadataDoc(c, current, 3, "hash(abc)", "path")
	metadata, err := s.storage.Metadata(current)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.Equals, binarystorage.Metadata{
		Version: current,
		Size:    3,
		SHA256:  "hash(abc)",
	})
}

func (s *binaryStorageSuite) TestOpen(c *gc.C) {
	_, _, err := s.storage.Open(context.Background(), current)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(err, gc.ErrorMatches, `.* binary metadata not found`)

	s.addMetadataDoc(c, current, 3, "hash(abc)", "path")
	_, _, err = s.storage.Open(context.Background(), current)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(err, gc.ErrorMatches, `resource at path "buckets/my-uuid/path" not found`)

	err = s.managedStorage.Put(context.Background(), "path", strings.NewReader("blah"), 4)
	c.Assert(err, jc.ErrorIsNil)

	metadata, r, err := s.storage.Open(context.Background(), current)
	c.Assert(err, jc.ErrorIsNil)
	defer r.Close()
	c.Assert(metadata, gc.Equals, binarystorage.Metadata{
		Version: current,
		Size:    3,
		SHA256:  "hash(abc)",
	})

	data, err := io.ReadAll(r)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "blah")
}

func (s *binaryStorageSuite) TestAddRemovesExisting(c *gc.C) {
	// Add a metadata doc and a blob at a known path, then
	// call Add and ensure the original blob is removed.
	s.addMetadataDoc(c, current, 3, "hash(abc)", "path")
	err := s.managedStorage.Put(context.Background(), "path", strings.NewReader("blah"), 4)
	c.Assert(err, jc.ErrorIsNil)

	addedMetadata := binarystorage.Metadata{
		Version: current,
		Size:    6,
		SHA256:  "hash(xyzzzz)",
	}
	err = s.storage.Add(context.Background(), strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, jc.ErrorIsNil)

	// old blob should be gone
	_, _, err = s.managedStorage.Get(context.Background(), "path")
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	s.assertMetadataAndContent(c, addedMetadata, "xyzzzz")
}

func (s *binaryStorageSuite) TestAddRemovesExistingRemoveFails(c *gc.C) {
	// Add a metadata doc and a blob at a known path, then
	// call Add and ensure that Add attempts to remove
	// the original blob, but does not return an error if it
	// fails.
	s.addMetadataDoc(c, current, 3, "hash(abc)", "path")
	err := s.managedStorage.Put(context.Background(), "path", strings.NewReader("blah"), 4)
	c.Assert(err, jc.ErrorIsNil)

	storage := binarystorage.New(
		removeFailsManagedStorage{ManagedStorage: s.managedStorage},
		s.metadataCollection,
		s.txnRunner,
	)
	addedMetadata := binarystorage.Metadata{
		Version: current,
		Size:    6,
		SHA256:  "hash(xyzzzz)",
	}
	err = storage.Add(context.Background(), strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, jc.ErrorIsNil)

	// old blob should still be there
	r, _, err := s.managedStorage.Get(context.Background(), "path")
	c.Assert(err, jc.ErrorIsNil)
	r.Close()

	s.assertMetadataAndContent(c, addedMetadata, "xyzzzz")
}

func (s *binaryStorageSuite) TestAddRemovesBlobOnFailure(c *gc.C) {
	storage := binarystorage.New(
		s.managedStorage,
		s.metadataCollection,
		errorTransactionRunner{s.txnRunner},
	)
	addedMetadata := binarystorage.Metadata{
		Version: current,
		Size:    6,
		SHA256:  "hash",
	}
	err := storage.Add(context.Background(), strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, gc.ErrorMatches, "cannot store binary metadata: Run fails")

	path := fmt.Sprintf("tools/%s-%s", addedMetadata.Version, addedMetadata.SHA256)
	_, _, err = s.managedStorage.Get(context.Background(), path)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *binaryStorageSuite) TestAddRemovesBlobOnFailureRemoveFails(c *gc.C) {
	storage := binarystorage.New(
		removeFailsManagedStorage{s.managedStorage},
		s.metadataCollection,
		errorTransactionRunner{s.txnRunner},
	)
	addedMetadata := binarystorage.Metadata{
		Version: current,
		Size:    6,
		SHA256:  "hash",
	}
	err := storage.Add(context.Background(), strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, gc.ErrorMatches, "cannot store binary metadata: Run fails")

	// blob should still be there, because the removal failed.
	path := fmt.Sprintf("tools/%s-%s", addedMetadata.Version, addedMetadata.SHA256)
	r, _, err := s.managedStorage.Get(context.Background(), path)
	c.Assert(err, jc.ErrorIsNil)
	r.Close()
}

func (s *binaryStorageSuite) TestAddSame(c *gc.C) {
	metadata := binarystorage.Metadata{Version: current, Size: 1, SHA256: "0"}
	for i := 0; i < 2; i++ {
		err := s.storage.Add(context.Background(), strings.NewReader("0"), metadata)
		c.Assert(err, jc.ErrorIsNil)
		s.assertMetadataAndContent(c, metadata, "0")
	}
}

func (s *binaryStorageSuite) TestAddConcurrent(c *gc.C) {
	metadata0 := binarystorage.Metadata{Version: current, Size: 1, SHA256: "0"}
	metadata1 := binarystorage.Metadata{Version: current, Size: 1, SHA256: "1"}

	addMetadata := func() {
		err := s.storage.Add(context.Background(), strings.NewReader("0"), metadata0)
		c.Assert(err, jc.ErrorIsNil)
		r, _, err := s.managedStorage.Get(context.Background(), fmt.Sprintf("tools/%s-0", current))
		c.Assert(err, jc.ErrorIsNil)
		r.Close()
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, addMetadata).Check()

	err := s.storage.Add(context.Background(), strings.NewReader("1"), metadata1)
	c.Assert(err, jc.ErrorIsNil)

	// Blob added in before-hook should be removed.
	_, _, err = s.managedStorage.Get(context.Background(), fmt.Sprintf("tools/%s-0", current))
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	s.assertMetadataAndContent(c, metadata1, "1")
}

func (s *binaryStorageSuite) TestAddExcessiveContention(c *gc.C) {
	metadata := []binarystorage.Metadata{
		{Version: current, Size: 1, SHA256: "0"},
		{Version: current, Size: 1, SHA256: "1"},
		{Version: current, Size: 1, SHA256: "2"},
		{Version: current, Size: 1, SHA256: "3"},
	}

	i := 1
	addMetadata := func() {
		err := s.storage.Add(context.Background(), strings.NewReader(metadata[i].SHA256), metadata[i])
		c.Assert(err, jc.ErrorIsNil)
		i++
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, addMetadata, addMetadata, addMetadata).Check()

	err := s.storage.Add(context.Background(), strings.NewReader(metadata[0].SHA256), metadata[0])
	c.Assert(err, gc.ErrorMatches, "cannot store binary metadata: state changing too quickly; try again soon")

	// There should be no blobs apart from the last one added by the before-hook.
	for _, metadata := range metadata[:3] {
		path := fmt.Sprintf("tools/%s-%s", metadata.Version, metadata.SHA256)
		_, _, err = s.managedStorage.Get(context.Background(), path)
		c.Assert(err, jc.ErrorIs, errors.NotFound)
	}

	s.assertMetadataAndContent(c, metadata[3], "3")
}

func (s *binaryStorageSuite) addMetadataDoc(c *gc.C, v string, size int64, hash, path string) {
	doc := struct {
		Id      string `bson:"_id"`
		Version string `bson:"version"`
		Size    int64  `bson:"size"`
		SHA256  string `bson:"sha256,omitempty"`
		Path    string `bson:"path"`
	}{
		Id:      v,
		Version: v,
		Size:    size,
		SHA256:  hash,
		Path:    path,
	}
	err := s.metadataCollection.Writeable().Insert(&doc)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *binaryStorageSuite) assertMetadataAndContent(c *gc.C, expected binarystorage.Metadata, content string) {
	metadata, r, err := s.storage.Open(context.Background(), expected.Version)
	c.Assert(err, jc.ErrorIsNil)
	defer r.Close()
	c.Assert(metadata, gc.Equals, expected)

	data, err := io.ReadAll(r)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, content)
}

type removeFailsManagedStorage struct {
	binarystorage.ManagedStorage
}

func (removeFailsManagedStorage) Remove(ctx context.Context, path string) error {
	return errors.Errorf("cannot remove %s", path)
}

type errorTransactionRunner struct {
	jujutxn.Runner
}

func (errorTransactionRunner) Run(transactions jujutxn.TransactionSource) error {
	return errors.New("Run fails")
}

type sessionShim struct {
	session *mgo.Session
}

func (s sessionShim) MongoSession() *mgo.Session {
	return s.session
}
