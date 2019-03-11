// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"fmt"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/juju/permission"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/credentialcommon"
	cloudfacade "github.com/juju/juju/apiserver/facades/client/cloud"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

type cloudSuite struct {
	gitjujutesting.IsolationSuite
	backend     *mockBackend
	ctlrBackend *mockBackend
	authorizer  *apiservertesting.FakeAuthorizer
	api         *cloudfacade.CloudAPI

	statePool   *mockStatePool
	pooledModel *mockPooledModel
}

var _ = gc.Suite(&cloudSuite{})

func (s *cloudSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	aCloud := cloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
		Regions:   []cloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}
	s.backend = &mockBackend{
		cloud: aCloud,
		creds: map[string]state.Credential{
			names.NewCloudCredentialTag("meep/bruce/one").Id(): statetesting.NewEmptyCredential(),
			names.NewCloudCredentialTag("meep/bruce/two").Id(): statetesting.CloudCredential(cloud.UserPassAuthType, map[string]string{
				"username": "admin",
				"password": "adm1n",
			}),
		},
		credentialModelsF: func(tag names.CloudCredentialTag) (map[string]string, error) { return nil, nil },
	}
	s.ctlrBackend = &mockBackend{
		cloud:             aCloud,
		credentialModelsF: func(tag names.CloudCredentialTag) (map[string]string, error) { return nil, nil },
	}

	newModel := func(uuid string) *mockPooledModel {
		return &mockPooledModel{
			model: &mockModelBackend{
				uuid: uuid,
				model: &mockModel{
					cloud:       "dummy",
					cloudRegion: "nether",
					cfg:         coretesting.ModelConfig(c),
				},
				cloud: aCloud,
			},
			release: true,
		}
	}
	s.statePool = &mockStatePool{
		getF: func(modelUUID string) (cloudfacade.PooledModelBackend, error) {
			return newModel(modelUUID), nil
		},
	}
	s.setTestAPIForUser(c, names.NewUserTag("admin"))
}

