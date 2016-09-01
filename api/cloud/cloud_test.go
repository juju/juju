// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	basetesting "github.com/juju/juju/api/base/testing"
	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
)

type cloudSuite struct {
	gitjujutesting.IsolationSuite
}

var _ = gc.Suite(&cloudSuite{})

func (s *cloudSuite) TestCloud(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Cloud")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Cloud")
			c.Check(a, jc.DeepEquals, params.Entities{
				Entities: []params.Entity{{Tag: "cloud-foo"}},
			})
			c.Assert(result, gc.FitsTypeOf, &params.CloudResults{})
			results := result.(*params.CloudResults)
			results.Results = append(results.Results, params.CloudResult{
				Cloud: &params.Cloud{
					Type:      "dummy",
					AuthTypes: []string{"empty", "userpass"},
					Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
				},
			})
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	result, err := client.Cloud(names.NewCloudTag("foo"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, cloud.Cloud{
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
		Regions:   []cloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	})
}

func (s *cloudSuite) TestClouds(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result_ interface{},
		) error {
			c.Check(objType, gc.Equals, "Cloud")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Clouds")
			c.Check(a, gc.IsNil)
			c.Assert(result_, gc.FitsTypeOf, &params.CloudsResult{})
			result := result_.(*params.CloudsResult)
			result.Clouds = map[string]params.Cloud{
				"cloud-foo": {
					Type: "bar",
				},
				"cloud-baz": {
					Type:      "qux",
					AuthTypes: []string{"empty", "userpass"},
					Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
				},
			}
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	clouds, err := client.Clouds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, jc.DeepEquals, map[names.CloudTag]cloud.Cloud{
		names.NewCloudTag("foo"): {
			Type: "bar",
		},
		names.NewCloudTag("baz"): {
			Type:      "qux",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
			Regions:   []cloud.Region{{Name: "nether", Endpoint: "endpoint"}},
		},
	})
}

func (s *cloudSuite) TestDefaultCloud(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Cloud")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "DefaultCloud")
			c.Assert(result, gc.FitsTypeOf, &params.StringResult{})
			results := result.(*params.StringResult)
			results.Result = "cloud-foo"
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	result, err := client.DefaultCloud()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, names.NewCloudTag("foo"))
}

func (s *cloudSuite) TestUserCredentials(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Cloud")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "UserCredentials")
			c.Assert(result, gc.FitsTypeOf, &params.StringsResults{})
			c.Assert(a, jc.DeepEquals, params.UserClouds{UserClouds: []params.UserCloud{{
				UserTag:  "user-bob@local",
				CloudTag: "cloud-foo",
			}}})
			*result.(*params.StringsResults) = params.StringsResults{
				Results: []params.StringsResult{{
					Result: []string{
						"cloudcred-foo_bob@local_one",
						"cloudcred-foo_bob@local_two",
					},
				}},
			}
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	result, err := client.UserCredentials(names.NewUserTag("bob@local"), names.NewCloudTag("foo"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.SameContents, []names.CloudCredentialTag{
		names.NewCloudCredentialTag("foo/bob@local/one"),
		names.NewCloudCredentialTag("foo/bob@local/two"),
	})
}

func (s *cloudSuite) TestUpdateCredentials(c *gc.C) {
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Cloud")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "UpdateCredentials")
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			c.Assert(a, jc.DeepEquals, params.UpdateCloudCredentials{Credentials: []params.UpdateCloudCredential{{
				Tag: "cloudcred-foo_bob@local_bar",
				Credential: params.CloudCredential{
					AuthType: "userpass",
					Attributes: map[string]string{
						"username": "admin",
						"password": "adm1n",
					},
				},
			}}})
			*result.(*params.ErrorResults) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
			called = true
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	tag := names.NewCloudCredentialTag("foo/bob@local/bar")
	err := client.UpdateCredential(tag, cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username": "admin",
		"password": "adm1n",
	}))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *cloudSuite) TestRevokeCredential(c *gc.C) {
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Cloud")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "RevokeCredentials")
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			c.Assert(a, jc.DeepEquals, params.Entities{Entities: []params.Entity{{
				Tag: "cloudcred-foo_bob@local_bar",
			}}})
			*result.(*params.ErrorResults) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
			called = true
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	tag := names.NewCloudCredentialTag("foo/bob@local/bar")
	err := client.RevokeCredential(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *cloudSuite) TestCredentials(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Cloud")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Credential")
			c.Assert(result, gc.FitsTypeOf, &params.CloudCredentialResults{})
			c.Assert(a, jc.DeepEquals, params.Entities{Entities: []params.Entity{{
				Tag: "cloudcred-foo_bob@local_bar",
			}}})
			*result.(*params.CloudCredentialResults) = params.CloudCredentialResults{
				Results: []params.CloudCredentialResult{
					{
						Result: &params.CloudCredential{
							AuthType:   "userpass",
							Attributes: map[string]string{"username": "fred"},
							Redacted:   []string{"password"},
						},
					}, {
						Result: &params.CloudCredential{
							AuthType:   "userpass",
							Attributes: map[string]string{"username": "mary"},
							Redacted:   []string{"password"},
						},
					},
				},
			}
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	tag := names.NewCloudCredentialTag("foo/bob@local/bar")
	result, err := client.Credentials(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []params.CloudCredentialResult{
		{
			Result: &params.CloudCredential{
				AuthType:   "userpass",
				Attributes: map[string]string{"username": "fred"},
				Redacted:   []string{"password"},
			},
		}, {
			Result: &params.CloudCredential{
				AuthType:   "userpass",
				Attributes: map[string]string{"username": "mary"},
				Redacted:   []string{"password"},
			},
		},
	})
}
