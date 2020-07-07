// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"encoding/json"
	http "net/http"
	"net/http/httptest"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	path "github.com/juju/juju/charmhub/path"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type InfoSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&InfoSuite{})

func (s *InfoSuite) TestInfo(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	path := path.MakePath(baseURL)
	name := "meshuggah"

	restClient := NewMockRESTClient(ctrl)
	s.expectGet(c, restClient, path, name)

	client := NewInfoClient(path, restClient)
	response, err := client.Info(context.TODO(), name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.Name, gc.Equals, name)
}

func (s *InfoSuite) TestInfoFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	path := path.MakePath(baseURL)
	name := "meshuggah"

	restClient := NewMockRESTClient(ctrl)
	s.expectGetFailure(c, restClient)

	client := NewInfoClient(path, restClient)
	_, err := client.Info(context.TODO(), name)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *InfoSuite) expectGet(c *gc.C, client *MockRESTClient, p path.Path, name string) {
	namedPath, err := p.Join(name)
	c.Assert(err, jc.ErrorIsNil)

	client.EXPECT().Get(gomock.Any(), namedPath, gomock.Any()).Do(func(_ context.Context, _ path.Path, response *transport.InfoResponse) {
		response.Name = name
	}).Return(nil)
}

func (s *InfoSuite) expectGetFailure(c *gc.C, client *MockRESTClient) {
	client.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.Errorf("boom"))
}

func (s *InfoSuite) TestInfoRequestPayload(c *gc.C) {
	infoResponse := transport.InfoResponse{
		Name: "wordpress",
		Type: "object",
		ID:   "charmCHARMcharmCHARMcharmCHARM01",
		ChannelMap: []transport.ChannelMap{{
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
		}},
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
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		err := json.NewEncoder(w).Encode(infoResponse)
		c.Assert(err, jc.ErrorIsNil)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	config := Config{
		URL: server.URL,
	}
	basePath, err := config.BasePath()
	c.Assert(err, jc.ErrorIsNil)

	infoPath, err := basePath.Join("info")
	c.Assert(err, jc.ErrorIsNil)

	apiRequester := NewAPIRequester(DefaultHTTPTransport())
	restClient := NewHTTPRESTClient(apiRequester)

	client := NewInfoClient(infoPath, restClient)
	response, err := client.Info(context.TODO(), "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response, gc.DeepEquals, infoResponse)
}
