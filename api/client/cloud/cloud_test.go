// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/kr/pretty"
	"go.uber.org/mock/gomock"

	basemocks "github.com/juju/juju/api/base/mocks"
	cloudapi "github.com/juju/juju/api/client/cloud"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cloud"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type cloudSuite struct {
}

var _ = tc.Suite(&cloudSuite{})

func (s *cloudSuite) SetUpTest(c *tc.C) {
}

func (s *cloudSuite) TestCloud(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{{Tag: "cloud-foo"}},
	}
	res := new(params.CloudResults)
	results := params.CloudResults{
		Results: []params.CloudResult{{
			Cloud: &params.Cloud{
				Type:      "dummy",
				AuthTypes: []string{"empty", "userpass"},
				Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
			}},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Cloud", args, res).SetArg(3, results).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	result, err := client.Cloud(context.Background(), names.NewCloudTag("foo"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, cloud.Cloud{
		Name:      "foo",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
		Regions:   []cloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	})
}

func (s *cloudSuite) TestCloudInfo(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "cloud-foo"}, {Tag: "cloud-bar"},
		},
	}
	res := new(params.CloudInfoResults)
	results := params.CloudInfoResults{
		Results: []params.CloudInfoResult{{
			Result: &params.CloudInfo{
				CloudDetails: params.CloudDetails{
					Type:      "dummy",
					AuthTypes: []string{"empty", "userpass"},
					Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
				},
				Users: []params.CloudUserInfo{{
					UserName:    "fred",
					DisplayName: "Fred",
					Access:      "admin",
				}, {
					UserName:    "bob",
					DisplayName: "Bob",
					Access:      "add-model",
				}},
			},
		}, {
			Result: &params.CloudInfo{
				CloudDetails: params.CloudDetails{
					Type:      "dummy",
					AuthTypes: []string{"empty", "userpass"},
					Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
				},
				Users: []params.CloudUserInfo{{
					UserName:    "mary",
					DisplayName: "Mary",
					Access:      "admin",
				}},
			},
		}},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "CloudInfo", args, res).SetArg(3, results).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	result, err := client.CloudInfo(context.Background(), []names.CloudTag{
		names.NewCloudTag("foo"),
		names.NewCloudTag("bar"),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []cloudapi.CloudInfo{{
		Cloud: cloud.Cloud{
			Name:      "foo",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
			Regions:   []cloud.Region{{Name: "nether", Endpoint: "endpoint"}},
		},
		Users: map[string]cloudapi.CloudUserInfo{
			"bob": {
				DisplayName: "Bob",
				Access:      "add-model",
			},
			"fred": {
				DisplayName: "Fred",
				Access:      "admin",
			},
		},
	}, {
		Cloud: cloud.Cloud{
			Name:      "bar",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
			Regions:   []cloud.Region{{Name: "nether", Endpoint: "endpoint"}},
		},
		Users: map[string]cloudapi.CloudUserInfo{
			"mary": {
				DisplayName: "Mary",
				Access:      "admin",
			},
		},
	}})
}

func (s *cloudSuite) TestClouds(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	res := new(params.CloudsResult)
	results := params.CloudsResult{
		Clouds: map[string]params.Cloud{
			"cloud-foo": {
				Type: "bar",
			},
			"cloud-baz": {
				Type:      "qux",
				AuthTypes: []string{"empty", "userpass"},
				Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
			},
		}}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Clouds", nil, res).SetArg(3, results).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	clouds, err := client.Clouds(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clouds, tc.DeepEquals, map[names.CloudTag]cloud.Cloud{
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
}

func (s *cloudSuite) TestUserCredentials(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.UserClouds{UserClouds: []params.UserCloud{{
		UserTag:  "user-bob",
		CloudTag: "cloud-foo",
	}}}
	res := new(params.StringsResults)
	results := params.StringsResults{
		Results: []params.StringsResult{{
			Result: []string{
				"cloudcred-foo_bob_one",
				"cloudcred-foo_bob_two",
			},
		}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UserCredentials", args, res).SetArg(3, results).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	result, err := client.UserCredentials(context.Background(), names.NewUserTag("bob"), names.NewCloudTag("foo"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []names.CloudCredentialTag{
		names.NewCloudCredentialTag("foo/bob/one"),
		names.NewCloudCredentialTag("foo/bob/two"),
	})
}

