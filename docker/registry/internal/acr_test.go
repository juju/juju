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
	"github.com/juju/juju/docker/registry/image"
	"github.com/juju/juju/docker/registry/internal"
	"github.com/juju/juju/docker/registry/mocks"
	"github.com/juju/juju/tools"
)

type azureContainerRegistrySuite struct {
	testing.IsolationSuite

	mockRoundTripper *mocks.MockRoundTripper
	imageRepoDetails docker.ImageRepoDetails
	isPrivate        bool
	authToken        string
}

var _ = gc.Suite(&azureContainerRegistrySuite{})

func (s *azureContainerRegistrySuite) getRegistry(c *gc.C) (*internal.AzureContainerRegistry, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	s.imageRepoDetails = docker.ImageRepoDetails{
		Repository: "jujuqa.azurecr.io",
	}
	s.authToken = base64.StdEncoding.EncodeToString([]byte("service-principal-id:service-principal-password"))
	if s.isPrivate {
		s.imageRepoDetails.BasicAuthConfig = docker.BasicAuthConfig{
			Auth:     docker.NewToken(s.authToken),
			Username: "service-principal-id",
			Password: "service-principal-password",
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
					c.Assert(req.URL.String(), gc.Equals, `https://jujuqa.azurecr.io/v2`)
					return &http.Response{
						Request:    req,
						StatusCode: http.StatusUnauthorized,
						Body:       ioutil.NopCloser(nil),
						Header: http.Header{
							http.CanonicalHeaderKey("WWW-Authenticate"): []string{
								`Bearer realm="https://jujuqa.azurecr.io/oauth2/token",service="jujuqa.azurecr.io",scope="repository:jujud-operator:metadata_read"`,
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
					c.Assert(req.URL.String(), gc.Equals, `https://jujuqa.azurecr.io/oauth2/token?scope=repository%3Ajujud-operator%3Ametadata_read&service=jujuqa.azurecr.io`)
					return &http.Response{
						Request:    req,
						StatusCode: http.StatusOK,
						Body:       ioutil.NopCloser(strings.NewReader(`{"access_token": "jwt-token"}`)),
					}, nil
				},
			),
			// registry.Ping()
			s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
				func(req *http.Request) (*http.Response, error) {
					c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Bearer " + `jwt-token`}})
					c.Assert(req.Method, gc.Equals, `GET`)
					c.Assert(req.URL.String(), gc.Equals, `https://jujuqa.azurecr.io/v2`)
					return &http.Response{StatusCode: http.StatusOK, Body: ioutil.NopCloser(nil)}, nil
				},
			),
		)
	}

	reg := internal.NewAzureContainerRegistry(s.imageRepoDetails, s.mockRoundTripper)
	err := internal.InitProvider(reg)
	if !s.imageRepoDetails.IsPrivate() {
		c.Assert(err, gc.ErrorMatches, `username and password are required for registry "jujuqa.azurecr.io"`)
		return nil, ctrl
	}
	c.Assert(err, jc.ErrorIsNil)
	client, ok := reg.(*internal.AzureContainerRegistry)
	c.Assert(ok, jc.IsTrue)
	err = reg.Ping()
	c.Assert(err, jc.ErrorIsNil)
	return client, ctrl
}

func (s *azureContainerRegistrySuite) TestPingPublicRepository(c *gc.C) {
	s.isPrivate = false
	_, ctrl := s.getRegistry(c)
	ctrl.Finish()
}

func (s *azureContainerRegistrySuite) TestPingPrivateRepository(c *gc.C) {
	s.isPrivate = true
	_, ctrl := s.getRegistry(c)
	ctrl.Finish()
}

func (s *azureContainerRegistrySuite) TestTagsV2(c *gc.C) {
	// Use v2 for private repository.
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	data := `
{"name":"jujud-operator","tags":["2.9.10.1","2.9.10.2","2.9.10"]}
`[1:]

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://jujuqa.azurecr.io/v2/jujud-operator/tags/list`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusUnauthorized,
				Body:       ioutil.NopCloser(nil),
				Header: http.Header{
					http.CanonicalHeaderKey("WWW-Authenticate"): []string{
						`Bearer realm="https://jujuqa.azurecr.io/oauth2/token",service="jujuqa.azurecr.io",scope="repository:jujud-operator:metadata_read"`,
					},
				},
			}, nil
		}),
		// Refresh OAuth Token
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
			func(req *http.Request) (*http.Response, error) {
				c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Basic " + s.authToken}})
				c.Assert(req.Method, gc.Equals, `GET`)
				c.Assert(req.URL.String(), gc.Equals, `https://jujuqa.azurecr.io/oauth2/token?scope=repository%3Ajujud-operator%3Ametadata_read&service=jujuqa.azurecr.io`)
				return &http.Response{
					Request:    req,
					StatusCode: http.StatusOK,
					Body:       ioutil.NopCloser(strings.NewReader(`{"access_token": "jwt-token"}`)),
				}, nil
			},
		),

		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Bearer jwt-token"}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://jujuqa.azurecr.io/v2/jujud-operator/tags/list`)
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

