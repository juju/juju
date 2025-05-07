// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudspec_test

import (
	"context"
	"errors"

	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/common/cloudspec"
	"github.com/juju/juju/cloud"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&CloudSpecSuite{})

type CloudSpecSuite struct {
	testing.IsolationSuite
}

func (s *CloudSpecSuite) TestNewCloudSpecAPI(c *tc.C) {
	api := cloudspec.NewCloudSpecAPI(nil, coretesting.ModelTag)
	c.Check(api, tc.NotNil)
}

func (s *CloudSpecSuite) TestCloudSpec(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, tc.Equals, "CloudSpec")
		c.Assert(args, jc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: coretesting.ModelTag.String()},
		}})
		*(response.(*params.CloudSpecResults)) = params.CloudSpecResults{
			Results: []params.CloudSpecResult{{
				Result: &params.CloudSpec{
					Type:             "type",
					Name:             "name",
					Region:           "region",
					Endpoint:         "endpoint",
					IdentityEndpoint: "identity-endpoint",
					StorageEndpoint:  "storage-endpoint",
					Credential: &params.CloudCredential{
						AuthType:   "auth-type",
						Attributes: map[string]string{"k": "v"},
					},
					CACertificates: []string{coretesting.CACert},
					SkipTLSVerify:  true,
				},
			}},
		}
		return nil
	}
	api := cloudspec.NewCloudSpecAPI(&facadeCaller, coretesting.ModelTag)
	cloudSpec, err := api.CloudSpec(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	credential := cloud.NewCredential(
		"auth-type",
		map[string]string{"k": "v"},
	)
	c.Assert(cloudSpec, jc.DeepEquals, environscloudspec.CloudSpec{
		Type:             "type",
		Name:             "name",
		Region:           "region",
		Endpoint:         "endpoint",
		IdentityEndpoint: "identity-endpoint",
		StorageEndpoint:  "storage-endpoint",
		Credential:       &credential,
		CACertificates:   []string{coretesting.CACert},
		SkipTLSVerify:    true,
	})
}

func (s *CloudSpecSuite) TestCloudSpecOverallError(c *tc.C) {
	expect := errors.New("bewm")
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		return expect
	}
	api := cloudspec.NewCloudSpecAPI(&facadeCaller, coretesting.ModelTag)
	_, err := api.CloudSpec(context.Background())
	c.Assert(err, tc.Equals, expect)
}

func (s *CloudSpecSuite) TestCloudSpecResultCountMismatch(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		return nil
	}
	api := cloudspec.NewCloudSpecAPI(&facadeCaller, coretesting.ModelTag)
	_, err := api.CloudSpec(context.Background())
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 0")
}

func (s *CloudSpecSuite) TestCloudSpecResultError(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		*(response.(*params.CloudSpecResults)) = params.CloudSpecResults{
			Results: []params.CloudSpecResult{{
				Error: &params.Error{
					Code:    params.CodeUnauthorized,
					Message: "dang",
				},
			}},
		}
		return nil
	}
	api := cloudspec.NewCloudSpecAPI(&facadeCaller, coretesting.ModelTag)
	_, err := api.CloudSpec(context.Background())
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(err, tc.ErrorMatches, "API request failed: dang")
}

func (s *CloudSpecSuite) TestCloudSpecInvalidCloudSpec(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		*(response.(*params.CloudSpecResults)) = params.CloudSpecResults{Results: []params.CloudSpecResult{{
			Result: &params.CloudSpec{
				Type: "",
			},
		}}}
		return nil
	}
	api := cloudspec.NewCloudSpecAPI(&facadeCaller, coretesting.ModelTag)
	_, err := api.CloudSpec(context.Background())
	c.Assert(err, tc.ErrorMatches, "validating CloudSpec: empty Type not valid")
}
