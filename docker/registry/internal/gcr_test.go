// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	"encoding/base64"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/docker"
	"github.com/juju/juju/docker/registry"
	"github.com/juju/juju/docker/registry/image"
	"github.com/juju/juju/docker/registry/internal"
	"github.com/juju/juju/docker/registry/mocks"
	"github.com/juju/juju/tools"
)

type googleContainerRegistrySuite struct {
	testing.IsolationSuite

	mockRoundTripper *mocks.MockRoundTripper
	imageRepoDetails docker.ImageRepoDetails
	isPrivate        bool
	authToken        string
}

var _ = gc.Suite(&googleContainerRegistrySuite{})

func (s *googleContainerRegistrySuite) getRegistry(c *gc.C) (registry.Registry, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	s.imageRepoDetails = docker.ImageRepoDetails{
		Repository: "gcr.io/jujuqa",
	}
	s.authToken = base64.StdEncoding.EncodeToString([]byte("_json_key:pwd"))
	if s.isPrivate {
		s.imageRepoDetails.BasicAuthConfig = docker.BasicAuthConfig{
			Auth: docker.NewToken(s.authToken),
		}
	}

	s.mockRoundTripper = mocks.NewMockRoundTripper(ctrl)
	if s.isPrivate {
		gomock.InOrder(
			// registry.Ping() 1st try failed - bearer token was missing.
			s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
				func(req *http.Request) (*http.Response, error) {
					c.Assert(req.Header, jc.DeepEquals, http.Header{})
					c.Assert(req.Method, gc.Equals, `GET`)
					c.Assert(req.URL.String(), gc.Equals, `https://gcr.io/v2/`)
					return &http.Response{
						Request:    req,
						StatusCode: http.StatusUnauthorized,
						Body:       ioutil.NopCloser(nil),
						Header: http.Header{
							http.CanonicalHeaderKey("WWW-Authenticate"): []string{
								`Bearer realm="https://gcr.io/v2/token",service="gcr.io",scope="repository:jujuqa/jujud-operator:pull"`,
							},
						},
					}, nil
				},
			),
			// Refresh OAuth Token
			s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
				func(req *http.Request) (*http.Response, error) {
					c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Basic " + s.authToken}})
					c.Assert(req.Method, gc.Equals, `GET`)
					c.Assert(req.URL.String(), gc.Equals, `https://gcr.io/v2/token?scope=repository%3Ajujuqa%2Fjujud-operator%3Apull&service=gcr.io`)
					return &http.Response{
						Request:    req,
						StatusCode: http.StatusOK,
						Body:       ioutil.NopCloser(strings.NewReader(`{"token": "jwt-token", "access_token": "jwt-token","expires_in": 300}`)),
					}, nil
				},
			),
			// registry.Ping()
			s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
				func(req *http.Request) (*http.Response, error) {
					c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Bearer jwt-token"}})
					c.Assert(req.Method, gc.Equals, `GET`)
					c.Assert(req.URL.String(), gc.Equals, `https://gcr.io/v2/`)
					return &http.Response{StatusCode: http.StatusOK, Body: ioutil.NopCloser(nil)}, nil
				},
			),
		)
	}
	s.PatchValue(&registry.DefaultTransport, s.mockRoundTripper)

	reg, err := registry.New(s.imageRepoDetails)
	c.Assert(err, jc.ErrorIsNil)
	_, ok := reg.(*internal.GoogleContainerRegistry)
	c.Assert(ok, jc.IsTrue)
	err = reg.Ping()
	c.Assert(err, jc.ErrorIsNil)
	return reg, ctrl
}

func (s *googleContainerRegistrySuite) TestInvalidUserName(c *gc.C) {
	imageRepoDetails := docker.ImageRepoDetails{
		Repository: "gcr.io/jujuqa",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken(base64.StdEncoding.EncodeToString([]byte("username:pwd"))),
		},
	}
	_, err := registry.New(imageRepoDetails)
	c.Assert(err, gc.ErrorMatches, `validating the google container registry credential: google container registry username has to be "_json_key"`)
}