func (s *cloudSuite) TestUpdateCredential(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.UpdateCredentialArgs{
		Credentials: []params.TaggedCredential{{
			Tag: "cloudcred-foo_bob_bar",
			Credential: params.CloudCredential{
				AuthType: "userpass",
				Attributes: map[string]string{
					"username": "admin",
					"password": "adm1n",
				},
			},
		}}}
	res := new(params.UpdateCredentialResults)
	results := params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UpdateCredentialsCheckModels", args, res).SetArg(3, results).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	result, err := client.UpdateCredentialsCheckModels(context.Background(), testCredentialTag, testCredential)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.IsNil)
}

func (s *cloudSuite) TestUpdateCredentialError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.UpdateCredentialArgs{
		Credentials: []params.TaggedCredential{{
			Tag: "cloudcred-foo_bob_bar",
			Credential: params.CloudCredential{
				AuthType: "userpass",
				Attributes: map[string]string{
					"username": "admin",
					"password": "adm1n",
				},
			},
		}}}
	res := new(params.UpdateCredentialResults)
	results := params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{
			{
				CredentialTag: "cloudcred-foo_bob_bar",
				Error:         apiservererrors.ServerError(errors.New("validation failure")),
			},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UpdateCredentialsCheckModels", args, res).SetArg(3, results).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	errs, err := client.UpdateCredentialsCheckModels(context.Background(), testCredentialTag, testCredential)
	c.Assert(err, tc.ErrorMatches, "validation failure")
	c.Assert(errs, tc.IsNil)
}

func (s *cloudSuite) TestUpdateCredentialManyResults(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.UpdateCredentialArgs{
		Credentials: []params.TaggedCredential{{
			Tag: "cloudcred-foo_bob_bar",
			Credential: params.CloudCredential{
				AuthType: "userpass",
				Attributes: map[string]string{
					"username": "admin",
					"password": "adm1n",
				},
			},
		}}}
	res := new(params.UpdateCredentialResults)
	results := params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{
			{},
			{},
		}}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UpdateCredentialsCheckModels", args, res).SetArg(3, results).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	result, err := client.UpdateCredentialsCheckModels(context.Background(), testCredentialTag, testCredential)
	c.Assert(err, tc.ErrorMatches, `expected 1 result got 2 when updating credentials`)
	c.Assert(result, tc.IsNil)
}

func (s *cloudSuite) TestUpdateCredentialModelErrors(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.UpdateCredentialArgs{
		Credentials: []params.TaggedCredential{{
			Tag: "cloudcred-foo_bob_bar",
			Credential: params.CloudCredential{
				AuthType: "userpass",
				Attributes: map[string]string{
					"username": "admin",
					"password": "adm1n",
				},
			},
		}}}
	res := new(params.UpdateCredentialResults)
	results := params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{
			{
				CredentialTag: testCredentialTag.String(),
				Models: []params.UpdateCredentialModelResult{
					{
						ModelUUID: coretesting.ModelTag.Id(),
						ModelName: "test-model",
						Errors: []params.ErrorResult{
							{apiservererrors.ServerError(errors.New("validation failure one"))},
							{apiservererrors.ServerError(errors.New("validation failure two"))},
						},
					},
				},
			},
		}}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UpdateCredentialsCheckModels", args, res).SetArg(3, results).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	errs, err := client.UpdateCredentialsCheckModels(context.Background(), testCredentialTag, testCredential)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs, tc.DeepEquals, []params.UpdateCredentialModelResult{
		{
			ModelUUID: "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			ModelName: "test-model",
			Errors: []params.ErrorResult{
				{Error: &params.Error{Message: "validation failure one", Code: ""}},
				{Error: &params.Error{Message: "validation failure two", Code: ""}},
			},
		},
	})
}

var (
	testCredentialTag = names.NewCloudCredentialTag("foo/bob/bar")
	testCredential    = cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username": "admin",
		"password": "adm1n",
	})
)

