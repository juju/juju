// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"strings"

	gc "launchpad.net/gocheck"
	//"launchpad.net/gojoyent/manta"

	//"launchpad.net/juju-core/environs/storage"
	//"launchpad.net/juju-core/errors"
	jc "launchpad.net/juju-core/testing/checkers"

	"launchpad.net/gojoyent/errors"
)

type storageSuite struct {
	providerSuite
}

const (
	storageName     = "testStorage"
	fileName        = "testFile"
	fileBlobContent = "Juju Joyent Provider Storage - Test"
)

var _ = gc.Suite(&storageSuite{})

// s.makeStorage creates a Manta storage object for the running test.
func (s *storageSuite) assertStorage(name string, c *gc.C) *joyentStorage {
	env := s.makeEnviron()
	env.name = name
	storage := NewStorage(env).(*joyentStorage)
	c.Assert(storage, gc.NotNil)
	return storage
}

func (s *storageSuite) assertContainer(storage *joyentStorage, c *gc.C) {
	err := storage.createContainer(storage.containerName)
	c.Assert(err, gc.IsNil)
}

func (s *storageSuite) assertFile(storage *joyentStorage, c *gc.C) {
	err := storage.Put(fileName, strings.NewReader(fileBlobContent), int64(len(fileBlobContent)))
	c.Assert(err, gc.IsNil)
}

// makeRandomBytes returns an array of arbitrary byte values.
func makeRandomBytes(length int) []byte {
	data := make([]byte, length)
	for index := range data {
		data[index] = byte(rand.Intn(256))
	}
	return data
}

func makeResponse(content string, status int) *http.Response {
	return &http.Response{
		Status:     fmt.Sprintf("%d", status),
		StatusCode: status,
		Body:       ioutil.NopCloser(strings.NewReader(content)),
	}
}

func (s *storageSuite) TestList(c *gc.C) {
	mantaStorage := s.assertStorage(storageName, c)
	s.assertContainer(mantaStorage, c)
	s.assertFile(mantaStorage, c)

	names, err := mantaStorage.List("prefix")
	c.Assert(err, gc.IsNil)
	c.Check(names, gc.DeepEquals, []string{fileName})
}

func (s *storageSuite) TestGet(c *gc.C) {
	mantaStorage := s.assertStorage(storageName, c)
	s.assertFile(mantaStorage, c)

	reader, err := mantaStorage.Get(fileName)
	c.Assert(err, gc.IsNil)
	c.Assert(reader, gc.NotNil)
	defer reader.Close()

	data, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(string(data), gc.Equals, fileBlobContent)
}

func (s *storageSuite) TestGetFileNotExists(c *gc.C) {
	mantaStorage := s.assertStorage(storageName, c)

	_, err := mantaStorage.Get("noFile")
	c.Assert(err, gc.NotNil)
	c.Assert(err, jc.Satisfies, errors.IsResourceNotFound)
}

func (s *storageSuite) TestPut(c *gc.C) {
	mantaStorage := s.assertStorage(storageName, c)

	s.assertFile(mantaStorage, c)
}

func (s *storageSuite) TestRemove(c *gc.C) {
	mantaStorage := s.assertStorage(storageName, c)
	s.assertFile(mantaStorage, c)

	err := mantaStorage.Remove(fileName)
	c.Assert(err, gc.IsNil)
}

func (s *storageSuite) TestRemoveFileNotExists(c *gc.C) {
	mantaStorage := s.assertStorage(storageName, c)

	err := mantaStorage.Remove("nofile")
	c.Assert(err, gc.NotNil)
	c.Assert(err, jc.Satisfies, errors.IsResourceNotFound)
}

func (s *storageSuite) TestRemoveAll(c *gc.C) {
	mantaStorage := s.assertStorage(storageName, c)

	err := mantaStorage.RemoveAll()
	c.Assert(err, gc.IsNil)
}

func (s *storageSuite) TestURL(c *gc.C) {
	mantaStorage := s.assertStorage(storageName, c)

	// Use a realistic service endpoint for this test, so that we can see
	// that we're really getting the expected kind of URL.
	//setStorageEndpoint(mantaStorage, gwacl.GetEndpoint("West US"))
	URL, err := mantaStorage.URL(fileName)
	c.Assert(err, gc.IsNil)
	parsedURL, err := url.Parse(URL)
	c.Assert(err, gc.IsNil)
	//c.Check(parsedURL.Host, gc.Matches, fmt.Sprintf("%s.blob.core.windows.net", account))
	//c.Check(parsedURL.Path, gc.Matches, fmt.Sprintf("/%s/%s", container, fileName))
	values, err := url.ParseQuery(parsedURL.RawQuery)
	c.Assert(err, gc.IsNil)
	signature := values.Get("sig")
	// The query string contains a non-empty signature.
	c.Check(signature, gc.Not(gc.HasLen), 0)
	// The signature is base64-encoded.
	_, err = base64.StdEncoding.DecodeString(signature)
	c.Assert(err, gc.IsNil)
	// If Key is empty, query string does not contain a signature.
	mantaStorage = s.assertStorage(storageName, c)

	URL, err = mantaStorage.URL(fileName)
	c.Assert(err, gc.IsNil)
	parsedURL, err = url.Parse(URL)
	c.Assert(err, gc.IsNil)
	values, err = url.ParseQuery(parsedURL.RawQuery)
	c.Assert(err, gc.IsNil)
	c.Check(values.Get("sig"), gc.HasLen, 0)
}

func (s *storageSuite) TestCreateContainer(c *gc.C) {
	mantaStorage := s.assertStorage(storageName, c)

	s.assertContainer(mantaStorage, c)
}

func (s *storageSuite) TestCreateContainerAlreadyExists(c *gc.C) {
	mantaStorage := s.assertStorage(storageName, c)

	s.assertContainer(mantaStorage, c)
	s.assertContainer(mantaStorage, c)
}

func (s *storageSuite) TestDeleteContainer(c *gc.C) {
	mantaStorage := s.assertStorage(storageName, c)
	s.assertContainer(mantaStorage, c)

	err := mantaStorage.deleteContainer(mantaStorage.containerName)
	c.Assert(err, gc.IsNil)
}

func (s *storageSuite) TestDeleteContainerNotEmpty(c *gc.C) {
	mantaStorage := s.assertStorage(storageName, c)
	s.assertContainer(mantaStorage, c)
	s.assertFile(mantaStorage, c)

	err := mantaStorage.deleteContainer(mantaStorage.containerName)
	c.Assert(err, gc.NotNil)
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (s *storageSuite) TestDeleteContainerNotExists(c *gc.C) {
	mantaStorage := s.assertStorage(storageName, c)

	err := mantaStorage.deleteContainer("noContainer")
	c.Assert(err, gc.NotNil)
	c.Assert(err, jc.Satisfies, errors.IsResourceNotFound)
}
