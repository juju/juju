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

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/txn"
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
	s.storage = imagestorage.NewStorage(s.session, "my-uuid")
	s.metadataCollection = imagestorage.MetadataCollection(s.storage)
	s.txnRunner = jujutxn.NewRunner(jujutxn.RunnerParams{Database: s.metadataCollection.Database})
	s.patchTransactionRunner()
}

func (s *ImageSuite) TearDownTest(c *gc.C) {
	s.session.Close()
	s.mongo.DestroyWithLog()
	s.BaseSuite.TearDownTest(c)
}
func (s *ImageSuite) patchTransactionRunner() {
	s.PatchValue(imagestorage.TxnRunner, func(db *mgo.Database) txn.Runner {
		return s.txnRunner
	})
}

func (s *ImageSuite) TestAddImage(c *gc.C) {
	s.testAddImage(c, "some-image")
}

func (s *ImageSuite) TestAddImageReplaces(c *gc.C) {
	s.testAddImage(c, "abc")
	s.testAddImage(c, "defghi")
}

func checkMetadata(c *gc.C, fromDb, metadata *imagestorage.Metadata) {
	c.Assert(fromDb.Created.IsZero(), jc.IsFalse)
	c.Assert(fromDb.Created.Before(time.Now()), jc.IsTrue)
	fromDb.Created = time.Time{}
	c.Assert(metadata, gc.DeepEquals, fromDb)
}

func checkAllMetadata(c *gc.C, fromDb []*imagestorage.Metadata, metadata ...*imagestorage.Metadata) {
	c.Assert(len(metadata), gc.Equals, len(fromDb))
	for i, m := range metadata {
		checkMetadata(c, fromDb[i], m)
	}
}

func (s *ImageSuite) testAddImage(c *gc.C, content string) {
	var r io.Reader = bytes.NewReader([]byte(content))
	addedMetadata := &imagestorage.Metadata{
		EnvUUID:   "my-uuid",
		Kind:      "lxc",
		Series:    "trusty",
		Arch:      "amd64",
		Size:      int64(len(content)),
		SHA256:    "hash(" + content + ")",
		SourceURL: "http://path",
	}
	err := s.storage.AddImage(r, addedMetadata)
	c.Assert(err, gc.IsNil)

	metadata, rc, err := s.storage.Image("lxc", "trusty", "amd64")
	c.Assert(err, gc.IsNil)
	c.Assert(r, gc.NotNil)
	defer rc.Close()
	checkMetadata(c, metadata, addedMetadata)

	data, err := ioutil.ReadAll(rc)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, content)
}

func (s *ImageSuite) TestImage(c *gc.C) {
	_, _, err := s.storage.Image("lxc", "trusty", "amd64")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `.* image metadata not found`)

	s.addMetadataDoc(c, "lxc", "trusty", "amd64", 3, "hash(abc)", "path", "http://path")
	_, _, err = s.storage.Image("lxc", "trusty", "amd64")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `resource at path "environs/my-uuid/path" not found`)

	managedStorage := imagestorage.ManagedStorage(s.storage, s.session)
	err = managedStorage.PutForEnvironment("my-uuid", "path", strings.NewReader("blah"), 4)
	c.Assert(err, gc.IsNil)

	metadata, r, err := s.storage.Image("lxc", "trusty", "amd64")
	c.Assert(err, gc.IsNil)
	defer r.Close()
	checkMetadata(c, metadata, &imagestorage.Metadata{
		EnvUUID:   "my-uuid",
		Kind:      "lxc",
		Series:    "trusty",
		Arch:      "amd64",
		Size:      3,
		SHA256:    "hash(abc)",
		SourceURL: "http://path",
	})

	data, err := ioutil.ReadAll(r)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, "blah")
}

func (s *ImageSuite) TestAddImageRemovesExisting(c *gc.C) {
	// Add a metadata doc and a blob at a known path, then
	// call AddImage and ensure the original blob is removed.
	s.addMetadataDoc(c, "lxc", "trusty", "amd64", 3, "hash(abc)", "path", "http://path")
	managedStorage := imagestorage.ManagedStorage(s.storage, s.session)
	err := managedStorage.PutForEnvironment("my-uuid", "path", strings.NewReader("blah"), 4)
	c.Assert(err, gc.IsNil)

	addedMetadata := &imagestorage.Metadata{
		EnvUUID:   "my-uuid",
		Kind:      "lxc",
		Series:    "trusty",
		Arch:      "amd64",
		Size:      6,
		SHA256:    "hash(xyzzzz)",
		SourceURL: "http://path",
	}
	err = s.storage.AddImage(strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, gc.IsNil)

	// old blob should be gone
	_, _, err = managedStorage.GetForEnvironment("my-uuid", "path")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.assertImage(c, addedMetadata, "xyzzzz")
}

