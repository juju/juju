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

type FindSuite struct {
	baseSuite
}

func TestFindSuite(t *stdtesting.T) {
	tc.Run(t, &FindSuite{})
}

func (s *FindSuite) TestFind(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	path := path.MakePath(baseURL)
	name := "meshuggah"

	restClient := NewMockRESTClient(ctrl)
	s.expectGet(c, restClient, path, name)

	client := newFindClient(path, restClient, s.logger)
	responses, err := client.Find(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(responses), tc.Equals, 1)
	c.Assert(responses[0].Name, tc.Equals, name)
}

func (s *FindSuite) TestFindWithOptions(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	name := "meshuggah"
	path := path.MakePath(baseURL)

	expect, err := path.Query("channel", "1.0/stable")
	c.Assert(err, tc.ErrorIsNil)
	expect, err = expect.Query("type", "bundle")
	c.Assert(err, tc.ErrorIsNil)

	restClient := NewMockRESTClient(ctrl)
	s.expectGet(c, restClient, expect, name)

	client := newFindClient(path, restClient, s.logger)
	responses, err := client.Find(c.Context(), name, WithFindChannel("1.0/stable"), WithFindType("bundle"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(responses), tc.Equals, 1)
	c.Assert(responses[0].Name, tc.Equals, name)
}

func (s *FindSuite) TestFindFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	path := path.MakePath(baseURL)
	name := "meshuggah"

	restClient := NewMockRESTClient(ctrl)
	s.expectGetFailure(restClient)

	client := newFindClient(path, restClient, s.logger)
	_, err := client.Find(c.Context(), name)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *FindSuite) expectGet(c *tc.C, client *MockRESTClient, p path.Path, name string) {
	namedPath, err := p.Query("q", name)
	c.Assert(err, tc.ErrorIsNil)
	namedPath, err = namedPath.Query("fields", defaultFindFilter())
	c.Assert(err, tc.ErrorIsNil)

	client.EXPECT().Get(gomock.Any(), namedPath, gomock.Any()).DoAndReturn(func(_ context.Context, _ path.Path, r any) (restResponse, error) {
		responses := r.(*transport.FindResponses)
		responses.Results = []transport.FindResponse{{
			Name: name,
		}}
		return restResponse{StatusCode: http.StatusOK}, nil
	})
}

func (s *FindSuite) expectGetFailure(client *MockRESTClient) {
	client.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(restResponse{StatusCode: http.StatusInternalServerError}, errors.Errorf("boom"))
}

func (s *FindSuite) TestFindRequestPayload(c *tc.C) {
	findResponses := transport.FindResponses{
		Results: []transport.FindResponse{{
			Name: "wordpress",
			Type: "object",
			ID:   "charmCHARMcharmCHARMcharmCHARM01",
			Entity: transport.Entity{
				Categories: []transport.Category{{
					Featured: true,
					Name:     "blog",
				}},
				Description: "This will install and setup WordPress optimized to run in the cloud. By default it will place Ngnix and php-fpm configured to scale horizontally with Nginx's reverse proxy.",
				License:     "Apache-2.0",
				Publisher: map[string]string{
					"display-name": "WordPress Charmers",
				},
				Summary: "WordPress is a full featured web blogging tool, this charm deploys it.",
				UsedBy: []string{
					"wordpress-everlast",
					"wordpress-jorge",
					"wordpress-site",
				},
			},
			DefaultRelease: transport.FindChannelMap{
				Channel: transport.Channel{
					Name: "latest/stable",
					Base: transport.Base{
						Architecture: "all",
						Name:         "ubuntu",
						Channel:      "18.04",
					},
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
					Risk:       "stable",
					Track:      "latest",
				},
				Revision: transport.FindRevision{
					CreatedAt: "2019-12-16T19:20:26.673192+00:00",
					Download: transport.Download{
						HashSHA256: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
						Size:       12042240,
						URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
					},
					Bases: []transport.Base{{
						Architecture: "all",
						Name:         "ubuntu",
						Channel:      "18.04",
					}},
					Revision: 16,
					Version:  "1.0.3",
				},
			},
		}},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		err := json.NewEncoder(w).Encode(findResponses)
		c.Assert(err, tc.ErrorIsNil)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	basePath, err := basePath(server.URL)
	c.Assert(err, tc.ErrorIsNil)

	findPath, err := basePath.Join("find")
	c.Assert(err, tc.ErrorIsNil)

	apiRequester := newAPIRequester(DefaultHTTPClient(s.logger), s.logger)
	restClient := newHTTPRESTClient(apiRequester)

	client := newFindClient(findPath, restClient, s.logger)
	responses, err := client.Find(c.Context(), "wordpress")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(responses, tc.DeepEquals, findResponses.Results)
}

func (s *FindSuite) TestFindErrorPayload(c *tc.C) {
	findResponses := transport.FindResponses{
		ErrorList: []transport.APIError{{
			Code:    "some-error-code",
			Message: "not found error code",
		}},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		err := json.NewEncoder(w).Encode(findResponses)
		c.Assert(err, tc.ErrorIsNil)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	basePath, err := basePath(server.URL)
	c.Assert(err, tc.ErrorIsNil)

	findPath, err := basePath.Join("find")
	c.Assert(err, tc.ErrorIsNil)

	apiRequester := newAPIRequester(DefaultHTTPClient(s.logger), s.logger)
	restClient := newHTTPRESTClient(apiRequester)

	client := newFindClient(findPath, restClient, s.logger)
	_, err = client.Find(c.Context(), "wordpress")
	c.Assert(err, tc.ErrorMatches, "not found error code")
}