func (s *cloudSuite) TestRevokeCredential(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.RevokeCredentialArgs{
		Credentials: []params.RevokeCredentialArg{
			{Tag: "cloudcred-foo_bob_bar", Force: true},
		},
	}
	res := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "RevokeCredentialsCheckModels", args, res).SetArg(3, results).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	tag := names.NewCloudCredentialTag("foo/bob/bar")
	err := client.RevokeCredential(context.Background(), tag, true)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *cloudSuite) TestCredentials(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{Entities: []params.Entity{{
		Tag: "cloudcred-foo_bob_bar",
	}}}
	res := new(params.CloudCredentialResults)
	results := params.CloudCredentialResults{
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
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Credential", args, res).SetArg(3, results).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	tag := names.NewCloudCredentialTag("foo/bob/bar")
	result, err := client.Credentials(context.Background(), tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []params.CloudCredentialResult{
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

var testCloud = cloud.Cloud{
	Name:      "foo",
	Type:      "dummy",
	AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
	Regions:   []cloud.Region{{Name: "nether", Endpoint: "endpoint"}},
}

func (s *cloudSuite) TestAddCloudForce(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	force := true
	args := params.AddCloudArgs{
		Name:  "foo",
		Cloud: cloudapi.CloudToParams(testCloud),
		Force: &force,
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddCloud", args, nil).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	err := client.AddCloud(context.Background(), testCloud, force)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *cloudSuite) TestCredentialContentsArgumentCheck(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	// Check supplying cloud name without credential name is invalid.
	result, err := client.CredentialContents(context.Background(), "cloud", "", true)
	c.Assert(result, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "credential name must be supplied")

	// Check supplying credential name without cloud name is invalid.
	result, err = client.CredentialContents(context.Background(), "", "credential", true)
	c.Assert(result, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "cloud name must be supplied")
}

func (s *cloudSuite) TestCredentialContentsAll(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

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
			Error: apiservererrors.ServerError(errors.New("boom")),
		},
	}

	args := params.CloudCredentialArgs{
		IncludeSecrets: true,
	}
	res := new(params.CredentialContentResults)
	results := params.CredentialContentResults{
		Results: expectedResults,
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "CredentialContents", args, res).SetArg(3, results).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	ress, err := client.CredentialContents(context.Background(), "", "", true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ress, tc.DeepEquals, expectedResults)
}

func (s *cloudSuite) TestCredentialContentsOne(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.CloudCredentialArgs{
		IncludeSecrets: true,
		Credentials: []params.CloudCredentialArg{
			{CloudName: "cloud-name", CredentialName: "credential-name"},
		},
	}
	res := new(params.CredentialContentResults)
	ress := params.CredentialContentResults{
		Results: []params.CredentialContentResult{
			{},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "CredentialContents", args, res).SetArg(3, ress).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	results, err := client.CredentialContents(context.Background(), "cloud-name", "credential-name", true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
}

func (s *cloudSuite) TestCredentialContentsGotMoreThanBargainedFor(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.CloudCredentialArgs{
		IncludeSecrets: true,
		Credentials: []params.CloudCredentialArg{
			{CloudName: "cloud-name", CredentialName: "credential-name"},
		},
	}
	res := new(params.CredentialContentResults)
	ress := params.CredentialContentResults{
		Results: []params.CredentialContentResult{
			{},
			{},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "CredentialContents", args, res).SetArg(3, ress).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	results, err := client.CredentialContents(context.Background(), "cloud-name", "credential-name", true)
	c.Assert(results, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "expected 1 result for credential \"cloud-name\" on cloud \"credential-name\", got 2")
}

func (s *cloudSuite) TestCredentialContentsServerError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.CloudCredentialArgs{
		IncludeSecrets: true,
	}
	res := new(params.CredentialContentResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "CredentialContents", args, res).Return(errors.New("boom"))
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	results, err := client.CredentialContents(context.Background(), "", "", true)
	c.Assert(results, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *cloudSuite) TestRemoveCloud(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{{Tag: "cloud-foo"}},
	}
	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: &params.Error{Message: "FAIL"},
		}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "RemoveClouds", args, res).SetArg(3, ress).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	err := client.RemoveCloud(context.Background(), "foo")
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *cloudSuite) TestRemoveCloudErrorMapping(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{{Tag: "cloud-foo"}},
	}
	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: `cloud "cloud-foo" not found`,
			}},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "RemoveClouds", args, res).SetArg(3, ress).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	err := client.RemoveCloud(context.Background(), "foo")
	c.Assert(err, tc.ErrorIs, errors.NotFound, tc.Commentf("expected client to be map server error into a NotFound error"))
}

