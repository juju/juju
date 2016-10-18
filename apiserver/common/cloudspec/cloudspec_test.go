// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudspec_test

import (
	"errors"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	coretesting "github.com/juju/juju/testing"
)

type CloudSpecSuite struct {
	testing.IsolationSuite
	testing.Stub
	result   environs.CloudSpec
	authFunc common.AuthFunc
	api      cloudspec.CloudSpecAPI
}

var _ = gc.Suite(&CloudSpecSuite{})

func (s *CloudSpecSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub.ResetCalls()

	s.authFunc = func(tag names.Tag) bool {
		s.AddCall("Auth", tag)
		return tag == coretesting.ModelTag
	}
	s.api = cloudspec.NewCloudSpec(func(tag names.ModelTag) (environs.CloudSpec, error) {
		s.AddCall("CloudSpec", tag)
		return s.result, s.NextErr()
	}, func() (common.AuthFunc, error) {
		s.AddCall("GetAuthFunc")
		return s.authFunc, s.NextErr()
	})

	credential := cloud.NewCredential(
		"auth-type",
		map[string]string{"k": "v"},
	)
	s.result = environs.CloudSpec{
		"type",
		"name",
		"region",
		"endpoint",
		"identity-endpoint",
		"storage-endpoint",
		&credential,
	}
}

func (s *CloudSpecSuite) TestCloudSpec(c *gc.C) {
	otherModelTag := names.NewModelTag(utils.MustNewUUID().String())
	machineTag := names.NewMachineTag("42")
	result, err := s.api.CloudSpec(params.Entities{Entities: []params.Entity{
		{coretesting.ModelTag.String()},
		{otherModelTag.String()},
		{machineTag.String()},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.CloudSpecResult{{
		Result: &params.CloudSpec{
			"type",
			"name",
			"region",
			"endpoint",
			"identity-endpoint",
			"storage-endpoint",
			&params.CloudCredential{
				AuthType:   "auth-type",
				Attributes: map[string]string{"k": "v"},
			},
		},
	}, {
		Error: &params.Error{
			Code:    params.CodeUnauthorized,
			Message: "permission denied",
		},
	}, {
		Error: &params.Error{
			Message: `"machine-42" is not a valid model tag`,
		},
	}})
	s.CheckCalls(c, []testing.StubCall{
		{"GetAuthFunc", nil},
		{"Auth", []interface{}{coretesting.ModelTag}},
		{"CloudSpec", []interface{}{coretesting.ModelTag}},
		{"Auth", []interface{}{otherModelTag}},
	})
}

func (s *CloudSpecSuite) TestCloudSpecNilCredential(c *gc.C) {
	s.result.Credential = nil
	result, err := s.api.CloudSpec(params.Entities{
		Entities: []params.Entity{{coretesting.ModelTag.String()}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.CloudSpecResult{{
		Result: &params.CloudSpec{
			"type",
			"name",
			"region",
			"endpoint",
			"identity-endpoint",
			"storage-endpoint",
			nil,
		},
	}})
}

func (s *CloudSpecSuite) TestCloudSpecGetAuthFuncError(c *gc.C) {
	expect := errors.New("bewm")
	s.SetErrors(expect)
	result, err := s.api.CloudSpec(params.Entities{
		Entities: []params.Entity{{coretesting.ModelTag.String()}},
	})
	c.Assert(err, gc.Equals, expect)
	c.Assert(result, jc.DeepEquals, params.CloudSpecResults{})
}

func (s *CloudSpecSuite) TestCloudSpecCloudSpecError(c *gc.C) {
	s.SetErrors(nil, errors.New("bewm"))
	result, err := s.api.CloudSpec(params.Entities{
		Entities: []params.Entity{{coretesting.ModelTag.String()}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.CloudSpecResults{Results: []params.CloudSpecResult{{
		Error: &params.Error{Message: "bewm"},
	}}})
}
