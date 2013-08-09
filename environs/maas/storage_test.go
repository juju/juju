// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"bytes"
	"encoding/base64"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"sync"

	gc "launchpad.net/gocheck"
	"launchpad.net/gomaasapi"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/errors"
	jc "launchpad.net/juju-core/testing/checkers"
)

type StorageSuite struct {
	ProviderSuite
}

var _ = gc.Suite(&StorageSuite{})

// makeStorage creates a MAAS storage object for the running test.
func (s *StorageSuite) makeStorage(name string) *maasStorage {
	maasobj := s.testMAASObject.MAASObject
	env := maasEnviron{name: name, maasClientUnlocked: &maasobj}
	return NewStorage(&env).(*maasStorage)
}

// makeRandomBytes returns an array of arbitrary byte values.
func makeRandomBytes(length int) []byte {
	data := make([]byte, length)
	for index := range data {
		data[index] = byte(rand.Intn(256))
	}
	return data
}

// fakeStoredFile creates a file directly in the (simulated) MAAS file store.
// It will contain an arbitrary amount of random data.  The contents are also
// returned.
//
// If you want properly random data here, initialize the randomizer first.
// Or don't, if you want consistent (and debuggable) results.
func (s *StorageSuite) fakeStoredFile(storage environs.Storage, name string) gomaasapi.MAASObject {
	data := makeRandomBytes(rand.Intn(10))
	return s.testMAASObject.TestServer.NewFile(name, data)
}

func (s *StorageSuite) TestGetSnapshotCreatesClone(c *gc.C) {
	original := s.makeStorage("storage-name")
	snapshot := original.getSnapshot()
	c.Check(snapshot.environUnlocked, gc.Equals, original.environUnlocked)
	c.Check(snapshot.maasClientUnlocked.URL().String(), gc.Equals, original.maasClientUnlocked.URL().String())
	// Snapshotting locks the original internally, but does not leave
	// either the original or the snapshot locked.
	unlockedMutexValue := sync.Mutex{}
	c.Check(original.Mutex, gc.Equals, unlockedMutexValue)
	c.Check(snapshot.Mutex, gc.Equals, unlockedMutexValue)
}

func (s *StorageSuite) TestGetRetrievesFile(c *gc.C) {
	const filename = "stored-data"
	storage := s.makeStorage("get-retrieves-file")
	file := s.fakeStoredFile(storage, filename)
	base64Content, err := file.GetField("content")
	c.Assert(err, gc.IsNil)
	content, err := base64.StdEncoding.DecodeString(base64Content)
	c.Assert(err, gc.IsNil)

	reader, err := storage.Get(filename)
	c.Assert(err, gc.IsNil)
	defer reader.Close()

	buf, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(len(buf), gc.Equals, len(content))
	c.Check(buf, gc.DeepEquals, content)
}

func (s *StorageSuite) TestRetrieveFileObjectReturnsFileObject(c *gc.C) {
	const filename = "myfile"
	stor := s.makeStorage("rfo-test")
	file := s.fakeStoredFile(stor, filename)
	fileURI, err := file.GetField("anon_resource_uri")
	c.Assert(err, gc.IsNil)
	fileContent, err := file.GetField("content")
	c.Assert(err, gc.IsNil)

	obj, err := stor.retrieveFileObject(filename)
	c.Assert(err, gc.IsNil)

	uri, err := obj.GetField("anon_resource_uri")
	c.Assert(err, gc.IsNil)
	c.Check(uri, gc.Equals, fileURI)
	content, err := obj.GetField("content")
	c.Check(content, gc.Equals, fileContent)
}

func (s *StorageSuite) TestRetrieveFileObjectReturnsNotFoundForMissingFile(c *gc.C) {
	stor := s.makeStorage("rfo-test")
	_, err := stor.retrieveFileObject("nonexistent-file")
	c.Assert(err, gc.NotNil)
	c.Check(err, jc.Satisfies, errors.IsNotFoundError)
}

func (s *StorageSuite) TestRetrieveFileObjectEscapesName(c *gc.C) {
	const filename = "#a?b c&d%e!"
	data := []byte("File contents here")
	stor := s.makeStorage("rfo-test")
	err := stor.Put(filename, bytes.NewReader(data), int64(len(data)))
	c.Assert(err, gc.IsNil)

	obj, err := stor.retrieveFileObject(filename)
	c.Assert(err, gc.IsNil)

	base64Content, err := obj.GetField("content")
	c.Assert(err, gc.IsNil)
	content, err := base64.StdEncoding.DecodeString(base64Content)
	c.Assert(err, gc.IsNil)
	c.Check(content, gc.DeepEquals, data)
}

