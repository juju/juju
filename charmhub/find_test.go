// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package charmhub

import (
	"context"
	"encoding/json"
	http "net/http"
	"net/http/httptest"

	gomock "github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	path "github.com/juju/juju/charmhub/path"
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

	client := NewFindClient(path, restClient)
	responses, err := client.Find(context.TODO(), name)
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
	s.expectGetFailure(c, restClient)

	client := NewFindClient(path, restClient)
	_, err := client.Find(context.TODO(), name)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *FindSuite) expectGet(c *gc.C, client *MockRESTClient, p path.Path, name string) {
	namedPath, err := p.Query("q", name)
	c.Assert(err, jc.ErrorIsNil)

	client.EXPECT().Get(gomock.Any(), namedPath, gomock.Any()).Do(func(_ context.Context, _ path.Path, responses *transport.FindResponses) {
		responses.Results = []transport.FindResponse{{
			Name: name,
		}}
	}).Return(nil)
}

func (s *FindSuite) expectGetFailure(c *gc.C, client *MockRESTClient) {
	client.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.Errorf("boom"))
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
				Media: []transport.Media{{
					Height: 256,
					Type:   "icon",
					URL:    "https://dashboard.snapcraft.io/site_media/appmedia/2017/04/wpcom.png",
					Width:  256,
				}},
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
			DefaultRelease: transport.ChannelMap{
				Channel: transport.Channel{
					Name: "latest/stable",
					Platform: transport.Platform{
						Architecture: "all",
						OS:           "ubuntu",
						Series:       "bionic",
					},
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
					Risk:       "stable",
					Track:      "latest",
				},
				Revision: transport.Revision{
					ConfigYAML: "one: 1\ntwo: 2\nitems: [1,2,3,4]\n\"",
					CreatedAt:  "2019-12-16T19:20:26.673192+00:00",
					Download: transport.Download{
						HashSHA265: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
						Size:       12042240,
						URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
					},
					MetadataYAML: "name: myname\nversion: 1.0.3\nsummary: A charm or bundle.\ndescription: |\n  This will install and setup services optimized to run in the cloud.\n  By default it will place Ngnix configured to scale horizontally\n  with Nginx's reverse proxy.\n",
					Platforms: []transport.Platform{{
						Architecture: "all",
						OS:           "ubuntu",
						Series:       "bionic",
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

	config := Config{
		URL: server.URL,
	}
	basePath, err := config.BasePath()
	c.Assert(err, jc.ErrorIsNil)

	findPath, err := basePath.Join("find")
	c.Assert(err, jc.ErrorIsNil)

	apiRequester := NewAPIRequester(DefaultHTTPTransport())
	restClient := NewHTTPRESTClient(apiRequester, nil)

	client := NewFindClient(findPath, restClient)
	responses, err := client.Find(context.TODO(), "wordpress")
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

	config := Config{
		URL: server.URL,
	}
	basePath, err := config.BasePath()
	c.Assert(err, jc.ErrorIsNil)

	findPath, err := basePath.Join("find")
	c.Assert(err, jc.ErrorIsNil)

	apiRequester := NewAPIRequester(DefaultHTTPTransport())
	restClient := NewHTTPRESTClient(apiRequester, nil)

	client := NewFindClient(findPath, restClient)
	_, err = client.Find(context.TODO(), "wordpress")
	c.Assert(err, gc.ErrorMatches, "not found error code")
}
