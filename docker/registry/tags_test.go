// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry_test

import (
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/docker/registry"
	"github.com/juju/juju/tools"
)

func (s *baseSuite) TestTagsV1(c *gc.C) {
	// Use v1 for public repository.
	s.isPrivate = false
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	data := `
[{"name": "2.9.10.1"},{"name": "2.9.10.2"},{"name": "2.9.10"}]
`[1:]

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://example.com/v1/repositories/jujuqa/jujud-operator/tags`)
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
		registry.NewImageInfo(version.MustParse("2.9.10.1")),
		registry.NewImageInfo(version.MustParse("2.9.10.2")),
		registry.NewImageInfo(version.MustParse("2.9.10")),
	})
}

func (s *baseSuite) TestTagsV2(c *gc.C) {
	// Use v2 for private repository.
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	data := `
{"name":"jujuqa/jujud-operator","tags":["2.9.10.1","2.9.10.2","2.9.10"]}
`[1:]

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Bearer jwt-token"}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://example.com/v2/jujuqa/jujud-operator/tags/list`)
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
		registry.NewImageInfo(version.MustParse("2.9.10.1")),
		registry.NewImageInfo(version.MustParse("2.9.10.2")),
		registry.NewImageInfo(version.MustParse("2.9.10")),
	})
}

func (s *baseSuite) TestTagsErrorResponseV1(c *gc.C) {
	s.isPrivate = false
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	data := `
{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}
`[1:]

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://example.com/v1/repositories/jujuqa/jujud-operator/tags`)
			resps := &http.Response{
				Request:    req,
				StatusCode: http.StatusForbidden,
				Body:       ioutil.NopCloser(strings.NewReader(data)),
			}
			return resps, nil
		}),
	)
	_, err := reg.Tags("jujud-operator")
	c.Assert(err, gc.ErrorMatches, `Get "https://example.com/v1/repositories/jujuqa/jujud-operator/tags": non-successful response status=403`)
}

func (s *baseSuite) TestTagsErrorResponseV2(c *gc.C) {
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	data := `
{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}
`[1:]

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Bearer jwt-token"}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://example.com/v2/jujuqa/jujud-operator/tags/list`)
			resps := &http.Response{
				Request:    req,
				StatusCode: http.StatusForbidden,
				Body:       ioutil.NopCloser(strings.NewReader(data)),
			}
			return resps, nil
		}),
	)
	_, err := reg.Tags("jujud-operator")
	c.Assert(err, gc.ErrorMatches, `Get "https://example.com/v2/jujuqa/jujud-operator/tags/list": non-successful response status=403`)
}
