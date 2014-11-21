// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagestorage_test

import (
	"bytes"
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
	jujutxn "github.com/juju/txn"
	txntesting "github.com/juju/txn/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/state/imagestorage"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&ImageSuite{})

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type ImageSuite struct {
	testing.BaseSuite
	mongo              *gitjujutesting.MgoInstance
	session            *mgo.Session
	storage            imagestorage.Storage
	managedStorage     blobstore.ManagedStorage
	metadataCollection *mgo.Collection
	txnRunner          jujutxn.Runner
}

func (s *ImageSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mongo = &gitjujutesting.MgoInstance{}
	s.mongo.Start(nil)

	var err error
	s.session, err = s.mongo.Dial()
	c.Assert(err, gc.IsNil)
	rs := blobstore.NewGridFS("blobstore", "my-uuid", s.session)
	catalogue := s.session.DB("catalogue")
	s.managedStorage = blobstore.NewManagedStorage(catalogue, rs)
	s.metadataCollection = catalogue.C("imagemetadata")
	s.txnRunner = jujutxn.NewRunner(jujutxn.RunnerParams{Database: catalogue})
	s.storage = imagestorage.NewStorage("my-uuid", s.managedStorage, s.metadataCollection, s.txnRunner)
}

func (s *ImageSuite) TearDownTest(c *gc.C) {
	s.session.Close()
	s.mongo.DestroyWithLog()
	s.BaseSuite.TearDownTest(c)
}

func (s *ImageSuite) TestAddImage(c *gc.C) {
	s.testAddImage(c, "some-image")
}

func (s *ImageSuite) TestAddImageReplaces(c *gc.C) {
	s.testAddImage(c, "abc")
	s.testAddImage(c, "defghi")
}

func checkMetadata(c *gc.C, metadata, fromDb *imagestorage.Metadata) {
	c.Assert(fromDb.Created.IsZero(), jc.IsFalse)
	c.Assert(fromDb.Created.Before(time.Now()), jc.IsTrue)
	fromDb.Created = time.Time{}
	c.Assert(metadata, gc.DeepEquals, fromDb)
}

func (s *ImageSuite) testAddImage(c *gc.C, content string) {
	var r io.Reader = bytes.NewReader([]byte(content))
	addedMetadata := &imagestorage.Metadata{
		Kind:     "lxc",
		Series:   "trusty",
		Arch:     "amd64",
		Size:     int64(len(content)),
		Checksum: "hash(" + content + ")",
	}
	err := s.storage.AddImage(r, addedMetadata)
	c.Assert(err, gc.IsNil)

	metadata, rc, err := s.storage.Image("lxc", "trusty", "amd64")
	c.Assert(err, gc.IsNil)
	c.Assert(r, gc.NotNil)
	defer rc.Close()
	checkMetadata(c, addedMetadata, metadata)

	data, err := ioutil.ReadAll(rc)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, content)
}

func (s *ImageSuite) TestImage(c *gc.C) {
	_, _, err := s.storage.Image("lxc", "trusty", "amd64")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `.* image metadata not found`)

	s.addMetadataDoc(c, "lxc", "trusty", "amd64", 3, "hash(abc)", "path")
	_, _, err = s.storage.Image("lxc", "trusty", "amd64")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `resource at path "environs/my-uuid/path" not found`)

	err = s.managedStorage.PutForEnvironment("my-uuid", "path", strings.NewReader("blah"), 4)
	c.Assert(err, gc.IsNil)

	metadata, r, err := s.storage.Image("lxc", "trusty", "amd64")
	c.Assert(err, gc.IsNil)
	defer r.Close()
	checkMetadata(c, &imagestorage.Metadata{
		Kind:     "lxc",
		Series:   "trusty",
		Arch:     "amd64",
		Size:     3,
		Checksum: "hash(abc)",
	}, metadata)

	data, err := ioutil.ReadAll(r)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, "blah")
}

func (s *ImageSuite) TestAddImageRemovesExisting(c *gc.C) {
	// Add a metadata doc and a blob at a known path, then
	// call AddImage and ensure the original blob is removed.
	s.addMetadataDoc(c, "lxc", "trusty", "amd64", 3, "hash(abc)", "path")
	err := s.managedStorage.PutForEnvironment("my-uuid", "path", strings.NewReader("blah"), 4)
	c.Assert(err, gc.IsNil)

	addedMetadata := &imagestorage.Metadata{
		Kind:     "lxc",
		Series:   "trusty",
		Arch:     "amd64",
		Size:     6,
		Checksum: "hash(xyzzzz)",
	}
	err = s.storage.AddImage(strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, gc.IsNil)

	// old blob should be gone
	_, _, err = s.managedStorage.GetForEnvironment("my-uuid", "path")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.assertImage(c, addedMetadata, "xyzzzz")
}

