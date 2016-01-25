// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolstorage_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	txntesting "github.com/juju/txn/testing"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/blobstore.v2"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/state/toolstorage"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

var _ = gc.Suite(&ToolsSuite{})
var current = version.Binary{
	Number: version.Current,
	Arch:   arch.HostArch(),
	Series: series.HostSeries(),
}

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type ToolsSuite struct {
	testing.BaseSuite
	mongo              *gitjujutesting.MgoInstance
	session            *mgo.Session
	storage            toolstorage.Storage
	managedStorage     blobstore.ManagedStorage
	metadataCollection *mgo.Collection
	txnRunner          jujutxn.Runner
}

func (s *ToolsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mongo = &gitjujutesting.MgoInstance{}
	s.mongo.Start(nil)

	var err error
	s.session, err = s.mongo.Dial()
	c.Assert(err, jc.ErrorIsNil)
	rs := blobstore.NewGridFS("blobstore", "my-uuid", s.session)
	catalogue := s.session.DB("catalogue")
	s.managedStorage = blobstore.NewManagedStorage(catalogue, rs)
	s.metadataCollection = catalogue.C("toolsmetadata")
	s.txnRunner = jujutxn.NewRunner(jujutxn.RunnerParams{Database: catalogue})
	s.storage = toolstorage.NewStorage("my-uuid", s.managedStorage, s.metadataCollection, s.txnRunner)
}

func (s *ToolsSuite) TearDownTest(c *gc.C) {
	s.session.Close()
	s.mongo.DestroyWithLog()
	s.BaseSuite.TearDownTest(c)
}

func (s *ToolsSuite) TestAddTools(c *gc.C) {
	s.testAddTools(c, "some-tools")
}

func (s *ToolsSuite) TestAddToolsReplaces(c *gc.C) {
	s.testAddTools(c, "abc")
	s.testAddTools(c, "def")
}

func (s *ToolsSuite) testAddTools(c *gc.C, content string) {
	r := bytes.NewReader([]byte(content))
	addedMetadata := toolstorage.Metadata{
		Version: current,
		Size:    int64(len(content)),
		SHA256:  "hash(" + content + ")",
	}
	err := s.storage.AddTools(r, addedMetadata)
	c.Assert(err, jc.ErrorIsNil)

	metadata, rc, err := s.storage.Tools(current)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r, gc.NotNil)
	defer rc.Close()
	c.Assert(metadata, gc.Equals, addedMetadata)

	data, err := ioutil.ReadAll(rc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, content)
}

func bumpVersion(v version.Binary) version.Binary {
	v.Build++
	return v
}

func (s *ToolsSuite) TestAllMetadata(c *gc.C) {
	metadata, err := s.storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 0)

	s.addMetadataDoc(c, current, 3, "hash(abc)", "path")
	metadata, err = s.storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 1)
	expected := []toolstorage.Metadata{{
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
	expected = append(expected, toolstorage.Metadata{
		Version: alias,
		Size:    3,
		SHA256:  "hash(abc)",
	})
	c.Assert(metadata, jc.SameContents, expected)
}

func (s *ToolsSuite) TestMetadata(c *gc.C) {
	metadata, err := s.storage.Metadata(current)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.addMetadataDoc(c, current, 3, "hash(abc)", "path")
	metadata, err = s.storage.Metadata(current)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.Equals, toolstorage.Metadata{
		Version: current,
		Size:    3,
		SHA256:  "hash(abc)",
	})
}

func (s *ToolsSuite) TestTools(c *gc.C) {
	_, _, err := s.storage.Tools(current)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `.* tools metadata not found`)

	s.addMetadataDoc(c, current, 3, "hash(abc)", "path")
	_, _, err = s.storage.Tools(current)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `resource at path "buckets/my-uuid/path" not found`)

	err = s.managedStorage.PutForBucket("my-uuid", "path", strings.NewReader("blah"), 4)
	c.Assert(err, jc.ErrorIsNil)

	metadata, r, err := s.storage.Tools(current)
	c.Assert(err, jc.ErrorIsNil)
	defer r.Close()
	c.Assert(metadata, gc.Equals, toolstorage.Metadata{
		Version: current,
		Size:    3,
		SHA256:  "hash(abc)",
	})

	data, err := ioutil.ReadAll(r)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "blah")
}