func (s *cloudSuite) setTestAPIForUser(c *gc.C, user names.UserTag) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: user,
	}
	var err error
	s.api, err = cloudfacade.NewCloudAPI(s.backend, s.ctlrBackend, s.statePool, s.authorizer, context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cloudSuite) TestCloud(c *gc.C) {
	s.ctlrBackend.cloudAccess = permission.AddModelAccess
	results, err := s.api.Cloud(params.Entities{
		Entities: []params.Entity{{Tag: "cloud-my-cloud"}, {Tag: "machine-0"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCalls(c, []gitjujutesting.StubCall{
		{"Cloud", []interface{}{"my-cloud"}},
	})
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[0].Cloud, jc.DeepEquals, &params.Cloud{
		Type:      "dummy",
		AuthTypes: []string{"empty", "userpass"},
		Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
	})
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: `"machine-0" is not a valid cloud tag`,
	})
}

func (s *cloudSuite) TestCloudNotFound(c *gc.C) {
	s.backend.SetErrors(errors.NotFoundf("cloud \"no-dice\""))
	s.setTestAPIForUser(c, names.NewUserTag("admin"))
	results, err := s.api.Cloud(params.Entities{
		Entities: []params.Entity{{Tag: "cloud-no-dice"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, "cloud \"no-dice\" not found")
}

func (s *cloudSuite) TestClouds(c *gc.C) {
	s.setTestAPIForUser(c, names.NewUserTag("bruce"))
	s.ctlrBackend.cloudAccess = permission.AddModelAccess
	result, err := s.api.Clouds()
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Clouds")
	s.ctlrBackend.CheckCallNames(c, "ControllerTag", "GetCloudAccess", "GetCloudAccess")
	c.Assert(result.Clouds, jc.DeepEquals, map[string]params.Cloud{
		"cloud-my-cloud": {
			Type:      "dummy",
			AuthTypes: []string{"empty", "userpass"},
			Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
		},
	})
}

func (s *cloudSuite) TestCloudInfoAdmin(c *gc.C) {
	result, err := s.api.CloudInfo(params.Entities{Entities: []params.Entity{{
		Tag: "cloud-my-cloud",
	}, {
		Tag: "machine-0",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Cloud", "User", "User")
	s.ctlrBackend.CheckCallNames(c, "ControllerTag", "GetCloudUsers")

	// Make sure that the slice is sorted in a predictable manor
	sort.Slice(result.Results[0].Result.Users, func(i, j int) bool {
		return result.Results[0].Result.Users[i].UserName < result.Results[0].Result.Users[j].UserName
	})

	c.Assert(result.Results, jc.DeepEquals, []params.CloudInfoResult{
		{
			Result: &params.CloudInfo{
				CloudDetails: params.CloudDetails{
					Type:      "dummy",
					AuthTypes: []string{"empty", "userpass"},
					Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
				},
				Users: []params.CloudUserInfo{
					{UserName: "fred", DisplayName: "display-fred", Access: "add-model"},
					{UserName: "mary", DisplayName: "display-mary", Access: "admin"},
				},
			},
		}, {
			Error: &params.Error{Message: `"machine-0" is not a valid cloud tag`},
		},
	})
}

func (s *cloudSuite) TestCloudInfoNonAdmin(c *gc.C) {
	s.setTestAPIForUser(c, names.NewUserTag("fred"))
	result, err := s.api.CloudInfo(params.Entities{Entities: []params.Entity{{
		Tag: "cloud-my-cloud",
	}, {
		Tag: "machine-0",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Cloud", "User")
	s.ctlrBackend.CheckCallNames(c, "ControllerTag", "GetCloudAccess", "GetCloudUsers")
	c.Assert(result.Results, jc.DeepEquals, []params.CloudInfoResult{
		{
			Result: &params.CloudInfo{
				CloudDetails: params.CloudDetails{
					Type:      "dummy",
					AuthTypes: []string{"empty", "userpass"},
					Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
				},
				Users: []params.CloudUserInfo{
					{UserName: "fred", DisplayName: "display-fred", Access: "add-model"},
				},
			},
		}, {
			Error: &params.Error{Message: `"machine-0" is not a valid cloud tag`},
		},
	})
}

func (s *cloudSuite) TestAddCloud(c *gc.C) {
	paramsCloud := params.AddCloudArgs{
		Name: "newcloudname",
		Cloud: params.Cloud{
			Type:      "fake",
			AuthTypes: []string{"empty", "userpass"},
			Endpoint:  "fake-endpoint",
			Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "nether-endpoint"}},
		}}
	err := s.api.AddCloud(paramsCloud)
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "AddCloud")
	s.backend.CheckCall(c, 0, "AddCloud", cloud.Cloud{
		Name:      "newcloudname",
		Type:      "fake",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
		Endpoint:  "fake-endpoint",
		Regions:   []cloud.Region{{Name: "nether", Endpoint: "nether-endpoint"}},
	}, "admin")
}

func (s *cloudSuite) TestAddCloudNoAdminPerms(c *gc.C) {
	s.setTestAPIForUser(c, names.NewUserTag("frank"))
	paramsCloud := params.AddCloudArgs{
		Name: "newcloudname",
		Cloud: params.Cloud{
			Type:      "fake",
			AuthTypes: []string{"empty", "userpass"},
			Endpoint:  "fake-endpoint",
			Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "nether-endpoint"}},
		}}
	err := s.api.AddCloud(paramsCloud)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	s.backend.CheckNoCalls(c)
}

func (s *cloudSuite) TestUpdateCloud(c *gc.C) {
	updatedCloud := cloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
		Regions:   []cloud.Region{{Name: "nether-updated", Endpoint: "endpoint-updated"}},
	}
	results, err := s.api.UpdateCloud(params.UpdateCloudArgs{
		[]params.AddCloudArgs{{
			Name:  "dummy",
			Cloud: common.CloudToParams(updatedCloud),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "UpdateCloud")
	s.backend.CheckCall(c, 0, "UpdateCloud", updatedCloud)

	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *cloudSuite) TestUpdateCloudNonAdminPerm(c *gc.C) {
	s.setTestAPIForUser(c, names.NewUserTag("frank"))
	updatedCloud := cloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
		Regions:   []cloud.Region{{Name: "nether-updated", Endpoint: "endpoint-updated"}},
	}
	results, err := s.api.UpdateCloud(params.UpdateCloudArgs{
		[]params.AddCloudArgs{{
			Name:  "dummy",
			Cloud: common.CloudToParams(updatedCloud),
		}},
	})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	s.backend.CheckNoCalls(c)

	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *cloudSuite) TestUpdateNonExistentCloud(c *gc.C) {
	updatedCloud := cloud.Cloud{
		Name:      "nope",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
		Regions:   []cloud.Region{{Name: "nether-updated", Endpoint: "endpoint-updated"}},
	}
	s.backend.SetErrors(errors.NotFoundf("cloud %q", updatedCloud.Name))
	results, err := s.api.UpdateCloud(params.UpdateCloudArgs{
		[]params.AddCloudArgs{{
			Name:  "nope",
			Cloud: common.CloudToParams(updatedCloud),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.backend.CheckCallNames(c, "UpdateCloud")
	s.backend.CheckCall(c, 0, "UpdateCloud", updatedCloud)

	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, fmt.Sprintf("cloud %q not found", updatedCloud.Name))
}

func (s *cloudSuite) TestListCloudInfo(c *gc.C) {
	result, err := s.api.ListCloudInfo(params.ListCloudsRequest{
		UserTag: "user-fred",
		All:     true,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckNoCalls(c)
	s.ctlrBackend.CheckCallNames(c, "CloudsForUser")
	c.Assert(result.Results, jc.DeepEquals, []params.ListCloudInfoResult{
		{
			Result: &params.ListCloudInfo{
				CloudDetails: params.CloudDetails{
					Type:      "dummy",
					AuthTypes: []string{"empty", "userpass"},
					Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
				},
				Access: "add-model",
			},
		},
	})
}

func (s *cloudSuite) TestDefaultCloud(c *gc.C) {
	result, err := s.api.DefaultCloud()
	c.Assert(err, jc.ErrorIsNil)
	s.ctlrBackend.CheckCallNames(c, "Model")
	c.Assert(result, jc.DeepEquals, params.StringResult{
		Result: "cloud-dummy",
	})
}

func (s *cloudSuite) TestUserCredentials(c *gc.C) {
	s.setTestAPIForUser(c, names.NewUserTag("bruce"))
	results, err := s.api.UserCredentials(params.UserClouds{UserClouds: []params.UserCloud{{
		UserTag:  "machine-0",
		CloudTag: "cloud-meep",
	}, {
		UserTag:  "user-admin",
		CloudTag: "cloud-meep",
	}, {
		UserTag:  "user-bruce",
		CloudTag: "cloud-meep",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CloudCredentials")
	s.backend.CheckCall(c, 1, "CloudCredentials", names.NewUserTag("bruce"), "meep")

	c.Assert(results.Results, gc.HasLen, 3)
	c.Assert(results.Results[0].Error, jc.DeepEquals, &params.Error{
		Message: `"machine-0" is not a valid user tag`,
	})
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: "permission denied", Code: params.CodeUnauthorized,
	})
	c.Assert(results.Results[2].Error, gc.IsNil)
	c.Assert(results.Results[2].Result, jc.SameContents, []string{
		"cloudcred-meep_bruce_one",
		"cloudcred-meep_bruce_two",
	})
}

func (s *cloudSuite) TestUserCredentialsAdminAccess(c *gc.C) {
	s.setTestAPIForUser(c, names.NewUserTag("admin"))
	results, err := s.api.UserCredentials(params.UserClouds{UserClouds: []params.UserCloud{{
		UserTag:  "user-julia",
		CloudTag: "cloud-meep",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CloudCredentials")
	c.Assert(results.Results, gc.HasLen, 1)
	// admin can access others' credentials
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *cloudSuite) TestUpdateCredentials(c *gc.C) {
	s.backend.SetErrors(nil, errors.NotFoundf("cloud"))
	s.setTestAPIForUser(c, names.NewUserTag("bruce"))
	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag: "machine-0",
		}, {
			Tag: "cloudcred-meep_admin_whatever",
		}, {
			Tag: "cloudcred-meep_bruce_three",
			Credential: params.CloudCredential{
				AuthType:   "oauth1",
				Attributes: map[string]string{"token": "foo:bar:baz"},
			},
		}, {
			Tag: "cloudcred-badcloud_bruce_three",
			Credential: params.CloudCredential{
				AuthType:   "oauth1",
				Attributes: map[string]string{"token": "foo:bar:baz"},
			},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CredentialModels", "UpdateCloudCredential", "CredentialModels", "UpdateCloudCredential")

	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{
			{
				CredentialTag: "machine-0",
				Error:         &params.Error{Message: `"machine-0" is not a valid cloudcred tag`},
			},
			{
				CredentialTag: "cloudcred-meep_admin_whatever",
				Error:         &params.Error{Message: "permission denied", Code: params.CodeUnauthorized},
			},
			{CredentialTag: "cloudcred-meep_bruce_three"},
			{
				CredentialTag: "cloudcred-badcloud_bruce_three",
				Error:         &params.Error{Message: `cannot update credential "three": controller does not manage cloud "badcloud"`},
			},
		},
	})

	s.backend.CheckCall(
		c, 2, "UpdateCloudCredential",
		names.NewCloudCredentialTag("meep/bruce/three"),
		cloud.NewCredential(
			cloud.OAuth1AuthType,
			map[string]string{"token": "foo:bar:baz"},
		),
	)
}

func (s *cloudSuite) TestCheckCredentialsModels(c *gc.C) {
	// Most of the actual validation functionality is tested by other tests in the suite.
	// All we need to know is that this call does not actually update existing controller credential content.
	s.backend.SetErrors(nil, errors.NotFoundf("cloud"))
	s.setTestAPIForUser(c, names.NewUserTag("bruce"))
	results, err := s.api.CheckCredentialsModels(params.TaggedCredentials{Credentials: []params.TaggedCredential{{
		Tag: "cloudcred-meep_bruce_three",
		Credential: params.CloudCredential{
			AuthType:   "oauth1",
			Attributes: map[string]string{"token": "foo:bar:baz"},
		},
	}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CredentialModels")

	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{
			{CredentialTag: "cloudcred-meep_bruce_three"},
		},
	})
}

func (s *cloudSuite) TestUpdateCredentialsAdminAccess(c *gc.C) {
	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CredentialModels", "UpdateCloudCredential")
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{CredentialTag: "cloudcred-meep_julia_three"}}})
}

func (s *cloudSuite) TestUpdateCredentialsNoModelsFound(c *gc.C) {
	s.backend.credentialModelsF = func(tag names.CloudCredentialTag) (map[string]string, error) {
		return nil, errors.NotFoundf("how about it")
	}
	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CredentialModels", "UpdateCloudCredential")
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{CredentialTag: "cloudcred-meep_julia_three"}}})
}

func (s *cloudSuite) TestUpdateCredentialsModelsError(c *gc.C) {
	s.backend.credentialModelsF = func(tag names.CloudCredentialTag) (map[string]string, error) {
		return nil, errors.New("cannot get models")
	}
	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CredentialModels")
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{
			{
				CredentialTag: "cloudcred-meep_julia_three",
				Error:         &params.Error{Message: "cannot get models"},
			},
		}})
}

func (s *cloudSuite) TestUpdateCredentialsModelsErrorForce(c *gc.C) {
	s.backend.credentialModelsF = func(tag names.CloudCredentialTag) (map[string]string, error) {
		return nil, errors.New("cannot get models")
	}
	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: true,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CredentialModels", "UpdateCloudCredential")
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{
			{
				CredentialTag: "cloudcred-meep_julia_three",
				Error:         &params.Error{Message: "cannot get models"},
			},
		}})
}

func (s *cloudSuite) TestUpdateCredentialsOneModelSuccess(c *gc.C) {
	s.backend.credentialModelsF = func(tag names.CloudCredentialTag) (map[string]string, error) {
		return map[string]string{
			coretesting.ModelTag.Id(): "testModel1",
		}, nil
	}

	s.PatchValue(cloudfacade.ValidateNewCredentialForModelFunc, func(backend credentialcommon.PersistentBackend, callCtx context.ProviderCallContext, credentialTag names.CloudCredentialTag, credential *cloud.Credential) (params.ErrorResults, error) {
		return params.ErrorResults{}, nil
	})

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CredentialModels", "UpdateCloudCredential")
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{
			CredentialTag: "cloudcred-meep_julia_three",
			Models: []params.UpdateCredentialModelResult{
				{
					ModelUUID: "deadbeef-0bad-400d-8000-4b1d0d06f00d",
					ModelName: "testModel1",
				},
			},
		}},
	})
}

func (s *cloudSuite) TestUpdateCredentialsModelGetError(c *gc.C) {
	s.backend.credentialModelsF = func(tag names.CloudCredentialTag) (map[string]string, error) {
		return map[string]string{
			coretesting.ModelTag.Id(): "testModel1",
		}, nil
	}
	s.statePool.getF = func(modelUUID string) (cloudfacade.PooledModelBackend, error) {
		return nil, errors.New("cannot get a model")
	}

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CredentialModels")
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{
			CredentialTag: "cloudcred-meep_julia_three",
			Error:         &params.Error{Message: "some models are no longer visible"},
			Models: []params.UpdateCredentialModelResult{
				{
					ModelUUID: "deadbeef-0bad-400d-8000-4b1d0d06f00d",
					ModelName: "testModel1",
					Errors:    []params.ErrorResult{{Error: &params.Error{Message: "cannot get a model", Code: ""}}},
				},
			},
		}},
	})
}

func (s *cloudSuite) TestUpdateCredentialsModelGetErrorForce(c *gc.C) {
	s.backend.credentialModelsF = func(tag names.CloudCredentialTag) (map[string]string, error) {
		return map[string]string{
			coretesting.ModelTag.Id(): "testModel1",
		}, nil
	}
	s.statePool.getF = func(modelUUID string) (cloudfacade.PooledModelBackend, error) {
		return nil, errors.New("cannot get a model")
	}

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: true,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CredentialModels", "UpdateCloudCredential")
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{
			CredentialTag: "cloudcred-meep_julia_three",
			Error:         &params.Error{Message: "some models are no longer visible"},
			Models: []params.UpdateCredentialModelResult{
				{
					ModelUUID: "deadbeef-0bad-400d-8000-4b1d0d06f00d",
					ModelName: "testModel1",
					Errors:    []params.ErrorResult{{Error: &params.Error{Message: "cannot get a model", Code: ""}}},
				},
			},
		}},
	})
}

