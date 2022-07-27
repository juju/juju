// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmhub/path"
	"github.com/juju/juju/charmhub/transport"
)

type FindSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FindSuite{})

func (s *FindSuite) TestFind(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	path := path.MakePath(baseURL)
	name := "meshuggah"

	restClient := NewMockRESTClient(ctrl)
	s.expectGet(c, restClient, path, name)

	client := newFindClient(path, restClient, &FakeLogger{})
	responses, err := client.Find(context.Background(), name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(responses), gc.Equals, 1)
	c.Assert(responses[0].Name, gc.Equals, name)
}

func (s *FindSuite) TestFindWithOptions(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	name := "meshuggah"
	path := path.MakePath(baseURL)

	expect, err := path.Query("channel", "1.0/stable")
	c.Assert(err, jc.ErrorIsNil)
	expect, err = expect.Query("type", "bundle")
	c.Assert(err, jc.ErrorIsNil)

	restClient := NewMockRESTClient(ctrl)
	s.expectGet(c, restClient, expect, name)

	client := newFindClient(path, restClient, &FakeLogger{})
	responses, err := client.Find(context.Background(), name, WithFindChannel("1.0/stable"), WithFindType("bundle"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(responses), gc.Equals, 1)
	c.Assert(responses[0].Name, gc.Equals, name)
}

func (s *FindSuite) TestFindFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	path := path.MakePath(baseURL)
	name := "meshuggah"

	restClient := NewMockRESTClient(ctrl)
	s.expectGetFailure(restClient)

	client := newFindClient(path, restClient, &FakeLogger{})
	_, err := client.Find(context.Background(), name)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *FindSuite) expectGet(c *gc.C, client *MockRESTClient, p path.Path, name string) {
	namedPath, err := p.Query("q", name)
	c.Assert(err, jc.ErrorIsNil)
	namedPath, err = namedPath.Query("fields", defaultFindFilter())
	c.Assert(err, jc.ErrorIsNil)

	client.EXPECT().Get(gomock.Any(), namedPath, gomock.Any()).Do(func(_ context.Context, _ path.Path, responses *transport.FindResponses) {
		responses.Results = []transport.FindResponse{{
			Name: name,
		}}
	}).Return(restResponse{StatusCode: http.StatusOK}, nil)
}

func (s *FindSuite) expectGetFailure(client *MockRESTClient) {
	client.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(restResponse{StatusCode: http.StatusInternalServerError}, errors.Errorf("boom"))
}

func (s *FindSuite) TestFindRequestPayload(c *gc.C) {
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
					"display-name": "Wordress Charmers",
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
		c.Assert(err, jc.ErrorIsNil)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	basePath, err := basePath(server.URL)
	c.Assert(err, jc.ErrorIsNil)

	findPath, err := basePath.Join("find")
	c.Assert(err, jc.ErrorIsNil)

	apiRequester := newAPIRequester(DefaultHTTPClient(&FakeLogger{}), &FakeLogger{})
	restClient := newHTTPRESTClient(apiRequester)

	client := newFindClient(findPath, restClient, &FakeLogger{})
	responses, err := client.Find(context.Background(), "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(responses, gc.DeepEquals, findResponses.Results)
}

func (s *FindSuite) TestFindErrorPayload(c *gc.C) {
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
		c.Assert(err, jc.ErrorIsNil)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	basePath, err := basePath(server.URL)
	c.Assert(err, jc.ErrorIsNil)

	findPath, err := basePath.Join("find")
	c.Assert(err, jc.ErrorIsNil)

	apiRequester := newAPIRequester(DefaultHTTPClient(&FakeLogger{}), &FakeLogger{})
	restClient := newHTTPRESTClient(apiRequester)

	client := newFindClient(findPath, restClient, &FakeLogger{})
	_, err = client.Find(context.Background(), "wordpress")
	c.Assert(err, gc.ErrorMatches, "not found error code")
}
