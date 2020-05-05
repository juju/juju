// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	coretesting "github.com/juju/juju/testing"
)

type cloudSuite struct {
	gitjujutesting.IsolationSuite

	called bool
}

var _ = gc.Suite(&cloudSuite{})

func (s *cloudSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.called = false
}

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
			s.called = true
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	result, err := client.Cloud(names.NewCloudTag("foo"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, cloud.Cloud{
		Name:      "foo",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
		Regions:   []cloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	})
	c.Assert(s.called, jc.IsTrue)
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
			s.called = true
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	clouds, err := client.Clouds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, jc.DeepEquals, map[names.CloudTag]cloud.Cloud{
		names.NewCloudTag("foo"): {
			Name: "foo",
			Type: "bar",
		},
		names.NewCloudTag("baz"): {
			Name:      "baz",
			Type:      "qux",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
			Regions:   []cloud.Region{{Name: "nether", Endpoint: "endpoint"}},
		},
	})
	c.Assert(s.called, jc.IsTrue)
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
				UserTag:  "user-bob",
				CloudTag: "cloud-foo",
			}}})
			*result.(*params.StringsResults) = params.StringsResults{
				Results: []params.StringsResult{{
					Result: []string{
						"cloudcred-foo_bob_one",
						"cloudcred-foo_bob_two",
					},
				}},
			}
			s.called = true
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	result, err := client.UserCredentials(names.NewUserTag("bob"), names.NewCloudTag("foo"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.SameContents, []names.CloudCredentialTag{
		names.NewCloudCredentialTag("foo/bob/one"),
		names.NewCloudCredentialTag("foo/bob/two"),
	})
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCredentialV2(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(objType, gc.Equals, "Cloud")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "UpdateCredentials")
				c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
				c.Assert(a, jc.DeepEquals, params.UpdateCredentialArgs{
					Credentials: []params.TaggedCredential{{
						Tag: "cloudcred-foo_bob_bar",
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
				s.called = true
				return nil
			},
		),
		BestVersion: 2,
	}
	client := cloudapi.NewClient(apiCaller)
	result, err := client.UpdateCredentialsCheckModels(testCredentialTag, testCredential)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.IsNil)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCredential(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(objType, gc.Equals, "Cloud")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "UpdateCredentialsCheckModels")
				c.Assert(result, gc.FitsTypeOf, &params.UpdateCredentialResults{})
				c.Assert(a, jc.DeepEquals, params.UpdateCredentialArgs{
					Credentials: []params.TaggedCredential{{
						Tag: "cloudcred-foo_bob_bar",
						Credential: params.CloudCredential{
							AuthType: "userpass",
							Attributes: map[string]string{
								"username": "admin",
								"password": "adm1n",
							},
						},
					}}})
				*result.(*params.UpdateCredentialResults) = params.UpdateCredentialResults{
					Results: []params.UpdateCredentialResult{{}},
				}
				s.called = true
				return nil
			},
		),
		BestVersion: 3,
	}
	client := cloudapi.NewClient(apiCaller)
	result, err := client.UpdateCredentialsCheckModels(testCredentialTag, testCredential)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.IsNil)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCredentialErrorV2(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(request, gc.Equals, "UpdateCredentials")
				*result.(*params.ErrorResults) = params.ErrorResults{
					Results: []params.ErrorResult{{common.ServerError(errors.New("validation failure"))}},
				}
				s.called = true
				return nil
			},
		),
		BestVersion: 2,
	}
	client := cloudapi.NewClient(apiCaller)
	errs, err := client.UpdateCredentialsCheckModels(testCredentialTag, testCredential)
	c.Assert(err, gc.ErrorMatches, "validation failure")
	c.Assert(errs, gc.IsNil)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCredentialError(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(request, gc.Equals, "UpdateCredentialsCheckModels")
				*result.(*params.UpdateCredentialResults) = params.UpdateCredentialResults{
					Results: []params.UpdateCredentialResult{
						{CredentialTag: "cloudcred-foo_bob_bar",
							Error: common.ServerError(errors.New("validation failure")),
						},
					},
				}
				s.called = true
				return nil
			},
		),
		BestVersion: 3,
	}
	client := cloudapi.NewClient(apiCaller)
	errs, err := client.UpdateCredentialsCheckModels(testCredentialTag, testCredential)
	c.Assert(err, gc.ErrorMatches, "validation failure")
	c.Assert(errs, gc.IsNil)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCredentialCallErrorV2(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(request, gc.Equals, "UpdateCredentials")
				s.called = true
				return errors.New("scary but true")
			},
		),
		BestVersion: 2,
	}
	client := cloudapi.NewClient(apiCaller)
	result, err := client.UpdateCredentialsCheckModels(testCredentialTag, testCredential)
	c.Assert(err, gc.ErrorMatches, "scary but true")
	c.Assert(result, gc.IsNil)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCredentialCallError(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(request, gc.Equals, "UpdateCredentialsCheckModels")
				s.called = true
				return errors.New("scary but true")
			},
		),
		BestVersion: 3,
	}
	client := cloudapi.NewClient(apiCaller)
	result, err := client.UpdateCredentialsCheckModels(testCredentialTag, testCredential)
	c.Assert(err, gc.ErrorMatches, "scary but true")
	c.Assert(result, gc.IsNil)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCredentialManyResultsV2(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(request, gc.Equals, "UpdateCredentials")
				*result.(*params.ErrorResults) = params.ErrorResults{
					Results: []params.ErrorResult{
						{},
						{},
					},
				}
				s.called = true
				return nil
			},
		),
		BestVersion: 2,
	}
	client := cloudapi.NewClient(apiCaller)
	result, err := client.UpdateCredentialsCheckModels(testCredentialTag, testCredential)
	c.Assert(err, gc.ErrorMatches, `expected 1 result got 2 when updating credentials`)
	c.Assert(result, gc.IsNil)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCredentialManyResults(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(request, gc.Equals, "UpdateCredentialsCheckModels")
				*result.(*params.UpdateCredentialResults) = params.UpdateCredentialResults{
					Results: []params.UpdateCredentialResult{
						{},
						{},
					}}
				s.called = true
				return nil
			},
		),
		BestVersion: 3,
	}
	client := cloudapi.NewClient(apiCaller)
	result, err := client.UpdateCredentialsCheckModels(testCredentialTag, testCredential)
	c.Assert(err, gc.ErrorMatches, `expected 1 result got 2 when updating credentials`)
	c.Assert(result, gc.IsNil)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCredentialModelErrors(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(request, gc.Equals, "UpdateCredentialsCheckModels")
				*result.(*params.UpdateCredentialResults) = params.UpdateCredentialResults{
					Results: []params.UpdateCredentialResult{
						{
							CredentialTag: testCredentialTag.String(),
							Models: []params.UpdateCredentialModelResult{
								{
									ModelUUID: coretesting.ModelTag.Id(),
									ModelName: "test-model",
									Errors: []params.ErrorResult{
										{common.ServerError(errors.New("validation failure one"))},
										{common.ServerError(errors.New("validation failure two"))},
									},
								},
							},
						},
					}}
				s.called = true
				return nil
			},
		),
		BestVersion: 3,
	}
	client := cloudapi.NewClient(apiCaller)
	errs, err := client.UpdateCredentialsCheckModels(testCredentialTag, testCredential)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.DeepEquals, []params.UpdateCredentialModelResult{
		{
			ModelUUID: "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			ModelName: "test-model",
			Errors: []params.ErrorResult{
				{Error: &params.Error{Message: "validation failure one", Code: ""}},
				{Error: &params.Error{Message: "validation failure two", Code: ""}},
			},
		},
	})
	c.Assert(s.called, jc.IsTrue)
}

