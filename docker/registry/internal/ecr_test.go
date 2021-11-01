// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	"context"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/docker"
	"github.com/juju/juju/docker/registry"
	"github.com/juju/juju/docker/registry/image"
	"github.com/juju/juju/docker/registry/internal"
	internalmocks "github.com/juju/juju/docker/registry/internal/mocks"
	"github.com/juju/juju/docker/registry/mocks"
	"github.com/juju/juju/tools"
)

type elasticContainerRegistrySuite struct {
	testing.IsolationSuite

	mockRoundTripper *mocks.MockRoundTripper
	mockECRAPI       *internalmocks.MockECRInterface
	imageRepoDetails docker.ImageRepoDetails
	isPrivate        bool
}

var _ = gc.Suite(&elasticContainerRegistrySuite{})

func (s *elasticContainerRegistrySuite) getRegistry(c *gc.C, ensureAsserts func()) (*internal.ElasticContainerRegistry, *gomock.Controller) {
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
			)
		}
	}

	reg := internal.NewElasticContainerRegistryForTest(
		s.imageRepoDetails, s.mockRoundTripper,
		func(context.Context, string, string, string) (internal.ECRInterface, error) {
			return s.mockECRAPI, nil
		},
	)
	err := internal.InitProvider(reg)
	if !s.imageRepoDetails.IsPrivate() {
		c.Assert(err, gc.ErrorMatches, `empty credential for elastic container registry`)
		return nil, ctrl
	}
	c.Assert(err, jc.ErrorIsNil)
	client, ok := reg.(*internal.ElasticContainerRegistry)
	c.Assert(ok, jc.IsTrue)
	err = reg.Ping()
	c.Assert(err, jc.ErrorIsNil)
	return client, ctrl
}

func (s *elasticContainerRegistrySuite) TestInvalidImageRepoDetails(c *gc.C) {
	imageRepoDetails := docker.ImageRepoDetails{
		Repository:      "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		ServerAddress:   "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		BasicAuthConfig: docker.BasicAuthConfig{},
	}
	_, err := registry.New(imageRepoDetails)
	c.Check(err, gc.ErrorMatches, `empty credential for elastic container registry`)

	imageRepoDetails = docker.ImageRepoDetails{
		Repository:    "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		ServerAddress: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		Region:        "ap-southeast-2",
		BasicAuthConfig: docker.BasicAuthConfig{
			Username: "aws_access_key_id",
		},
	}
	_, err = registry.New(imageRepoDetails)
	c.Check(err, gc.ErrorMatches, `username and password are required for registry "66668888.dkr.ecr.eu-west-1.amazonaws.com"`)

	imageRepoDetails = docker.ImageRepoDetails{
		Repository:    "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		ServerAddress: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		Region:        "ap-southeast-2",
		BasicAuthConfig: docker.BasicAuthConfig{
			Password: "aws_secret_access_key",
		},
	}
	_, err = registry.New(imageRepoDetails)
	c.Check(err, gc.ErrorMatches, `username and password are required for registry "66668888.dkr.ecr.eu-west-1.amazonaws.com"`)

	imageRepoDetails = docker.ImageRepoDetails{
		Repository:    "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		ServerAddress: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		BasicAuthConfig: docker.BasicAuthConfig{
			Username: "aws_access_key_id",
			Password: "aws_secret_access_key",
		},
	}
	_, err = registry.New(imageRepoDetails)
	c.Check(err, gc.ErrorMatches, `region is required`)
}

func setImageRepoDetails(c *gc.C, reg registry.Registry, i docker.ImageRepoDetails) {
	registry, ok := reg.(*internal.ElasticContainerRegistry)
	c.Assert(ok, jc.IsTrue)
	registry.SetImageRepoDetails(i)
}

func (s *elasticContainerRegistrySuite) TestShouldRefreshAuthAuthTokenMissing(c *gc.C) {
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
	c.Assert(tick, gc.IsNil)
	c.Assert(shouldRefreshAuth, jc.IsTrue)
}

func (s *elasticContainerRegistrySuite) TestShouldRefreshNoExpireTime(c *gc.C) {
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
	c.Assert(tick, gc.IsNil)
	c.Assert(shouldRefreshAuth, jc.IsTrue)
}

