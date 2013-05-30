// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"fmt"
	"io"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
)

type EmptyStorageSuite struct{}

var _ = Suite(&EmptyStorageSuite{})

func (s *EmptyStorageSuite) TestGet(c *C) {
	f, err := environs.EmptyStorage.Get("anything")
	c.Assert(f, IsNil)
	c.Assert(err, ErrorMatches, `file "anything" not found`)
}

func (s *EmptyStorageSuite) TestURL(c *C) {
	url, err := environs.EmptyStorage.URL("anything")
	c.Assert(url, Equals, "")
	c.Assert(err, ErrorMatches, `file "anything" not found`)
}

func (s *EmptyStorageSuite) TestList(c *C) {
	names, err := environs.EmptyStorage.List("anything")
	c.Assert(names, IsNil)
	c.Assert(err, IsNil)
}


type MockStorage struct {
	StorageRequests []string
}

func (ms *MockStorage) Put(filename string, reader io.Reader, length int64) error {
	var content []byte
	content, _ = ioutil.ReadAll(reader)
	log_message := fmt.Sprintf(
		"Put('%s', '%s', %d)", filename, content, length)
	ms.StorageRequests = append(ms.StorageRequests, log_message)
	return nil
}

func (ms *MockStorage) Get(name string) (io.ReadCloser, error) {
	log_message := fmt.Sprintf("Get('%s')", name)
	ms.StorageRequests = append(ms.StorageRequests, log_message)
	return nil, &environs.NotFoundError{fmt.Errorf("file %q not found", name)}
}

func (ms *MockStorage) List(prefix string) ([]string, error) {
	return nil, nil
}

func (ms *MockStorage) Remove(file string) error {
	return nil
}

func (ms *MockStorage) URL(name string) (string, error) {
	return "", fmt.Errorf("file %q not found", name)
}


type VerifyStorageSuite struct{}

var _ = Suite(&VerifyStorageSuite{})

func (s *VerifyStorageSuite) TestVerifyStorage(c *C) {
	storage := MockStorage{}
	error := environs.VerifyStorage(&storage)
	c.Assert(error, IsNil)
	c.Assert(storage.StorageRequests[0], Equals,
		"Put('bootstrap-verify', 'juju-core storage writing verified: ok', 38)")
}