var (
	testCredentialTag = names.NewCloudCredentialTag("foo/bob/bar")
	testCredential    = cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username": "admin",
		"password": "adm1n",
	})
)

func (s *cloudSuite) TestRevokeCredential(c *gc.C) {
	apiCallerF := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "Cloud")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "RevokeCredentialsCheckModels")
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			c.Assert(a, jc.DeepEquals, params.RevokeCredentialArgs{
				Credentials: []params.RevokeCredentialArg{
					{Tag: "cloudcred-foo_bob_bar", Force: true},
				},
			})
			*result.(*params.ErrorResults) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
			s.called = true
			return nil
		},
	)
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: apiCallerF,
		BestVersion:   3,
	}

	client := cloudapi.NewClient(apiCaller)
	tag := names.NewCloudCredentialTag("foo/bob/bar")
	err := client.RevokeCredential(tag, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestRevokeCredentialV2(c *gc.C) {
	apiCallerF := basetesting.APICallerFunc(
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
				Tag: "cloudcred-foo_bob_bar",
			}}})
			*result.(*params.ErrorResults) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
			s.called = true
			return nil
		},
	)
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: apiCallerF,
		BestVersion:   2,
	}

	client := cloudapi.NewClient(apiCaller)
	tag := names.NewCloudCredentialTag("foo/bob/bar")
	err := client.RevokeCredential(tag, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.called, jc.IsTrue)
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
				Tag: "cloudcred-foo_bob_bar",
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
			s.called = true
			return nil
		},
	)

	client := cloudapi.NewClient(apiCaller)
	tag := names.NewCloudCredentialTag("foo/bob/bar")
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
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) createVersionedAddCloudCall(c *gc.C, v int, expectedArg params.AddCloudArgs) basetesting.BestVersionCaller {
	return basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(objType, gc.Equals, "Cloud")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "AddCloud")
				c.Check(a, jc.DeepEquals, expectedArg)
				s.called = true
				return nil
			},
		),
		BestVersion: v,
	}
}

