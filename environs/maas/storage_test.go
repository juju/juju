package maas

import (
	"bytes"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"math/rand"
	"net/http"
	"path/filepath"
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

// fakeStoredFile creates a file directly in the (simulated) MAAS file store.
// It will contain an arbitrary amount of random data.  The contents are also
// returned.
//
// If you want properly random data here, initialize the randomizer first.
// Or don't, if you want consistent (and debuggable) results.
func (s *StorageSuite) fakeStoredFile(storage environs.Storage, name string) []byte {
	length := rand.Intn(10)
	data := make([]byte, length)
	for index := range data {
		data[index] = byte(rand.Intn(256))
	}
	fullPath := filepath.Join(storage.(*maasStorage).namingPrefix, name)
	s.testMAASObject.TestServer.NewFile(fullPath, data)
	return data
}

func (s *StorageSuite) TestGetRetrievesFile(c *C) {
	const filename = "stored-data"
	storage := s.makeStorage("get-retrieves-file")
	contents := s.fakeStoredFile(storage, filename)
	buf := make([]byte, len(contents)+1)

	reader, err := storage.Get(filename)
	c.Assert(err, IsNil)
	defer reader.Close()

	numBytes, err := reader.Read(buf)
	c.Assert(err, IsNil)
	c.Check(numBytes, Equals, len(contents))
	c.Check(buf[:numBytes], DeepEquals, contents)
	c.Check(buf[numBytes], Equals, 0)
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

func (s *StorageSuite) TestAddressFileObjectComputesFileURL(c *C) {
	c.Assert("TEST THIS", IsNil)
}

func (s *StorageSuite) TestAddressFileObjectEscapesPrefix(c *C) {
	const name = "a/b c"
	c.Assert("TEST THIS", IsNil)
}

func (s *StorageSuite) TestAddressFileObjectEscapesName(c *C) {
	c.Assert("TEST THIS", IsNil)
}

func (s *StorageSuite) TestComposeAnonymousFileURLComputesFileURL(c *C) {
	c.Assert("TEST THIS", IsNil)
}

func (s *StorageSuite) TestComposeAnonymousFileURLEscapesPrefix(c *C) {
	c.Assert("TEST THIS", IsNil)
}

func (s *StorageSuite) TestComposeAnonymousFileURLEscapesName(c *C) {
	c.Assert("TEST THIS", IsNil)
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

func getURL(fileURL string) ([]byte, error) {
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
	storage := NewStorage(s.environ)
	contents := s.fakeStoredFile(storage, filename)

	fileURL, err := storage.URL(filename)
	c.Assert(err, IsNil)
	c.Check(fileURL, NotNil)

	body, err := getURL(fileURL)
	c.Assert(err, IsNil)
	c.Check(body, DeepEquals, contents)
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