func (s *cloudSuite) TestUpdateCredentialsModelFailedValidation(c *gc.C) {
	s.backend.credentialModelsF = func(tag names.CloudCredentialTag) (map[string]string, error) {
		return map[string]string{
			coretesting.ModelTag.Id(): "testModel1",
		}, nil
	}

	s.PatchValue(cloudfacade.ValidateNewCredentialForModelFunc, func(backend credentialcommon.PersistentBackend, callCtx context.ProviderCallContext, credentialTag names.CloudCredentialTag, credential *cloud.Credential) (params.ErrorResults, error) {
		return params.ErrorResults{[]params.ErrorResult{{&params.Error{Message: "not valid for model"}}}}, nil
	})

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CredentialModels")
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{
			CredentialTag: "cloudcred-meep_julia_three",
			Error:         &params.Error{Message: "some models are no longer visible"},
			Models: []params.UpdateCredentialModelResult{
				{
					ModelUUID: coretesting.ModelTag.Id(),
					ModelName: "testModel1",
					Errors:    []params.ErrorResult{{Error: &params.Error{Message: "not valid for model", Code: ""}}},
				},
			},
		}},
	})
}

func (s *cloudSuite) TestUpdateCredentialsModelFailedValidationForce(c *gc.C) {
	s.backend.credentialModelsF = func(tag names.CloudCredentialTag) (map[string]string, error) {
		return map[string]string{
			coretesting.ModelTag.Id(): "testModel1",
		}, nil
	}

	s.PatchValue(cloudfacade.ValidateNewCredentialForModelFunc, func(backend credentialcommon.PersistentBackend, callCtx context.ProviderCallContext, credentialTag names.CloudCredentialTag, credential *cloud.Credential) (params.ErrorResults, error) {
		return params.ErrorResults{[]params.ErrorResult{{&params.Error{Message: "not valid for model"}}}}, nil
	})

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: true,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CredentialModels", "UpdateCloudCredential")
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{
			CredentialTag: "cloudcred-meep_julia_three",
			Error:         &params.Error{Message: "some models are no longer visible"},
			Models: []params.UpdateCredentialModelResult{
				{
					ModelUUID: coretesting.ModelTag.Id(),
					ModelName: "testModel1",
					Errors:    []params.ErrorResult{{Error: &params.Error{Message: "not valid for model", Code: ""}}},
				},
			},
		}},
	})
}