func (s *ImageSuite) TestAddImageRemovesExistingRemoveFails(c *gc.C) {
	// Add a metadata doc and a blob at a known path, then
	// call AddImage and ensure that AddImage attempts to remove
	// the original blob, but does not return an error if it
	// fails.
	s.addMetadataDoc(c, "lxc", "trusty", "amd64", 3, "hash(abc)", "path", "http://path")
	managedStorage := imagestorage.ManagedStorage(s.storage, s.session)
	err := managedStorage.PutForEnvironment("my-uuid", "path", strings.NewReader("blah"), 4)
	c.Assert(err, gc.IsNil)

	storage := imagestorage.NewStorage(s.session, "my-uuid")
	s.PatchValue(imagestorage.GetManagedStorage, imagestorage.RemoveFailsManagedStorage)
	addedMetadata := &imagestorage.Metadata{
		EnvUUID:   "my-uuid",
		Kind:      "lxc",
		Series:    "trusty",
		Arch:      "amd64",
		Size:      6,
		SHA256:    "hash(xyzzzz)",
		SourceURL: "http://path",
	}
	err = storage.AddImage(strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, gc.IsNil)

	// old blob should still be there
	r, _, err := managedStorage.GetForEnvironment("my-uuid", "path")
	c.Assert(err, gc.IsNil)
	r.Close()

	s.assertImage(c, addedMetadata, "xyzzzz")
}

type errorTransactionRunner struct {
	txn.Runner
}

func (errorTransactionRunner) Run(transactions txn.TransactionSource) error {
	return errors.New("Run fails")
}

func (s *ImageSuite) TestAddImageRemovesBlobOnFailure(c *gc.C) {
	storage := imagestorage.NewStorage(s.session, "my-uuid")
	s.txnRunner = errorTransactionRunner{s.txnRunner}
	addedMetadata := &imagestorage.Metadata{
		EnvUUID: "my-uuid",
		Kind:    "lxc",
		Series:  "trusty",
		Arch:    "amd64",
		Size:    6,
		SHA256:  "hash",
	}
	err := storage.AddImage(strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, gc.ErrorMatches, "cannot store image metadata: Run fails")

	path := fmt.Sprintf(
		"images/%s-%s-%s:%s", addedMetadata.Kind, addedMetadata.Series, addedMetadata.Arch, addedMetadata.SHA256)
	managedStorage := imagestorage.ManagedStorage(s.storage, s.session)
	_, _, err = managedStorage.GetForEnvironment("my-uuid", path)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ImageSuite) TestAddImageRemovesBlobOnFailureRemoveFails(c *gc.C) {
	storage := imagestorage.NewStorage(s.session, "my-uuid")
	s.PatchValue(imagestorage.GetManagedStorage, imagestorage.RemoveFailsManagedStorage)
	s.txnRunner = errorTransactionRunner{s.txnRunner}
	addedMetadata := &imagestorage.Metadata{
		EnvUUID: "my-uuid",
		Kind:    "lxc",
		Series:  "trusty",
		Arch:    "amd64",
		Size:    6,
		SHA256:  "hash",
	}
	err := storage.AddImage(strings.NewReader("xyzzzz"), addedMetadata)
	c.Assert(err, gc.ErrorMatches, "cannot store image metadata: Run fails")

	// blob should still be there, because the removal failed.
	path := fmt.Sprintf(
		"images/%s-%s-%s:%s", addedMetadata.Kind, addedMetadata.Series, addedMetadata.Arch, addedMetadata.SHA256)
	managedStorage := imagestorage.ManagedStorage(s.storage, s.session)
	r, _, err := managedStorage.GetForEnvironment("my-uuid", path)
	c.Assert(err, gc.IsNil)
	r.Close()
}

func (s *ImageSuite) TestAddImageSame(c *gc.C) {
	metadata := &imagestorage.Metadata{
		EnvUUID: "my-uuid", Kind: "lxc", Series: "trusty", Arch: "amd64", Size: 1, SHA256: "0", SourceURL: "http://path",
	}
	for i := 0; i < 2; i++ {
		err := s.storage.AddImage(strings.NewReader("0"), metadata)
		c.Assert(err, gc.IsNil)
		s.assertImage(c, metadata, "0")
	}
}

func (s *ImageSuite) TestAddImageAndJustMetadataExists(c *gc.C) {
	s.addMetadataDoc(c, "lxc", "trusty", "amd64", 3, "hash(abc)", "images/lxc-trusty-amd64:hash(abc)", "http://path")
	n, err := s.metadataCollection.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1)
	s.testAddImage(c, "abc")
	n, err = s.metadataCollection.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1)
}

