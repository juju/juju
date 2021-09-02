// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry_test

import (
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
	"github.com/juju/juju/docker/registry/mocks"
	"github.com/juju/juju/feature"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
)

type registrySuite struct {
	testing.IsolationSuite
	coretesting.JujuOSEnvSuite

	mockRoundTripper *mocks.MockRoundTripper
	imageRepoDetails docker.ImageRepoDetails
	tokenAuth        bool
}

var _ = gc.Suite(&registrySuite{})

func (s *registrySuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.PrivateRegistry)
}

func (s *registrySuite) TearDownTest(c *gc.C) {
	s.IsolationSuite.TearDownTest(c)
	s.JujuOSEnvSuite.TearDownTest(c)

	s.mockRoundTripper = nil
	s.tokenAuth = false
}

func (s *registrySuite) getRegistry(c *gc.C) (registry.Registry, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	s.mockRoundTripper = mocks.NewMockRoundTripper(ctrl)
	if s.tokenAuth {
		s.imageRepoDetails = docker.ImageRepoDetails{
			Repository:    "jujuqa",
			ServerAddress: "quay.io",
			TokenAuthConfig: docker.TokenAuthConfig{
				RegistryToken: "xxxxx==",
			},
		}
		gomock.InOrder(
			// registry.Ping()
			s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
				func(req *http.Request) (*http.Response, error) {
					c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Bearer xxxxx=="}})
					c.Assert(req.Method, gc.Equals, `GET`)
					c.Assert(req.URL.String(), gc.Equals, `https://quay.io/v2`)
					return &http.Response{StatusCode: http.StatusOK, Body: ioutil.NopCloser(nil)}, nil
				},
			),
		)
	} else {
		s.imageRepoDetails = docker.ImageRepoDetails{
			Repository:    "jujuqa",
			ServerAddress: "quay.io",
			BasicAuthConfig: docker.BasicAuthConfig{
				Auth: "xxxxx==",
			},
		}
		gomock.InOrder(
			// registry.Ping()
			s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
				func(req *http.Request) (*http.Response, error) {
					c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Basic xxxxx=="}})
					c.Assert(req.Method, gc.Equals, `GET`)
					c.Assert(req.URL.String(), gc.Equals, `https://quay.io/v1`)
					return &http.Response{StatusCode: http.StatusOK, Body: ioutil.NopCloser(nil)}, nil
				},
			),
		)
	}
	s.PatchValue(&registry.DefaultTransport, s.mockRoundTripper)

	reg, err := registry.New(s.imageRepoDetails)
	c.Assert(err, jc.ErrorIsNil)
	return reg, ctrl
}

func (s *registrySuite) TestPingBasicAuth(c *gc.C) {
	reg, ctrl := s.getRegistry(c)
	err := reg.Close()
	c.Assert(err, jc.ErrorIsNil)
	defer ctrl.Finish()
}

func (s *registrySuite) TestPingTokenAuth(c *gc.C) {
	s.tokenAuth = true
	reg, ctrl := s.getRegistry(c)
	err := reg.Close()
	c.Assert(err, jc.ErrorIsNil)
	defer ctrl.Finish()
}

func (s *registrySuite) TestTagsV1(c *gc.C) {
	reg, ctrl := s.getRegistry(c)
	err := reg.Close()
	c.Assert(err, jc.ErrorIsNil)
	defer ctrl.Finish()

	data := `
[{"name": "2.9.10.1"},{"name": "2.9.10.2"},{"name": "2.9.10"}]
`[1:]

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Basic": []string{"xxxxx=="}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://quay.io/v1/repositories/jujuqa/jujud-operator/tags`)
			resps := &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(strings.NewReader(data)),
			}
			return resps, nil
		}),
	)
	vers, err := reg.Tags("jujud-operator")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vers, jc.DeepEquals, tools.Versions{
		docker.NewImageInfo(version.MustParse("2.9.10.1")),
		docker.NewImageInfo(version.MustParse("2.9.10.2")),
		docker.NewImageInfo(version.MustParse("2.9.10")),
	})
}

func (s *registrySuite) TestTagsV2(c *gc.C) {
	s.tokenAuth = true
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	data := `
{"name":"jujuqa/jujud-operator","tags":["2.9.10.1","2.9.10.2","2.9.10"]}
`[1:]

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Bearer xxxxx=="}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://quay.io/v2/jujuqa/jujud-operator/tags/list`)
			resps := &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(strings.NewReader(data)),
			}
			return resps, nil
		}),
	)
	vers, err := reg.Tags("jujud-operator")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vers, jc.DeepEquals, tools.Versions{
		docker.NewImageInfo(version.MustParse("2.9.10.1")),
		docker.NewImageInfo(version.MustParse("2.9.10.2")),
		docker.NewImageInfo(version.MustParse("2.9.10")),
	})
}

func (s *registrySuite) TestTagsErrorResponse(c *gc.C) {
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	data := `
{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}
`[1:]

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Basic": []string{"xxxxx=="}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://quay.io/v1/repositories/jujuqa/jujud-operator/tags`)
			resps := &http.Response{
				StatusCode: http.StatusForbidden,
				Body:       ioutil.NopCloser(strings.NewReader(data)),
			}
			return resps, nil
		}),
	)
	_, err := reg.Tags("jujud-operator")
	c.Assert(err, gc.ErrorMatches, `Get "https://quay.io/v1/repositories/jujuqa/jujud-operator/tags": non-successful response \(status=403 body=.*\)`)
}
