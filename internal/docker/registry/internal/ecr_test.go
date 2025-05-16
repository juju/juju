// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry"
	"github.com/juju/juju/internal/docker/registry/image"
	"github.com/juju/juju/internal/docker/registry/internal"
	internalmocks "github.com/juju/juju/internal/docker/registry/internal/mocks"
	"github.com/juju/juju/internal/docker/registry/mocks"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/tools"
)

type elasticContainerRegistrySuite struct {
	testhelpers.IsolationSuite

	mockRoundTripper *mocks.MockRoundTripper
	mockECRAPI       *internalmocks.MockECRInterface
	imageRepoDetails docker.ImageRepoDetails
	isPrivate        bool
}

func TestElasticContainerRegistrySuite(t *stdtesting.T) {
	tc.Run(t, &elasticContainerRegistrySuite{})
}
func (s *elasticContainerRegistrySuite) getRegistry(c *tc.C, ensureAsserts func()) (*internal.ElasticContainerRegistry, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	s.mockRoundTripper = mocks.NewMockRoundTripper(ctrl)
	s.mockECRAPI = internalmocks.NewMockECRInterface(ctrl)

	if s.imageRepoDetails.Empty() {
		s.imageRepoDetails = docker.ImageRepoDetails{
			Repository: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
			Region:     "ap-southeast-2",
		}
		if s.isPrivate {
			s.imageRepoDetails.BasicAuthConfig = docker.BasicAuthConfig{
				Username: "aws_access_key_id",
				Password: "aws_secret_access_key",
			}
		}
	}
	if ensureAsserts != nil {
		ensureAsserts()
	} else {
		if s.imageRepoDetails.IsPrivate() {
			s.mockECRAPI.EXPECT().GetAuthorizationToken(gomock.Any(), &ecr.GetAuthorizationTokenInput{}).Return(
				&ecr.GetAuthorizationTokenOutput{
					AuthorizationData: []types.AuthorizationData{
						{AuthorizationToken: aws.String(`xxxx===`)},
					},
				}, nil,
			).AnyTimes()
		}
	}

	reg, err := internal.NewElasticContainerRegistryForTest(
		s.imageRepoDetails, s.mockRoundTripper,
		func(context.Context, string, string, string) (internal.ECRInterface, error) {
			return s.mockECRAPI, nil
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	err = internal.InitProvider(reg)
	if !s.imageRepoDetails.IsPrivate() {
		c.Assert(err, tc.ErrorMatches, `empty credential for elastic container registry`)
		return nil, ctrl
	}
	c.Assert(err, tc.ErrorIsNil)
	client, ok := reg.(*internal.ElasticContainerRegistry)
	c.Assert(ok, tc.IsTrue)
	err = reg.Ping()
	c.Assert(err, tc.ErrorIsNil)
	return client, ctrl
}

func (s *elasticContainerRegistrySuite) TestInvalidImageRepoDetails(c *tc.C) {
	imageRepoDetails := docker.ImageRepoDetails{
		Repository:      "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		ServerAddress:   "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		BasicAuthConfig: docker.BasicAuthConfig{},
	}
	_, err := registry.New(imageRepoDetails)
	c.Check(err, tc.ErrorMatches, `empty credential for elastic container registry`)

	imageRepoDetails = docker.ImageRepoDetails{
		Repository:    "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		ServerAddress: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		Region:        "ap-southeast-2",
		BasicAuthConfig: docker.BasicAuthConfig{
			Username: "aws_access_key_id",
		},
	}
	_, err = registry.New(imageRepoDetails)
	c.Check(err, tc.ErrorMatches, `username and password are required for registry "66668888.dkr.ecr.eu-west-1.amazonaws.com"`)

	imageRepoDetails = docker.ImageRepoDetails{
		Repository:    "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		ServerAddress: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		Region:        "ap-southeast-2",
		BasicAuthConfig: docker.BasicAuthConfig{
			Password: "aws_secret_access_key",
		},
	}
	_, err = registry.New(imageRepoDetails)
	c.Check(err, tc.ErrorMatches, `username and password are required for registry "66668888.dkr.ecr.eu-west-1.amazonaws.com"`)

	imageRepoDetails = docker.ImageRepoDetails{
		Repository:    "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		ServerAddress: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		BasicAuthConfig: docker.BasicAuthConfig{
			Username: "aws_access_key_id",
			Password: "aws_secret_access_key",
		},
	}
	_, err = registry.New(imageRepoDetails)
	c.Check(err, tc.ErrorMatches, `region is required`)
}

func setImageRepoDetails(c *tc.C, reg registry.Registry, i docker.ImageRepoDetails) {
	registry, ok := reg.(*internal.ElasticContainerRegistry)
	c.Assert(ok, tc.IsTrue)
	registry.SetImageRepoDetails(i)
}

func (s *elasticContainerRegistrySuite) TestShouldRefreshAuthAuthTokenMissing(c *tc.C) {
	reg, ctrl := s.getRegistry(c, nil)
	defer ctrl.Finish()
	repoDetails := docker.ImageRepoDetails{
		Repository:    "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		ServerAddress: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		Region:        "ap-southeast-2",
		BasicAuthConfig: docker.BasicAuthConfig{
			Username: "aws_access_key_id",
			Password: "aws_secret_access_key",
		},
	}
	setImageRepoDetails(c, reg, repoDetails)
	shouldRefreshAuth, tick := reg.ShouldRefreshAuth()
	c.Assert(tick, tc.Equals, time.Duration(0))
	c.Assert(shouldRefreshAuth, tc.IsTrue)
}

func (s *elasticContainerRegistrySuite) TestShouldRefreshNoExpireTime(c *tc.C) {
	reg, ctrl := s.getRegistry(c, nil)
	defer ctrl.Finish()
	repoDetails := docker.ImageRepoDetails{
		Repository:    "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		ServerAddress: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		Region:        "ap-southeast-2",
		BasicAuthConfig: docker.BasicAuthConfig{
			Username: "aws_access_key_id",
			Password: "aws_secret_access_key",
		},
	}
	repoDetails.Auth = docker.NewToken(`xxx===`)
	setImageRepoDetails(c, reg, repoDetails)
	shouldRefreshAuth, tick := reg.ShouldRefreshAuth()
	c.Assert(tick, tc.Equals, time.Duration(0))
	c.Assert(shouldRefreshAuth, tc.IsTrue)
}

func (s *elasticContainerRegistrySuite) TestShouldRefreshTokenExpired(c *tc.C) {
	reg, ctrl := s.getRegistry(c, nil)
	defer ctrl.Finish()
	repoDetails := docker.ImageRepoDetails{
		Repository:    "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		ServerAddress: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		Region:        "ap-southeast-2",
		BasicAuthConfig: docker.BasicAuthConfig{
			Username: "aws_access_key_id",
			Password: "aws_secret_access_key",
		},
	}
	// expires in 5 mins.
	expiredTime := time.Now().Add(-1 * time.Second).Add(5 * time.Minute)
	repoDetails.Auth = &docker.Token{
		Value:     `xxx===`,
		ExpiresAt: &expiredTime,
	}
	setImageRepoDetails(c, reg, repoDetails)
	shouldRefreshAuth, tick := reg.ShouldRefreshAuth()
	c.Assert(tick, tc.Equals, time.Duration(0))
	c.Assert(shouldRefreshAuth, tc.IsTrue)

	// // already expired.
	expiredTime = time.Now().Add(-1 * time.Second)
	repoDetails.Auth = &docker.Token{
		Value:     `xxx===`,
		ExpiresAt: &expiredTime,
	}
	setImageRepoDetails(c, reg, repoDetails)
	shouldRefreshAuth, tick = reg.ShouldRefreshAuth()
	c.Assert(tick, tc.Equals, time.Duration(0))
	c.Assert(shouldRefreshAuth, tc.IsTrue)
}

func (s *elasticContainerRegistrySuite) TestShouldRefreshTokenNoNeedRefresh(c *tc.C) {
	expiredTime := time.Now().Add(3 * time.Minute).Add(5 * time.Minute)
	reg, ctrl := s.getRegistry(c, nil)
	defer ctrl.Finish()
	repoDetails := docker.ImageRepoDetails{
		Repository:    "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		ServerAddress: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		Region:        "ap-southeast-2",
		BasicAuthConfig: docker.BasicAuthConfig{
			Username: "aws_access_key_id",
			Password: "aws_secret_access_key",
		},
	}
	repoDetails.Auth = &docker.Token{
		Value:     `xxx===`,
		ExpiresAt: &expiredTime,
	}
	setImageRepoDetails(c, reg, repoDetails)
	shouldRefreshAuth, tick := reg.ShouldRefreshAuth()
	c.Assert(shouldRefreshAuth, tc.IsFalse)
	c.Assert(tick, tc.NotNil)
	c.Assert(tick.Round(time.Minute), tc.DeepEquals, 3*time.Minute)
}

func (s *elasticContainerRegistrySuite) TestPingPublicRepository(c *tc.C) {
	s.isPrivate = false
	_, ctrl := s.getRegistry(c, nil)
	ctrl.Finish()
}

func (s *elasticContainerRegistrySuite) TestPingPrivateRepository(c *tc.C) {
	s.isPrivate = true
	_, ctrl := s.getRegistry(c, nil)
	ctrl.Finish()
}

func (s *elasticContainerRegistrySuite) TestTags(c *tc.C) {
	// Use v2 for private repository.
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c, nil)
	defer ctrl.Finish()

	data := `
{"name":"jujuqa/jujud-operator","tags":["2.9.10.1","2.9.10.2","2.9.10"]}
`[1:]

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, tc.DeepEquals, http.Header{"Authorization": []string{"Basic xxxx==="}})
			c.Assert(req.Method, tc.Equals, `GET`)
			c.Assert(req.URL.String(), tc.Equals, `https://66668888.dkr.ecr.eu-west-1.amazonaws.com/v2/jujud-operator/tags/list`)
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