func (s *ImageSuite) TestJustMetadataFails(c *gc.C) {
	s.addMetadataDoc(c, "lxc", "trusty", "amd64", 3, "hash(abc)", "images/lxc-trusty-amd64:hash(abc)", "http://path")
	_, rc, err := s.storage.Image("lxc", "trusty", "amd64")
	c.Assert(rc, gc.IsNil)
	c.Assert(err, gc.NotNil)
}

func (s *ImageSuite) TestAddImageConcurrent(c *gc.C) {
	metadata0 := &imagestorage.Metadata{
		EnvUUID: "my-uuid", Kind: "lxc", Series: "trusty", Arch: "amd64", Size: 1, SHA256: "0", SourceURL: "http://path",
	}
	metadata1 := &imagestorage.Metadata{
		EnvUUID: "my-uuid", Kind: "lxc", Series: "trusty", Arch: "amd64", Size: 1, SHA256: "1", SourceURL: "http://path",
	}

	addMetadata := func() {
		err := s.storage.AddImage(strings.NewReader("0"), metadata0)
		c.Assert(err, gc.IsNil)
		managedStorage := imagestorage.ManagedStorage(s.storage, s.session)
		r, _, err := managedStorage.GetForEnvironment("my-uuid", "images/lxc-trusty-amd64:0")
		c.Assert(err, gc.IsNil)
		r.Close()
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, addMetadata).Check()

	err := s.storage.AddImage(strings.NewReader("1"), metadata1)
	c.Assert(err, gc.IsNil)

	// Blob added in before-hook should be removed.
	managedStorage := imagestorage.ManagedStorage(s.storage, s.session)
	_, _, err = managedStorage.GetForEnvironment("my-uuid", "images/lxc-trusty-amd64:0")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	s.assertImage(c, metadata1, "1")
}

func (s *ImageSuite) TestAddImageExcessiveContention(c *gc.C) {
	metadata := []*imagestorage.Metadata{
		{EnvUUID: "my-uuid", Kind: "lxc", Series: "trusty", Arch: "amd64", Size: 1, SHA256: "0", SourceURL: "http://path"},
		{EnvUUID: "my-uuid", Kind: "lxc", Series: "trusty", Arch: "amd64", Size: 1, SHA256: "1", SourceURL: "http://path"},
		{EnvUUID: "my-uuid", Kind: "lxc", Series: "trusty", Arch: "amd64", Size: 1, SHA256: "2", SourceURL: "http://path"},
		{EnvUUID: "my-uuid", Kind: "lxc", Series: "trusty", Arch: "amd64", Size: 1, SHA256: "3", SourceURL: "http://path"},
	}

	i := 1
	addMetadata := func() {
		err := s.storage.AddImage(strings.NewReader(metadata[i].SHA256), metadata[i])
		c.Assert(err, gc.IsNil)
		i++
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, addMetadata, addMetadata, addMetadata).Check()

	err := s.storage.AddImage(strings.NewReader(metadata[0].SHA256), metadata[0])
	c.Assert(err, gc.ErrorMatches, "cannot store image metadata: state changing too quickly; try again soon")

	// There should be no blobs apart from the last one added by the before-hook.
	for _, metadata := range metadata[:3] {
		path := fmt.Sprintf("images/%s-%s-%s:%s", metadata.Kind, metadata.Series, metadata.Arch, metadata.SHA256)
		managedStorage := imagestorage.ManagedStorage(s.storage, s.session)
		_, _, err = managedStorage.GetForEnvironment("my-uuid", path)
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}

	s.assertImage(c, metadata[3], "3")
}

func (s *ImageSuite) TestDeleteImage(c *gc.C) {
	s.addMetadataDoc(c, "lxc", "trusty", "amd64", 3, "hash(abc)", "images/lxc-trusty-amd64:sha256", "http://lxc-trusty-amd64")
	managedStorage := imagestorage.ManagedStorage(s.storage, s.session)
	err := managedStorage.PutForEnvironment("my-uuid", "images/lxc-trusty-amd64:sha256", strings.NewReader("blah"), 4)
	c.Assert(err, gc.IsNil)

	_, rc, err := s.storage.Image("lxc", "trusty", "amd64")
	c.Assert(err, gc.IsNil)
	c.Assert(rc, gc.NotNil)
	rc.Close()

	metadata := &imagestorage.Metadata{
		EnvUUID: "my-uuid",
		Kind:    "lxc",
		Series:  "trusty",
		Arch:    "amd64",
		SHA256:  "sha256",
	}
	err = s.storage.DeleteImage(metadata)
	c.Assert(err, gc.IsNil)

	_, _, err = managedStorage.GetForEnvironment("my-uuid", "images/lxc-trusty-amd64:sha256")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, _, err = s.storage.Image("lxc", "trusty", "amd64")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ImageSuite) TestDeleteNotExistentImage(c *gc.C) {
	metadata := &imagestorage.Metadata{
		EnvUUID: "my-uuid",
		Kind:    "lxc",
		Series:  "trusty",
		Arch:    "amd64",
		SHA256:  "sha256",
	}
	err := s.storage.DeleteImage(metadata)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ImageSuite) addMetadataDoc(c *gc.C, kind, series, arch string, size int64, checksum, path, sourceURL string) {
	doc := struct {
		Id        string    `bson:"_id"`
		EnvUUID   string    `bson:"envuuid"`
		Kind      string    `bson:"kind"`
		Series    string    `bson:"series"`
		Arch      string    `bson:"arch"`
		Size      int64     `bson:"size"`
		SHA256    string    `bson:"sha256,omitempty"`
		Path      string    `bson:"path"`
		Created   time.Time `bson:"created"`
		SourceURL string    `bson:"sourceurl"`
	}{
		Id:        fmt.Sprintf("my-uuid-%s-%s-%s", kind, series, arch),
		EnvUUID:   "my-uuid",
		Kind:      kind,
		Series:    series,
		Arch:      arch,
		Size:      size,
		SHA256:    checksum,
		Path:      path,
		Created:   time.Now(),
		SourceURL: sourceURL,
	}
	err := s.metadataCollection.Insert(&doc)
	c.Assert(err, gc.IsNil)
}

