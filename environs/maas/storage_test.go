package maas

import (
	"bytes"
	"encoding/base64"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/gomaasapi"
	"launchpad.net/juju-core/environs"
	"math/rand"
	"net/http"
)

type StorageSuite struct {
	ProviderSuite
}

var _ = Suite(new(StorageSuite))

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
	fullName := storage.(*maasStorage).namingPrefix + name
	return s.testMAASObject.TestServer.NewFile(fullName, data)
}

func (s *StorageSuite) TestGetRetrievesFile(c *C) {
	const filename = "stored-data"
	storage := s.makeStorage("get-retrieves-file")
	file := s.fakeStoredFile(storage, filename)
	base64Content, err := file.GetField("content")
	c.Assert(err, IsNil)
	content, err := base64.StdEncoding.DecodeString(base64Content)
	c.Assert(err, IsNil)
	buf := make([]byte, len(content)+1)

	reader, err := storage.Get(filename)
	c.Assert(err, IsNil)
	defer reader.Close()

	numBytes, err := reader.Read(buf)
	c.Assert(err, IsNil)
	c.Check(numBytes, Equals, len(content))
	c.Check(buf[:numBytes], DeepEquals, content)
	c.Check(buf[numBytes], Equals, uint8(0))
}

func (s *StorageSuite) TestNamingPrefixDiffersBetweenEnvironments(c *C) {
	storA := s.makeStorage("A")
	storB := s.makeStorage("B")
	c.Check(storA.namingPrefix, Not(Equals), storB.namingPrefix)
}

func (s *StorageSuite) TestNamingPrefixIsConsistentForEnvironment(c *C) {
	stor1 := s.makeStorage("name")
	stor2 := s.makeStorage("name")
	c.Check(stor1.namingPrefix, Equals, stor2.namingPrefix)
}

func (s *StorageSuite) TestRetrieveFileObjectReturnsFileObject(c *C) {
	const filename = "myfile"
	stor := s.makeStorage("rfo-test")
	file := s.fakeStoredFile(stor, filename)
	fileURI, err := file.GetField("anon_resource_uri")
	c.Assert(err, IsNil)
	fileContent, err := file.GetField("content")
	c.Assert(err, IsNil)

	obj, err := stor.retrieveFileObject(filename)
	c.Assert(err, IsNil)

	uri, err := obj.GetField("anon_resource_uri")
	c.Assert(err, IsNil)
	c.Check(uri, Equals, fileURI)
	content, err := obj.GetField("content")
	c.Check(content, Equals, fileContent)
}

func (s *StorageSuite) TestRetrieveFileObjectReturnsNotFound(c *C) {
	stor := s.makeStorage("rfo-test")
	_, err := stor.retrieveFileObject("nonexistent-file")
	c.Assert(err, NotNil)
	c.Check(err, FitsTypeOf, environs.NotFoundError{})
}

func (s *StorageSuite) TestRetrieveFileObjectEscapesName(c *C) {
	const filename = "a/b c?d%e"
	stor := s.makeStorage("rfo-test")
	file := s.fakeStoredFile(stor, filename)
	fileContent, err := file.GetField("content")
	c.Assert(err, IsNil)

	obj, err := stor.retrieveFileObject(filename)
	c.Assert(err, IsNil)

	content, err := obj.GetField("content")
	c.Check(content, Equals, fileContent)
}

func (s *StorageSuite) TestRetrieveFileObjectEscapesPrefix(c *C) {
	const filename = "myfile"
	stor := s.makeStorage("&?%!")
	file := s.fakeStoredFile(stor, filename)
	fileContent, err := file.GetField("content")
	c.Assert(err, IsNil)

	obj, err := stor.retrieveFileObject(filename)
	c.Assert(err, IsNil)

	content, err := obj.GetField("content")
	c.Check(content, Equals, fileContent)
}

func (s *StorageSuite) TestGetReturnsNotFoundErrorIfNotFound(c *C) {
	const filename = "lost-data"
	storage := NewStorage(s.environ)
	_, err := storage.Get(filename)
	c.Assert(err, FitsTypeOf, environs.NotFoundError{})
}

func (s *StorageSuite) TestListReturnsEmptyIfNoFilesStored(c *C) {
	storage := NewStorage(s.environ)
	listing, err := storage.List("")
	c.Assert(err, IsNil)
	c.Check(listing, DeepEquals, []string{})
}

func (s *StorageSuite) TestListReturnsAllFilesIfPrefixEmpty(c *C) {
	storage := NewStorage(s.environ)
	files := []string{"1a", "2b", "3c"}
	for _, name := range files {
		s.fakeStoredFile(storage, name)
	}

	listing, err := storage.List("")
	c.Assert(err, IsNil)
	c.Check(listing, DeepEquals, files)
}

func (s *StorageSuite) TestListSortsResults(c *C) {
	storage := NewStorage(s.environ)
	files := []string{"4d", "1a", "3c", "2b"}
	for _, name := range files {
		s.fakeStoredFile(storage, name)
	}

	listing, err := storage.List("")
	c.Assert(err, IsNil)
	c.Check(listing, DeepEquals, []string{"1a", "2b", "3c", "4d"})
}

func (s *StorageSuite) TestListReturnsNoFilesIfNoFilesMatchPrefix(c *C) {
	storage := NewStorage(s.environ)
	s.fakeStoredFile(storage, "foo")

	listing, err := storage.List("bar")
	c.Assert(err, IsNil)
	c.Check(listing, DeepEquals, []string{})
}