func (s *elasticContainerRegistrySuite) TestTagsErrorResponse(c *tc.C) {
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c, nil)
	defer ctrl.Finish()

	data := `
{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}
`[1:]

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, tc.DeepEquals, http.Header{"Authorization": []string{"Basic xxxx==="}})
			c.Assert(req.Method, tc.Equals, `GET`)
			c.Assert(req.URL.String(), tc.Equals, `https://66668888.dkr.ecr.eu-west-1.amazonaws.com/v2/jujud-operator/tags/list`)
			resps := &http.Response{
				Request:    req,
				StatusCode: http.StatusForbidden,
				Body:       io.NopCloser(strings.NewReader(data)),
			}
			return resps, nil
		}),
	)
	_, err := reg.Tags("jujud-operator")
	c.Assert(err, tc.ErrorMatches, `Get "https://66668888.dkr.ecr.eu-west-1.amazonaws.com/v2/jujud-operator/tags/list": non-successful response status=403`)
}

func (s *elasticContainerRegistrySuite) assertGetManifestsSchemaVersion1(c *tc.C, responseData, contentType string, result *internal.ManifestsResult) {
	// Use v2 for private repository.
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c, nil)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, tc.DeepEquals, http.Header{"Authorization": []string{"Basic xxxx==="}})
			c.Assert(req.Method, tc.Equals, `GET`)
			c.Assert(req.URL.String(), tc.Equals, `https://66668888.dkr.ecr.eu-west-1.amazonaws.com/v2/jujud-operator/manifests/2.9.10`)
			resps := &http.Response{
				Header: http.Header{
					http.CanonicalHeaderKey("Content-Type"): []string{contentType},
				},
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(responseData)),
			}
			return resps, nil
		}),
	)
	manifests, err := reg.GetManifests("jujud-operator", "2.9.10")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(manifests, tc.DeepEquals, result)
}

