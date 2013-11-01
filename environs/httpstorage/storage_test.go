// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpstorage_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/httpstorage"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/errors"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

type storageSuite struct{}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) TestClientTLS(c *gc.C) {
	listener, _, storageDir := startServerTLS(c)
	defer listener.Close()
	stor, err := httpstorage.ClientTLS(listener.Addr().String(), []byte(coretesting.CACert), testAuthkey)
	c.Assert(err, gc.IsNil)

	data := []byte("hello")
	err = ioutil.WriteFile(filepath.Join(storageDir, "filename"), data, 0644)
	c.Assert(err, gc.IsNil)
	names, err := storage.List(stor, "filename")
	c.Assert(err, gc.IsNil)
	c.Assert(names, gc.DeepEquals, []string{"filename"})
	checkFileHasContents(c, stor, "filename", data)

	// Put, Remove and RemoveAll should all succeed.
	checkPutFile(c, stor, "filenamethesecond", data)
	checkFileHasContents(c, stor, "filenamethesecond", data)
	c.Assert(stor.Remove("filenamethesecond"), gc.IsNil)
	c.Assert(stor.RemoveAll(), gc.IsNil)
}

func (s *storageSuite) TestClientTLSInvalidAuth(c *gc.C) {
	listener, _, storageDir := startServerTLS(c)
	defer listener.Close()
	const invalidAuthkey = testAuthkey + "!"
	stor, err := httpstorage.ClientTLS(listener.Addr().String(), []byte(coretesting.CACert), invalidAuthkey)
	c.Assert(err, gc.IsNil)

	// Get and List should succeed.
	data := []byte("hello")
	err = ioutil.WriteFile(filepath.Join(storageDir, "filename"), data, 0644)
	c.Assert(err, gc.IsNil)
	names, err := storage.List(stor, "filename")
	c.Assert(err, gc.IsNil)
	c.Assert(names, gc.DeepEquals, []string{"filename"})
	checkFileHasContents(c, stor, "filename", data)

	// Put, Remove and RemoveAll should all fail.
	const authErrorPattern = ".*401 Unauthorized"
	err = putFile(c, stor, "filenamethesecond", data)
	c.Assert(err, gc.ErrorMatches, authErrorPattern)
	c.Assert(stor.Remove("filenamethesecond"), gc.ErrorMatches, authErrorPattern)
	c.Assert(stor.RemoveAll(), gc.ErrorMatches, authErrorPattern)
}

func (s *storageSuite) TestList(c *gc.C) {
	listener, _, _ := startServer(c)
	defer listener.Close()
	stor := httpstorage.Client(listener.Addr().String())
	names, err := storage.List(stor, "a/b/c")
	c.Assert(err, gc.IsNil)
	c.Assert(names, gc.HasLen, 0)
}

// TestPersistence tests the adding, reading, listing and removing
// of files from the local storage.
func (s *storageSuite) TestPersistence(c *gc.C) {
	listener, _, _ := startServer(c)
	defer listener.Close()

	stor := httpstorage.Client(listener.Addr().String())
	names := []string{
		"aa",
		"zzz/aa",
		"zzz/bb",
	}
	for _, name := range names {
		checkFileDoesNotExist(c, stor, name)
		checkPutFile(c, stor, name, []byte(name))
	}
	checkList(c, stor, "", names)
	checkList(c, stor, "a", []string{"aa"})
	checkList(c, stor, "zzz/", []string{"zzz/aa", "zzz/bb"})

	storage2 := httpstorage.Client(listener.Addr().String())
	for _, name := range names {
		checkFileHasContents(c, storage2, name, []byte(name))
	}

	// remove the first file and check that the others remain.
	err := storage2.Remove(names[0])
	c.Check(err, gc.IsNil)

	// check that it's ok to remove a file twice.
	err = storage2.Remove(names[0])
	c.Check(err, gc.IsNil)

	// ... and check it's been removed in the other environment
	checkFileDoesNotExist(c, stor, names[0])

	// ... and that the rest of the files are still around
	checkList(c, storage2, "", names[1:])

	for _, name := range names[1:] {
		err := storage2.Remove(name)
		c.Assert(err, gc.IsNil)
	}

	// check they've all gone
	checkList(c, storage2, "", nil)

	// Check that RemoveAll works.
	checkRemoveAll(c, storage2)
}

func checkList(c *gc.C, stor storage.StorageReader, prefix string, names []string) {
	lnames, err := storage.List(stor, prefix)
	c.Assert(err, gc.IsNil)
	c.Assert(lnames, gc.DeepEquals, names)
}

type readerWithClose struct {
	*bytes.Buffer
	closeCalled bool
}

var _ io.Reader = (*readerWithClose)(nil)
var _ io.Closer = (*readerWithClose)(nil)

func (r *readerWithClose) Close() error {
	r.closeCalled = true
	return nil
}

func putFile(c *gc.C, stor storage.StorageWriter, name string, contents []byte) error {
	c.Logf("check putting file %s ...", name)
	reader := &readerWithClose{bytes.NewBuffer(contents), false}
	err := stor.Put(name, reader, int64(len(contents)))
	c.Assert(reader.closeCalled, jc.IsFalse)
	return err
}

func checkPutFile(c *gc.C, stor storage.StorageWriter, name string, contents []byte) {
	err := putFile(c, stor, name, contents)
	c.Assert(err, gc.IsNil)
}

func checkFileDoesNotExist(c *gc.C, stor storage.StorageReader, name string) {
	r, err := storage.Get(stor, name)
	c.Assert(r, gc.IsNil)
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func checkFileHasContents(c *gc.C, stor storage.StorageReader, name string, contents []byte) {
	r, err := storage.Get(stor, name)
	c.Assert(err, gc.IsNil)
	c.Check(r, gc.NotNil)
	defer r.Close()
	data, err := ioutil.ReadAll(r)
	c.Check(err, gc.IsNil)
	c.Check(data, gc.DeepEquals, contents)

	url, err := stor.URL(name)
	c.Assert(err, gc.IsNil)
	resp, err := http.Get(url)
	c.Assert(err, gc.IsNil)
	data, err = ioutil.ReadAll(resp.Body)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK, gc.Commentf("error response: %s", data))
	c.Check(data, gc.DeepEquals, contents)
}

func checkRemoveAll(c *gc.C, stor storage.Storage) {
	contents := []byte("File contents.")
	aFile := "a-file.txt"
	err := stor.Put(aFile, bytes.NewBuffer(contents), int64(len(contents)))
	c.Assert(err, gc.IsNil)
	err = stor.Put("empty-file", bytes.NewBuffer(nil), 0)
	c.Assert(err, gc.IsNil)

	err = stor.RemoveAll()
	c.Assert(err, gc.IsNil)

	files, err := storage.List(stor, "")
	c.Assert(err, gc.IsNil)
	c.Check(files, gc.HasLen, 0)

	_, err = storage.Get(stor, aFile)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, fmt.Sprintf("file %q not found", aFile))
}
