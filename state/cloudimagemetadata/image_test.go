// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	stdtesting "testing"

	"github.com/juju/blobstore"
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	txntesting "github.com/juju/txn/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

var _ = gc.Suite(&cloudImageMetadataSuite{})

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type cloudImageMetadataSuite struct {
	testing.BaseSuite
	mongo              *gitjujutesting.MgoInstance
	session            *mgo.Session
	storage            cloudimagemetadata.Storage
	metadataCollection *mgo.Collection
	txnRunner          jujutxn.Runner
}

func (s *cloudImageMetadataSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mongo = &gitjujutesting.MgoInstance{}
	s.mongo.Start(nil)

	var err error
	s.session, err = s.mongo.Dial()
	c.Assert(err, jc.ErrorIsNil)
	catalogue := s.session.DB("catalogue")
	s.metadataCollection = catalogue.C("cloudimagesmetadata")
	s.txnRunner = jujutxn.NewRunner(jujutxn.RunnerParams{Database: catalogue})
	s.storage = cloudimagemetadata.NewStorage("my-uuid", s.metadataCollection, s.txnRunner)
}

func (s *cloudImageMetadataSuite) TearDownTest(c *gc.C) {
	s.session.Close()
	s.mongo.DestroyWithLog()
	s.BaseSuite.TearDownTest(c)
}

func (s *cloudImageMetadataSuite) TestAddImages(c *gc.C) {
	s.testAddImages(c, "some-cloudimages")
}

func (s *cloudImageMetadataSuite) TestAddToolsReplaces(c *gc.C) {
	s.testAddImages(c, "abc")
	s.testAddImages(c, "def")
}

func (s *cloudImageMetadataSuite) testAddImages(c *gc.C, content string) {
	var r io.Reader = bytes.NewReader([]byte(content))
	addedMetadata := cloudimagemetadata.Metadata{
		Version: version.Current,
		Size:    int64(len(content)),
		SHA256:  "hash(" + content + ")",
	}
	err := s.storage.AddCloudImages(r, addedMetadata)
	c.Assert(err, jc.ErrorIsNil)

	metadata, rc, err := s.storage.CloudImages(version.Current)
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

func (s *cloudImageMetadataSuite) TestAllMetadata(c *gc.C) {
	metadata, err := s.storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 0)

	s.addMetadataDoc(c, version.Current, 3, "hash(abc)", "path")
	metadata, err = s.storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 1)
	expected := []cloudimagemetadata.Metadata{{
		Version: version.Current,
		Size:    3,
		SHA256:  "hash(abc)",
	}}
	c.Assert(metadata, jc.SameContents, expected)

	alias := bumpVersion(version.Current)
	s.addMetadataDoc(c, alias, 3, "hash(abc)", "path")

	metadata, err = s.storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 2)
	expected = append(expected, cloudimagemetadata.Metadata{
		Version: alias,
		Size:    3,
		SHA256:  "hash(abc)",
	})
	c.Assert(metadata, jc.SameContents, expected)
}

func (s *cloudImageMetadataSuite) TestMetadata(c *gc.C) {
	metadata, err := s.storage.Metadata(version.Current)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.addMetadataDoc(c, version.Current, 3, "hash(abc)", "path")
	metadata, err = s.storage.Metadata(version.Current)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.Equals, cloudimagemetadata.Metadata{
		Version: version.Current,
		Size:    3,
		SHA256:  "hash(abc)",
	})
}

func (s *cloudImageMetadataSuite) TestCloudImages(c *gc.C) {
	_, _, err := s.storage.CloudImages(version.Current)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `.* cloud images metadata not found`)

	s.addMetadataDoc(c, version.Current, 3, "hash(abc)", "path")
	_, _, err = s.storage.CloudImages(version.Current)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `resource at path "environs/my-uuid/path" not found`)

	err = s.managedStorage.PutForEnvironment("my-uuid", "path", strings.NewReader("blah"), 4)
	c.Assert(err, jc.ErrorIsNil)

	metadata, r, err := s.storage.CloudImages(version.Current)
	c.Assert(err, jc.ErrorIsNil)
	defer r.Close()
	c.Assert(metadata, gc.Equals, cloudimagemetadata.Metadata{
		Version: version.Current,
		Size:    3,
		SHA256:  "hash(abc)",
	})

	data, err := ioutil.ReadAll(r)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "blah")
}

