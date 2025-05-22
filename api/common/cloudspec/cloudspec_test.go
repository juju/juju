// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudspec_test

import (
	"errors"
	stdtesting "testing"

	"github.com/juju/tc"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/common/cloudspec"
	"github.com/juju/juju/cloud"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func TestCloudSpecSuite(t *stdtesting.T) {
	tc.Run(t, &CloudSpecSuite{})
}

type CloudSpecSuite struct {
	testhelpers.IsolationSuite
}

func (s *CloudSpecSuite) TestNewCloudSpecAPI(c *tc.C) {
	api := cloudspec.NewCloudSpecAPI(nil, coretesting.ModelTag)
	c.Check(api, tc.NotNil)
}

func (s *CloudSpecSuite) TestCloudSpec(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testhelpers.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, tc.Equals, "CloudSpec")
		c.Assert(args, tc.DeepEquals, params.Entities{Entities: []params.Entity{
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
	cloudSpec, err := api.CloudSpec(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	credential := cloud.NewCredential(
		"auth-type",
		map[string]string{"k": "v"},
	)
	c.Assert(cloudSpec, tc.DeepEquals, environscloudspec.CloudSpec{
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
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testhelpers.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		return expect
	}
	api := cloudspec.NewCloudSpecAPI(&facadeCaller, coretesting.ModelTag)
	_, err := api.CloudSpec(c.Context())
	c.Assert(err, tc.Equals, expect)
}

func (s *CloudSpecSuite) TestCloudSpecResultCountMismatch(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testhelpers.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		return nil
	}
	api := cloudspec.NewCloudSpecAPI(&facadeCaller, coretesting.ModelTag)
	_, err := api.CloudSpec(c.Context())
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 0")
}

func (s *CloudSpecSuite) TestCloudSpecResultError(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testhelpers.Stub{}}
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
	_, err := api.CloudSpec(c.Context())
	c.Assert(err, tc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(err, tc.ErrorMatches, "API request failed: dang")
}

func (s *CloudSpecSuite) TestCloudSpecInvalidCloudSpec(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testhelpers.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		*(response.(*params.CloudSpecResults)) = params.CloudSpecResults{Results: []params.CloudSpecResult{{
			Result: &params.CloudSpec{
				Type: "",
			},
		}}}
		return nil
	}
	api := cloudspec.NewCloudSpecAPI(&facadeCaller, coretesting.ModelTag)
	_, err := api.CloudSpec(c.Context())
	c.Assert(err, tc.ErrorMatches, "validating CloudSpec: empty Type not valid")
}
