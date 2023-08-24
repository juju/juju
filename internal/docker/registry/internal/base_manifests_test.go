// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	"io"
	"net/http"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/docker/registry/internal"
	"github.com/juju/juju/internal/docker/registry/internal/mocks"
)

func (s *baseSuite) assertGetManifestsSchemaVersion1(c *gc.C, responseData, contentType string, statusCode int, f func(*internal.ManifestsResult, error)) {
	// Use v2 for private repository.
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://example.com/v2/jujuqa/jujud-operator/manifests/2.9.10`)
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
		}),
		// Refresh OAuth Token.
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
			func(req *http.Request) (*http.Response, error) {
				c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Basic " + s.getAuthToken("username", "pwd")}})
				c.Assert(req.Method, gc.Equals, `GET`)
				c.Assert(req.URL.String(), gc.Equals, `https://auth.example.com/token?scope=repository%3Ajujuqa%2Fjujud-operator%3Apull&service=registry.example.com`)
				return &http.Response{
					Request:    req,
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"token": "jwt-token", "access_token": "jwt-token","expires_in": 300}`)),
				}, nil
			},
		),
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Bearer jwt-token"}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://example.com/v2/jujuqa/jujud-operator/manifests/2.9.10`)
			resps := &http.Response{
				Header: http.Header{
					http.CanonicalHeaderKey("Content-Type"): []string{contentType},
				},
				Request:    req,
				StatusCode: statusCode,
				Body:       io.NopCloser(strings.NewReader(responseData)),
			}
			return resps, nil
		}),
	)
	manifests, err := reg.GetManifests("jujud-operator", "2.9.10")
	f(manifests, err)
}

func (s *baseSuite) TestGetManifestsSchemaVersion1(c *gc.C) {
	s.assertGetManifestsSchemaVersion1(c,
		`
{ "schemaVersion": 1, "name": "jujuqa/jujud-operator", "tag": "2.9.13", "architecture": "amd64"}
`[1:],
		`application/vnd.docker.distribution.manifest.v1+prettyjws`,
		http.StatusOK,
		func(result *internal.ManifestsResult, err error) {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(result, jc.DeepEquals, &internal.ManifestsResult{Architecture: "amd64"})
		},
	)
}

func (s *baseSuite) TestGetManifestsSchemaVersion2(c *gc.C) {
	s.assertGetManifestsSchemaVersion1(c,
		`
{
    "schemaVersion": 2,
    "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
    "config": {
        "mediaType": "application/vnd.docker.container.image.v1+json",
        "size": 4596,
        "digest": "sha256:f0609d8a844f7271411c1a9c5d7a898fd9f9c5a4844e3bc7db6d725b54671ac1"
    }
}
`[1:],
		`application/vnd.docker.distribution.manifest.v2+prettyjws`,
		http.StatusOK,
		func(result *internal.ManifestsResult, err error) {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(result, jc.DeepEquals, &internal.ManifestsResult{Digest: "sha256:f0609d8a844f7271411c1a9c5d7a898fd9f9c5a4844e3bc7db6d725b54671ac1"})
		},
	)
}

func (s *baseSuite) TestGetManifestsSchemaVersion2NotFound(c *gc.C) {
	s.assertGetManifestsSchemaVersion1(c,
		`
{
    "schemaVersion": 2,
    "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
    "config": {
        "mediaType": "application/vnd.docker.container.image.v1+json",
        "size": 4596,
        "digest": "sha256:f0609d8a844f7271411c1a9c5d7a898fd9f9c5a4844e3bc7db6d725b54671ac1"
    }
}
`[1:],
		`application/vnd.docker.distribution.manifest.v2+prettyjws`,
		http.StatusNotFound,
		func(_ *internal.ManifestsResult, err error) {
			c.Assert(err, jc.Satisfies, errors.IsNotFound)
			c.Assert(err, gc.ErrorMatches, `Get "https://example.com/v2/jujuqa/jujud-operator/manifests/2.9.10": non-successful response status=404 not found`)
		},
	)
}

func (s *baseSuite) TestGetBlobs(c *gc.C) {
	// Use v2 for private repository.
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals,
				`https://example.com/v2/jujuqa/jujud-operator/blobs/sha256:f0609d8a844f7271411c1a9c5d7a898fd9f9c5a4844e3bc7db6d725b54671ac1`,
			)
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
		}),
		// Refresh OAuth Token.
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
			func(req *http.Request) (*http.Response, error) {
				c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Basic " + s.getAuthToken("username", "pwd")}})
				c.Assert(req.Method, gc.Equals, `GET`)
				c.Assert(req.URL.String(), gc.Equals, `https://auth.example.com/token?scope=repository%3Ajujuqa%2Fjujud-operator%3Apull&service=registry.example.com`)
				return &http.Response{
					Request:    req,
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"token": "jwt-token", "access_token": "jwt-token","expires_in": 300}`)),
				}, nil
			},
		),
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Bearer jwt-token"}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals,
				`https://example.com/v2/jujuqa/jujud-operator/blobs/sha256:f0609d8a844f7271411c1a9c5d7a898fd9f9c5a4844e3bc7db6d725b54671ac1`,
			)
			resps := &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`
{"architecture":"amd64"}
`[1:])),
			}
			return resps, nil
		}),
	)
	manifests, err := reg.GetBlobs("jujud-operator", "sha256:f0609d8a844f7271411c1a9c5d7a898fd9f9c5a4844e3bc7db6d725b54671ac1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(manifests, jc.DeepEquals, &internal.BlobsResponse{Architecture: "amd64"})
}

func (s *baseSuite) TestGetArchitectureV1(c *gc.C) {
	ctrl := gomock.NewController(c)
	client := mocks.NewMockArchitectureGetter(ctrl)

	gomock.InOrder(
		client.EXPECT().GetManifests("jujud-operator", "2.9.12").Return(
			&internal.ManifestsResult{Architecture: "amd64"}, nil,
		),
	)
	arch, err := internal.GetArchitecture("jujud-operator", "2.9.12", client)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(arch, jc.DeepEquals, "amd64")
}

func (s *baseSuite) TestGetArchitectureV2(c *gc.C) {
	ctrl := gomock.NewController(c)
	client := mocks.NewMockArchitectureGetter(ctrl)

	gomock.InOrder(
		client.EXPECT().GetManifests("jujud-operator", "2.9.12").Return(
			&internal.ManifestsResult{Digest: "sha256:f0609d8a844f7271411c1a9c5d7a898fd9f9c5a4844e3bc7db6d725b54671ac1"}, nil,
		),
		client.EXPECT().GetBlobs("jujud-operator", "sha256:f0609d8a844f7271411c1a9c5d7a898fd9f9c5a4844e3bc7db6d725b54671ac1").Return(
			&internal.BlobsResponse{Architecture: "amd64"}, nil,
		),
	)
	arch, err := internal.GetArchitecture("jujud-operator", "2.9.12", client)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(arch, jc.DeepEquals, "amd64")
}

func (s *baseSuite) TestGetArchitectureInvalidResponse(c *gc.C) {
	ctrl := gomock.NewController(c)
	client := mocks.NewMockArchitectureGetter(ctrl)

	gomock.InOrder(
		client.EXPECT().GetManifests("jujud-operator", "2.9.12").Return(
			&internal.ManifestsResult{}, nil,
		),
	)
	_, err := internal.GetArchitecture("jujud-operator", "2.9.12", client)
	c.Assert(err, gc.ErrorMatches, `faild to get manifests for "jujud-operator" "2.9.12"`)
}
