// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/charmhub/path"
	"github.com/juju/juju/internal/charmhub/transport"
)

type InfoSuite struct {
	baseSuite
}

var _ = tc.Suite(&InfoSuite{})

func (s *InfoSuite) TestInfoCharm(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	path := path.MakePath(baseURL)
	name := "meshuggah"

	restClient := NewMockRESTClient(ctrl)
	s.expectCharmGet(c, restClient, path, name)

	client := newInfoClient(path, restClient, s.logger)
	response, err := client.Info(context.Background(), name)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(response.Name, tc.Equals, name)
	c.Assert(response.DefaultRelease.Revision.MetadataYAML, tc.Equals, "YAML")
}

func (s *InfoSuite) TestInfoBundle(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	path := path.MakePath(baseURL)
	name := "meshuggah"

	restClient := NewMockRESTClient(ctrl)
	s.expectBundleGet(c, restClient, path, name)

	client := newInfoClient(path, restClient, s.logger)
	response, err := client.Info(context.Background(), name)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(response.Name, tc.Equals, name)
	c.Assert(response.DefaultRelease.Revision.BundleYAML, tc.Equals, "YAML")
}

func (s *InfoSuite) TestInfoFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	path := path.MakePath(baseURL)
	name := "meshuggah"

	restClient := NewMockRESTClient(ctrl)
	s.expectGetFailure(restClient)

	client := newInfoClient(path, restClient, s.logger)
	_, err := client.Info(context.Background(), name)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *InfoSuite) TestInfoError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	path := path.MakePath(baseURL)
	name := "meshuggah"

	restClient := NewMockRESTClient(ctrl)
	s.expectGetError(c, restClient, path, name)

	client := newInfoClient(path, restClient, s.logger)
	_, err := client.Info(context.Background(), name)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *InfoSuite) expectCharmGet(c *tc.C, client *MockRESTClient, p path.Path, name string) {
	namedPath, err := p.Join(name)
	c.Assert(err, tc.ErrorIsNil)
	namedPath, err = namedPath.Query("fields", defaultInfoFilter())
	c.Assert(err, tc.ErrorIsNil)

	client.EXPECT().Get(gomock.Any(), namedPath, gomock.Any()).DoAndReturn(func(_ context.Context, _ path.Path, r any) (restResponse, error) {
		response := r.(*transport.InfoResponse)
		response.Type = "charm"
		response.Name = name
		response.DefaultRelease = transport.InfoChannelMap{
			Revision: transport.InfoRevision{
				MetadataYAML: "YAML",
			},
		}
		return restResponse{}, nil
	})
}

func (s *InfoSuite) expectBundleGet(c *tc.C, client *MockRESTClient, p path.Path, name string) {
	namedPath, err := p.Join(name)
	c.Assert(err, tc.ErrorIsNil)
	namedPath, err = namedPath.Query("fields", defaultInfoFilter())
	c.Assert(err, tc.ErrorIsNil)

	client.EXPECT().Get(gomock.Any(), namedPath, gomock.Any()).Do(func(_ context.Context, _ path.Path, r any) (restResponse, error) {
		response := r.(*transport.InfoResponse)
		response.Type = "bundle"
		response.Name = name
		response.DefaultRelease = transport.InfoChannelMap{
			Revision: transport.InfoRevision{
				BundleYAML: "YAML",
			},
		}
		return restResponse{}, nil
	})
}

func (s *InfoSuite) expectGetFailure(client *MockRESTClient) {
	client.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(restResponse{StatusCode: http.StatusInternalServerError}, errors.Errorf("boom"))
}

func (s *InfoSuite) expectGetError(c *tc.C, client *MockRESTClient, p path.Path, name string) {
	namedPath, err := p.Join(name)
	c.Assert(err, tc.ErrorIsNil)
	namedPath, err = namedPath.Query("fields", defaultInfoFilter())
	c.Assert(err, tc.ErrorIsNil)

	client.EXPECT().Get(gomock.Any(), namedPath, gomock.Any()).Do(func(_ context.Context, _ path.Path, r any) (restResponse, error) {
		response := r.(*transport.InfoResponse)
		response.ErrorList = []transport.APIError{{
			Message: "not found",
		}}
		return restResponse{StatusCode: http.StatusNotFound}, nil
	})
}

func (s *InfoSuite) TestInfoRequestPayload(c *tc.C) {
	infoResponse := transport.InfoResponse{
		Name: "wordpress",
		Type: "charm",
		ID:   "charmCHARMcharmCHARMcharmCHARM01",
		ChannelMap: []transport.InfoChannelMap{{
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
			Revision: transport.InfoRevision{
				ConfigYAML: "one: 1\ntwo: 2\nitems: [1,2,3,4]\n\"",
				CreatedAt:  "2019-12-16T19:20:26.673192+00:00",
				Download: transport.Download{
					HashSHA256: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
					Size:       12042240,
					URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
				},
				MetadataYAML: "name: myname\nversion: 1.0.3\nsummary: A charm or bundle.\ndescription: |\n  This will install and setup services optimized to run in the cloud.\n  By default it will place Ngnix configured to scale horizontally\n  with Nginx's reverse proxy.\n",
				Bases: []transport.Base{{
					Architecture: "all",
					Name:         "ubuntu",
					Channel:      "18.04",
				}},
				Revision: 16,
				Version:  "1.0.3",
			},
		}},
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
		DefaultRelease: transport.InfoChannelMap{
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
			Revision: transport.InfoRevision{
				ConfigYAML: "one: 1\ntwo: 2\nitems: [1,2,3,4]\n\"",
				CreatedAt:  "2019-12-16T19:20:26.673192+00:00",
				Download: transport.Download{
					HashSHA256: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
					Size:       12042240,
					URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
				},
				MetadataYAML: "name: myname\nversion: 1.0.3\nsummary: A charm or bundle.\ndescription: |\n  This will install and setup services optimized to run in the cloud.\n  By default it will place Ngnix configured to scale horizontally\n  with Nginx's reverse proxy.\n",
				Bases: []transport.Base{{
					Architecture: "all",
					Name:         "ubuntu",
					Channel:      "18.04",
				}},
				Revision: 16,
				Version:  "1.0.3",
			},
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		err := json.NewEncoder(w).Encode(infoResponse)
		c.Assert(err, tc.ErrorIsNil)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	basePath, err := basePath(server.URL)
	c.Assert(err, tc.ErrorIsNil)

	infoPath, err := basePath.Join("info")
	c.Assert(err, tc.ErrorIsNil)

	apiRequester := newAPIRequester(DefaultHTTPClient(s.logger), s.logger)
	restClient := newHTTPRESTClient(apiRequester)

	client := newInfoClient(infoPath, restClient, s.logger)
	response, err := client.Info(context.Background(), "wordpress")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(response, tc.DeepEquals, infoResponse)
}