func (s *elasticContainerRegistrySuite) TestShouldRefreshTokenExpired(c *gc.C) {
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
	c.Assert(tick, gc.IsNil)
	c.Assert(shouldRefreshAuth, jc.IsTrue)

	// // already expired.
	expiredTime = time.Now().Add(-1 * time.Second)
	repoDetails.Auth = &docker.Token{
		Value:     `xxx===`,
		ExpiresAt: &expiredTime,
	}
	setImageRepoDetails(c, reg, repoDetails)
	shouldRefreshAuth, tick = reg.ShouldRefreshAuth()
	c.Assert(tick, gc.IsNil)
	c.Assert(shouldRefreshAuth, jc.IsTrue)
}

func (s *elasticContainerRegistrySuite) TestShouldRefreshTokenNoNeedRefresh(c *gc.C) {
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
	c.Assert(shouldRefreshAuth, jc.IsFalse)
	c.Assert(tick, gc.NotNil)
	c.Assert(tick.Round(time.Minute), gc.DeepEquals, 3*time.Minute)
}

func (s *elasticContainerRegistrySuite) TestPingPublicRepository(c *gc.C) {
	s.isPrivate = false
	_, ctrl := s.getRegistry(c, nil)
	ctrl.Finish()
}

func (s *elasticContainerRegistrySuite) TestPingPrivateRepository(c *gc.C) {
	s.isPrivate = true
	_, ctrl := s.getRegistry(c, nil)
	ctrl.Finish()
}

func (s *elasticContainerRegistrySuite) TestTags(c *gc.C) {
	// Use v2 for private repository.
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c, nil)
	defer ctrl.Finish()

	data := `
{"name":"jujuqa/jujud-operator","tags":["2.9.10.1","2.9.10.2","2.9.10"]}
`[1:]

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Basic xxxx==="}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://66668888.dkr.ecr.eu-west-1.amazonaws.com/v2/jujud-operator/tags/list`)
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

func (s *elasticContainerRegistrySuite) TestTagsErrorResponse(c *gc.C) {
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c, nil)
	defer ctrl.Finish()

	data := `
{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}
`[1:]

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Basic xxxx==="}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://66668888.dkr.ecr.eu-west-1.amazonaws.com/v2/jujud-operator/tags/list`)
			resps := &http.Response{
				Request:    req,
				StatusCode: http.StatusForbidden,
				Body:       ioutil.NopCloser(strings.NewReader(data)),
			}
			return resps, nil
		}),
	)
	_, err := reg.Tags("jujud-operator")
	c.Assert(err, gc.ErrorMatches, `Get "https://66668888.dkr.ecr.eu-west-1.amazonaws.com/v2/jujud-operator/tags/list": non-successful response status=403`)
}

func (s *elasticContainerRegistrySuite) assertGetManifestsSchemaVersion1(c *gc.C, responseData, contentType string, result *internal.ManifestsResult) {
	// Use v2 for private repository.
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c, nil)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Basic xxxx==="}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals, `https://66668888.dkr.ecr.eu-west-1.amazonaws.com/v2/jujud-operator/manifests/2.9.10`)
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

func (s *elasticContainerRegistrySuite) TestGetManifestsSchemaVersion1(c *gc.C) {
	s.assertGetManifestsSchemaVersion1(c,
		`
{ "schemaVersion": 1, "name": "jujuqa/jujud-operator", "tag": "2.9.13", "architecture": "amd64"}
`[1:],
		`application/vnd.docker.distribution.manifest.v1+prettyjws`,
		&internal.ManifestsResult{Architecture: "amd64"},
	)
}

func (s *elasticContainerRegistrySuite) TestGetManifestsSchemaVersion2(c *gc.C) {
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

func (s *elasticContainerRegistrySuite) TestGetBlobs(c *gc.C) {
	// Use v2 for private repository.
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c, nil)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			c.Assert(req.Header, jc.DeepEquals, http.Header{"Authorization": []string{"Basic xxxx==="}})
			c.Assert(req.Method, gc.Equals, `GET`)
			c.Assert(req.URL.String(), gc.Equals,
				`https://66668888.dkr.ecr.eu-west-1.amazonaws.com/v2/jujud-operator/blobs/sha256:f0609d8a844f7271411c1a9c5d7a898fd9f9c5a4844e3bc7db6d725b54671ac1`,
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