func (s *cloudImageMetadataSuite) TestAddCloudImagesRemovesExisting(c *gc.C) {
	// Add a metadata doc and a blob at a known path, then
	// call AddCloudImages and ensure the original blob is removed.
	s.addMetadataDoc(c, version.Current, 3, "hash(abc)", "path")
	err := s.managedStorage.PutForEnvironment("my-uuid", "path", strings.NewReader("blah"), 4)
	c.Assert(err, jc.ErrorIsNil)

	addedMetadata := cloudimagemetadata.Metadata{
		Version: version.Current,
		Size:    6,
		SHA256:  "hash(xyzzzz)",
	}
	err = s.storage.AddCloudImages(strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, jc.ErrorIsNil)

	// old blob should be gone
	_, _, err = s.managedStorage.GetForEnvironment("my-uuid", "path")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.assertImages(c, addedMetadata, "xyzzzz")
}

func (s *cloudImageMetadataSuite) TestAddCloudImagesRemovesExistingRemoveFails(c *gc.C) {
	// Add a metadata doc and a blob at a known path, then
	// call AddCloudImages and ensure that AddCloudImages attempts to remove
	// the original blob, but does not return an error if it
	// fails.
	s.addMetadataDoc(c, version.Current, 3, "hash(abc)", "path")
	err := s.managedStorage.PutForEnvironment("my-uuid", "path", strings.NewReader("blah"), 4)
	c.Assert(err, jc.ErrorIsNil)

	storage := cloudimagemetadata.NewStorage(
		"my-uuid",
		removeFailsManagedStorage{s.managedStorage},
		s.metadataCollection,
		s.txnRunner,
	)
	addedMetadata := cloudimagemetadata.Metadata{
		Version: version.Current,
		Size:    6,
		SHA256:  "hash(xyzzzz)",
	}
	err = storage.AddCloudImages(strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, jc.ErrorIsNil)

	// old blob should still be there
	r, _, err := s.managedStorage.GetForEnvironment("my-uuid", "path")
	c.Assert(err, jc.ErrorIsNil)
	r.Close()

	s.assertImages(c, addedMetadata, "xyzzzz")
}

func (s *cloudImageMetadataSuite) TestAddCloudImagesRemovesBlobOnFailure(c *gc.C) {
	storage := cloudimagemetadata.NewStorage(
		"my-uuid",
		s.managedStorage,
		s.metadataCollection,
		errorTransactionRunner{s.txnRunner},
	)
	addedMetadata := cloudimagemetadata.Metadata{
		Version: version.Current,
		Size:    6,
		SHA256:  "hash",
	}
	err := storage.AddCloudImages(strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, gc.ErrorMatches, "cannot store cloud images metadata: Run fails")

	path := fmt.Sprintf("cloudimages/%s-%s", addedMetadata.Version, addedMetadata.SHA256)
	_, _, err = s.managedStorage.GetForEnvironment("my-uuid", path)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *cloudImageMetadataSuite) TestAddCloudImagesRemovesBlobOnFailureRemoveFails(c *gc.C) {
	storage := cloudimagemetadata.NewStorage(
		"my-uuid",
		removeFailsManagedStorage{s.managedStorage},
		s.metadataCollection,
		errorTransactionRunner{s.txnRunner},
	)
	addedMetadata := cloudimagemetadata.Metadata{
		Version: version.Current,
		Size:    6,
		SHA256:  "hash",
	}
	err := storage.AddCloudImages(strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, gc.ErrorMatches, "cannot store cloud images metadata: Run fails")

	// blob should still be there, because the removal failed.
	path := fmt.Sprintf("cloudimages/%s-%s", addedMetadata.Version, addedMetadata.SHA256)
	r, _, err := s.managedStorage.GetForEnvironment("my-uuid", path)
	c.Assert(err, jc.ErrorIsNil)
	r.Close()
}