func (s *ToolsSuite) TestAddToolsRemovesExisting(c *gc.C) {
	// Add a metadata doc and a blob at a known path, then
	// call AddTools and ensure the original blob is removed.
	s.addMetadataDoc(c, current, 3, "hash(abc)", "path")
	err := s.managedStorage.PutForBucket("my-uuid", "path", strings.NewReader("blah"), 4)
	c.Assert(err, jc.ErrorIsNil)

	addedMetadata := toolstorage.Metadata{
		Version: current,
		Size:    6,
		SHA256:  "hash(xyzzzz)",
	}
	err = s.storage.AddTools(strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, jc.ErrorIsNil)

	// old blob should be gone
	_, _, err = s.managedStorage.GetForBucket("my-uuid", "path")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.assertTools(c, addedMetadata, "xyzzzz")
}

func (s *ToolsSuite) TestAddToolsRemovesExistingRemoveFails(c *gc.C) {
	// Add a metadata doc and a blob at a known path, then
	// call AddTools and ensure that AddTools attempts to remove
	// the original blob, but does not return an error if it
	// fails.
	s.addMetadataDoc(c, current, 3, "hash(abc)", "path")
	err := s.managedStorage.PutForBucket("my-uuid", "path", strings.NewReader("blah"), 4)
	c.Assert(err, jc.ErrorIsNil)

	storage := toolstorage.NewStorage(
		"my-uuid",
		removeFailsManagedStorage{s.managedStorage},
		s.metadataCollection,
		s.txnRunner,
	)
	addedMetadata := toolstorage.Metadata{
		Version: current,
		Size:    6,
		SHA256:  "hash(xyzzzz)",
	}
	err = storage.AddTools(strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, jc.ErrorIsNil)

	// old blob should still be there
	r, _, err := s.managedStorage.GetForBucket("my-uuid", "path")
	c.Assert(err, jc.ErrorIsNil)
	r.Close()

	s.assertTools(c, addedMetadata, "xyzzzz")
}

func (s *ToolsSuite) TestAddToolsRemovesBlobOnFailure(c *gc.C) {
	storage := toolstorage.NewStorage(
		"my-uuid",
		s.managedStorage,
		s.metadataCollection,
		errorTransactionRunner{s.txnRunner},
	)
	addedMetadata := toolstorage.Metadata{
		Version: current,
		Size:    6,
		SHA256:  "hash",
	}
	err := storage.AddTools(strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, gc.ErrorMatches, "cannot store tools metadata: Run fails")

	path := fmt.Sprintf("tools/%s-%s", addedMetadata.Version, addedMetadata.SHA256)
	_, _, err = s.managedStorage.GetForBucket("my-uuid", path)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ToolsSuite) TestAddToolsRemovesBlobOnFailureRemoveFails(c *gc.C) {
	storage := toolstorage.NewStorage(
		"my-uuid",
		removeFailsManagedStorage{s.managedStorage},
		s.metadataCollection,
		errorTransactionRunner{s.txnRunner},
	)
	addedMetadata := toolstorage.Metadata{
		Version: current,
		Size:    6,
		SHA256:  "hash",
	}
	err := storage.AddTools(strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, gc.ErrorMatches, "cannot store tools metadata: Run fails")

	// blob should still be there, because the removal failed.
	path := fmt.Sprintf("tools/%s-%s", addedMetadata.Version, addedMetadata.SHA256)
	r, _, err := s.managedStorage.GetForBucket("my-uuid", path)
	c.Assert(err, jc.ErrorIsNil)
	r.Close()
}