func (s *cloudSuite) TestUpdateCredentialsSomeModelsFailedValidation(c *gc.C) {
	s.backend.credentialModelsF = func(tag names.CloudCredentialTag) (map[string]string, error) {
		return map[string]string{
			coretesting.ModelTag.Id():              "testModel1",
			"deadbeef-2f18-4fd2-967d-db9663db7bea": "testModel2",
		}, nil
	}

	s.PatchValue(cloudfacade.ValidateNewCredentialForModelFunc, func(backend credentialcommon.PersistentBackend, callCtx context.ProviderCallContext, credentialTag names.CloudCredentialTag, credential *cloud.Credential) (params.ErrorResults, error) {
		if backend.(*mockModelBackend).uuid == "deadbeef-0bad-400d-8000-4b1d0d06f00d" {
			return params.ErrorResults{[]params.ErrorResult{{&params.Error{Message: "not valid for model"}}}}, nil
		}
		return params.ErrorResults{[]params.ErrorResult{}}, nil
	})

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CredentialModels")
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{
			CredentialTag: "cloudcred-meep_julia_three",
			Error:         &params.Error{Message: "some models are no longer visible"},
			Models: []params.UpdateCredentialModelResult{{
				ModelUUID: "deadbeef-0bad-400d-8000-4b1d0d06f00d",
				ModelName: "testModel1",
				Errors: []params.ErrorResult{{
					Error: &params.Error{Message: "not valid for model", Code: ""},
				}},
			}, {
				ModelUUID: "deadbeef-2f18-4fd2-967d-db9663db7bea",
				ModelName: "testModel2",
			}},
		}},
	})
}