func (s *cloudImageMetadataSuite) TestAddCloudImagesSame(c *gc.C) {
	metadata := cloudimagemetadata.Metadata{Version: version.Current, Size: 1, SHA256: "0"}
	for i := 0; i < 2; i++ {
		err := s.storage.AddCloudImages(strings.NewReader("0"), metadata)
		c.Assert(err, jc.ErrorIsNil)
		s.assertImages(c, metadata, "0")
	}
}

func (s *cloudImageMetadataSuite) TestAddCloudImagesConcurrent(c *gc.C) {
	metadata0 := cloudimagemetadata.Metadata{Version: version.Current, Size: 1, SHA256: "0"}
	metadata1 := cloudimagemetadata.Metadata{Version: version.Current, Size: 1, SHA256: "1"}

	addMetadata := func() {
		err := s.storage.AddCloudImages(strings.NewReader("0"), metadata0)
		c.Assert(err, jc.ErrorIsNil)
		r, _, err := s.managedStorage.GetForEnvironment("my-uuid", fmt.Sprintf("cloudimages/%s-0", version.Current))
		c.Assert(err, jc.ErrorIsNil)
		r.Close()
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, addMetadata).Check()

	err := s.storage.AddCloudImages(strings.NewReader("1"), metadata1)
	c.Assert(err, jc.ErrorIsNil)

	// Blob added in before-hook should be removed.
	_, _, err = s.managedStorage.GetForEnvironment("my-uuid", fmt.Sprintf("cloudimages/%s-0", version.Current))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.assertImages(c, metadata1, "1")
}

func (s *cloudImageMetadataSuite) TestAddCloudImagesExcessiveContention(c *gc.C) {
	metadata := []cloudimagemetadata.Metadata{
		{Version: version.Current, Size: 1, SHA256: "0"},
		{Version: version.Current, Size: 1, SHA256: "1"},
		{Version: version.Current, Size: 1, SHA256: "2"},
		{Version: version.Current, Size: 1, SHA256: "3"},
	}

	i := 1
	addMetadata := func() {
		err := s.storage.AddCloudImages(strings.NewReader(metadata[i].SHA256), metadata[i])
		c.Assert(err, jc.ErrorIsNil)
		i++
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, addMetadata, addMetadata, addMetadata).Check()

	err := s.storage.AddCloudImages(strings.NewReader(metadata[0].SHA256), metadata[0])
	c.Assert(err, gc.ErrorMatches, "cannot store cloud images metadata: state changing too quickly; try again soon")

	// There should be no blobs apart from the last one added by the before-hook.
	for _, metadata := range metadata[:3] {
		path := fmt.Sprintf("cloudimages/%s-%s", metadata.Version, metadata.SHA256)
		_, _, err = s.managedStorage.GetForEnvironment("my-uuid", path)
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}

	s.assertImages(c, metadata[3], "3")
}

func (s *cloudImageMetadataSuite) addMetadataDoc(c *gc.C, v version.Binary, size int64, hash, path string) {
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

func (s *cloudImageMetadataSuite) assertImages(c *gc.C, ex0 (~menn0@4.13pected cloudimagemetadata.Metadata, content string) {
	metadata, r, err := s.storage.CloudImages(expected.Version)
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

func (removeFailsManagedStorage) RemoveForEnvironment(uuid, path string) error {
	return errors.Errorf("cannot remove %s:%s", uuid, path)
}

type errorTransactionRunner struct {
	jujutxn.Runner
}

func (errorTransactionRunner) Run(transactions jujutxn.TransactionSource) error {
	return errors.New("Run fails")
}
