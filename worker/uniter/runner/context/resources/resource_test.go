// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"io/ioutil"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/worker/uniter/runner/context/mocks"
	"github.com/juju/juju/v2/worker/uniter/runner/context/resources"
)

var _ = gc.Suite(&OpenedResourceSuite{})

type OpenedResourceSuite struct {
	testing.IsolationSuite

	stub *testing.Stub
}

func (s *OpenedResourceSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
}

func (s *OpenedResourceSuite) TestOpenResource(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockOpenedResourceClient(ctrl)
	info, reader := newResource(c, s.stub, "spam", "some data")
	client.EXPECT().GetResource("spam").Return(info, reader, nil)

	opened, err := resources.OpenResource("spam", client)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(opened, jc.DeepEquals, &resources.OpenedResource{
		Resource:   info,
		ReadCloser: reader,
	})
}

func (s *OpenedResourceSuite) TestContent(c *gc.C) {
	info, reader := newResource(c, s.stub, "spam", "some data")
	opened := resources.OpenedResource{
		Resource:   info,
		ReadCloser: reader,
	}

	content := opened.Content()
	c.Assert(content, jc.DeepEquals, resources.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	})
}

func (s *OpenedResourceSuite) TestDockerImage(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockOpenedResourceClient(ctrl)
	jsonContent := `{"ImageName":"image-name","Username":"docker-registry","Password":"secret"}`
	info, reader := newDockerResource(c, s.stub, "spam", jsonContent)
	client.EXPECT().GetResource("spam").Return(info, reader, nil)

	opened, err := resources.OpenResource("spam", client)
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