func (s *ImageSuite) TestAddImageRemovesExistingRemoveFails(c *gc.C) {
	// Add a metadata doc and a blob at a known path, then
	// call AddImage and ensure that AddImage attempts to remove
	// the original blob, but does not return an error if it
	// fails.
	s.addMetadataDoc(c, "lxc", "trusty", "amd64", 3, "hash(abc)", "path")
	err := s.managedStorage.PutForEnvironment("my-uuid", "path", strings.NewReader("blah"), 4)
	c.Assert(err, gc.IsNil)

	storage := imagestorage.NewStorage(
		"my-uuid",
		removeFailsManagedStorage{s.managedStorage},
		s.metadataCollection,
		s.txnRunner,
	)
	addedMetadata := &imagestorage.Metadata{
		Kind:     "lxc",
		Series:   "trusty",
		Arch:     "amd64",
		Size:     6,
		Checksum: "hash(xyzzzz)",
	}
	err = storage.AddImage(strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, gc.IsNil)

	// old blob should still be there
	r, _, err := s.managedStorage.GetForEnvironment("my-uuid", "path")
	c.Assert(err, gc.IsNil)
	r.Close()

	s.assertImage(c, addedMetadata, "xyzzzz")
}

func (s *ImageSuite) TestAddImageRemovesBlobOnFailure(c *gc.C) {
	storage := imagestorage.NewStorage(
		"my-uuid",
		s.managedStorage,
		s.metadataCollection,
		errorTransactionRunner{s.txnRunner},
	)
	addedMetadata := &imagestorage.Metadata{
		Kind:     "lxc",
		Series:   "trusty",
		Arch:     "amd64",
		Size:     6,
		Checksum: "hash",
	}
	err := storage.AddImage(strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, gc.ErrorMatches, "cannot store image metadata: Run fails")

	path := fmt.Sprintf(
		"images/%s-%s-%s:%s", addedMetadata.Kind, addedMetadata.Series, addedMetadata.Arch, addedMetadata.Checksum)
	_, _, err = s.managedStorage.GetForEnvironment("my-uuid", path)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ImageSuite) TestAddImageRemovesBlobOnFailureRemoveFails(c *gc.C) {
	storage := imagestorage.NewStorage(
		"my-uuid",
		removeFailsManagedStorage{s.managedStorage},
		s.metadataCollection,
		errorTransactionRunner{s.txnRunner},
	)
	addedMetadata := &imagestorage.Metadata{
		Kind:     "lxc",
		Series:   "trusty",
		Arch:     "amd64",
		Size:     6,
		Checksum: "hash",
	}
	err := storage.AddImage(strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, gc.ErrorMatches, "cannot store image metadata: Run fails")

	// blob should still be there, because the removal failed.
	path := fmt.Sprintf(
		"images/%s-%s-%s:%s", addedMetadata.Kind, addedMetadata.Series, addedMetadata.Arch, addedMetadata.Checksum)
	r, _, err := s.managedStorage.GetForEnvironment("my-uuid", path)
	c.Assert(err, gc.IsNil)
	r.Close()
}

func (s *ImageSuite) TestAddImageSame(c *gc.C) {
	metadata := &imagestorage.Metadata{Kind: "lxc", Series: "trusty", Arch: "amd64", Size: 1, Checksum: "0"}
	for i := 0; i < 2; i++ {
		err := s.storage.AddImage(strings.NewReader("0"), metadata)
		c.Assert(err, gc.IsNil)
		s.assertImage(c, metadata, "0")
	}
}

func (s *ImageSuite) TestAddImageConcurrent(c *gc.C) {
	metadata0 := &imagestorage.Metadata{Kind: "lxc", Series: "trusty", Arch: "amd64", Size: 1, Checksum: "0"}
	metadata1 := &imagestorage.Metadata{Kind: "lxc", Series: "trusty", Arch: "amd64", Size: 1, Checksum: "1"}

	addMetadata := func() {
		err := s.storage.AddImage(strings.NewReader("0"), metadata0)
		c.Assert(err, gc.IsNil)
		r, _, err := s.managedStorage.GetForEnvironment("my-uuid", "images/lxc-trusty-amd64:0")
		c.Assert(err, gc.IsNil)
		r.Close()
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, addMetadata).Check()

	err := s.storage.AddImage(strings.NewReader("1"), metadata1)
	c.Assert(err, gc.IsNil)

	// Blob added in before-hook should be removed.
	_, _, err = s.managedStorage.GetForEnvironment("my-uuid", "images/lxc-trusty-amd64:0")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.assertImage(c, metadata1, "1")
}

