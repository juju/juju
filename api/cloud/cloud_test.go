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
				[]params.Entity{{"cloud-foo"}},
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

func (s *cloudSuite) TestCredentials(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Cloud")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Credentials")
			c.Assert(result, gc.FitsTypeOf, &params.CloudCredentialsResults{})
			c.Assert(a, jc.DeepEquals, params.UserClouds{[]params.UserCloud{{
				UserTag:  "user-bob@local",
				CloudTag: "cloud-foo",
			}}})
			*result.(*params.CloudCredentialsResults) = params.CloudCredentialsResults{
				Results: []params.CloudCredentialsResult{{
					Credentials: map[string]params.CloudCredential{
						"one": {
							AuthType: "empty",
						},
						"two": {
							AuthType: "userpass",
							Attributes: map[string]string{
								"username": "admin",
								"password": "adm1n",
							},
						},
					},
				}},
			}
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	result, err := client.Credentials(names.NewUserTag("bob@local"), names.NewCloudTag("foo"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, map[string]cloud.Credential{
		"one": cloud.NewEmptyCredential(),
		"two": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
			"username": "admin",
			"password": "adm1n",
		}),
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
			c.Assert(a, jc.DeepEquals, params.UsersCloudCredentials{[]params.UserCloudCredentials{{
				UserTag:  "user-bob@local",
				CloudTag: "cloud-foo",
				Credentials: map[string]params.CloudCredential{
					"one": {
						AuthType: "empty",
					},
					"two": {
						AuthType: "userpass",
						Attributes: map[string]string{
							"username": "admin",
							"password": "adm1n",
						},
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
	err := client.UpdateCredentials(names.NewUserTag("bob@local"), names.NewCloudTag("foo"), map[string]cloud.Credential{
		"one": cloud.NewEmptyCredential(),
		"two": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
			"username": "admin",
			"password": "adm1n",
		}),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}
