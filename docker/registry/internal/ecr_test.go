// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	"context"
	"io/ioutil"
	"net/http"
	"strings"

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

func (s *elasticContainerRegistrySuite) getRegistry(c *gc.C) (registry.Registry, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	s.mockRoundTripper = mocks.NewMockRoundTripper(ctrl)
	s.PatchValue(&registry.DefaultTransport, s.mockRoundTripper)

	s.mockECRAPI = internalmocks.NewMockECRInterface(ctrl)
	s.PatchValue(&internal.GetECRClient, func(context.Context, aws.HTTPClient, string, string, string) (internal.ECRInterface, error) {
		return s.mockECRAPI, nil
	})

	s.imageRepoDetails = docker.ImageRepoDetails{
		Repository:    "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		ServerAddress: "66668888.dkr.ecr.eu-west-1.amazonaws.com",
		Region:        "ap-southeast-2",
	}
	if s.isPrivate {
		s.imageRepoDetails.BasicAuthConfig = docker.BasicAuthConfig{
			Username: "aws_access_key_id",
			Password: "aws_secret_access_key",
		}
		s.mockECRAPI.EXPECT().GetAuthorizationToken(gomock.Any(), &ecr.GetAuthorizationTokenInput{}).Return(
			&ecr.GetAuthorizationTokenOutput{
				AuthorizationData: []types.AuthorizationData{
					{AuthorizationToken: aws.String(`xxxx===`)},
				},
			}, nil,
		)
	}

	reg, err := registry.New(s.imageRepoDetails)
	if !s.isPrivate {
		c.Assert(err, gc.ErrorMatches, `empty credential for elastic container registry`)
		return nil, ctrl
	}
	c.Assert(err, jc.ErrorIsNil)
	_, ok := reg.(*internal.ElasticContainerRegistry)
	c.Assert(ok, jc.IsTrue)
	err = reg.Ping()
	c.Assert(err, jc.ErrorIsNil)
	return reg, ctrl
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

func (s *elasticContainerRegistrySuite) TestPingPublicRepository(c *gc.C) {
	s.isPrivate = false
	_, ctrl := s.getRegistry(c)
	ctrl.Finish()
}

func (s *elasticContainerRegistrySuite) TestPingPrivateRepository(c *gc.C) {
	s.isPrivate = true
	_, ctrl := s.getRegistry(c)
	ctrl.Finish()
}

func (s *elasticContainerRegistrySuite) TestTags(c *gc.C) {
	// Use v2 for private repository.
	s.isPrivate = true
	reg, ctrl := s.getRegistry(c)
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
	reg, ctrl := s.getRegistry(c)
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