func (s *azureContainerRegistrySuite) TestTagsErrorResponseV2(c *gc.C) {
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
			c.Assert(req.URL.String(), gc.Equals, `https://jujuqa.azurecr.io/v2/jujud-operator/tags/list`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusUnauthorized,
				Body:       ioutil.NopCloser(nil),
				Header: http.Header{
					http.CanonicalHeaderKey("WWW-Authenticate"): []string{
						`Bearer realm="https://jujuqa.azurecr.io/oauth2/token",service="jujuqa.azurecr.io",scope="repository:jujud-operator:metadata_read"`,
					},
				},
			}, nil
		}),
		// Refresh OAuth Token
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
			func(req *http.Request) (*http.Response, error) {
				c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Basic " + s.authToken}})
				c.Assert(req.Method, gc.Equals, `GET`)
				c.Assert(req.URL.String(), gc.Equals, `https://jujuqa.azurecr.io/oauth2/token?scope=repository%3Ajujud-operator%3Ametadata_read&service=jujuqa.azurecr.io`)
				return &http.Response{
					Request:    req,
					StatusCode: http.StatusOK,
					Body:       ioutil.NopCloser(strings.NewReader(`{"access_token": "jwt-token"}`)),
				}, nil
			},
		),
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Bearer jwt-token"}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://jujuqa.azurecr.io/v2/jujud-operator/tags/list`)
			resps := &http.Response{
				Request:    req,
				StatusCode: http.StatusForbidden,
				Body:       ioutil.NopCloser(strings.NewReader(data)),
			}
			return resps, nil
		}),
	)
	_, err := reg.Tags("jujud-operator")
	c.Assert(err, gc.ErrorMatches, `Get "https://jujuqa.azurecr.io/v2/jujud-operator/tags/list": non-successful response status=403`)
}

func (s *azureContainerRegistrySuite) assertGetManifestsSchemaVersion1(c *gc.C, responseData, contentType string, result *internal.ManifestsResult) {
	// Use v2 for private repository.
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://jujuqa.azurecr.io/v2/jujud-operator/manifests/2.9.10`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusUnauthorized,
				Body:       ioutil.NopCloser(nil),
				Header: http.Header{
					http.CanonicalHeaderKey("WWW-Authenticate"): []string{
						`Bearer realm="https://jujuqa.azurecr.io/oauth2/token",service="jujuqa.azurecr.io",scope="repository:jujud-operator:metadata_read"`,
					},
				},
			}, nil
		}),
		// Refresh OAuth Token
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
			func(req *http.Request) (*http.Response, error) {
				c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Basic " + s.authToken}})
				c.Assert(req.Method, gc.Equals, `GET`)
				c.Assert(req.URL.String(), gc.Equals, `https://jujuqa.azurecr.io/oauth2/token?scope=repository%3Ajujud-operator%3Ametadata_read&service=jujuqa.azurecr.io`)
				return &http.Response{
					Request:    req,
					StatusCode: http.StatusOK,
					Body:       ioutil.NopCloser(strings.NewReader(`{"access_token": "jwt-token"}`)),
				}, nil
			},
		),
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Bearer jwt-token"}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://jujuqa.azurecr.io/v2/jujud-operator/manifests/2.9.10`)
			resps := &http.Response{
				Header: http.Header{
					http.CanonicalHeaderKey("Content-Type"): []string{contentType},
				},
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(strings.NewReader(responseData)),
			}
			return resps, nil
		}),
	)
	manifests, err := reg.GetManifests("jujud-operator", "2.9.10")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(manifests, jc.DeepEquals, result)
}

func (s *azureContainerRegistrySuite) TestGetManifestsSchemaVersion1(c *gc.C) {
	s.assertGetManifestsSchemaVersion1(c,
		`
{ "schemaVersion": 1, "name": "jujuqa/jujud-operator", "tag": "2.9.13", "architecture": "amd64"}
`[1:],
		`application/vnd.docker.distribution.manifest.v1+prettyjws`,
		&internal.ManifestsResult{Architecture: "amd64"},
	)
}

func (s *azureContainerRegistrySuite) TestGetManifestsSchemaVersion2(c *gc.C) {
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
		&internal.ManifestsResult{Digest: "sha256:f0609d8a844f7271411c1a9c5d7a898fd9f9c5a4844e3bc7db6d725b54671ac1"},
	)
}

func (s *azureContainerRegistrySuite) TestGetBlobs(c *gc.C) {
	// Use v2 for private repository.
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals,
				`https://jujuqa.azurecr.io/v2/jujud-operator/blobs/sha256:f0609d8a844f7271411c1a9c5d7a898fd9f9c5a4844e3bc7db6d725b54671ac1`,
			)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusUnauthorized,
				Body:       ioutil.NopCloser(nil),
				Header: http.Header{
					http.CanonicalHeaderKey("WWW-Authenticate"): []string{
						`Bearer realm="https://jujuqa.azurecr.io/oauth2/token",service="jujuqa.azurecr.io",scope="repository:jujud-operator:metadata_read"`,
					},
				},
			}, nil
		}),
		// Refresh OAuth Token
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
			func(req *http.Request) (*http.Response, error) {
				c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Basic " + s.authToken}})
				c.Assert(req.Method, gc.Equals, `GET`)
				c.Assert(req.URL.String(), gc.Equals, `https://jujuqa.azurecr.io/oauth2/token?scope=repository%3Ajujud-operator%3Ametadata_read&service=jujuqa.azurecr.io`)
				return &http.Response{
					Request:    req,
					StatusCode: http.StatusOK,
					Body:       ioutil.NopCloser(strings.NewReader(`{"access_token": "jwt-token"}`)),
				}, nil
			},
		),
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Bearer jwt-token"}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals,
				`https://jujuqa.azurecr.io/v2/jujud-operator/blobs/sha256:f0609d8a844f7271411c1a9c5d7a898fd9f9c5a4844e3bc7db6d725b54671ac1`,
			)
			resps := &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body: ioutil.NopCloser(strings.NewReader(`
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
