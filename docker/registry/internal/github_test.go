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
	"github.com/juju/juju/docker/registry/utils"
	"github.com/juju/juju/tools"
)

type githubSuite struct {
	testing.IsolationSuite

	mockRoundTripper *mocks.MockRoundTripper
	imageRepoDetails *docker.ImageRepoDetails
	isPrivate        bool
}

var _ = gc.Suite(&githubSuite{})

func (s *githubSuite) TearDownTest(c *gc.C) {
	s.imageRepoDetails = nil
	s.IsolationSuite.TearDownTest(c)
}

func (s *githubSuite) getRegistry(c *gc.C) (registry.Registry, *gomock.Controller) {
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
					c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Bearer " + authToken}})
					c.Assert(req.Method, gc.Equals, `GET`)
					c.Assert(req.URL.String(), gc.Equals, `https://ghcr.io/v2/`)
					return &http.Response{StatusCode: http.StatusOK, Body: ioutil.NopCloser(nil)}, nil
				},
			),
		)
	}
	s.PatchValue(&registry.DefaultTransport, s.mockRoundTripper)

	reg, err := registry.New(*s.imageRepoDetails)
	c.Assert(err, jc.ErrorIsNil)
	_, ok := reg.(*internal.GithubContainerRegistry)
	c.Assert(ok, jc.IsTrue)
	err = reg.Ping()
	if s.isPrivate {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, jc.Satisfies, utils.IsPublicAPINotAvailableError)
		c.Assert(err, gc.ErrorMatches, `.*public registry API is not available for "ghcr.io"`)
	}
	return reg, ctrl
}

func (s *githubSuite) TestPingPublicRepository(c *gc.C) {
	s.isPrivate = false
	_, ctrl := s.getRegistry(c)
	ctrl.Finish()
}

func (s *githubSuite) TestPingPrivateRepository(c *gc.C) {
	s.isPrivate = true
	_, ctrl := s.getRegistry(c)
	ctrl.Finish()
}

func (s *githubSuite) TestPingPrivateRepositoryUserNamePassword(c *gc.C) {
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

func (s *githubSuite) TestPingPrivateRepositoryNoCredential(c *gc.C) {
	imageRepoDetails := docker.ImageRepoDetails{
		Repository: "ghcr.io/jujuqa",
		BasicAuthConfig: docker.BasicAuthConfig{
			Username: "username",
		},
	}
	_, err := registry.New(imageRepoDetails)
	c.Assert(err, gc.ErrorMatches, `github container registry requires {"username", "password"} or {"auth"} token`)
}

func (s *githubSuite) TestPingPrivateRepositoryBadAuthTokenFormat(c *gc.C) {
	authToken := base64.StdEncoding.EncodeToString([]byte("bad-auth"))
	imageRepoDetails := docker.ImageRepoDetails{
		Repository: "ghcr.io/jujuqa",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken(authToken),
		},
	}
	_, err := registry.New(imageRepoDetails)
	c.Assert(err, gc.ErrorMatches, `getting password from the github container registry auth token: registry auth token not valid`)
}

func (s *githubSuite) TestPingPrivateRepositoryBadAuthTokenNoPasswordIncluded(c *gc.C) {
	authToken := base64.StdEncoding.EncodeToString([]byte("username:"))
	imageRepoDetails := docker.ImageRepoDetails{
		Repository: "ghcr.io/jujuqa",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken(authToken),
		},
	}
	_, err := registry.New(imageRepoDetails)
	c.Assert(err, gc.ErrorMatches, `github container registry auth token contains empty password`)
}

func (s *githubSuite) TestTagsPublic(c *gc.C) {
	s.isPrivate = false
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	_, err := reg.Tags("jujud-operator")
	c.Assert(err, gc.ErrorMatches, `Get "https://ghcr.io/v1/repositories/jujuqa/jujud-operator/tags": public registry API is not available for "ghcr.io"`)
}

func (s *githubSuite) TestTagsV2(c *gc.C) {
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
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Bearer " + authToken}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://ghcr.io/v2/jujuqa/jujud-operator/tags/list`)
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

func (s *githubSuite) TestTagsErrorResponsePublic(c *gc.C) {
	s.isPrivate = false
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	_, err := reg.Tags("jujud-operator")
	c.Assert(err, gc.ErrorMatches, `Get "https://ghcr.io/v1/repositories/jujuqa/jujud-operator/tags": public registry API is not available for "ghcr.io"`)
}

func (s *githubSuite) TestTagsErrorResponseV2(c *gc.C) {
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	data := `
{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}
`[1:]

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			authToken := base64.StdEncoding.EncodeToString([]byte("pwd"))
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Bearer " + authToken}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://ghcr.io/v2/jujuqa/jujud-operator/tags/list`)
			resps := &http.Response{
				Request:    req,
				StatusCode: http.StatusForbidden,
				Body:       ioutil.NopCloser(strings.NewReader(data)),
			}
			return resps, nil
		}),
	)
	_, err := reg.Tags("jujud-operator")
	c.Assert(err, gc.ErrorMatches, `Get "https://ghcr.io/v2/jujuqa/jujud-operator/tags/list": non-successful response status=403`)
}
