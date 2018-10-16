// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	"io/ioutil"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource/context/internal"
)

var _ = gc.Suite(&OpenedResourceSuite{})

type OpenedResourceSuite struct {
	testing.IsolationSuite

	stub *internalStub
}

func (s *OpenedResourceSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = newInternalStub()
}

func (s *OpenedResourceSuite) TestOpenResource(c *gc.C) {
	info, reader := newResource(c, s.stub.Stub, "spam", "some data")
	s.stub.ReturnGetResourceInfo = info
	s.stub.ReturnGetResourceData = reader

	opened, err := internal.OpenResource("spam", s.stub)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "GetResource")
	c.Check(opened, jc.DeepEquals, &internal.OpenedResource{
		Resource:   info,
		ReadCloser: reader,
	})
}

func (s *OpenedResourceSuite) TestContent(c *gc.C) {
	info, reader := newResource(c, s.stub.Stub, "spam", "some data")
	opened := internal.OpenedResource{
		Resource:   info,
		ReadCloser: reader,
	}

	content := opened.Content()

	s.stub.CheckNoCalls(c)
	c.Check(content, jc.DeepEquals, internal.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	})
}

func (s *OpenedResourceSuite) TestInfo(c *gc.C) {
	expected, reader := newResource(c, s.stub.Stub, "spam", "some data")
	opened := internal.OpenedResource{
		Resource:   expected,
		ReadCloser: reader,
	}

	info := opened.Info()

	s.stub.CheckNoCalls(c)
	c.Check(info, jc.DeepEquals, expected)
}

func (s *OpenedResourceSuite) TestDockerImage(c *gc.C) {
	jsonContent := `{"ImageName":"image-name","Username":"docker-registry","Password":"secret"}`
	info, reader := newDockerResource(c, s.stub.Stub, "spam", jsonContent)
	s.stub.ReturnGetResourceInfo = info
	s.stub.ReturnGetResourceData = reader

	opened, err := internal.OpenResource("spam", s.stub)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(opened.Path, gc.Equals, "content.yaml")
	content := opened.Content()
	data, err := ioutil.ReadAll(content.Data)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, `
registrypath: image-name
username: docker-registry
password: secret
`[1:])
}
