// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/state"
)

type dockerMetadataStorageSuite struct {
	ConnSuite
	metadataStorage state.DockerMetadataStorage
}

var _ = gc.Suite(&dockerMetadataStorageSuite{})

func (s *dockerMetadataStorageSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.metadataStorage = state.NewDockerMetadataStorage(s.State)
}

func (s *dockerMetadataStorageSuite) Test(c *gc.C) {}

func (s *dockerMetadataStorageSuite) TestSaveNewResource(c *gc.C) {
	id := "test-123"
	registryPath := "url@sha256:abc123"
	resource := resources.DockerImageDetails{
		RegistryPath: registryPath,
	}
	err := s.metadataStorage.Save(id, resource)

	c.Assert(err, jc.ErrorIsNil)
	s.assertSavedDockerResource(c, id, resource)
}

func (s *dockerMetadataStorageSuite) TestSaveUpdatesExistingResource(c *gc.C) {
	id := "test-123"
	resource := resources.DockerImageDetails{
		RegistryPath: "url@sha256:abc123",
	}
	err := s.metadataStorage.Save(id, resource)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSavedDockerResource(c, id, resource)

	resource2 := resources.DockerImageDetails{
		RegistryPath: "url@sha256:deadbeef",
	}
	err = s.metadataStorage.Save(id, resource2)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSavedDockerResource(c, id, resource2)
}

func (s *dockerMetadataStorageSuite) TestSaveIdempotent(c *gc.C) {
	id := "test-123"
	resource := resources.DockerImageDetails{
		RegistryPath: "url@sha256:abc123",
	}
	err := s.metadataStorage.Save(id, resource)
	c.Assert(err, jc.ErrorIsNil)
	err = s.metadataStorage.Save(id, resource)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSavedDockerResource(c, id, resource)
}

func (s *dockerMetadataStorageSuite) assertSavedDockerResource(c *gc.C, resourceID string, registryInfo resources.DockerImageDetails) {
	coll, closer := state.GetCollection(s.State, "dockerResources")
	defer closer()

	var raw bson.M
	err := coll.FindId(resourceID).One(&raw)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(raw["_id"], gc.Equals, fmt.Sprintf("%s:%s", s.State.ModelUUID(), resourceID))
	c.Assert(raw["registry-path"], gc.Equals, registryInfo.RegistryPath)
	c.Assert(raw["password"], gc.Equals, registryInfo.Password)
	c.Assert(raw["username"], gc.Equals, registryInfo.Username)
}

func (s *dockerMetadataStorageSuite) TestGet(c *gc.C) {
	id := "test-123"
	resource := resources.DockerImageDetails{
		RegistryPath: "url@sha256:abc123",
		Username:     "testuser",
		Password:     "hunter2",
	}
	err := s.metadataStorage.Save(id, resource)
	c.Assert(err, jc.ErrorIsNil)

	retrieved, num, err := s.metadataStorage.Get(id)
	c.Assert(err, jc.ErrorIsNil)
	retrievedInfo := readerToDockerDetails(c, retrieved)
	c.Assert(num, gc.Equals, int64(76))
	c.Assert(retrievedInfo.RegistryPath, gc.Equals, "url@sha256:abc123")
	c.Assert(retrievedInfo.Username, gc.Equals, "testuser")
	c.Assert(retrievedInfo.Password, gc.Equals, "hunter2")

}

func (s *dockerMetadataStorageSuite) TestRemove(c *gc.C) {
	id := "test-123"
	resource := resources.DockerImageDetails{
		RegistryPath: "url@sha256:abc123",
	}
	err := s.metadataStorage.Save(id, resource)
	c.Assert(err, jc.ErrorIsNil)

	err = s.metadataStorage.Remove(id)
	c.Assert(err, jc.ErrorIsNil)
	_, _, err = s.metadataStorage.Get(id)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func readerToDockerDetails(c *gc.C, r io.ReadCloser) *resources.DockerImageDetails {
	var info resources.DockerImageDetails
	respBuf := new(bytes.Buffer)
	_, err := respBuf.ReadFrom(r)
	c.Assert(err, jc.ErrorIsNil)
	err = json.Unmarshal(respBuf.Bytes(), &info)
	c.Assert(err, jc.ErrorIsNil)
	return &info
}