var testCloud = cloud.Cloud{
	Name:      "foo",
	Type:      "dummy",
	AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
	Regions:   []cloud.Region{{Name: "nether", Endpoint: "endpoint"}},
}

func (s *cloudSuite) TestAddCloudNotInV1API(c *gc.C) {
	apiCaller := s.createVersionedAddCloudCall(c, 1, params.AddCloudArgs{})
	client := cloudapi.NewClient(apiCaller)
	err := client.AddCloud(testCloud, false)
	c.Assert(err, gc.ErrorMatches, "AddCloud\\(\\).* not implemented")
	c.Assert(s.called, jc.IsFalse)
}

func (s *cloudSuite) TestAddCloudV2API(c *gc.C) {
	apiCaller := s.createVersionedAddCloudCall(c, 2, params.AddCloudArgs{
		Name:  "foo",
		Cloud: common.CloudToParams(testCloud),
	})
	client := cloudapi.NewClient(apiCaller)
	err := client.AddCloud(testCloud, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestAddCloudForceNotInVPriorTo6(c *gc.C) {
	apiCaller := s.createVersionedAddCloudCall(c, 5, params.AddCloudArgs{})
	client := cloudapi.NewClient(apiCaller)
	err := client.AddCloud(testCloud, true)
	c.Assert(err, gc.ErrorMatches, "AddCloud\\(\\).* not implemented")
	c.Assert(s.called, jc.IsFalse)
}

func (s *cloudSuite) TestAddCloudForceFromV6(c *gc.C) {
	force := true
	apiCaller := s.createVersionedAddCloudCall(c, 6, params.AddCloudArgs{
		Name:  "foo",
		Cloud: common.CloudToParams(testCloud),
		Force: &force,
	})
	client := cloudapi.NewClient(apiCaller)
	err := client.AddCloud(testCloud, force)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCloud(c *gc.C) {

	updatedCloud := cloud.Cloud{
		Name:      "foo",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
		Regions:   []cloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				s.called = true
				c.Check(objType, gc.Equals, "Cloud")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "UpdateCloud")
				c.Assert(a, jc.DeepEquals, params.UpdateCloudArgs{Clouds: []params.AddCloudArgs{{
					Name:  "foo",
					Cloud: common.CloudToParams(updatedCloud),
				}}})
				c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
				*result.(*params.ErrorResults) = params.ErrorResults{
					Results: []params.ErrorResult{{}},
				}
				return nil
			},
		),
		BestVersion: 4,
	}

	client := cloudapi.NewClient(apiCaller)
	err := client.UpdateCloud(updatedCloud)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestAddCredentialNotInV1API(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				s.called = true
				return nil
			},
		),
		BestVersion: 1,
	}
	client := cloudapi.NewClient(apiCaller)
	err := client.AddCredential("cloudcred-acloud-user-credname",
		cloud.NewCredential(cloud.UserPassAuthType, map[string]string{}))

	c.Assert(s.called, jc.IsFalse)
	c.Assert(err, gc.ErrorMatches, "AddCredential\\(\\).* not implemented")
}

