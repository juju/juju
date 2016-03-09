// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bytes"
	"io"
	"os"

	"github.com/juju/errors"
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
	deps := uploadDeps{s.stub, rsc{&bytes.Buffer{}}}
	du := deployUploader{
		serviceID: "mysql",
		client:    deps,
		resources: map[string]charmresource.Meta{
			"upload": {
				Name: "upload",
				Type: charmresource.TypeFile,
				Path: "upload",
			},
			"store": {
				Name: "store",
				Type: charmresource.TypeFile,
				Path: "store",
			},
		},
		osOpen: deps.Open,
		osStat: deps.Stat,
	}

	files := map[string]string{
		"upload": "foobar.txt",
	}
	ids, err := du.upload(files)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ids, gc.DeepEquals, map[string]string{
		"upload": "id-upload",
		"store":  "id-store",
	})

	expectedStore := []charmresource.Resource{
		{
			Meta:   du.resources["store"],
			Origin: charmresource.OriginStore,
		},
	}
	s.stub.CheckCall(c, 1, "AddPendingResources", "mysql", expectedStore)
	s.stub.CheckCall(c, 2, "Open", "foobar.txt")

	expectedUpload := charmresource.Resource{
		Meta:   du.resources["upload"],
		Origin: charmresource.OriginUpload,
	}
	s.stub.CheckCall(c, 3, "AddPendingResource", "mysql", expectedUpload, deps.ReadSeekCloser)
}

func (s DeploySuite) TestUploadUnexpectedResource(c *gc.C) {
	deps := uploadDeps{s.stub, rsc{&bytes.Buffer{}}}
	du := deployUploader{
		serviceID: "mysql",
		client:    deps,
		resources: map[string]charmresource.Meta{
			"res1": {
				Name: "res1",
				Type: charmresource.TypeFile,
				Path: "path",
			},
		},
		osOpen: deps.Open,
		osStat: deps.Stat,
	}

	files := map[string]string{"some bad resource": "foobar.txt"}
	_, err := du.upload(files)
	c.Check(err, gc.ErrorMatches, `unrecognized resource "some bad resource"`)

	s.stub.CheckNoCalls(c)
}

func (s DeploySuite) TestMissingResource(c *gc.C) {
	deps := uploadDeps{s.stub, rsc{&bytes.Buffer{}}}
	du := deployUploader{
		serviceID: "mysql",
		client:    deps,
		resources: map[string]charmresource.Meta{
			"res1": {
				Name: "res1",
				Type: charmresource.TypeFile,
				Path: "path",
			},
		},
		osOpen: deps.Open,
		osStat: deps.Stat,
	}

	// set the error that will be returned by os.Stat
	s.stub.SetErrors(os.ErrNotExist)

	files := map[string]string{"res1": "foobar.txt"}
	_, err := du.upload(files)
	c.Check(err, gc.ErrorMatches, `file for resource "res1".*`)
	c.Check(errors.Cause(err), jc.Satisfies, os.IsNotExist)
}

type uploadDeps struct {
	stub           *testing.Stub
	ReadSeekCloser ReadSeekCloser
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

func (s uploadDeps) AddPendingResource(serviceID string, resource charmresource.Resource, r io.ReadSeeker) (id string, err error) {
	s.stub.AddCall("AddPendingResource", serviceID, resource, r)
	if err := s.stub.NextErr(); err != nil {
		return "", err
	}
	return "id-" + resource.Name, nil
}

func (s uploadDeps) Open(name string) (ReadSeekCloser, error) {
	s.stub.AddCall("Open", name)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.ReadSeekCloser, nil
}

func (s uploadDeps) Stat(name string) error {
	s.stub.AddCall("Stat", name)
	return s.stub.NextErr()
}

type rsc struct {
	*bytes.Buffer
}

func (rsc) Close() error {
	return nil
}
func (rsc) Seek(offset int64, whence int) (int64, error) {
	return 0, nil
}
