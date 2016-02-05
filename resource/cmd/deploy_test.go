// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bytes"
	"io"
	"io/ioutil"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

type DeploySuite struct {
	testing.IsolationSuite

	stub *testing.Stub
}

var _ = gc.Suite(&DeploySuite{})

func (s *DeploySuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
}

func (s DeploySuite) TestUploadOK(c *gc.C) {
	deps := uploadDeps{s.stub, ioutil.NopCloser(&bytes.Buffer{})}
	du := deployUploader{
		serviceID: "mysql",
		client:    deps,
		resources: map[string]charmresource.Meta{
			"upload": {
				Name: "upload",
			},
			"store": {
				Name: "store",
			},
		},
		osOpen: deps.Open,
	}

	files := map[string]string{
		"upload": "foobar.txt",
	}
	ids, err := du.upload(files)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ids, gc.DeepEquals, map[string]string{
		"upload": "id-upload",
		"store":  "id-store",
	})

	expectedStore := []charmresource.Resource{
		{
			Meta:   du.resources["store"],
			Origin: charmresource.OriginStore,
		},
	}
	s.stub.CheckCall(c, 0, "AddPendingResources", "mysql", expectedStore)
	s.stub.CheckCall(c, 1, "Open", "foobar.txt")

	expectedUpload := charmresource.Resource{
		Meta:   du.resources["upload"],
		Origin: charmresource.OriginUpload,
	}
	s.stub.CheckCall(c, 2, "AddPendingResource", "mysql", expectedUpload, deps.readCloser)
}

func (s DeploySuite) TestUploadUnexpectedResource(c *gc.C) {
	deps := uploadDeps{s.stub, ioutil.NopCloser(&bytes.Buffer{})}
	du := deployUploader{
		serviceID: "mysql",
		client:    deps,
		resources: map[string]charmresource.Meta{
			"res1": {
				Name: "res1",
			},
		},
		osOpen: deps.Open,
	}

	files := map[string]string{"some bad resource": "foobar.txt"}
	_, err := du.upload(files)
	c.Assert(err, gc.ErrorMatches, `unrecognized resource "some bad resource"`)

	s.stub.CheckNoCalls(c)
}

type uploadDeps struct {
	stub       *testing.Stub
	readCloser io.ReadCloser
}

func (s uploadDeps) AddPendingResources(serviceID string, resources []charmresource.Resource) (ids []string, err error) {
	s.stub.AddCall("AddPendingResources", serviceID, resources)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	ids = make([]string, len(resources))
	for i, res := range resources {
		ids[i] = "id-" + res.Name
	}
	return ids, nil
}

func (s uploadDeps) AddPendingResource(serviceID string, resource charmresource.Resource, r io.Reader) (id string, err error) {
	s.stub.AddCall("AddPendingResource", serviceID, resource, r)
	if err := s.stub.NextErr(); err != nil {
		return "", err
	}
	return "id-" + resource.Name, nil
}

func (s uploadDeps) Open(name string) (io.ReadCloser, error) {
	s.stub.AddCall("Open", name)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.readCloser, nil
}