func (s *ImageSuite) TestAddImageExcessiveContention(c *gc.C) {
	metadata := []*imagestorage.Metadata{
		{Kind: "lxc", Series: "trusty", Arch: "amd64", Size: 1, Checksum: "0"},
		{Kind: "lxc", Series: "trusty", Arch: "amd64", Size: 1, Checksum: "1"},
		{Kind: "lxc", Series: "trusty", Arch: "amd64", Size: 1, Checksum: "2"},
		{Kind: "lxc", Series: "trusty", Arch: "amd64", Size: 1, Checksum: "3"},
	}

	i := 1
	addMetadata := func() {
		err := s.storage.AddImage(strings.NewReader(metadata[i].Checksum), metadata[i])
		c.Assert(err, gc.IsNil)
		i++
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, addMetadata, addMetadata, addMetadata).Check()

	err := s.storage.AddImage(strings.NewReader(metadata[0].Checksum), metadata[0])
	c.Assert(err, gc.ErrorMatches, "cannot store image metadata: state changing too quickly; try again soon")

	// There should be no blobs apart from the last one added by the before-hook.
	for _, metadata := range metadata[:3] {
		path := fmt.Sprintf("images/%s-%s-%s:%s", metadata.Kind, metadata.Series, metadata.Arch, metadata.Checksum)
		_, _, err = s.managedStorage.GetForEnvironment("my-uuid", path)
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}

	s.assertImage(c, metadata[3], "3")
}

func (s *ImageSuite) TestDeleteImage(c *gc.C) {
	s.addMetadataDoc(c, "lxc", "trusty", "amd64", 3, "hash(abc)", "images/lxc-trusty-amd64:sha256")
	err := s.managedStorage.PutForEnvironment("my-uuid", "images/lxc-trusty-amd64:sha256", strings.NewReader("blah"), 4)
	c.Assert(err, gc.IsNil)

	_, rc, err := s.storage.Image("lxc", "trusty", "amd64")
	c.Assert(err, gc.IsNil)
	c.Assert(rc, gc.NotNil)
	rc.Close()

	metadata := &imagestorage.Metadata{
		Kind:     "lxc",
		Series:   "trusty",
		Arch:     "amd64",
		Checksum: "sha256",
	}
	err = s.storage.DeleteImage(metadata)
	c.Assert(err, gc.IsNil)

	_, _, err = s.managedStorage.GetForEnvironment("my-uuid", "images/lxc-trusty-amd64:sha256")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, _, err = s.storage.Image("lxc", "trusty", "amd64")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ImageSuite) TestDeleteNotExistentImage(c *gc.C) {
	metadata := &imagestorage.Metadata{
		Kind:     "lxc",
		Series:   "trusty",
		Arch:     "amd64",
		Checksum: "sha256",
	}
	err := s.storage.DeleteImage(metadata)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ImageSuite) addMetadataDoc(c *gc.C, kind, series, arch string, size int64, checksum, path string) {
	doc := struct {
		Id      string `bson:"_id"`
		Kind    string `bson:"kind"`
		Series  string `bson:"series"`
		Arch    string `bson:"arch"`
		Size    int64  `bson:"size"`
		SHA256  string `bson:"sha256,omitempty"`
		Path    string `bson:"path"`
		Created string `bson:"created"`
	}{
		Id:      fmt.Sprintf("%s-%s-%s", kind, series, arch),
		Kind:    kind,
		Series:  series,
		Arch:    arch,
		Size:    size,
		SHA256:  checksum,
		Path:    path,
		Created: time.Now().Format(time.RFC3339),
	}
	err := s.metadataCollection.Insert(&doc)
	c.Assert(err, gc.IsNil)
}

func (s *ImageSuite) assertImage(c *gc.C, expected *imagestorage.Metadata, content string) {
	metadata, r, err := s.storage.Image(expected.Kind, expected.Series, expected.Arch)
	c.Assert(err, gc.IsNil)
	defer r.Close()
	checkMetadata(c, expected, metadata)

	data, err := ioutil.ReadAll(r)
	c.Assert(err, gc.IsNil)
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