func (s *googleContainerRegistrySuite) TestPingPrivateRepository(c *gc.C) {
	s.isPrivate = true
	_, ctrl := s.getRegistry(c)
	ctrl.Finish()
}

func (s *googleContainerRegistrySuite) TestTags(c *gc.C) {
	// Use v2 for private repository.
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	data := `
{"name":"jujuqa/jujud-operator","tags":["2.9.10.1","2.9.10.2","2.9.10"]}
`[1:]

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://gcr.io/v2/jujuqa/jujud-operator/tags/list`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusUnauthorized,
				Body:       ioutil.NopCloser(nil),
				Header: http.Header{
					http.CanonicalHeaderKey("WWW-Authenticate"): []string{
						`Bearer realm="https://gcr.io/v2/token",service="gcr.io",scope="repository:jujuqa/jujud-operator:pull"`,
					},
				},
			}, nil
		}),
		// Refresh OAuth Token
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
			func(req *http.Request) (*http.Response, error) {
				c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Basic " + s.authToken}})
				c.Assert(req.Method, gc.Equals, `GET`)
				c.Assert(req.URL.String(), gc.Equals, `https://gcr.io/v2/token?scope=repository%3Ajujuqa%2Fjujud-operator%3Apull&service=gcr.io`)
				return &http.Response{
					Request:    req,
					StatusCode: http.StatusOK,
					Body:       ioutil.NopCloser(strings.NewReader(`{"token": "jwt-token", "access_token": "jwt-token","expires_in": 300}`)),
				}, nil
			},
		),
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Bearer jwt-token"}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://gcr.io/v2/jujuqa/jujud-operator/tags/list`)
			resps := &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(strings.NewReader(data)),
			}
			return resps, nil
		}),
	)
	vers, err := reg.Tags("jujud-operator")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vers, jc.DeepEquals, tools.Versions{
		image.NewImageInfo(version.MustParse("2.9.10.1")),
		image.NewImageInfo(version.MustParse("2.9.10.2")),
		image.NewImageInfo(version.MustParse("2.9.10")),
	})
}

func (s *googleContainerRegistrySuite) TestTagsErrorResponse(c *gc.C) {
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	data := `
{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}
`[1:]

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://gcr.io/v2/jujuqa/jujud-operator/tags/list`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusUnauthorized,
				Body:       ioutil.NopCloser(nil),
				Header: http.Header{
					http.CanonicalHeaderKey("WWW-Authenticate"): []string{
						`Bearer realm="https://gcr.io/v2/token",service="gcr.io",scope="repository:jujuqa/jujud-operator:pull"`,
					},
				},
			}, nil
		}),
		// Refresh OAuth Token
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
			func(req *http.Request) (*http.Response, error) {
				c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Basic " + s.authToken}})
				c.Assert(req.Method, gc.Equals, `GET`)
				c.Assert(req.URL.String(), gc.Equals, `https://gcr.io/v2/token?scope=repository%3Ajujuqa%2Fjujud-operator%3Apull&service=gcr.io`)
				return &http.Response{
					Request:    req,
					StatusCode: http.StatusOK,
					Body:       ioutil.NopCloser(strings.NewReader(`{"token": "jwt-token", "access_token": "jwt-token","expires_in": 300}`)),
				}, nil
			},
		),
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Bearer jwt-token"}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://gcr.io/v2/jujuqa/jujud-operator/tags/list`)
			resps := &http.Response{
				Request:    req,
				StatusCode: http.StatusForbidden,
				Body:       ioutil.NopCloser(strings.NewReader(data)),
			}
			return resps, nil
		}),
	)
	_, err := reg.Tags("jujud-operator")
	c.Assert(err, gc.ErrorMatches, `Get "https://gcr.io/v2/jujuqa/jujud-operator/tags/list": non-successful response status=403`)
}