func (s *StorageSuite) TestFileContentsAreBinary(c *gc.C) {
	const filename = "myfile.bin"
	data := []byte{0, 1, 255, 2, 254, 3}
	stor := s.makeStorage("binary-test")

	err := stor.Put(filename, bytes.NewReader(data), int64(len(data)))
	c.Assert(err, gc.IsNil)
	file, err := stor.Get(filename)
	c.Assert(err, gc.IsNil)
	content, err := ioutil.ReadAll(file)
	c.Assert(err, gc.IsNil)

	c.Check(content, gc.DeepEquals, data)
}

func (s *StorageSuite) TestGetReturnsNotFoundErrorIfNotFound(c *gc.C) {
	const filename = "lost-data"
	storage := NewStorage(s.environ)
	_, err := storage.Get(filename)
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func (s *StorageSuite) TestListReturnsEmptyIfNoFilesStored(c *gc.C) {
	storage := NewStorage(s.environ)
	listing, err := storage.List("")
	c.Assert(err, gc.IsNil)
	c.Check(listing, gc.DeepEquals, []string{})
}

func (s *StorageSuite) TestListReturnsAllFilesIfPrefixEmpty(c *gc.C) {
	storage := NewStorage(s.environ)
	files := []string{"1a", "2b", "3c"}
	for _, name := range files {
		s.fakeStoredFile(storage, name)
	}

	listing, err := storage.List("")
	c.Assert(err, gc.IsNil)
	c.Check(listing, gc.DeepEquals, files)
}

func (s *StorageSuite) TestListSortsResults(c *gc.C) {
	storage := NewStorage(s.environ)
	files := []string{"4d", "1a", "3c", "2b"}
	for _, name := range files {
		s.fakeStoredFile(storage, name)
	}

	listing, err := storage.List("")
	c.Assert(err, gc.IsNil)
	c.Check(listing, gc.DeepEquals, []string{"1a", "2b", "3c", "4d"})
}

func (s *StorageSuite) TestListReturnsNoFilesIfNoFilesMatchPrefix(c *gc.C) {
	storage := NewStorage(s.environ)
	s.fakeStoredFile(storage, "foo")

	listing, err := storage.List("bar")
	c.Assert(err, gc.IsNil)
	c.Check(listing, gc.DeepEquals, []string{})
}

func (s *StorageSuite) TestListReturnsOnlyFilesWithMatchingPrefix(c *gc.C) {
	storage := NewStorage(s.environ)
	s.fakeStoredFile(storage, "abc")
	s.fakeStoredFile(storage, "xyz")

	listing, err := storage.List("x")
	c.Assert(err, gc.IsNil)
	c.Check(listing, gc.DeepEquals, []string{"xyz"})
}

func (s *StorageSuite) TestListMatchesPrefixOnly(c *gc.C) {
	storage := NewStorage(s.environ)
	s.fakeStoredFile(storage, "abc")
	s.fakeStoredFile(storage, "xabc")

	listing, err := storage.List("a")
	c.Assert(err, gc.IsNil)
	c.Check(listing, gc.DeepEquals, []string{"abc"})
}

func (s *StorageSuite) TestListOperatesOnFlatNamespace(c *gc.C) {
	storage := NewStorage(s.environ)
	s.fakeStoredFile(storage, "a/b/c/d")

	listing, err := storage.List("a/b")
	c.Assert(err, gc.IsNil)
	c.Check(listing, gc.DeepEquals, []string{"a/b/c/d"})
}

// getFileAtURL requests, and returns, the file at the given URL.
func getFileAtURL(fileURL string) ([]byte, error) {
	response, err := http.Get(fileURL)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (s *StorageSuite) TestURLReturnsURLCorrespondingToFile(c *gc.C) {
	const filename = "my-file.txt"
	storage := NewStorage(s.environ).(*maasStorage)
	file := s.fakeStoredFile(storage, filename)
	// The file contains an anon_resource_uri, which lacks a network part
	// (but will probably contain a query part).  anonURL will be the
	// file's full URL.
	anonURI, err := file.GetField("anon_resource_uri")
	c.Assert(err, gc.IsNil)
	parsedURI, err := url.Parse(anonURI)
	c.Assert(err, gc.IsNil)
	anonURL := storage.maasClientUnlocked.URL().ResolveReference(parsedURI)
	c.Assert(err, gc.IsNil)

	fileURL, err := storage.URL(filename)
	c.Assert(err, gc.IsNil)

	c.Check(fileURL, gc.NotNil)
	c.Check(fileURL, gc.Equals, anonURL.String())
}

func (s *StorageSuite) TestPutStoresRetrievableFile(c *gc.C) {
	const filename = "broken-toaster.jpg"
	contents := []byte("Contents here")
	length := int64(len(contents))
	storage := NewStorage(s.environ)

	err := storage.Put(filename, bytes.NewReader(contents), length)

	reader, err := storage.Get(filename)
	c.Assert(err, gc.IsNil)
	defer reader.Close()

	buf, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(buf, gc.DeepEquals, contents)
}

func (s *StorageSuite) TestPutOverwritesFile(c *gc.C) {
	const filename = "foo.bar"
	storage := NewStorage(s.environ)
	s.fakeStoredFile(storage, filename)
	newContents := []byte("Overwritten")

	err := storage.Put(filename, bytes.NewReader(newContents), int64(len(newContents)))
	c.Assert(err, gc.IsNil)

	reader, err := storage.Get(filename)
	c.Assert(err, gc.IsNil)
	defer reader.Close()

	buf, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(len(buf), gc.Equals, len(newContents))
	c.Check(buf, gc.DeepEquals, newContents)
}

func (s *StorageSuite) TestPutStopsAtGivenLength(c *gc.C) {
	const filename = "xyzzyz.2.xls"
	const length = 5
	contents := []byte("abcdefghijklmnopqrstuvwxyz")
	storage := NewStorage(s.environ)

	err := storage.Put(filename, bytes.NewReader(contents), length)
	c.Assert(err, gc.IsNil)

	reader, err := storage.Get(filename)
	c.Assert(err, gc.IsNil)
	defer reader.Close()

	buf, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(len(buf), gc.Equals, length)
}

func (s *StorageSuite) TestPutToExistingFileTruncatesAtGivenLength(c *gc.C) {
	const filename = "a-file-which-is-mine"
	oldContents := []byte("abcdefghijklmnopqrstuvwxyz")
	newContents := []byte("xyz")
	storage := NewStorage(s.environ)
	err := storage.Put(filename, bytes.NewReader(oldContents), int64(len(oldContents)))
	c.Assert(err, gc.IsNil)

	err = storage.Put(filename, bytes.NewReader(newContents), int64(len(newContents)))
	c.Assert(err, gc.IsNil)

	reader, err := storage.Get(filename)
	c.Assert(err, gc.IsNil)
	defer reader.Close()

	buf, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(len(buf), gc.Equals, len(newContents))
	c.Check(buf, gc.DeepEquals, newContents)
}

func (s *StorageSuite) TestRemoveDeletesFile(c *gc.C) {
	const filename = "doomed.txt"
	storage := NewStorage(s.environ)
	s.fakeStoredFile(storage, filename)

	err := storage.Remove(filename)
	c.Assert(err, gc.IsNil)

	_, err = storage.Get(filename)
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	listing, err := storage.List(filename)
	c.Assert(err, gc.IsNil)
	c.Assert(listing, gc.DeepEquals, []string{})
}

func (s *StorageSuite) TestRemoveIsIdempotent(c *gc.C) {
	const filename = "half-a-file"
	storage := NewStorage(s.environ)
	s.fakeStoredFile(storage, filename)

	err := storage.Remove(filename)
	c.Assert(err, gc.IsNil)

	err = storage.Remove(filename)
	c.Assert(err, gc.IsNil)
}

func (s *StorageSuite) TestNamesMayHaveSlashes(c *gc.C) {
	const filename = "name/with/slashes"
	content := []byte("File contents")
	storage := NewStorage(s.environ)

	err := storage.Put(filename, bytes.NewReader(content), int64(len(content)))
	c.Assert(err, gc.IsNil)

	// There's not much we can say about the anonymous URL, except that
	// we get one.
	anonURL, err := storage.URL(filename)
	c.Assert(err, gc.IsNil)
	c.Check(anonURL, gc.Matches, "http[s]*://.*")

	reader, err := storage.Get(filename)
	c.Assert(err, gc.IsNil)
	defer reader.Close()
	data, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(data, gc.DeepEquals, content)
}

func (s *StorageSuite) TestRemoveAllDeletesAllFiles(c *gc.C) {
	storage := s.makeStorage("get-retrieves-file")
	const filename1 = "stored-data1"
	s.fakeStoredFile(storage, filename1)
	const filename2 = "stored-data2"
	s.fakeStoredFile(storage, filename2)

	err := storage.RemoveAll()
	c.Assert(err, gc.IsNil)
	listing, err := storage.List("")
	c.Assert(err, gc.IsNil)
	c.Assert(listing, gc.DeepEquals, []string{})
}