func (s *cloudSuite) TestUpdateCredentialsSomeModelsFailedValidationForce(c *gc.C) {
	s.backend.credentialModelsF = func(tag names.CloudCredentialTag) (map[string]string, error) {
		return map[string]string{
			coretesting.ModelTag.Id():              "testModel1",
			"deadbeef-2f18-4fd2-967d-db9663db7bea": "testModel2",
		}, nil
	}

	s.PatchValue(cloudfacade.ValidateNewCredentialForModelFunc, func(backend credentialcommon.PersistentBackend, callCtx context.ProviderCallContext, credentialTag names.CloudCredentialTag, credential *cloud.Credential) (params.ErrorResults, error) {
		if backend.(*mockModelBackend).uuid == "deadbeef-0bad-400d-8000-4b1d0d06f00d" {
			return params.ErrorResults{[]params.ErrorResult{{&params.Error{Message: "not valid for model"}}}}, nil
		}
		return params.ErrorResults{[]params.ErrorResult{}}, nil
	})

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: true,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CredentialModels", "UpdateCloudCredential")
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{
			{
				CredentialTag: "cloudcred-meep_julia_three",
				Error:         &params.Error{Message: "some models are no longer visible"},
				Models: []params.UpdateCredentialModelResult{
					{
						ModelUUID: "deadbeef-0bad-400d-8000-4b1d0d06f00d",
						ModelName: "testModel1",
						Errors:    []params.ErrorResult{{Error: &params.Error{Message: "not valid for model", Code: ""}}},
					},
					{
						ModelUUID: "deadbeef-2f18-4fd2-967d-db9663db7bea",
						ModelName: "testModel2",
					},
				},
			},
		},
	})
}

func (s *cloudSuite) TestUpdateCredentialsAllModelsFailedValidation(c *gc.C) {
	s.backend.credentialModelsF = func(tag names.CloudCredentialTag) (map[string]string, error) {
		return map[string]string{
			coretesting.ModelTag.Id():              "testModel1",
			"deadbeef-2f18-4fd2-967d-db9663db7bea": "testModel2",
		}, nil
	}

	s.PatchValue(cloudfacade.ValidateNewCredentialForModelFunc, func(backend credentialcommon.PersistentBackend, callCtx context.ProviderCallContext, credentialTag names.CloudCredentialTag, credential *cloud.Credential) (params.ErrorResults, error) {
		return params.ErrorResults{[]params.ErrorResult{{&params.Error{Message: "not valid for model"}}}}, nil
	})

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CredentialModels")
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{
			CredentialTag: "cloudcred-meep_julia_three",
			Error:         &params.Error{Message: "some models are no longer visible"},
			Models: []params.UpdateCredentialModelResult{
				{
					ModelUUID: coretesting.ModelTag.Id(),
					ModelName: "testModel1",
					Errors:    []params.ErrorResult{{Error: &params.Error{Message: "not valid for model"}}},
				},
				{
					ModelUUID: "deadbeef-2f18-4fd2-967d-db9663db7bea",
					ModelName: "testModel2",
					Errors:    []params.ErrorResult{{Error: &params.Error{Message: "not valid for model"}}},
				},
			},
		}}},
	)
}

func (s *cloudSuite) TestUpdateCredentialsAllModelsFailedValidationForce(c *gc.C) {
	s.backend.credentialModelsF = func(tag names.CloudCredentialTag) (map[string]string, error) {
		return map[string]string{
			coretesting.ModelTag.Id():              "testModel1",
			"deadbeef-2f18-4fd2-967d-db9663db7bea": "testModel2",
		}, nil
	}

	s.PatchValue(cloudfacade.ValidateNewCredentialForModelFunc, func(backend credentialcommon.PersistentBackend, callCtx context.ProviderCallContext, credentialTag names.CloudCredentialTag, credential *cloud.Credential) (params.ErrorResults, error) {
		return params.ErrorResults{[]params.ErrorResult{{&params.Error{Message: "not valid for model"}}}}, nil
	})

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: true,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CredentialModels", "UpdateCloudCredential")
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{
			CredentialTag: "cloudcred-meep_julia_three",
			Error:         &params.Error{Message: "some models are no longer visible"},
			Models: []params.UpdateCredentialModelResult{
				{
					ModelUUID: coretesting.ModelTag.Id(),
					ModelName: "testModel1",
					Errors:    []params.ErrorResult{{Error: &params.Error{Message: "not valid for model"}}},
				},
				{
					ModelUUID: "deadbeef-2f18-4fd2-967d-db9663db7bea",
					ModelName: "testModel2",
					Errors:    []params.ErrorResult{{Error: &params.Error{Message: "not valid for model"}}},
				},
			},
		}}},
	)
}

