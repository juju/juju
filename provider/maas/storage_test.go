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

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"
	"launchpad.net/gomaasapi"

	"launchpad.net/juju-core/environs/storage"
)

type storageSuite struct {
	providerSuite
}

var _ = gc.Suite(&storageSuite{})

// makeStorage creates a MAAS storage object for the running test.
func (s *storageSuite) makeStorage(name string) *maasStorage {
	maasobj := s.testMAASObject.MAASObject
	env := s.makeEnviron()
	env.name = name
	env.maasClientUnlocked = &maasobj
	return NewStorage(env).(*maasStorage)
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
func (s *storageSuite) fakeStoredFile(stor storage.Storage, name string) gomaasapi.MAASObject {
	data := makeRandomBytes(rand.Intn(10))
	// The filename must be prefixed with the private namespace as we're
	// bypassing the Put() method that would normally do that.
	prefixFilename := stor.(*maasStorage).prefixWithPrivateNamespace("") + name
	return s.testMAASObject.TestServer.NewFile(prefixFilename, data)
}

func (s *storageSuite) TestGetSnapshotCreatesClone(c *gc.C) {
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

func (s *storageSuite) TestGetRetrievesFile(c *gc.C) {
	const filename = "stored-data"
	stor := s.makeStorage("get-retrieves-file")
	file := s.fakeStoredFile(stor, filename)
	base64Content, err := file.GetField("content")
	c.Assert(err, gc.IsNil)
	content, err := base64.StdEncoding.DecodeString(base64Content)
	c.Assert(err, gc.IsNil)

	reader, err := storage.Get(stor, filename)
	c.Assert(err, gc.IsNil)
	defer reader.Close()

	buf, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(len(buf), gc.Equals, len(content))
	c.Check(buf, gc.DeepEquals, content)
}

func (s *storageSuite) TestRetrieveFileObjectReturnsFileObject(c *gc.C) {
	const filename = "myfile"
	stor := s.makeStorage("rfo-test")
	file := s.fakeStoredFile(stor, filename)
	fileURI, err := file.GetField("anon_resource_uri")
	c.Assert(err, gc.IsNil)
	fileContent, err := file.GetField("content")
	c.Assert(err, gc.IsNil)

	prefixFilename := stor.prefixWithPrivateNamespace(filename)
	obj, err := stor.retrieveFileObject(prefixFilename)
	c.Assert(err, gc.IsNil)

	uri, err := obj.GetField("anon_resource_uri")
	c.Assert(err, gc.IsNil)
	c.Check(uri, gc.Equals, fileURI)
	content, err := obj.GetField("content")
	c.Check(content, gc.Equals, fileContent)
}

func (s *storageSuite) TestRetrieveFileObjectReturnsNotFoundForMissingFile(c *gc.C) {
	stor := s.makeStorage("rfo-test")
	_, err := stor.retrieveFileObject("nonexistent-file")
	c.Assert(err, gc.NotNil)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *storageSuite) TestRetrieveFileObjectEscapesName(c *gc.C) {
	const filename = "#a?b c&d%e!"
	data := []byte("File contents here")
	stor := s.makeStorage("rfo-test")
	err := stor.Put(filename, bytes.NewReader(data), int64(len(data)))
	c.Assert(err, gc.IsNil)

	prefixFilename := stor.prefixWithPrivateNamespace(filename)
	obj, err := stor.retrieveFileObject(prefixFilename)
	c.Assert(err, gc.IsNil)

	base64Content, err := obj.GetField("content")
	c.Assert(err, gc.IsNil)
	content, err := base64.StdEncoding.DecodeString(base64Content)
	c.Assert(err, gc.IsNil)
	c.Check(content, gc.DeepEquals, data)
}

func (s *storageSuite) TestFileContentsAreBinary(c *gc.C) {
	const filename = "myfile.bin"
	data := []byte{0, 1, 255, 2, 254, 3}
	stor := s.makeStorage("binary-test")

	err := stor.Put(filename, bytes.NewReader(data), int64(len(data)))
	c.Assert(err, gc.IsNil)
	file, err := storage.Get(stor, filename)
	c.Assert(err, gc.IsNil)
	content, err := ioutil.ReadAll(file)
	c.Assert(err, gc.IsNil)

	c.Check(content, gc.DeepEquals, data)
}

func (s *storageSuite) TestGetReturnsNotFoundErrorIfNotFound(c *gc.C) {
	const filename = "lost-data"
	stor := NewStorage(s.makeEnviron())
	_, err := storage.Get(stor, filename)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *storageSuite) TestListReturnsEmptyIfNoFilesStored(c *gc.C) {
	stor := NewStorage(s.makeEnviron())
	listing, err := storage.List(stor, "")
	c.Assert(err, gc.IsNil)
	c.Check(listing, gc.DeepEquals, []string{})
}

func (s *storageSuite) TestListReturnsAllFilesIfPrefixEmpty(c *gc.C) {
	stor := NewStorage(s.makeEnviron())
	files := []string{"1a", "2b", "3c"}
	for _, name := range files {
		s.fakeStoredFile(stor, name)
	}

	listing, err := storage.List(stor, "")
	c.Assert(err, gc.IsNil)
	c.Check(listing, gc.DeepEquals, files)
}

func (s *storageSuite) TestListSortsResults(c *gc.C) {
	stor := NewStorage(s.makeEnviron())
	files := []string{"4d", "1a", "3c", "2b"}
	for _, name := range files {
		s.fakeStoredFile(stor, name)
	}

	listing, err := storage.List(stor, "")
	c.Assert(err, gc.IsNil)
	c.Check(listing, gc.DeepEquals, []string{"1a", "2b", "3c", "4d"})
}

func (s *storageSuite) TestListReturnsNoFilesIfNoFilesMatchPrefix(c *gc.C) {
	stor := NewStorage(s.makeEnviron())
	s.fakeStoredFile(stor, "foo")

	listing, err := storage.List(stor, "bar")
	c.Assert(err, gc.IsNil)
	c.Check(listing, gc.DeepEquals, []string{})
}

func (s *storageSuite) TestListReturnsOnlyFilesWithMatchingPrefix(c *gc.C) {
	stor := NewStorage(s.makeEnviron())
	s.fakeStoredFile(stor, "abc")
	s.fakeStoredFile(stor, "xyz")

	listing, err := storage.List(stor, "x")
	c.Assert(err, gc.IsNil)
	c.Check(listing, gc.DeepEquals, []string{"xyz"})
}

func (s *storageSuite) TestListMatchesPrefixOnly(c *gc.C) {
	stor := NewStorage(s.makeEnviron())
	s.fakeStoredFile(stor, "abc")
	s.fakeStoredFile(stor, "xabc")

	listing, err := storage.List(stor, "a")
	c.Assert(err, gc.IsNil)
	c.Check(listing, gc.DeepEquals, []string{"abc"})
}

func (s *storageSuite) TestListOperatesOnFlatNamespace(c *gc.C) {
	stor := NewStorage(s.makeEnviron())
	s.fakeStoredFile(stor, "a/b/c/d")

	listing, err := storage.List(stor, "a/b")
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

func (s *storageSuite) TestURLReturnsURLCorrespondingToFile(c *gc.C) {
	const filename = "my-file.txt"
	stor := NewStorage(s.makeEnviron()).(*maasStorage)
	file := s.fakeStoredFile(stor, filename)
	// The file contains an anon_resource_uri, which lacks a network part
	// (but will probably contain a query part).  anonURL will be the
	// file's full URL.
	anonURI, err := file.GetField("anon_resource_uri")
	c.Assert(err, gc.IsNil)
	parsedURI, err := url.Parse(anonURI)
	c.Assert(err, gc.IsNil)
	anonURL := stor.maasClientUnlocked.URL().ResolveReference(parsedURI)
	c.Assert(err, gc.IsNil)

	fileURL, err := stor.URL(filename)
	c.Assert(err, gc.IsNil)

	c.Check(fileURL, gc.NotNil)
	c.Check(fileURL, gc.Equals, anonURL.String())
}

func (s *storageSuite) TestPutStoresRetrievableFile(c *gc.C) {
	const filename = "broken-toaster.jpg"
	contents := []byte("Contents here")
	length := int64(len(contents))
	stor := NewStorage(s.makeEnviron())

	err := stor.Put(filename, bytes.NewReader(contents), length)

	reader, err := storage.Get(stor, filename)
	c.Assert(err, gc.IsNil)
	defer reader.Close()

	buf, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(buf, gc.DeepEquals, contents)
}

func (s *storageSuite) TestPutOverwritesFile(c *gc.C) {
	const filename = "foo.bar"
	stor := NewStorage(s.makeEnviron())
	s.fakeStoredFile(stor, filename)
	newContents := []byte("Overwritten")

	err := stor.Put(filename, bytes.NewReader(newContents), int64(len(newContents)))
	c.Assert(err, gc.IsNil)

	reader, err := storage.Get(stor, filename)
	c.Assert(err, gc.IsNil)
	defer reader.Close()

	buf, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(len(buf), gc.Equals, len(newContents))
	c.Check(buf, gc.DeepEquals, newContents)
}

func (s *storageSuite) TestPutStopsAtGivenLength(c *gc.C) {
	const filename = "xyzzyz.2.xls"
	const length = 5
	contents := []byte("abcdefghijklmnopqrstuvwxyz")
	stor := NewStorage(s.makeEnviron())

	err := stor.Put(filename, bytes.NewReader(contents), length)
	c.Assert(err, gc.IsNil)

	reader, err := storage.Get(stor, filename)
	c.Assert(err, gc.IsNil)
	defer reader.Close()

	buf, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(len(buf), gc.Equals, length)
}

func (s *storageSuite) TestPutToExistingFileTruncatesAtGivenLength(c *gc.C) {
	const filename = "a-file-which-is-mine"
	oldContents := []byte("abcdefghijklmnopqrstuvwxyz")
	newContents := []byte("xyz")
	stor := NewStorage(s.makeEnviron())
	err := stor.Put(filename, bytes.NewReader(oldContents), int64(len(oldContents)))
	c.Assert(err, gc.IsNil)

	err = stor.Put(filename, bytes.NewReader(newContents), int64(len(newContents)))
	c.Assert(err, gc.IsNil)

	reader, err := storage.Get(stor, filename)
	c.Assert(err, gc.IsNil)
	defer reader.Close()

	buf, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(len(buf), gc.Equals, len(newContents))
	c.Check(buf, gc.DeepEquals, newContents)
}

func (s *storageSuite) TestRemoveDeletesFile(c *gc.C) {
	const filename = "doomed.txt"
	stor := NewStorage(s.makeEnviron())
	s.fakeStoredFile(stor, filename)

	err := stor.Remove(filename)
	c.Assert(err, gc.IsNil)

	_, err = storage.Get(stor, filename)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	listing, err := storage.List(stor, filename)
	c.Assert(err, gc.IsNil)
	c.Assert(listing, gc.DeepEquals, []string{})
}

func (s *storageSuite) TestRemoveIsIdempotent(c *gc.C) {
	const filename = "half-a-file"
	stor := NewStorage(s.makeEnviron())
	s.fakeStoredFile(stor, filename)

	err := stor.Remove(filename)
	c.Assert(err, gc.IsNil)

	err = stor.Remove(filename)
	c.Assert(err, gc.IsNil)
}

func (s *storageSuite) TestNamesMayHaveSlashes(c *gc.C) {
	const filename = "name/with/slashes"
	content := []byte("File contents")
	stor := NewStorage(s.makeEnviron())

	err := stor.Put(filename, bytes.NewReader(content), int64(len(content)))
	c.Assert(err, gc.IsNil)

	// There's not much we can say about the anonymous URL, except that
	// we get one.
	anonURL, err := stor.URL(filename)
	c.Assert(err, gc.IsNil)
	c.Check(anonURL, gc.Matches, "http[s]*://.*")

	reader, err := storage.Get(stor, filename)
	c.Assert(err, gc.IsNil)
	defer reader.Close()
	data, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(data, gc.DeepEquals, content)
}

func (s *storageSuite) TestRemoveAllDeletesAllFiles(c *gc.C) {
	stor := s.makeStorage("get-retrieves-file")
	const filename1 = "stored-data1"
	s.fakeStoredFile(stor, filename1)
	const filename2 = "stored-data2"
	s.fakeStoredFile(stor, filename2)

	err := stor.RemoveAll()
	c.Assert(err, gc.IsNil)
	listing, err := storage.List(stor, "")
	c.Assert(err, gc.IsNil)
	c.Assert(listing, gc.DeepEquals, []string{})
}

func (s *storageSuite) TestprefixWithPrivateNamespacePrefixesWithAgentName(c *gc.C) {
	sstor := NewStorage(s.makeEnviron())
	stor := sstor.(*maasStorage)
	agentName := stor.environUnlocked.ecfg().maasAgentName()
	c.Assert(agentName, gc.Not(gc.Equals), "")
	expectedPrefix := agentName + "-"
	const name = "myname"
	expectedResult := expectedPrefix + name
	c.Assert(stor.prefixWithPrivateNamespace(name), gc.Equals, expectedResult)
}

func (s *storageSuite) TesttprefixWithPrivateNamespaceIgnoresAgentName(c *gc.C) {
	sstor := NewStorage(s.makeEnviron())
	stor := sstor.(*maasStorage)
	ecfg := stor.environUnlocked.ecfg()
	ecfg.attrs["maas-agent-name"] = ""

	const name = "myname"
	c.Assert(stor.prefixWithPrivateNamespace(name), gc.Equals, name)
}
