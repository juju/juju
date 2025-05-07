// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	"encoding/base64"
	"io"
	"net/http"
	"strings"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry"
	"github.com/juju/juju/internal/docker/registry/internal"
	"github.com/juju/juju/internal/docker/registry/mocks"
	"github.com/juju/juju/internal/testhelpers"
)

type baseSuite struct {
	testhelpers.IsolationSuite

	mockRoundTripper *mocks.MockRoundTripper
	imageRepoDetails docker.ImageRepoDetails
	isPrivate        bool
}

var _ = tc.Suite(&baseSuite{})

func (s *baseSuite) getAuthToken(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}

func (s *baseSuite) getRegistry(c *tc.C) (*internal.BaseClient, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	s.imageRepoDetails = docker.ImageRepoDetails{
		Repository:    "example.com/jujuqa",
		ServerAddress: "example.com",
	}
	authToken := s.getAuthToken("username", "pwd")
	if s.isPrivate {
		s.imageRepoDetails.BasicAuthConfig = docker.BasicAuthConfig{
			Auth: docker.NewToken(authToken),
		}
	}

	s.mockRoundTripper = mocks.NewMockRoundTripper(ctrl)
	gomock.InOrder(
		// registry.Ping() 1st try failed - bearer token was missing.
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
			func(req *http.Request) (*http.Response, error) {
				c.Assert(req.Header, tc.DeepEquals, http.Header{})
				c.Assert(req.Method, tc.Equals, `GET`)
				c.Assert(req.URL.String(), tc.Equals, `https://example.com/v2`)
				return &http.Response{
					Request:    req,
					StatusCode: http.StatusUnauthorized,
					Body:       io.NopCloser(nil),
					Header: http.Header{
						http.CanonicalHeaderKey("WWW-Authenticate"): []string{
							`Bearer realm="https://auth.example.com/token",service="registry.example.com",scope="repository:jujuqa/jujud-operator:pull"`,
						},
					},
				}, nil
			},
		),
		// Refresh OAuth Token
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
			func(req *http.Request) (*http.Response, error) {
				if s.isPrivate {
					c.Assert(req.Header, tc.DeepEquals, http.Header{"Authorization": []string{"Basic " + authToken}})
				}
				c.Assert(req.Method, tc.Equals, `GET`)
				c.Assert(req.URL.String(), tc.Equals, `https://auth.example.com/token?scope=repository%3Ajujuqa%2Fjujud-operator%3Apull&service=registry.example.com`)
				return &http.Response{
					Request:    req,
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"token": "jwt-token", "access_token": "jwt-token","expires_in": 300}`)),
				}, nil
			},
		),
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
			func(req *http.Request) (*http.Response, error) {
				c.Assert(req.Header, tc.DeepEquals, http.Header{"Authorization": []string{"Bearer " + `jwt-token`}})
				c.Assert(req.Method, tc.Equals, `GET`)
				c.Assert(req.URL.String(), tc.Equals, `https://example.com/v2`)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(nil)}, nil
			},
		),
	)
	s.PatchValue(&registry.DefaultTransport, s.mockRoundTripper)

	reg, err := registry.New(s.imageRepoDetails)
	c.Assert(err, tc.ErrorIsNil)
	client, ok := reg.(*internal.BaseClient)
	c.Assert(ok, tc.IsTrue)
	err = reg.Ping()
	c.Assert(err, tc.ErrorIsNil)
	return client, ctrl
}

func (s *baseSuite) TestPingPublicRepository(c *tc.C) {
	s.isPrivate = false
	_, ctrl := s.getRegistry(c)
	ctrl.Finish()
}

func (s *baseSuite) TestPingPrivateRepository(c *tc.C) {
	s.isPrivate = true
	_, ctrl := s.getRegistry(c)
	ctrl.Finish()
}

func (s *baseSuite) TestInvalidAuth(c *tc.C) {
	s.imageRepoDetails = docker.ImageRepoDetails{
		Repository:    "example.com/jujuqa",
		ServerAddress: "example.com",
	}
	s.imageRepoDetails.TokenAuthConfig = docker.TokenAuthConfig{
		RegistryToken: &docker.Token{Value: `xxxxx==`},
	}

	_, err := registry.New(s.imageRepoDetails)
	c.Assert(err, tc.ErrorMatches, `only {"username", "password"} or {"auth"} authorization is supported for registry "example.com"`)
}
