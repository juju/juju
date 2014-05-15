// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"strings"

	"github.com/joyent/gocommon/errors"
	jujuerrors "github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/provider/joyent"
	jp "launchpad.net/juju-core/provider/joyent"
)

type storageSuite struct {
	providerSuite
	localMantaServer
}

const (
	storageName     = "testStorage"
	fileName        = "testFile"
	fileBlobContent = "Juju Joyent Provider Storage - Test"
)

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) SetUpSuite(c *gc.C) {
	s.providerSuite.SetUpSuite(c)
	s.localMantaServer.setupServer(c)
}

func (s *storageSuite) TearDownSuite(c *gc.C) {
	s.localMantaServer.destroyServer()
	s.providerSuite.TearDownSuite(c)
}

// makeStorage creates a Manta storage object for the running test.
func (s *storageSuite) makeStorage(name string, c *gc.C) *jp.JoyentStorage {
	stor := joyent.MakeStorage(c, GetFakeConfig("localhost", s.localMantaServer.Server.URL))
	return stor.(*jp.JoyentStorage)
}

func (s *storageSuite) assertContainer(storage *jp.JoyentStorage, c *gc.C) {
	err := jp.CreateContainer(storage)
	c.Assert(err, gc.IsNil)
}

func (s *storageSuite) assertFile(storage *jp.JoyentStorage, c *gc.C) {
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
	mantaStorage := s.makeStorage(storageName, c)
	s.assertContainer(mantaStorage, c)
	s.assertFile(mantaStorage, c)

	names, err := mantaStorage.List("")
	c.Assert(err, gc.IsNil)
	c.Check(names, gc.DeepEquals, []string{fileName})
}

func (s *storageSuite) TestListWithPrefix(c *gc.C) {
	mantaStorage := s.makeStorage(storageName, c)
	s.assertContainer(mantaStorage, c)
	s.assertFile(mantaStorage, c)
	err := mantaStorage.Put("pr/fileName", strings.NewReader(fileBlobContent), int64(len(fileBlobContent)))
	c.Assert(err, gc.IsNil)

	names, err := mantaStorage.List("p")
	c.Assert(err, gc.IsNil)
	c.Check(names, gc.DeepEquals, []string{"pr/fileName"})
}

func (s *storageSuite) TestGet(c *gc.C) {
	mantaStorage := s.makeStorage(storageName, c)
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
	mantaStorage := s.makeStorage(storageName, c)

	_, err := mantaStorage.Get("noFile")
	c.Assert(err, gc.NotNil)
	c.Assert(err, jc.Satisfies, jujuerrors.IsNotFound)
}

func (s *storageSuite) TestPut(c *gc.C) {
	mantaStorage := s.makeStorage(storageName, c)

	s.assertFile(mantaStorage, c)
}

func (s *storageSuite) TestRemove(c *gc.C) {
	mantaStorage := s.makeStorage(storageName, c)
	s.assertFile(mantaStorage, c)

	err := mantaStorage.Remove(fileName)
	c.Assert(err, gc.IsNil)
}

func (s *storageSuite) TestRemoveFileNotExists(c *gc.C) {
	mantaStorage := s.makeStorage(storageName, c)

	err := mantaStorage.Remove("nofile")
	c.Assert(err, gc.IsNil)
}

func (s *storageSuite) TestRemoveAll(c *gc.C) {
	mantaStorage := s.makeStorage(storageName, c)

	err := mantaStorage.RemoveAll()
	c.Assert(err, gc.IsNil)
}

func (s *storageSuite) TestURL(c *gc.C) {
	mantaStorage := s.makeStorage(storageName, c)

	URL, err := mantaStorage.URL(fileName)
	c.Assert(err, gc.IsNil)
	parsedURL, err := url.Parse(URL)
	c.Assert(err, gc.IsNil)
	c.Check(parsedURL.Host, gc.Matches, mantaStorage.GetMantaUrl()[strings.LastIndex(mantaStorage.GetMantaUrl(), "/")+1:])
	c.Check(parsedURL.Path, gc.Matches, fmt.Sprintf("/%s/stor/%s/%s", mantaStorage.GetMantaUser(), mantaStorage.GetContainerName(), fileName))
}

func (s *storageSuite) TestCreateContainer(c *gc.C) {
	mantaStorage := s.makeStorage(storageName, c)

	s.assertContainer(mantaStorage, c)
}

func (s *storageSuite) TestCreateContainerAlreadyExists(c *gc.C) {
	mantaStorage := s.makeStorage(storageName, c)

	s.assertContainer(mantaStorage, c)
	s.assertContainer(mantaStorage, c)
}

func (s *storageSuite) TestDeleteContainer(c *gc.C) {
	mantaStorage := s.makeStorage(storageName, c)
	s.assertContainer(mantaStorage, c)

	err := mantaStorage.DeleteContainer(mantaStorage.GetContainerName())
	c.Assert(err, gc.IsNil)
}

func (s *storageSuite) TestDeleteContainerNotEmpty(c *gc.C) {
	mantaStorage := s.makeStorage(storageName, c)
	s.assertContainer(mantaStorage, c)
	s.assertFile(mantaStorage, c)

	err := mantaStorage.DeleteContainer(mantaStorage.GetContainerName())
	c.Assert(err, gc.NotNil)
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (s *storageSuite) TestDeleteContainerNotExists(c *gc.C) {
	mantaStorage := s.makeStorage(storageName, c)

	err := mantaStorage.DeleteContainer("noContainer")
	c.Assert(err, gc.NotNil)
	c.Assert(err, jc.Satisfies, jujuerrors.IsNotFound)
}