func (s *StorageSuite) TestListReturnsOnlyFilesWithMatchingPrefix(c *C) {
	storage := NewStorage(s.environ)
	s.fakeStoredFile(storage, "abc")
	s.fakeStoredFile(storage, "xyz")

	listing, err := storage.List("x")
	c.Assert(err, IsNil)
	c.Check(listing, DeepEquals, []string{"xyz"})
}

func (s *StorageSuite) TestListMatchesPrefixOnly(c *C) {
	storage := NewStorage(s.environ)
	s.fakeStoredFile(storage, "abc")
	s.fakeStoredFile(storage, "xabc")

	listing, err := storage.List("a")
	c.Assert(err, IsNil)
	c.Check(listing, DeepEquals, []string{"abc"})
}

func (s *StorageSuite) TestListOperatesOnFlatNamespace(c *C) {
	storage := NewStorage(s.environ)
	s.fakeStoredFile(storage, "a/b/c/d")

	listing, err := storage.List("a/b")
	c.Assert(err, IsNil)
	c.Check(listing, DeepEquals, []string{"a/b/c/d"})
}

// getFileAtURL requests, and returns, the file at the given URL.
func getFileAtURL(fileURL string) ([]byte, error) {
	request, err := http.NewRequest("GET", fileURL, nil)
	if err != nil {
		return nil, err
	}
	client := http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (s *StorageSuite) TestURLReturnsURLCorrespondingToFile(c *C) {
	const filename = "my-file.txt"
	storage := NewStorage(s.environ).(*maasStorage)
	file := s.fakeStoredFile(storage, filename)
	// The file contains an anon_resource_uri, which consists of a path
	// only.  anonURL will be the file's full URL.
	anonURI, err := file.GetField("anon_resource_uri")
	anonURL := storage.maasClientUnlocked.GetSubObject(anonURI).URL()
	c.Assert(err, IsNil)

	fileURL, err := storage.URL(filename)
	c.Assert(err, IsNil)

	c.Check(fileURL, NotNil)
	c.Check(fileURL, Equals, anonURL.String())
}

func (s *StorageSuite) TestPutStoresRetrievableFile(c *C) {
	const filename = "broken-toaster.jpg"
	contents := []byte("Contents here")
	storage := NewStorage(s.environ)

	err := storage.Put(filename, bytes.NewReader(contents), int64(len(contents)))

	buf := make([]byte, len(contents))
	reader, err := storage.Get(filename)
	c.Assert(err, IsNil)
	defer reader.Close()

	_, err = reader.Read(buf)
	c.Assert(err, IsNil)
	c.Check(buf, DeepEquals, contents)
}

func (s *StorageSuite) TestPutOverwritesFile(c *C) {
	const filename = "foo.bar"
	storage := NewStorage(s.environ)
	s.fakeStoredFile(storage, filename)
	newContents := []byte("Overwritten")

	err := storage.Put(filename, bytes.NewReader(newContents), int64(len(newContents)))
	c.Assert(err, IsNil)

	buf := make([]byte, len(newContents))
	reader, err := storage.Get(filename)
	c.Assert(err, IsNil)
	defer reader.Close()

	_, err = reader.Read(buf)
	c.Assert(err, IsNil)
	c.Check(buf, DeepEquals, newContents)
}

func (s *StorageSuite) TestPutStopsAtGivenLength(c *C) {
	const filename = "xyzzyz.2.xls"
	const length = 5
	contents := []byte("abcdefghijklmnopqrstuvwxyz")
	storage := NewStorage(s.environ)

	err := storage.Put(filename, bytes.NewReader(contents), length)
	c.Assert(err, IsNil)

	buf := make([]byte, length+1)
	reader, err := storage.Get(filename)
	c.Assert(err, IsNil)
	defer reader.Close()

	numBytes, err := reader.Read(buf)
	c.Assert(err, IsNil)
	c.Check(numBytes, Equals, length)
}

func (s *StorageSuite) TestPutToExistingFileTruncatesAtGivenLength(c *C) {
	const filename = "a-file-which-is-mine"
	oldContents := []byte("abcdefghijklmnopqrstuvwxyz")
	newContents := []byte("xyz")
	storage := NewStorage(s.environ)
	err := storage.Put(filename, bytes.NewReader(oldContents), int64(len(oldContents)))
	c.Assert(err, IsNil)

	err = storage.Put(filename, bytes.NewReader(newContents), int64(len(newContents)))
	c.Assert(err, IsNil)

	buf := make([]byte, len(newContents)+1)
	reader, err := storage.Get(filename)
	c.Assert(err, IsNil)
	defer reader.Close()

	numBytes, err := reader.Read(buf)
	c.Assert(err, IsNil)
	c.Check(numBytes, Equals, len(newContents))
	c.Check(buf[:len(newContents)], DeepEquals, newContents)
	c.Check(buf[len(newContents)], Equals, 0)
}

func (s *StorageSuite) TestRemoveDeletesFile(c *C) {
	const filename = "doomed.txt"
	storage := NewStorage(s.environ)
	s.fakeStoredFile(storage, filename)

	err := storage.Remove(filename)
	c.Assert(err, IsNil)

	_, err = storage.Get(filename)
	c.Assert(err, FitsTypeOf, environs.NotFoundError{})

	listing, err := storage.List(filename)
	c.Assert(err, IsNil)
	c.Assert(listing, DeepEquals, []string{})
}

func (s *StorageSuite) TestRemoveIsIdempotent(c *C) {
	const filename = "half-a-file"
	storage := NewStorage(s.environ)
	s.fakeStoredFile(storage, filename)

	err := storage.Remove(filename)
	c.Assert(err, IsNil)

	err = storage.Remove(filename)
	c.Assert(err, IsNil)
}