func (s *cloudSuite) TestAddCredentialV2API(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				s.called = true
				c.Check(objType, gc.Equals, "Cloud")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "AddCredentials")
				c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
				*result.(*params.ErrorResults) = params.ErrorResults{
					Results: []params.ErrorResult{{}},
				}

				return nil
			},
		),
		BestVersion: 2,
	}

	client := cloudapi.NewClient(apiCaller)
	err := client.AddCredential("cloudcred-acloud-user-credname",
		cloud.NewCredential(cloud.UserPassAuthType,
			map[string]string{}))

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestCredentialContentsArgumentCheck(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{BestVersion: 2}
	client := cloudapi.NewClient(apiCaller)

	// Check supplying cloud name without credential name is invalid.
	result, err := client.CredentialContents("cloud", "", true)
	c.Assert(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "credential name must be supplied")

	// Check supplying credential name without cloud name is invalid.
	result, err = client.CredentialContents("", "credential", true)
	c.Assert(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "cloud name must be supplied")
}

func (s *cloudSuite) TestCredentialContentsAll(c *gc.C) {
	expectedResults := []params.CredentialContentResult{
		{
			Result: &params.ControllerCredentialInfo{
				Content: params.CredentialContent{
					Cloud:    "cloud-name",
					Name:     "credential-name",
					AuthType: "userpass",
					Attributes: map[string]string{
						"username": "fred",
						"password": "sekret"},
				},
				Models: []params.ModelAccess{
					{Model: "abcmodel", Access: "admin"},
					{Model: "xyzmodel", Access: "read"},
					{Model: "no-access-model"}, // no access
				},
			},
		}, {
			Error: common.ServerError(errors.New("boom")),
		},
	}
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(objType, gc.Equals, "Cloud")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "CredentialContents")
				c.Assert(result, gc.FitsTypeOf, &params.CredentialContentResults{})
				c.Assert(a, jc.DeepEquals, params.CloudCredentialArgs{
					IncludeSecrets: true,
				})
				*result.(*params.CredentialContentResults) = params.CredentialContentResults{
					Results: expectedResults,
				}
				s.called = true
				return nil
			},
		),
		BestVersion: 2,
	}

	client := cloudapi.NewClient(apiCaller)
	results, err := client.CredentialContents("", "", true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expectedResults)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestCredentialContentsOne(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(objType, gc.Equals, "Cloud")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "CredentialContents")
				c.Assert(result, gc.FitsTypeOf, &params.CredentialContentResults{})
				c.Assert(a, jc.DeepEquals, params.CloudCredentialArgs{
					IncludeSecrets: true,
					Credentials: []params.CloudCredentialArg{
						{CloudName: "cloud-name", CredentialName: "credential-name"},
					},
				})
				*result.(*params.CredentialContentResults) = params.CredentialContentResults{
					Results: []params.CredentialContentResult{
						{},
					},
				}
				s.called = true
				return nil
			},
		),
		BestVersion: 2,
	}

	client := cloudapi.NewClient(apiCaller)
	results, err := client.CredentialContents("cloud-name", "credential-name", true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestCredentialContentsGotMoreThanBargainedFor(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				*result.(*params.CredentialContentResults) = params.CredentialContentResults{
					Results: []params.CredentialContentResult{
						{},
						{},
					},
				}
				s.called = true
				return nil
			},
		),
		BestVersion: 2,
	}

	client := cloudapi.NewClient(apiCaller)
	results, err := client.CredentialContents("cloud-name", "credential-name", true)
	c.Assert(results, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "expected 1 result for credential \"cloud-name\" on cloud \"credential-name\", got 2")
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestCredentialContentsServerError(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				s.called = true
				return errors.New("boom")
			}),
		BestVersion: 2,
	}

	client := cloudapi.NewClient(apiCaller)
	results, err := client.CredentialContents("", "", true)
	c.Assert(results, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestCredentialContentsNotInV2API(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				s.called = true
				return nil
			},
		),
		BestVersion: 1,
	}
	client := cloudapi.NewClient(apiCaller)
	_, err := client.CredentialContents("", "", true)
	c.Assert(err, gc.ErrorMatches, "CredentialContents\\(\\).* not implemented")
	c.Assert(s.called, jc.IsFalse)
}

