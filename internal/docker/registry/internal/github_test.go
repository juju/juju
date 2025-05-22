// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry"
	"github.com/juju/juju/internal/docker/registry/image"
	"github.com/juju/juju/internal/docker/registry/internal"
	"github.com/juju/juju/internal/docker/registry/mocks"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/tools"
)

type githubSuite struct {
	testhelpers.IsolationSuite

	mockRoundTripper *mocks.MockRoundTripper
	imageRepoDetails *docker.ImageRepoDetails
	isPrivate        bool
}

func TestGithubSuite(t *stdtesting.T) {
	tc.Run(t, &githubSuite{})
}

func (s *githubSuite) TearDownTest(c *tc.C) {
	s.imageRepoDetails = nil
	s.IsolationSuite.TearDownTest(c)
}

func (s *githubSuite) getRegistry(c *tc.C) (registry.Registry, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	if s.imageRepoDetails == nil {
		s.imageRepoDetails = &docker.ImageRepoDetails{
			Repository: "ghcr.io/jujuqa",
		}
		if s.isPrivate {
			authToken := base64.StdEncoding.EncodeToString([]byte("username:pwd"))
			s.imageRepoDetails.BasicAuthConfig = docker.BasicAuthConfig{
				Auth: docker.NewToken(authToken),
			}
		}
	}

	s.mockRoundTripper = mocks.NewMockRoundTripper(ctrl)
	if s.isPrivate {
		gomock.InOrder(
			// registry.Ping()
			s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
				func(req *http.Request) (*http.Response, error) {
					authToken := base64.StdEncoding.EncodeToString([]byte("pwd"))
					c.Assert(req.Header, tc.DeepEquals, http.Header{"Authorization": []string{"Bearer " + authToken}})
					c.Assert(req.Method, tc.Equals, `GET`)
					c.Assert(req.URL.String(), tc.Equals, `https://ghcr.io/v2/`)
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(nil)}, nil
				},
			),
		)
	}
	s.PatchValue(&registry.DefaultTransport, s.mockRoundTripper)

	reg, err := registry.New(*s.imageRepoDetails)
	c.Assert(err, tc.ErrorIsNil)
	_, ok := reg.(*internal.GithubContainerRegistry)
	c.Assert(ok, tc.IsTrue)
	err = reg.Ping()
	c.Assert(err, tc.ErrorIsNil)
	return reg, ctrl
}

func (s *githubSuite) TestPingPublicRepository(c *tc.C) {
	s.isPrivate = false
	_, ctrl := s.getRegistry(c)
	ctrl.Finish()
}

func (s *githubSuite) TestPingPrivateRepository(c *tc.C) {
	s.isPrivate = true
	_, ctrl := s.getRegistry(c)
	ctrl.Finish()
}

func (s *githubSuite) TestPingPrivateRepositoryUserNamePassword(c *tc.C) {
	s.imageRepoDetails = &docker.ImageRepoDetails{
		Repository: "ghcr.io/jujuqa",
		BasicAuthConfig: docker.BasicAuthConfig{
			Username: "username",
			Password: "pwd",
		},
	}
	s.isPrivate = true
	_, ctrl := s.getRegistry(c)
	ctrl.Finish()
}

func (s *githubSuite) TestPingPrivateRepositoryNoCredential(c *tc.C) {
	imageRepoDetails := docker.ImageRepoDetails{
		Repository: "ghcr.io/jujuqa",
		BasicAuthConfig: docker.BasicAuthConfig{
			Username: "username",
		},
	}
	_, err := registry.New(imageRepoDetails)
	c.Assert(err, tc.ErrorMatches, `github container registry requires {"username", "password"} or {"auth"} token`)
}

func (s *githubSuite) TestPingPrivateRepositoryBadAuthTokenFormat(c *tc.C) {
	authToken := base64.StdEncoding.EncodeToString([]byte("bad-auth"))
	imageRepoDetails := docker.ImageRepoDetails{
		Repository: "ghcr.io/jujuqa",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken(authToken),
		},
	}
	_, err := registry.New(imageRepoDetails)
	c.Assert(err, tc.ErrorMatches, `getting password from the github container registry auth token: registry auth token not valid`)
}