func (s *elasticContainerRegistrySuite) TestGetManifestsSchemaVersion1(c *tc.C) {
	s.assertGetManifestsSchemaVersion1(c,
		`
{ "schemaVersion": 1, "name": "jujuqa/jujud-operator", "tag": "2.9.13", "architecture": "ppc64le"}
`[1:],
		`application/vnd.docker.distribution.manifest.v1+prettyjws`,
		&internal.ManifestsResult{Architectures: []string{"ppc64el"}},
	)
}

func (s *elasticContainerRegistrySuite) TestGetManifestsSchemaVersion2(c *tc.C) {
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

func (s *elasticContainerRegistrySuite) TestGetManifestsSchemaVersion2List(c *tc.C) {
	s.assertGetManifestsSchemaVersion1(c,
		`
{
    "schemaVersion": 2,
    "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
    "manifests": [
        {"platform": {"architecture": "amd64"}},
        {"platform": {"architecture": "ppc64le"}}
    ]
}
`[1:],
		`application/vnd.docker.distribution.manifest.list.v2+prettyjws`,
		&internal.ManifestsResult{Architectures: []string{"amd64", "ppc64el"}},
	)
}

func (s *elasticContainerRegistrySuite) TestGetBlobs(c *tc.C) {
	// Use v2 for private repository.
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c, nil)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, tc.DeepEquals, http.Header{"Authorization": []string{"Basic xxxx==="}})
			c.Assert(req.Method, tc.Equals, `GET`)
			c.Assert(req.URL.String(), tc.Equals,
				`https://66668888.dkr.ecr.eu-west-1.amazonaws.com/v2/jujud-operator/blobs/sha256:f0609d8a844f7271411c1a9c5d7a898fd9f9c5a4844e3bc7db6d725b54671ac1`,
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(manifests, tc.DeepEquals, &internal.BlobsResponse{Architecture: "amd64"})
}
