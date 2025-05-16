// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"io"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/runner/context/mocks"
	"github.com/juju/juju/internal/worker/uniter/runner/context/resources"
)

func TestOpenedResourceSuite(t *stdtesting.T) { tc.Run(t, &OpenedResourceSuite{}) }

type OpenedResourceSuite struct {
	testhelpers.IsolationSuite

	stub *testhelpers.Stub
}

func (s *OpenedResourceSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.stub = &testhelpers.Stub{}
}

func (s *OpenedResourceSuite) TestOpenResource(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockOpenedResourceClient(ctrl)
	info, reader := newResource(c, s.stub, "spam", "some data")
	client.EXPECT().GetResource(gomock.Any(), "spam").Return(info, reader, nil)

	opened, err := resources.OpenResource(c.Context(), "spam", client)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(opened, tc.DeepEquals, &resources.OpenedResource{
		Resource:   info,
		ReadCloser: reader,
	})
}

func (s *OpenedResourceSuite) TestContent(c *tc.C) {
	info, reader := newResource(c, s.stub, "spam", "some data")
	opened := resources.OpenedResource{
		Resource:   info,
		ReadCloser: reader,
	}

	content := opened.Content()
	c.Assert(content, tc.DeepEquals, resources.Content{
		Data:        reader,
		Size:        info.Size,
		Fingerprint: info.Fingerprint,
	})
}

func (s *OpenedResourceSuite) TestDockerImage(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockOpenedResourceClient(ctrl)
	jsonContent := `{"ImageName":"image-name","Username":"docker-registry","Password":"secret"}`
	info, reader := newDockerResource(c, s.stub, "spam", jsonContent)
	client.EXPECT().GetResource(gomock.Any(), "spam").Return(info, reader, nil)

	opened, err := resources.OpenResource(c.Context(), "spam", client)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(opened.Path, tc.Equals, "content.yaml")
	content := opened.Content()
	data, err := io.ReadAll(content.Data)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, `
registrypath: image-name
username: docker-registry
password: secret
`[1:])
}