func (s *cloudSuite) TestGrantCloud(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.ModifyCloudAccessRequest{
		Changes: []params.ModifyCloudAccess{
			{UserTag: "user-fred", CloudTag: "cloud-fluffy", Action: "grant", Access: "admin"},
		},
	}
	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: &params.Error{Message: "FAIL"}},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ModifyCloudAccess", args, res).SetArg(3, ress).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	err := client.GrantCloud(context.Background(), "fred", "admin", "fluffy")
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *cloudSuite) TestRevokeCloud(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.ModifyCloudAccessRequest{
		Changes: []params.ModifyCloudAccess{
			{UserTag: "user-fred", CloudTag: "cloud-fluffy", Action: "revoke", Access: "admin"},
		},
	}
	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: &params.Error{Message: "FAIL"}},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ModifyCloudAccess", args, res).SetArg(3, ress).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	err := client.RevokeCloud(context.Background(), "fred", "admin", "fluffy")
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func createCredentials(n int) map[string]cloud.Credential {
	result := map[string]cloud.Credential{}
	for i := 0; i < n; i++ {
		result[names.NewCloudCredentialTag(fmt.Sprintf("foo/bob/bar%d", i)).String()] = testCredential
	}
	return result
}

func (s *cloudSuite) TestUpdateCloud(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	updatedCloud := cloud.Cloud{
		Name:      "foo",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
		Regions:   []cloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	args := params.UpdateCloudArgs{Clouds: []params.AddCloudArgs{{
		Name:  "foo",
		Cloud: cloudapi.CloudToParams(updatedCloud),
	}}}
	res := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UpdateCloud", args, res).SetArg(3, results).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	err := client.UpdateCloud(context.Background(), updatedCloud)

	c.Assert(err, tc.ErrorIsNil)
}

func (s *cloudSuite) TestUpdateCloudsCredentials(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.UpdateCredentialArgs{
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
		}}}
	res := new(params.UpdateCredentialResults)
	results := params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UpdateCredentialsCheckModels", args, res).SetArg(3, results).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	result, err := client.UpdateCloudsCredentials(context.Background(), createCredentials(1), true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []params.UpdateCredentialResult{{}})
}

func (s *cloudSuite) TestUpdateCloudsCredentialsError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag: "cloudcred-foo_bob_bar0",
			Credential: params.CloudCredential{
				AuthType: "userpass",
				Attributes: map[string]string{
					"username": "admin",
					"password": "adm1n",
				},
			},
		}}}
	res := new(params.UpdateCredentialResults)
	results := params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{
			{CredentialTag: "cloudcred-foo_bob_bar0",
				Error: apiservererrors.ServerError(errors.New("validation failure")),
			},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UpdateCredentialsCheckModels", args, res).SetArg(3, results).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	errs, err := client.UpdateCloudsCredentials(context.Background(), createCredentials(1), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs, tc.DeepEquals, []params.UpdateCredentialResult{
		{CredentialTag: "cloudcred-foo_bob_bar0", Error: apiservererrors.ServerError(errors.New("validation failure"))},
	})
}

func (s *cloudSuite) TestUpdateCloudsCredentialsManyResults(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag: "cloudcred-foo_bob_bar0",
			Credential: params.CloudCredential{
				AuthType: "userpass",
				Attributes: map[string]string{
					"username": "admin",
					"password": "adm1n",
				},
			},
		}}}
	res := new(params.UpdateCredentialResults)
	results := params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{
			{},
			{},
		}}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UpdateCredentialsCheckModels", args, res).SetArg(3, results).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	result, err := client.UpdateCloudsCredentials(context.Background(), createCredentials(1), false)
	c.Assert(err, tc.ErrorMatches, `expected 1 result got 2 when updating credentials`)
	c.Assert(result, tc.IsNil)
}