func (s *githubSuite) TestPingPrivateRepositoryBadAuthTokenNoPasswordIncluded(c *tc.C) {
	authToken := base64.StdEncoding.EncodeToString([]byte("username:"))
	imageRepoDetails := docker.ImageRepoDetails{
		Repository: "ghcr.io/jujuqa",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken(authToken),
		},
	}
	_, err := registry.New(imageRepoDetails)
	c.Assert(err, tc.ErrorMatches, `github container registry auth token contains empty password`)
}

func (s *githubSuite) TestTagsPublicRegistry(c *tc.C) {
	// Use anonymous login for public repository.
	s.isPrivate = false
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	data := `
{"name":"jujuqa/jujud-operator","tags":["2.9.10.1","2.9.10.2","2.9.10"]}
`[1:]

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, tc.DeepEquals, http.Header{})
			c.Assert(req.Method, tc.Equals, `GET`)
			c.Assert(req.URL.String(), tc.Equals, `https://ghcr.io/v2/jujuqa/jujud-operator/tags/list`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(nil),
				Header: http.Header{
					http.CanonicalHeaderKey("WWW-Authenticate"): []string{
						`Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:jujuqa/jujud-operator:pull"`,
					},
				},
			}, nil
		}),
		// Refresh OAuth Token without credential.
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
			func(req *http.Request) (*http.Response, error) {
				c.Assert(req.Header, tc.DeepEquals, http.Header{})
				c.Assert(req.Method, tc.Equals, `GET`)
				c.Assert(req.URL.String(), tc.Equals, `https://ghcr.io/token?scope=repository%3Ajujuqa%2Fjujud-operator%3Apull&service=ghcr.io`)
				return &http.Response{
					Request:    req,
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"token": "jwt-token", "access_token": "jwt-token","expires_in": 300}`)),
				}, nil
			},
		),
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, tc.DeepEquals, http.Header{"Authorization": []string{"Bearer jwt-token"}})
			c.Assert(req.Method, tc.Equals, `GET`)
			c.Assert(req.URL.String(), tc.Equals, `https://ghcr.io/v2/jujuqa/jujud-operator/tags/list`)
			resps := &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(data)),
			}
			return resps, nil
		}),
	)
	vers, err := reg.Tags("jujud-operator")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(vers, tc.DeepEquals, tools.Versions{
		image.NewImageInfo(semversion.MustParse("2.9.10.1")),
		image.NewImageInfo(semversion.MustParse("2.9.10.2")),
		image.NewImageInfo(semversion.MustParse("2.9.10")),
	})
}

func (s *githubSuite) TestTagsPrivateRegistry(c *tc.C) {
	// Use v2 for private repository.
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	data := `
{"name":"jujuqa/jujud-operator","tags":["2.9.10.1","2.9.10.2","2.9.10"]}
`[1:]

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			authToken := base64.StdEncoding.EncodeToString([]byte("pwd"))
			c.Assert(req.Header, tc.DeepEquals, http.Header{"Authorization": []string{"Bearer " + authToken}})
			c.Assert(req.Method, tc.Equals, `GET`)
			c.Assert(req.URL.String(), tc.Equals, `https://ghcr.io/v2/jujuqa/jujud-operator/tags/list`)
			resps := &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(data)),
			}
			return resps, nil
		}),
	)
	vers, err := reg.Tags("jujud-operator")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(vers, tc.DeepEquals, tools.Versions{
		image.NewImageInfo(semversion.MustParse("2.9.10.1")),
		image.NewImageInfo(semversion.MustParse("2.9.10.2")),
		image.NewImageInfo(semversion.MustParse("2.9.10")),
	})
}

func (s *githubSuite) TestTagsErrorResponse(c *tc.C) {
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	data := `
{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}
`[1:]

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			authToken := base64.StdEncoding.EncodeToString([]byte("pwd"))
			c.Assert(req.Header, tc.DeepEquals, http.Header{"Authorization": []string{"Bearer " + authToken}})
			c.Assert(req.Method, tc.Equals, `GET`)
			c.Assert(req.URL.String(), tc.Equals, `https://ghcr.io/v2/jujuqa/jujud-operator/tags/list`)
			resps := &http.Response{
				Request:    req,
				StatusCode: http.StatusForbidden,
				Body:       io.NopCloser(strings.NewReader(data)),
			}
			return resps, nil
		}),
	)
	_, err := reg.Tags("jujud-operator")
	c.Assert(err, tc.ErrorMatches, `Get "https://ghcr.io/v2/jujuqa/jujud-operator/tags/list": non-successful response status=403`)
}