func (s *cloudSuite) TestRemoveCloud(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				s.called = true
				c.Check(objType, gc.Equals, "Cloud")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "RemoveClouds")
				c.Check(a, jc.DeepEquals, params.Entities{
					Entities: []params.Entity{{Tag: "cloud-foo"}},
				})
				c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
				results := result.(*params.ErrorResults)
				results.Results = append(results.Results, params.ErrorResult{
					Error: &params.Error{Message: "FAIL"},
				})
				return nil
			},
		),
		BestVersion: 2,
	}

	client := cloudapi.NewClient(apiCaller)
	err := client.RemoveCloud("foo")
	c.Assert(err, gc.ErrorMatches, "FAIL")
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestRemoveCloudNotInV1API(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(objType, gc.Equals, "Cloud")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "RemoveCloud")
				s.called = true
				return nil
			},
		),
		BestVersion: 1,
	}
	client := cloudapi.NewClient(apiCaller)
	err := client.RemoveCloud("foo")

	c.Assert(s.called, jc.IsFalse)
	c.Assert(err, gc.ErrorMatches, "RemoveCloud\\(\\).* not implemented")
}

func (s *cloudSuite) TestGrantCloud(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				s.called = true
				c.Check(objType, gc.Equals, "Cloud")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "ModifyCloudAccess")
				c.Check(a, jc.DeepEquals, params.ModifyCloudAccessRequest{
					Changes: []params.ModifyCloudAccess{
						{UserTag: "user-fred", CloudTag: "cloud-fluffy", Action: "grant", Access: "admin"},
					},
				})
				c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
				results := result.(*params.ErrorResults)
				results.Results = append(results.Results, params.ErrorResult{
					Error: &params.Error{Message: "FAIL"},
				})
				return nil
			},
		),
		BestVersion: 3,
	}

	client := cloudapi.NewClient(apiCaller)
	err := client.GrantCloud("fred", "admin", "fluffy")
	c.Assert(err, gc.ErrorMatches, "FAIL")
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestGrantCloudAccessNotInV2API(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Fail()
				return nil
			},
		),
		BestVersion: 2,
	}
	client := cloudapi.NewClient(apiCaller)
	err := client.GrantCloud("foo", "admin", "fluffy")
	c.Assert(err, gc.ErrorMatches, "GrantCloud\\(\\).* not implemented")
}

func (s *cloudSuite) TestRevokeCloud(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				s.called = true
				c.Check(objType, gc.Equals, "Cloud")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "ModifyCloudAccess")
				c.Check(a, jc.DeepEquals, params.ModifyCloudAccessRequest{
					Changes: []params.ModifyCloudAccess{
						{UserTag: "user-fred", CloudTag: "cloud-fluffy", Action: "revoke", Access: "admin"},
					},
				})
				c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
				results := result.(*params.ErrorResults)
				results.Results = append(results.Results, params.ErrorResult{
					Error: &params.Error{Message: "FAIL"},
				})
				return nil
			},
		),
		BestVersion: 3,
	}

	client := cloudapi.NewClient(apiCaller)
	err := client.RevokeCloud("fred", "admin", "fluffy")
	c.Assert(err, gc.ErrorMatches, "FAIL")
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestRevokeCloudAccessNotInV2API(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Fail()
				return nil
			},
		),
		BestVersion: 2,
	}
	client := cloudapi.NewClient(apiCaller)
	err := client.RevokeCloud("foo", "admin", "fluffy")
	c.Assert(err, gc.ErrorMatches, "RevokeCloud\\(\\).* not implemented")
}

func createCredentials(n int) map[string]cloud.Credential {
	result := map[string]cloud.Credential{}
	for i := 0; i < n; i++ {
		result[names.NewCloudCredentialTag(fmt.Sprintf("foo/bob/bar%d", i)).String()] = testCredential
	}
	return result
}