func (s *cloudSuite) TestUpdateCloudsCredentialsManyMatchingResults(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.UpdateCredentialArgs{
		Force: false,
	}
	count := 2
	for tag, credential := range createCredentials(count) {
		args.Credentials = append(args.Credentials, params.TaggedCredential{
			Tag: tag,
			Credential: params.CloudCredential{
				AuthType:   string(credential.AuthType()),
				Attributes: credential.Attributes(),
			},
		})
	}

	res := new(params.UpdateCredentialResults)
	results := params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{
			{},
			{},
		}}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UpdateCredentialsCheckModels", cloudCredentialMatcher{args}, res).SetArg(3, results).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	result, err := client.UpdateCloudsCredentials(context.Background(), createCredentials(count), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, count)
}

func (s *cloudSuite) TestUpdateCloudsCredentialsModelErrors(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag: "cloudcred-foo_bob_bar0",
			Credential: params.CloudCredential{
				AuthType: "userpass",
				Attributes: map[string]string{
					"username": "admin",
					"password": "adm1n",
				},
			},
		}}}
	res := new(params.UpdateCredentialResults)
	results := params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{
			{
				CredentialTag: testCredentialTag.String(),
				Models: []params.UpdateCredentialModelResult{
					{
						ModelUUID: coretesting.ModelTag.Id(),
						ModelName: "test-model",
						Errors: []params.ErrorResult{
							{apiservererrors.ServerError(errors.New("validation failure one"))},
							{apiservererrors.ServerError(errors.New("validation failure two"))},
						},
					},
				},
			},
		}}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UpdateCredentialsCheckModels", args, res).SetArg(3, results).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	errs, err := client.UpdateCloudsCredentials(context.Background(), createCredentials(1), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs, tc.DeepEquals, []params.UpdateCredentialResult{
		{CredentialTag: "cloudcred-foo_bob_bar",
			Models: []params.UpdateCredentialModelResult{
				{ModelUUID: "deadbeef-0bad-400d-8000-4b1d0d06f00d",
					ModelName: "test-model",
					Errors: []params.ErrorResult{
						{Error: apiservererrors.ServerError(errors.New("validation failure one"))},
						{Error: apiservererrors.ServerError(errors.New("validation failure two"))},
					},
				},
			},
		},
	})
}

func (s *cloudSuite) TestAddCloudsCredentials(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.UpdateCredentialArgs{
		Credentials: []params.TaggedCredential{{
			Tag: "cloudcred-foo_bob_bar0",
			Credential: params.CloudCredential{
				AuthType: "userpass",
				Attributes: map[string]string{
					"username": "admin",
					"password": "adm1n",
				},
			},
		}}}
	res := new(params.UpdateCredentialResults)
	results := params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UpdateCredentialsCheckModels", args, res).SetArg(3, results).Return(nil)
	client := cloudapi.NewClientFromCaller(mockFacadeCaller)

	result, err := client.AddCloudsCredentials(context.Background(), createCredentials(1))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []params.UpdateCredentialResult{{}})
}

type cloudCredentialMatcher struct {
	arg params.UpdateCredentialArgs
}

func (c cloudCredentialMatcher) Matches(x interface{}) bool {
	cred, ok := x.(params.UpdateCredentialArgs)
	if !ok {
		return false
	}
	if len(cred.Credentials) != len(c.arg.Credentials) {
		return false
	}
	// sort both input and expected slices the same way to avoid ordering discrepancies when ranging.
	sort.Slice(cred.Credentials, func(i, j int) bool { return cred.Credentials[i].Tag < cred.Credentials[j].Tag })
	sort.Slice(c.arg.Credentials, func(i, j int) bool { return c.arg.Credentials[i].Tag < c.arg.Credentials[j].Tag })
	for idx, taggedCred := range cred.Credentials {
		if taggedCred.Tag != c.arg.Credentials[idx].Tag {
			return false
		}
		if !reflect.DeepEqual(taggedCred.Credential, c.arg.Credentials[idx].Credential) {
			return false
		}
	}

	if cred.Force != c.arg.Force {
		return false
	}
	return true
}

func (c cloudCredentialMatcher) String() string {
	return pretty.Sprint(c.arg)
}
