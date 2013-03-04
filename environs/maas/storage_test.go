package maas

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gomaasapi"
	"launchpad.net/juju-core/environs"
	"math/rand"
	"sync"
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

func (s *StorageSuite) TestGetSnapshotCreatesClone(c *C) {
	original := s.makeStorage("storage-name")
	snapshot := original.getSnapshot()
	c.Check(snapshot.namingPrefix, Equals, original.namingPrefix)
	c.Check(snapshot.environUnlocked, Equals, original.environUnlocked)
	c.Check(snapshot.maasClientUnlocked.URL().String(), Equals, original.maasClientUnlocked.URL().String())
	// Snapshotting locks the original internally, but does not leave
	// either the original or the snapshot locked.
	c.Check(original.Mutex, Equals, sync.Mutex{})
	c.Check(snapshot.Mutex, Equals, sync.Mutex{})
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