func (s *ToolsSuite) TestAddToolsSame(c *gc.C) {
	metadata := toolstorage.Metadata{Version: current, Size: 1, SHA256: "0"}
	for i := 0; i < 2; i++ {
		err := s.storage.AddTools(strings.NewReader("0"), metadata)
		c.Assert(err, jc.ErrorIsNil)
		s.assertTools(c, metadata, "0")
	}
}

func (s *ToolsSuite) TestAddToolsConcurrent(c *gc.C) {
	metadata0 := toolstorage.Metadata{Version: current, Size: 1, SHA256: "0"}
	metadata1 := toolstorage.Metadata{Version: current, Size: 1, SHA256: "1"}

	addMetadata := func() {
		err := s.storage.AddTools(strings.NewReader("0"), metadata0)
		c.Assert(err, jc.ErrorIsNil)
		r, _, err := s.managedStorage.GetForBucket("my-uuid", fmt.Sprintf("tools/%s-0", current))
		c.Assert(err, jc.ErrorIsNil)
		r.Close()
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, addMetadata).Check()

	err := s.storage.AddTools(strings.NewReader("1"), metadata1)
	c.Assert(err, jc.ErrorIsNil)

	// Blob added in before-hook should be removed.
	_, _, err = s.managedStorage.GetForBucket("my-uuid", fmt.Sprintf("tools/%s-0", current))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.assertTools(c, metadata1, "1")
}

func (s *ToolsSuite) TestAddToolsExcessiveContention(c *gc.C) {
	metadata := []toolstorage.Metadata{
		{Version: current, Size: 1, SHA256: "0"},
		{Version: current, Size: 1, SHA256: "1"},
		{Version: current, Size: 1, SHA256: "2"},
		{Version: current, Size: 1, SHA256: "3"},
	}

	i := 1
	addMetadata := func() {
		err := s.storage.AddTools(strings.NewReader(metadata[i].SHA256), metadata[i])
		c.Assert(err, jc.ErrorIsNil)
		i++
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, addMetadata, addMetadata, addMetadata).Check()

	err := s.storage.AddTools(strings.NewReader(metadata[0].SHA256), metadata[0])
	c.Assert(err, gc.ErrorMatches, "cannot store tools metadata: state changing too quickly; try again soon")

	// There should be no blobs apart from the last one added by the before-hook.
	for _, metadata := range metadata[:3] {
		path := fmt.Sprintf("tools/%s-%s", metadata.Version, metadata.SHA256)
		_, _, err = s.managedStorage.GetForBucket("my-uuid", path)
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}

	s.assertTools(c, metadata[3], "3")
}

func (s *ToolsSuite) addMetadataDoc(c *gc.C, v version.Binary, size int64, hash, path string) {
	doc := struct {
		Id      string         `bson:"_id"`
		Version version.Binary `bson:"version"`
		Size    int64          `bson:"size"`
		SHA256  string         `bson:"sha256,omitempty"`
		Path    string         `bson:"path"`
	}{
		Id:      v.String(),
		Version: v,
		Size:    size,
		SHA256:  hash,
		Path:    path,
	}
	err := s.metadataCollection.Insert(&doc)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ToolsSuite) assertTools(c *gc.C, expected toolstorage.Metadata, content string) {
	metadata, r, err := s.storage.Tools(expected.Version)
	c.Assert(err, jc.ErrorIsNil)
	defer r.Close()
	c.Assert(metadata, gc.Equals, expected)

	data, err := ioutil.ReadAll(r)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, content)
}

type removeFailsManagedStorage struct {
	blobstore.ManagedStorage
}

func (removeFailsManagedStorage) RemoveForBucket(uuid, path string) error {
	return errors.Errorf("cannot remove %s:%s", uuid, path)
}

type errorTransactionRunner struct {
	jujutxn.Runner
}

func (errorTransactionRunner) Run(transactions jujutxn.TransactionSource) error {
	return errors.New("Run fails")
}