func (s *cloudSuite) TestUpdateCloudsCredentials(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(objType, gc.Equals, "Cloud")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "UpdateCredentialsCheckModels")
				c.Assert(result, gc.FitsTypeOf, &params.UpdateCredentialResults{})
				c.Assert(a, jc.DeepEquals, params.UpdateCredentialArgs{
					Force: true,
					Credentials: []params.TaggedCredential{{
						Tag: "cloudcred-foo_bob_bar0",
						Credential: params.CloudCredential{
							AuthType: "userpass",
							Attributes: map[string]string{
								"username": "admin",
								"password": "adm1n",
							},
						},
					}}})
				*result.(*params.UpdateCredentialResults) = params.UpdateCredentialResults{
					Results: []params.UpdateCredentialResult{{}},
				}
				s.called = true
				return nil
			},
		),
		BestVersion: 3,
	}
	client := cloudapi.NewClient(apiCaller)
	result, err := client.UpdateCloudsCredentials(createCredentials(1), true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []params.UpdateCredentialResult{{}})
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCloudsCredentialsErrorV2(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(request, gc.Equals, "UpdateCredentials")
				*result.(*params.ErrorResults) = params.ErrorResults{
					Results: []params.ErrorResult{{common.ServerError(errors.New("validation failure"))}},
				}
				s.called = true
				return nil
			},
		),
		BestVersion: 2,
	}
	client := cloudapi.NewClient(apiCaller)
	errs, err := client.UpdateCloudsCredentials(createCredentials(1), true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.DeepEquals, []params.UpdateCredentialResult{
		{CredentialTag: "cloudcred-foo_bob_bar0", Error: &params.Error{Message: "validation failure", Code: ""}},
	})
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCloudsCredentialsError(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(request, gc.Equals, "UpdateCredentialsCheckModels")
				*result.(*params.UpdateCredentialResults) = params.UpdateCredentialResults{
					Results: []params.UpdateCredentialResult{
						{CredentialTag: "cloudcred-foo_bob_bar0",
							Error: common.ServerError(errors.New("validation failure")),
						},
					},
				}
				s.called = true
				return nil
			},
		),
		BestVersion: 3,
	}
	client := cloudapi.NewClient(apiCaller)
	errs, err := client.UpdateCloudsCredentials(createCredentials(1), false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.DeepEquals, []params.UpdateCredentialResult{
		{CredentialTag: "cloudcred-foo_bob_bar0", Error: common.ServerError(errors.New("validation failure"))},
	})
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCloudsCredentialsMasksLegacyError(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(request, gc.Equals, "UpdateCredentialsCheckModels")
				*result.(*params.UpdateCredentialResults) = params.UpdateCredentialResults{
					Results: []params.UpdateCredentialResult{
						{CredentialTag: "cloudcred-foo_bob_bar0",
							Error: common.ServerError(errors.New("some models are no longer visible")),
						},
					},
				}
				s.called = true
				return nil
			},
		),
		BestVersion: 6,
	}
	client := cloudapi.NewClient(apiCaller)
	errs, err := client.UpdateCloudsCredentials(createCredentials(1), false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.DeepEquals, []params.UpdateCredentialResult{
		{CredentialTag: "cloudcred-foo_bob_bar0"},
	})
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCloudsCredentialsCallErrorV2(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(request, gc.Equals, "UpdateCredentials")
				s.called = true
				return errors.New("scary but true")
			},
		),
		BestVersion: 2,
	}
	client := cloudapi.NewClient(apiCaller)
	result, err := client.UpdateCloudsCredentials(createCredentials(1), false)
	c.Assert(err, gc.ErrorMatches, "scary but true")
	c.Assert(result, gc.IsNil)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCloudsCredentialsCallError(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(request, gc.Equals, "UpdateCredentialsCheckModels")
				s.called = true
				return errors.New("scary but true")
			},
		),
		BestVersion: 3,
	}
	client := cloudapi.NewClient(apiCaller)
	result, err := client.UpdateCloudsCredentials(createCredentials(1), false)
	c.Assert(err, gc.ErrorMatches, "scary but true")
	c.Assert(result, gc.IsNil)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCloudsCredentialsManyResultsV2(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(request, gc.Equals, "UpdateCredentials")
				*result.(*params.ErrorResults) = params.ErrorResults{
					Results: []params.ErrorResult{
						{},
						{},
					},
				}
				s.called = true
				return nil
			},
		),
		BestVersion: 2,
	}
	client := cloudapi.NewClient(apiCaller)
	result, err := client.UpdateCloudsCredentials(createCredentials(1), false)
	c.Assert(err, gc.ErrorMatches, `expected 1 result got 2 when updating credentials`)
	c.Assert(result, gc.IsNil)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCloudsCredentialsManyMatchingResultsV2(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(request, gc.Equals, "UpdateCredentials")
				*result.(*params.ErrorResults) = params.ErrorResults{
					Results: []params.ErrorResult{
						{},
						{},
					},
				}
				s.called = true
				return nil
			},
		),
		BestVersion: 2,
	}
	client := cloudapi.NewClient(apiCaller)
	in := createCredentials(2)
	result, err := client.UpdateCloudsCredentials(in, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, len(in))
	for _, one := range result {
		_, ok := in[one.CredentialTag]
		c.Assert(ok, jc.IsTrue)
	}
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCloudsCredentialsManyResults(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(request, gc.Equals, "UpdateCredentialsCheckModels")
				*result.(*params.UpdateCredentialResults) = params.UpdateCredentialResults{
					Results: []params.UpdateCredentialResult{
						{},
						{},
					}}
				s.called = true
				return nil
			},
		),
		BestVersion: 3,
	}
	client := cloudapi.NewClient(apiCaller)
	result, err := client.UpdateCloudsCredentials(createCredentials(1), false)
	c.Assert(err, gc.ErrorMatches, `expected 1 result got 2 when updating credentials`)
	c.Assert(result, gc.IsNil)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCloudsCredentialsManyMatchingResults(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(request, gc.Equals, "UpdateCredentialsCheckModels")
				*result.(*params.UpdateCredentialResults) = params.UpdateCredentialResults{
					Results: []params.UpdateCredentialResult{
						{},
						{},
					}}
				s.called = true
				return nil
			},
		),
		BestVersion: 3,
	}
	client := cloudapi.NewClient(apiCaller)
	count := 2
	result, err := client.UpdateCloudsCredentials(createCredentials(count), false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, count)
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestUpdateCloudsCredentialsModelErrors(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(request, gc.Equals, "UpdateCredentialsCheckModels")
				*result.(*params.UpdateCredentialResults) = params.UpdateCredentialResults{
					Results: []params.UpdateCredentialResult{
						{
							CredentialTag: testCredentialTag.String(),
							Models: []params.UpdateCredentialModelResult{
								{
									ModelUUID: coretesting.ModelTag.Id(),
									ModelName: "test-model",
									Errors: []params.ErrorResult{
										{common.ServerError(errors.New("validation failure one"))},
										{common.ServerError(errors.New("validation failure two"))},
									},
								},
							},
						},
					}}
				s.called = true
				return nil
			},
		),
		BestVersion: 3,
	}
	client := cloudapi.NewClient(apiCaller)
	errs, err := client.UpdateCloudsCredentials(createCredentials(1), false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.DeepEquals, []params.UpdateCredentialResult{
		{CredentialTag: "cloudcred-foo_bob_bar",
			Models: []params.UpdateCredentialModelResult{
				{ModelUUID: "deadbeef-0bad-400d-8000-4b1d0d06f00d",
					ModelName: "test-model",
					Errors: []params.ErrorResult{
						{common.ServerError(errors.New("validation failure one"))},
						{common.ServerError(errors.New("validation failure two"))},
					},
				},
			},
		},
	})
	c.Assert(s.called, jc.IsTrue)
}

func (s *cloudSuite) TestAddCloudsCredentials(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(objType, gc.Equals, "Cloud")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "UpdateCredentialsCheckModels")
				c.Assert(result, gc.FitsTypeOf, &params.UpdateCredentialResults{})
				c.Assert(a, jc.DeepEquals, params.UpdateCredentialArgs{
					Credentials: []params.TaggedCredential{{
						Tag: "cloudcred-foo_bob_bar0",
						Credential: params.CloudCredential{
							AuthType: "userpass",
							Attributes: map[string]string{
								"username": "admin",
								"password": "adm1n",
							},
						},
					}}})
				*result.(*params.UpdateCredentialResults) = params.UpdateCredentialResults{
					Results: []params.UpdateCredentialResult{{}},
				}
				s.called = true
				return nil
			},
		),
		BestVersion: 3,
	}
	client := cloudapi.NewClient(apiCaller)
	result, err := client.AddCloudsCredentials(createCredentials(1))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []params.UpdateCredentialResult{{}})
	c.Assert(s.called, jc.IsTrue)
}