func (s *ImageSuite) assertImage(c *gc.C, expected *imagestorage.Metadata, content string) {
	metadata, r, err := s.storage.Image(expected.Kind, expected.Series, expected.Arch)
	c.Assert(err, gc.IsNil)
	defer r.Close()
	checkMetadata(c, metadata, expected)

	data, err := ioutil.ReadAll(r)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, content)
}

func (s *ImageSuite) createListImageMetadata(c *gc.C) []*imagestorage.Metadata {
	s.addMetadataDoc(c, "lxc", "trusty", "amd64", 3, "hash(abc)", "images/lxc-trusty-amd64:sha256", "http://lxc-trusty-amd64")
	metadataLxc := &imagestorage.Metadata{
		EnvUUID:   "my-uuid",
		Kind:      "lxc",
		Series:    "trusty",
		Arch:      "amd64",
		SHA256:    "hash(abc)",
		Size:      3,
		SourceURL: "http://lxc-trusty-amd64",
	}
	s.addMetadataDoc(c, "kvm", "precise", "amd64", 4, "hash(abcd)", "images/kvm-precise-amd64:sha256", "http://kvm-precise-amd64")
	metadataKvm := &imagestorage.Metadata{
		EnvUUID:   "my-uuid",
		Kind:      "kvm",
		Series:    "precise",
		Arch:      "amd64",
		SHA256:    "hash(abcd)",
		Size:      4,
		SourceURL: "http://kvm-precise-amd64",
	}
	return []*imagestorage.Metadata{metadataLxc, metadataKvm}
}

func (s *ImageSuite) TestListAllImages(c *gc.C) {
	testMetadata := s.createListImageMetadata(c)
	metadata, err := s.storage.ListImages(imagestorage.ImageFilter{})
	c.Assert(err, gc.IsNil)
	checkAllMetadata(c, metadata, testMetadata...)
}

func (s *ImageSuite) TestListImagesByKind(c *gc.C) {
	testMetadata := s.createListImageMetadata(c)
	metadata, err := s.storage.ListImages(imagestorage.ImageFilter{Kind: "lxc"})
	c.Assert(err, gc.IsNil)
	checkAllMetadata(c, metadata, testMetadata[0])
}

func (s *ImageSuite) TestListImagesBySeries(c *gc.C) {
	testMetadata := s.createListImageMetadata(c)
	metadata, err := s.storage.ListImages(imagestorage.ImageFilter{Series: "precise"})
	c.Assert(err, gc.IsNil)
	checkAllMetadata(c, metadata, testMetadata[1])
}

func (s *ImageSuite) TestListImagesByArch(c *gc.C) {
	testMetadata := s.createListImageMetadata(c)
	metadata, err := s.storage.ListImages(imagestorage.ImageFilter{Arch: "amd64"})
	c.Assert(err, gc.IsNil)
	checkAllMetadata(c, metadata, testMetadata...)
}

func (s *ImageSuite) TestListImagesNoMatch(c *gc.C) {
	metadata, err := s.storage.ListImages(imagestorage.ImageFilter{Series: "utopic"})
	c.Assert(err, gc.IsNil)
	checkAllMetadata(c, metadata)
}

func (s *ImageSuite) TestListImagesMultiFilter(c *gc.C) {
	testMetadata := s.createListImageMetadata(c)
	metadata, err := s.storage.ListImages(imagestorage.ImageFilter{Series: "trusty", Arch: "amd64"})
	c.Assert(err, gc.IsNil)
	checkAllMetadata(c, metadata, testMetadata[0])
}