func (s *cloudSuite) TestRevokeCredentials(c *gc.C) {
	s.setTestAPIForUser(c, names.NewUserTag("bruce"))
	results, err := s.api.RevokeCredentialsCheckModels(params.RevokeCredentialArgs{
		Credentials: []params.RevokeCredentialArg{
			{Tag: "machine-0"},
			{Tag: "cloudcred-meep_admin_whatever"},
			{Tag: "cloudcred-meep_bruce_three"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CredentialModels", "RemoveCloudCredential")
	c.Assert(results.Results, gc.HasLen, 3)
	c.Assert(results.Results[0].Error, jc.DeepEquals, &params.Error{
		Message: `"machine-0" is not a valid cloudcred tag`,
	})
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: "permission denied", Code: params.CodeUnauthorized,
	})
	c.Assert(results.Results[2].Error, gc.IsNil)

	s.backend.CheckCall(
		c, 2, "RemoveCloudCredential",
		names.NewCloudCredentialTag("meep/bruce/three"),
	)
}

func (s *cloudSuite) TestRevokeCredentialsAdminAccess(c *gc.C) {
	results, err := s.api.RevokeCredentialsCheckModels(params.RevokeCredentialArgs{
		Credentials: []params.RevokeCredentialArg{
			{Tag: "cloudcred-meep_julia_three"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CredentialModels", "RemoveCloudCredential")
	c.Assert(results.Results, gc.HasLen, 1)
	// admin can revoke others' credentials
	c.Assert(results.Results[0].Error, gc.IsNil)
}

type revokeCredentialData struct {
	f           credentialModelFunction
	args        []params.RevokeCredentialArg
	results     params.ErrorResults
	expectedLog []string
	callsMade   []string
}

func (s *cloudSuite) assertRevokeCredentials(c *gc.C, test revokeCredentialData) {
	s.backend.credentialModelsF = test.f
	results, err := s.api.RevokeCredentialsCheckModels(params.RevokeCredentialArgs{test.args})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, test.callsMade...)
	c.Assert(results, gc.DeepEquals, test.results)
	for _, l := range test.expectedLog {
		c.Assert(c.GetTestLog(), jc.Contains, l)
	}
}

func (s *cloudSuite) TestRevokeCredentialsCantGetModels(c *gc.C) {
	t := revokeCredentialData{
		f: func(tag names.CloudCredentialTag) (map[string]string, error) {
			return nil, errors.New("no niet nope")
		},
		args: []params.RevokeCredentialArg{
			{Tag: "cloudcred-meep_julia_three"},
		},
		callsMade: []string{"ControllerTag", "CredentialModels"},
		results: params.ErrorResults{
			Results: []params.ErrorResult{
				{common.ServerError(errors.New("no niet nope"))},
			},
		},
		expectedLog: []string{},
	}
	s.assertRevokeCredentials(c, t)
}

func (s *cloudSuite) TestRevokeCredentialsForceCantGetModels(c *gc.C) {
	t := revokeCredentialData{
		f: func(tag names.CloudCredentialTag) (map[string]string, error) {
			return nil, errors.New("no niet nope")
		},
		args: []params.RevokeCredentialArg{
			{Tag: "cloudcred-meep_julia_three", Force: true},
		},
		callsMade: []string{"ControllerTag", "CredentialModels", "RemoveCloudCredential"},
		results: params.ErrorResults{
			Results: []params.ErrorResult{
				{}, // no error: credential deleted
			},
		},
		expectedLog: []string{" WARNING juju.apiserver.cloud could not get models that use credential cloudcred-meep_julia_three: no niet nope"},
	}
	s.assertRevokeCredentials(c, t)
}

func (s *cloudSuite) TestRevokeCredentialsHasModel(c *gc.C) {
	t := revokeCredentialData{
		f: func(tag names.CloudCredentialTag) (map[string]string, error) {
			return map[string]string{
				coretesting.ModelTag.Id(): "modelName",
			}, nil
		},
		args: []params.RevokeCredentialArg{
			{Tag: "cloudcred-meep_julia_three"},
		},
		callsMade: []string{"ControllerTag", "CredentialModels"},
		results: params.ErrorResults{
			Results: []params.ErrorResult{
				{common.ServerError(errors.New("cannot delete credential cloudcred-meep_julia_three: it is still used by 1 model"))},
			},
		},
		expectedLog: []string{" WARNING juju.apiserver.cloud credential cloudcred-meep_julia_three cannot be deleted as it is used by model deadbeef-0bad-400d-8000-4b1d0d06f00d"},
	}
	s.assertRevokeCredentials(c, t)
}

func (s *cloudSuite) TestRevokeCredentialsHasModels(c *gc.C) {
	t := revokeCredentialData{
		f: func(tag names.CloudCredentialTag) (map[string]string, error) {
			return map[string]string{
				coretesting.ModelTag.Id():              "modelName",
				"deadbeef-1bad-511d-8000-4b1d0d06f00d": "anotherModelName",
			}, nil
		},
		args: []params.RevokeCredentialArg{
			{Tag: "cloudcred-meep_julia_three"},
		},
		callsMade: []string{"ControllerTag", "CredentialModels"},
		results: params.ErrorResults{
			Results: []params.ErrorResult{
				{common.ServerError(errors.New("cannot delete credential cloudcred-meep_julia_three: it is still used by 2 models"))},
			},
		},
		expectedLog: []string{` WARNING juju.apiserver.cloud credential cloudcred-meep_julia_three cannot be deleted as it is used by models:
- deadbeef-0bad-400d-8000-4b1d0d06f00d
- deadbeef-1bad-511d-8000-4b1d0d06f00d
`},
	}
	s.assertRevokeCredentials(c, t)
}

func (s *cloudSuite) TestRevokeCredentialsForceHasModel(c *gc.C) {
	t := revokeCredentialData{
		f: func(tag names.CloudCredentialTag) (map[string]string, error) {
			return map[string]string{
				coretesting.ModelTag.Id(): "modelName",
			}, nil
		},

		args: []params.RevokeCredentialArg{
			{Tag: "cloudcred-meep_julia_three", Force: true},
		},
		callsMade: []string{"ControllerTag", "CredentialModels", "RemoveCloudCredential"},
		results: params.ErrorResults{
			Results: []params.ErrorResult{
				{},
			},
		},
		expectedLog: []string{` WARNING juju.apiserver.cloud credential cloudcred-meep_julia_three will be deleted but it is used by model deadbeef-0bad-400d-8000-4b1d0d06f00d`},
	}
	s.assertRevokeCredentials(c, t)
}

func (s *cloudSuite) TestRevokeCredentialsForceMany(c *gc.C) {
	t := revokeCredentialData{
		f: func(tag names.CloudCredentialTag) (map[string]string, error) {
			return map[string]string{
				coretesting.ModelTag.Id(): "modelName",
			}, nil
		},
		args: []params.RevokeCredentialArg{
			{Tag: "cloudcred-meep_julia_three", Force: true},
			{Tag: "cloudcred-meep_bruce_three"},
		},
		callsMade: []string{"ControllerTag", "CredentialModels", "RemoveCloudCredential", "CredentialModels"},
		results: params.ErrorResults{
			Results: []params.ErrorResult{
				{},
				{common.ServerError(errors.New("cannot delete credential cloudcred-meep_bruce_three: it is still used by 1 model"))},
			},
		},
		expectedLog: []string{
			` WARNING juju.apiserver.cloud credential cloudcred-meep_julia_three will be deleted but it is used by model deadbeef-0bad-400d-8000-4b1d0d06f00d`,
			` WARNING juju.apiserver.cloud credential cloudcred-meep_bruce_three cannot be deleted as it is used by model deadbeef-0bad-400d-8000-4b1d0d06f00d`,
		},
	}
	s.assertRevokeCredentials(c, t)
}

func (s *cloudSuite) TestCredential(c *gc.C) {
	s.setTestAPIForUser(c, names.NewUserTag("bruce"))
	results, err := s.api.Credential(params.Entities{Entities: []params.Entity{{
		Tag: "machine-0",
	}, {
		Tag: "cloudcred-meep_admin_foo",
	}, {
		Tag: "cloudcred-meep_bruce_two",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CloudCredentials", "Cloud")
	s.backend.CheckCall(c, 1, "CloudCredentials", names.NewUserTag("bruce"), "meep")

	c.Assert(results.Results, gc.HasLen, 3)
	c.Assert(results.Results[0].Error, jc.DeepEquals, &params.Error{
		Message: `"machine-0" is not a valid cloudcred tag`,
	})
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: "permission denied", Code: params.CodeUnauthorized,
	})
	c.Assert(results.Results[2].Error, gc.IsNil)
	c.Assert(results.Results[2].Result, jc.DeepEquals, &params.CloudCredential{
		AuthType:   "userpass",
		Attributes: map[string]string{"username": "admin"},
		Redacted:   []string{"password"},
	})
}

func (s *cloudSuite) TestCredentialAdminAccess(c *gc.C) {
	results, err := s.api.Credential(params.Entities{Entities: []params.Entity{{
		Tag: "cloudcred-meep_bruce_two",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "CloudCredentials", "Cloud")
	c.Assert(results.Results, gc.HasLen, 1)
	// admin can access others' credentials
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *cloudSuite) TestModifyCloudAccess(c *gc.C) {
	results, err := s.api.ModifyCloudAccess(params.ModifyCloudAccessRequest{
		Changes: []params.ModifyCloudAccess{
			{
				Action:   params.GrantCloudAccess,
				CloudTag: names.NewCloudTag("fluffy").String(),
				UserTag:  names.NewUserTag("fred").String(),
				Access:   "add-model",
			}, {
				Action:   params.RevokeCloudAccess,
				CloudTag: names.NewCloudTag("fluffy").String(),
				UserTag:  names.NewUserTag("mary").String(),
				Access:   "add-model",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Cloud", "ControllerTag", "CreateCloudAccess", "Cloud", "ControllerTag", "RemoveCloudAccess")
	s.backend.CheckCall(c, 2, "CreateCloudAccess", "fluffy", names.NewUserTag("fred"), permission.AddModelAccess)
	s.backend.CheckCall(c, 5, "RemoveCloudAccess", "fluffy", names.NewUserTag("mary"))
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{
		{}, {},
	})
}

func (s *cloudSuite) TestModifyCloudUpdateAccess(c *gc.C) {
	s.backend.cloudAccess = permission.AddModelAccess
	results, err := s.api.ModifyCloudAccess(params.ModifyCloudAccessRequest{
		Changes: []params.ModifyCloudAccess{
			{
				Action:   params.GrantCloudAccess,
				CloudTag: names.NewCloudTag("fluffy").String(),
				UserTag:  names.NewUserTag("fred").String(),
				Access:   "admin",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Cloud", "ControllerTag", "CreateCloudAccess", "GetCloudAccess", "UpdateCloudAccess")
	s.backend.CheckCall(c, 2, "CreateCloudAccess", "fluffy", names.NewUserTag("fred"), permission.AdminAccess)
	s.backend.CheckCall(c, 4, "UpdateCloudAccess", "fluffy", names.NewUserTag("fred"), permission.AdminAccess)
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{
		{},
	})
}

func (s *cloudSuite) TestModifyCloudAlreadyHasAccess(c *gc.C) {
	s.backend.cloudAccess = permission.AdminAccess
	results, err := s.api.ModifyCloudAccess(params.ModifyCloudAccessRequest{
		Changes: []params.ModifyCloudAccess{
			{
				Action:   params.GrantCloudAccess,
				CloudTag: names.NewCloudTag("fluffy").String(),
				UserTag:  names.NewUserTag("fred").String(),
				Access:   "admin",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Cloud", "ControllerTag", "CreateCloudAccess", "GetCloudAccess")
	s.backend.CheckCall(c, 2, "CreateCloudAccess", "fluffy", names.NewUserTag("fred"), permission.AdminAccess)
	s.backend.CheckCall(c, 3, "GetCloudAccess", "fluffy", names.NewUserTag("fred"))
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{
		{Error: &params.Error{Message: `could not grant cloud access: user already has "admin" access or greater`}},
	})
}

type mockBackend struct {
	gitjujutesting.Stub
	cloudfacade.Backend
	cloud       cloud.Cloud
	creds       map[string]state.Credential
	cloudAccess permission.Access

	credentialModelsF func(tag names.CloudCredentialTag) (map[string]string, error)
}

func (st *mockBackend) ControllerTag() names.ControllerTag {
	st.MethodCall(st, "ControllerTag")
	return names.NewControllerTag("deadbeef-1bad-500d-9000-4b1d0d06f00d")
}

func (st *mockBackend) Model() (cloudfacade.Model, error) {
	st.MethodCall(st, "Model")
	return &mockModel{
		cloud: st.cloud.Name,
	}, st.NextErr()
}

func (st *mockBackend) Cloud(name string) (cloud.Cloud, error) {
	st.MethodCall(st, "Cloud", name)
	return st.cloud, st.NextErr()
}

func (st *mockBackend) Clouds() (map[names.CloudTag]cloud.Cloud, error) {
	st.MethodCall(st, "Clouds")
	return map[names.CloudTag]cloud.Cloud{
		names.NewCloudTag("my-cloud"):   st.cloud,
		names.NewCloudTag("your-cloud"): st.cloud,
	}, st.NextErr()
}

func (st *mockBackend) CloudCredentials(user names.UserTag, cloudName string) (map[string]state.Credential, error) {
	st.MethodCall(st, "CloudCredentials", user, cloudName)
	return st.creds, st.NextErr()
}

func (st *mockBackend) UpdateCloudCredential(tag names.CloudCredentialTag, cred cloud.Credential) error {
	st.MethodCall(st, "UpdateCloudCredential", tag, cred)
	return st.NextErr()
}

func (st *mockBackend) RemoveCloudCredential(tag names.CloudCredentialTag) error {
	st.MethodCall(st, "RemoveCloudCredential", tag)
	return st.NextErr()
}

func (st *mockBackend) AddCloud(cloud cloud.Cloud, user string) error {
	st.MethodCall(st, "AddCloud", cloud, user)
	return st.NextErr()
}

func (st *mockBackend) UpdateCloud(cloud cloud.Cloud) error {
	st.MethodCall(st, "UpdateCloud", cloud)
	if cloud.Name == st.cloud.Name {
		return nil
	}
	return st.NextErr()
}

func (st *mockBackend) RemoveCloud(name string) error {
	st.MethodCall(st, "RemoveCloud", name)
	return errors.NewNotImplemented(nil, "This mock is used for v1, so RemoveCloud")
}

func (st *mockBackend) AllCloudCredentials(user names.UserTag) ([]state.Credential, error) {
	st.MethodCall(st, "AllCloudCredentials", user)
	return nil, errors.NewNotImplemented(nil, "This mock is used for v1, so AllCloudCredentials")
}

func (st *mockBackend) CredentialModelsAndOwnerAccess(tag names.CloudCredentialTag) ([]state.CredentialOwnerModelAccess, error) {
	st.MethodCall(st, "CredentialModelsAndOwnerAccess", tag)
	return nil, errors.NewNotImplemented(nil, "This mock is used for v1, so CredentialModelsAndOwnerAccess")
}

func (st *mockBackend) CredentialModels(tag names.CloudCredentialTag) (map[string]string, error) {
	st.MethodCall(st, "CredentialModels", tag)
	return st.credentialModelsF(tag)
}

func (st *mockBackend) GetCloudAccess(cloud string, user names.UserTag) (permission.Access, error) {
	st.MethodCall(st, "GetCloudAccess", cloud, user)
	if cloud == "your-cloud" {
		return permission.NoAccess, errors.NotFoundf("cloud your-cloud")
	}
	return st.cloudAccess, nil
}

func (st *mockBackend) GetCloudUsers(cloud string) (map[string]permission.Access, error) {
	st.MethodCall(st, "GetCloudUsers", cloud)
	return map[string]permission.Access{
		"fred": permission.AddModelAccess,
		"mary": permission.AdminAccess,
	}, nil
}

func (st *mockBackend) CloudsForUser(user names.UserTag, all bool) ([]state.CloudInfo, error) {
	st.MethodCall(st, "CloudsForUser", user, all)
	return []state.CloudInfo{
		{
			Cloud:  st.cloud,
			Access: permission.AddModelAccess,
		},
	}, nil
}

func (st *mockBackend) User(tag names.UserTag) (cloudfacade.User, error) {
	st.MethodCall(st, "User", tag)
	return &mockUser{tag.Name()}, nil
}

func (st *mockBackend) CreateCloudAccess(cloud string, user names.UserTag, access permission.Access) error {
	st.MethodCall(st, "CreateCloudAccess", cloud, user, access)
	if st.cloudAccess != permission.NoAccess {
		return errors.AlreadyExistsf("access %s", access)
	}
	st.cloudAccess = access
	return nil
}

func (st *mockBackend) UpdateCloudAccess(cloud string, user names.UserTag, access permission.Access) error {
	st.MethodCall(st, "UpdateCloudAccess", cloud, user, access)
	st.cloudAccess = access
	return nil
}

func (st *mockBackend) RemoveCloudAccess(cloud string, user names.UserTag) error {
	st.MethodCall(st, "RemoveCloudAccess", cloud, user)
	st.cloudAccess = permission.NoAccess
	return nil
}

type mockUser struct {
	name string
}

func (m *mockUser) DisplayName() string {
	return "display-" + m.name
}

type mockModel struct {
	cloud              string
	cloudRegion        string
	cloudCredentialTag names.CloudCredentialTag
	cfg                *config.Config
}

func (m *mockModel) Cloud() string {
	return m.cloud
}

func (m *mockModel) CloudRegion() string {
	return m.cloudRegion
}

func (m *mockModel) CloudCredential() (names.CloudCredentialTag, bool) {
	return m.cloudCredentialTag, true
}

func (m *mockModel) ValidateCloudCredential(tag names.CloudCredentialTag, credential cloud.Credential) error {
	return nil
}

func (m *mockModel) Config() (*config.Config, error) {
	return m.cfg, nil
}

func (m *mockModel) Type() state.ModelType {
	return state.ModelType("")
}

type mockStatePool struct {
	getF func(modelUUID string) (cloudfacade.PooledModelBackend, error)
}

func (m *mockStatePool) Get(modelUUID string) (cloudfacade.PooledModelBackend, error) {
	return m.getF(modelUUID)
}

func (m *mockStatePool) SystemState() *state.State {
	return nil
}

type mockPooledModel struct {
	release bool
	model   *mockModelBackend
}

func (m *mockPooledModel) Model() credentialcommon.PersistentBackend {
	return m.model
}

func (m *mockPooledModel) Release() bool {
	return m.release
}

type mockModelBackend struct {
	uuid  string
	model *mockModel
	cloud cloud.Cloud
}

func (m *mockModelBackend) Model() (credentialcommon.Model, error) {
	return m.model, nil
}

func (m *mockModelBackend) Cloud(name string) (cloud.Cloud, error) {
	return m.cloud, nil
}

func (m *mockModelBackend) AllMachines() ([]credentialcommon.Machine, error) {
	return nil, nil
}

func (m *mockModelBackend) CloudCredential(tag names.CloudCredentialTag) (state.Credential, error) {
	return state.Credential{}, nil
}
