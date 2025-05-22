// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/charmhub/path"
	"github.com/juju/juju/internal/charmhub/transport"
)

type ResourcesSuite struct {
	baseSuite
}

func TestResourcesSuite(t *stdtesting.T) {
	tc.Run(t, &ResourcesSuite{})
}

func (s *ResourcesSuite) TestListResourceRevisions(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	path := path.MakePath(baseURL)
	name := "meshuggah"
	resource := "image"

	restClient := NewMockRESTClient(ctrl)
	s.expectGet(c, restClient, path, name, resource)

	client := newResourcesClient(path, restClient, s.logger)
	response, err := client.ListResourceRevisions(c.Context(), name, resource)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(response, tc.HasLen, 3)
}

func (s *ResourcesSuite) TestListResourceRevisionsFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	path := path.MakePath(baseURL)
	name := "meshuggah"
	resource := "image"

	restClient := NewMockRESTClient(ctrl)
	s.expectGetFailure(restClient)

	client := newResourcesClient(path, restClient, s.logger)
	_, err := client.ListResourceRevisions(c.Context(), name, resource)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *ResourcesSuite) expectGet(c *tc.C, client *MockRESTClient, p path.Path, charm, resource string) {
	namedPath, err := p.Join(charm, resource, "revisions")
	c.Assert(err, tc.ErrorIsNil)

	client.EXPECT().Get(gomock.Any(), namedPath, gomock.Any()).Do(func(_ context.Context, _ path.Path, r any) (restResponse, error) {
		response := r.(*transport.ResourcesResponse)
		response.Revisions = make([]transport.ResourceRevision, 3)
		return restResponse{}, nil
	})
}

func (s *ResourcesSuite) expectGetFailure(client *MockRESTClient) {
	client.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(restResponse{StatusCode: http.StatusInternalServerError}, errors.Errorf("boom"))
}

func (s *ResourcesSuite) TestListResourceRevisionsRequestPayload(c *tc.C) {
	resourcesResponse := transport.ResourcesResponse{Revisions: []transport.ResourceRevision{
		{Name: "image", Revision: 3, Type: "image"},
		{Name: "image", Revision: 2, Type: "image"},
		{Name: "image", Revision: 1, Type: "image"},
	}}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		err := json.NewEncoder(w).Encode(resourcesResponse)
		c.Assert(err, tc.ErrorIsNil)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	basePath, err := basePath(server.URL)
	c.Assert(err, tc.ErrorIsNil)

	resourcesPath, err := basePath.Join("resources")
	c.Assert(err, tc.ErrorIsNil)

	apiRequester := newAPIRequester(DefaultHTTPClient(s.logger), s.logger)
	restClient := newHTTPRESTClient(apiRequester)

	client := newResourcesClient(resourcesPath, restClient, s.logger)
	response, err := client.ListResourceRevisions(c.Context(), "wordpress", "image")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(response, tc.DeepEquals, resourcesResponse.Revisions)
}
