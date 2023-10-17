// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	stdcontext "context"
	"fmt"
	"regexp"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/facades/client/cloud"
	"github.com/juju/juju/apiserver/facades/client/cloud/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/credential"
	credentialservice "github.com/juju/juju/domain/credential/service"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type cloudSuite struct {
	jujutesting.LoggingCleanupSuite

	userService            *mocks.MockUserService
	modelCredentialService *mocks.MockModelCredentialService
	cloudPermissionService *mocks.MockCloudPermissionService
	cloudService           *mocks.MockCloudService
	credService            *mocks.MockCredentialService
	api                    *cloud.CloudAPI
	authorizer             *apiservertesting.FakeAuthorizer

	credentialValidator credentialcommon.CredentialValidator
}

func (s *cloudSuite) setup(c *gc.C, userTag names.UserTag) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: userTag,
	}
	s.userService = mocks.NewMockUserService(ctrl)
	s.modelCredentialService = mocks.NewMockModelCredentialService(ctrl)
	s.cloudPermissionService = mocks.NewMockCloudPermissionService(ctrl)
	s.cloudService = mocks.NewMockCloudService(ctrl)
	s.credService = mocks.NewMockCredentialService(ctrl)
	s.credentialValidator = mocks.NewMockCredentialValidator(ctrl)

	api, err := cloud.NewCloudAPI(
		coretesting.ControllerTag, "dummy",
		s.userService, s.modelCredentialService,
		s.cloudService, s.cloudPermissionService, s.credService,
		s.authorizer, loggo.GetLogger("juju.apiserver.cloud"))
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
	return ctrl
}

var _ = gc.Suite(&cloudSuite{})

func (s *cloudSuite) TestCloud(c *gc.C) {
	defer s.setup(c, names.NewUserTag("admin")).Finish()

	backend := s.cloudService.EXPECT()
	backend.Get(gomock.Any(), "my-cloud").Return(&jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}, nil)

	results, err := s.api.Cloud(stdcontext.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "cloud-my-cloud"}, {Tag: "machine-0"}},
	})
	c.Assert(err, jc.ErrorIsNil)
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
	defer s.setup(c, names.NewUserTag("admin")).Finish()

	backend := s.cloudService.EXPECT()
	backend.Get(gomock.Any(), "no-dice").Return(&jujucloud.Cloud{}, errors.NotFoundf("cloud \"no-dice\""))

	results, err := s.api.Cloud(stdcontext.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "cloud-no-dice"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, "cloud \"no-dice\" not found")
}

func (s *cloudSuite) TestClouds(c *gc.C) {
	bruce := names.NewUserTag("bruce")
	defer s.setup(c, bruce).Finish()

	cloudPermissionService := s.cloudPermissionService.EXPECT()

	cloudPermissionService.GetCloudAccess("my-cloud",
		bruce).Return(permission.AddModelAccess, nil)
	cloudPermissionService.GetCloudAccess("your-cloud",
		bruce).Return(permission.NoAccess, nil)

	backend := s.cloudService.EXPECT()
	backend.ListAll(gomock.Any()).Return([]jujucloud.Cloud{
		{
			Name:      "my-cloud",
			Type:      "dummy",
			AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
			Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
		}, {
			Name:      "your-cloud",
			Type:      "dummy",
			AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
			Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
		},
	}, nil)

	result, err := s.api.Clouds(stdcontext.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Clouds, jc.DeepEquals, map[string]params.Cloud{
		"cloud-my-cloud": {
			Type:      "dummy",
			AuthTypes: []string{"empty", "userpass"},
			Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
		},
	})
}

func (s *cloudSuite) TestCloudInfoAdmin(c *gc.C) {
	ctrl := s.setup(c, names.NewUserTag("admin"))
	defer ctrl.Finish()

	cloudPermissionService := s.cloudPermissionService.EXPECT()
	userPerm := map[string]permission.Access{"fred": permission.AddModelAccess,
		"mary": permission.AdminAccess}
	cloudPermissionService.GetCloudUsers("my-cloud").Return(userPerm,
		nil)

	cloudService := s.cloudService.EXPECT()
	cloudService.Get(gomock.Any(), "my-cloud").Return(&jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}, nil)

	mary := mocks.NewMockUser(ctrl)
	fred := mocks.NewMockUser(ctrl)
	mary.EXPECT().DisplayName().Return("display-mary")
	fred.EXPECT().DisplayName().Return("display-fred")

	userService := s.userService.EXPECT()
	maryTag := names.NewUserTag("mary")
	userService.User(maryTag).Return(mary, nil)
	fredTag := names.NewUserTag("fred")
	userService.User(fredTag).Return(fred, nil)

	result, err := s.api.CloudInfo(stdcontext.Background(), params.Entities{Entities: []params.Entity{{
		Tag: "cloud-my-cloud",
	}, {
		Tag: "machine-0",
	}}})
	c.Assert(err, jc.ErrorIsNil)
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
	fredTag := names.NewUserTag("fred")
	ctrl := s.setup(c, fredTag)
	defer ctrl.Finish()

	fred := mocks.NewMockUser(ctrl)
	fred.EXPECT().DisplayName().Return("display-fred")

	cloudPermissionService := s.cloudPermissionService.EXPECT()
	cloudPermissionService.GetCloudAccess("my-cloud",
		fredTag).Return(permission.AddModelAccess, nil)
	userPerm := map[string]permission.Access{"fred": permission.AddModelAccess,
		"mary": permission.AdminAccess}
	cloudPermissionService.GetCloudUsers("my-cloud").Return(userPerm,
		nil)

	userService := s.userService.EXPECT()
	userService.User(fredTag).Return(fred, nil)
	s.cloudService.EXPECT().Get(gomock.Any(), "my-cloud").Return(&jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}, nil)

	result, err := s.api.CloudInfo(stdcontext.Background(), params.Entities{Entities: []params.Entity{{
		Tag: "cloud-my-cloud",
	}, {
		Tag: "machine-0",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 2)
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
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	newCloud := jujucloud.Cloud{
		Name:      "newcloudname",
		Type:      "maas",
		Endpoint:  "fake-endpoint",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "nether-endpoint"}},
	}

	cloud := jujucloud.Cloud{
		Name: "newcloudname",
		Type: "maas",
	}

	cloudPermissionService := s.cloudPermissionService.EXPECT()
	cloudPermissionService.CreateCloudAccess("newcloudname", adminTag, permission.AdminAccess).Return(nil)
	cloudservice := s.cloudService.EXPECT()
	cloudservice.Get(gomock.Any(), "dummy").Return(&cloud, nil)
	cloudservice.Save(gomock.Any(), newCloud).Return(nil)
	paramsCloud := params.AddCloudArgs{
		Name: "newcloudname",
		Cloud: params.Cloud{
			Type:      "maas",
			AuthTypes: []string{"empty", "userpass"},
			Endpoint:  "fake-endpoint",
			Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "nether-endpoint"}},
		}}

	err := s.api.AddCloud(stdcontext.Background(), paramsCloud)
	c.Assert(err, jc.ErrorIsNil)
}

func createAddCloudParam(cloudType string) params.AddCloudArgs {
	if cloudType == "" {
		cloudType = "fake"
	}
	return params.AddCloudArgs{
		Name: "newcloudname",
		Cloud: params.Cloud{
			Type:      cloudType,
			AuthTypes: []string{"empty", "userpass"},
			Endpoint:  "fake-endpoint",
			Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "nether-endpoint"}},
		},
	}
}

func (s *cloudSuite) TestAddCloudNotWhitelisted(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	cloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	s.cloudService.EXPECT().Get(gomock.Any(), "dummy").Return(&cloud, nil)

	err := s.api.AddCloud(stdcontext.Background(), createAddCloudParam(""))
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`
controller cloud type "dummy" is not whitelisted, current whitelist: 
 - controller cloud type "kubernetes" supports [lxd maas openstack]
 - controller cloud type "lxd" supports [lxd maas openstack]
 - controller cloud type "maas" supports [maas openstack]
 - controller cloud type "openstack" supports [openstack]`[1:]))
}

func (s *cloudSuite) TestAddCloudNotWhitelistedButForceAdded(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	newCloud := jujucloud.Cloud{
		Name:      "newcloudname",
		Type:      "fake",
		Endpoint:  "fake-endpoint",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "nether-endpoint"}},
	}

	cloud := jujucloud.Cloud{
		Name: "newcloudname",
		Type: "maas",
	}

	s.cloudPermissionService.EXPECT().CreateCloudAccess("newcloudname", adminTag, permission.AdminAccess).Return(nil)
	cloudService := s.cloudService.EXPECT()
	cloudService.Get(gomock.Any(), "dummy").Return(&cloud, nil)
	cloudService.Save(gomock.Any(), newCloud).Return(nil)

	force := true
	addCloudArg := createAddCloudParam("")
	addCloudArg.Force = &force
	err := s.api.AddCloud(stdcontext.Background(), addCloudArg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cloudSuite) TestAddCloudControllerCloudErr(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	s.cloudService.EXPECT().Get(gomock.Any(), "dummy").Return(&jujucloud.Cloud{}, errors.New("kaboom"))

	err := s.api.AddCloud(stdcontext.Background(), createAddCloudParam(""))
	c.Assert(err, gc.ErrorMatches, "kaboom")
}

func (s *cloudSuite) TestAddCloudK8sForceIrrelevant(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	cloud := jujucloud.Cloud{
		Name:      "newcloudname",
		Type:      string(k8sconstants.CAASProviderType),
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Endpoint:  "fake-endpoint",
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "nether-endpoint"}},
	}

	s.cloudService.EXPECT().Save(gomock.Any(), cloud).Return(nil).Times(2)
	s.cloudPermissionService.EXPECT().CreateCloudAccess("newcloudname", adminTag, permission.AdminAccess).Return(nil).Times(2)

	addCloudArg := createAddCloudParam(string(k8sconstants.CAASProviderType))

	add := func() {
		err := s.api.AddCloud(stdcontext.Background(), addCloudArg)
		c.Assert(err, jc.ErrorIsNil)
	}
	add()
	force := true
	addCloudArg.Force = &force
	add()
}

func (s *cloudSuite) TestAddCloudNoRegion(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	newCloud := jujucloud.Cloud{
		Name:      "newcloudname",
		Type:      "maas",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Endpoint:  "fake-endpoint",
		Regions: []jujucloud.Region{{
			Name: "default",
		}},
	}

	cloud := jujucloud.Cloud{
		Name: "newcloudname",
		Type: "maas",
	}

	s.cloudPermissionService.EXPECT().CreateCloudAccess("newcloudname", adminTag, permission.AdminAccess).Return(nil)
	cloudService := s.cloudService.EXPECT()
	cloudService.Get(gomock.Any(), "dummy").Return(&cloud, nil)
	cloudService.Save(gomock.Any(), newCloud).Return(nil)
	paramsCloud := params.AddCloudArgs{
		Name: "newcloudname",
		Cloud: params.Cloud{
			Type:      "maas",
			AuthTypes: []string{"empty", "userpass"},
			Endpoint:  "fake-endpoint",
		}}
	err := s.api.AddCloud(stdcontext.Background(), paramsCloud)
	c.Assert(err, jc.ErrorIsNil)

}

func (s *cloudSuite) TestAddCloudNoAdminPerms(c *gc.C) {
	frankTag := names.NewUserTag("frank")
	defer s.setup(c, frankTag).Finish()

	paramsCloud := params.AddCloudArgs{
		Name: "newcloudname",
		Cloud: params.Cloud{
			Type:      "fake",
			AuthTypes: []string{"empty", "userpass"},
			Endpoint:  "fake-endpoint",
			Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "nether-endpoint"}},
		}}
	err := s.api.AddCloud(stdcontext.Background(), paramsCloud)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *cloudSuite) TestUpdateCloud(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	dummyCloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether-updated", Endpoint: "endpoint-updated"}},
	}

	s.cloudService.EXPECT().Save(gomock.Any(), dummyCloud).Return(nil)

	updatedCloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether-updated", Endpoint: "endpoint-updated"}},
	}
	results, err := s.api.UpdateCloud(stdcontext.Background(), params.UpdateCloudArgs{
		Clouds: []params.AddCloudArgs{{
			Name:  "dummy",
			Cloud: cloud.CloudToParams(updatedCloud),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *cloudSuite) TestUpdateCloudNonAdminPerm(c *gc.C) {
	frankTag := names.NewUserTag("frank")
	defer s.setup(c, frankTag).Finish()

	updatedCloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether-updated", Endpoint: "endpoint-updated"}},
	}
	results, err := s.api.UpdateCloud(stdcontext.Background(), params.UpdateCloudArgs{
		Clouds: []params.AddCloudArgs{{
			Name:  "dummy",
			Cloud: cloud.CloudToParams(updatedCloud),
		}},
	})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *cloudSuite) TestUpdateNonExistentCloud(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	dummyCloud := jujucloud.Cloud{
		Name:      "nope",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether-updated", Endpoint: "endpoint-updated"}},
	}

	s.cloudService.EXPECT().Save(gomock.Any(), dummyCloud).Return(errors.New("cloud \"nope\" not found"))

	updatedCloud := jujucloud.Cloud{
		Name:      "nope",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether-updated", Endpoint: "endpoint-updated"}},
	}

	results, err := s.api.UpdateCloud(stdcontext.Background(), params.UpdateCloudArgs{
		Clouds: []params.AddCloudArgs{{
			Name:  "nope",
			Cloud: cloud.CloudToParams(updatedCloud),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, fmt.Sprintf("cloud %q not found", updatedCloud.Name))
}

func (s *cloudSuite) TestListCloudInfo(c *gc.C) {
	fredTag := names.NewUserTag("admin")
	defer s.setup(c, fredTag).Finish()

	s.cloudService.EXPECT().ListAll(gomock.Any()).Return([]jujucloud.Cloud{
		{
			Name:      "my-cloud",
			Type:      "dummy",
			AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
			Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
		},
	}, nil)

	result, err := s.api.ListCloudInfo(stdcontext.Background(), params.ListCloudsRequest{
		UserTag: "user-admin",
		All:     true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, []params.ListCloudInfoResult{
		{
			Result: &params.ListCloudInfo{
				CloudDetails: params.CloudDetails{
					Type:      "dummy",
					AuthTypes: []string{"empty", "userpass"},
					Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
				},
				Access: "admin",
			},
		},
	})
}

func (s *cloudSuite) TestUserCredentials(c *gc.C) {
	bruceTag := names.NewUserTag("bruce")
	defer s.setup(c, bruceTag).Finish()

	credentialOne, tagOne := cloudCredentialTag(credParams{name: "one", owner: "bruce", cloudName: "meep", authType: jujucloud.EmptyAuthType,
		attrs: map[string]string{}})
	credentialTwo, tagTwo := cloudCredentialTag(credParams{name: "two", owner: "bruce", cloudName: "meep", authType: jujucloud.UserPassAuthType,
		attrs: map[string]string{
			"username": "admin",
			"password": "adm1n",
		}})

	creds := map[string]jujucloud.Credential{
		tagOne.Id(): credentialOne,
		tagTwo.Id(): credentialTwo,
	}

	s.credService.EXPECT().CloudCredentialsForOwner(gomock.Any(), bruceTag.Id(), "meep").Return(creds, nil)

	results, err := s.api.UserCredentials(stdcontext.Background(), params.UserClouds{UserClouds: []params.UserCloud{{
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
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	julia := names.NewUserTag("julia")
	s.credService.EXPECT().CloudCredentialsForOwner(gomock.Any(), julia.Id(), "meep").Return(map[string]jujucloud.Credential{}, nil)

	results, err := s.api.UserCredentials(stdcontext.Background(), params.UserClouds{UserClouds: []params.UserCloud{{
		UserTag:  "user-julia",
		CloudTag: "cloud-meep",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	// admin can access others' credentials
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *cloudSuite) TestUpdateCredentials(c *gc.C) {
	bruceTag := names.NewUserTag("bruce")
	defer s.setup(c, bruceTag).Finish()

	_, tagOne := cloudCredentialTag(credParams{name: "three", owner: "bruce", cloudName: "meep", authType: jujucloud.EmptyAuthType,
		attrs: map[string]string{}})
	_, tagTwo := cloudCredentialTag(credParams{name: "three", owner: "bruce", cloudName: "badcloud", authType: jujucloud.EmptyAuthType,
		attrs: map[string]string{}})

	cred := jujucloud.NewCredential(
		jujucloud.OAuth1AuthType,
		map[string]string{"token": "foo:bar:baz"},
	)
	s.credService.EXPECT().CheckAndUpdateCredential(gomock.Any(), credential.IdFromTag(tagTwo), cred, false).Return(
		nil, errors.New("cannot update credential \"three\": controller does not manage cloud \"badcloud\""))
	s.credService.EXPECT().CheckAndUpdateCredential(gomock.Any(), credential.IdFromTag(tagOne), cred, false).Return(
		[]credentialservice.UpdateCredentialModelResult{}, nil)

	results, err := s.api.UpdateCredentialsCheckModels(stdcontext.Background(), params.UpdateCredentialArgs{
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
}

func (s *cloudSuite) TestUpdateCredentialsAdminAccess(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", authType: jujucloud.EmptyAuthType,
		attrs: map[string]string{}})

	cred := jujucloud.Credential{}
	s.credService.EXPECT().CheckAndUpdateCredential(gomock.Any(), credential.IdFromTag(tag), cred, false).Return(
		[]credentialservice.UpdateCredentialModelResult{}, nil)

	results, err := s.api.UpdateCredentialsCheckModels(stdcontext.Background(), params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{CredentialTag: "cloudcred-meep_julia_three"}}})
}

func (s *cloudSuite) TestUpdateCredentialsOneModelSuccess(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", authType: jujucloud.EmptyAuthType,
		attrs: map[string]string{}})

	cred := jujucloud.Credential{}

	s.credService.EXPECT().CheckAndUpdateCredential(gomock.Any(), credential.IdFromTag(tag), cred, false).Return(
		[]credentialservice.UpdateCredentialModelResult{{
			ModelUUID: "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			ModelName: "testModel1",
		}}, nil)

	results, err := s.api.UpdateCredentialsCheckModels(stdcontext.Background(), params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
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

func (s *cloudSuite) TestUpdateCredentialsModelFailedValidation(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag)

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", authType: jujucloud.EmptyAuthType,
		attrs: map[string]string{}})

	results, err := s.api.UpdateCredentialsCheckModels(stdcontext.Background(), params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag:        tag.String(),
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{
			CredentialTag: "cloudcred-meep_julia_three",
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

func (s *cloudSuite) TestRevokeCredentials(c *gc.C) {
	bruceTag := names.NewUserTag("bruce")
	defer s.setup(c, bruceTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "bruce", cloudName: "meep", authType: jujucloud.EmptyAuthType,
		attrs: map[string]string{}})

	s.credService.EXPECT().CheckAndRevokeCredential(gomock.Any(), credential.IdFromTag(tag), false).Return(nil)

	results, err := s.api.RevokeCredentialsCheckModels(stdcontext.Background(), params.RevokeCredentialArgs{
		Credentials: []params.RevokeCredentialArg{
			{Tag: "machine-0"},
			{Tag: "cloudcred-meep_admin_whatever"},
			{Tag: "cloudcred-meep_bruce_three"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 3)
	c.Assert(results.Results[0].Error, jc.DeepEquals, &params.Error{
		Message: `"machine-0" is not a valid cloudcred tag`,
	})
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: "permission denied", Code: params.CodeUnauthorized,
	})
	c.Assert(results.Results[2].Error, gc.IsNil)
}

func (s *cloudSuite) TestRevokeCredentialsAdminAccess(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", authType: jujucloud.EmptyAuthType,
		attrs: map[string]string{}})

	s.credService.EXPECT().CheckAndRevokeCredential(gomock.Any(), credential.IdFromTag(tag), false).Return(nil)

	results, err := s.api.RevokeCredentialsCheckModels(stdcontext.Background(), params.RevokeCredentialArgs{
		Credentials: []params.RevokeCredentialArg{
			{Tag: "cloudcred-meep_julia_three"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	// admin can revoke others' credentials
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *cloudSuite) TestCredential(c *gc.C) {
	bruceTag := names.NewUserTag("bruce")
	defer s.setup(c, bruceTag).Finish()

	credentialOne, tagOne := cloudCredentialTag(credParams{name: "foo", owner: "admin", cloudName: "meep", authType: jujucloud.EmptyAuthType,
		attrs: map[string]string{}})
	credentialTwo, tagTwo := cloudCredentialTag(credParams{name: "two", owner: "bruce", cloudName: "meep", authType: jujucloud.UserPassAuthType,
		attrs: map[string]string{
			"username": "admin",
			"password": "adm1n",
		}})

	creds := map[string]jujucloud.Credential{
		tagOne.Id(): credentialOne,
		tagTwo.Id(): credentialTwo,
	}

	cloud := jujucloud.Cloud{
		Name:      "meep",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	s.cloudService.EXPECT().Get(gomock.Any(), "meep").Return(&cloud, nil)
	s.credService.EXPECT().CloudCredentialsForOwner(gomock.Any(), "bruce", "meep").Return(creds, nil)

	results, err := s.api.Credential(stdcontext.Background(), params.Entities{Entities: []params.Entity{{
		Tag: "machine-0",
	}, {
		Tag: "cloudcred-meep_admin_foo",
	}, {
		Tag: "cloudcred-meep_bruce_two",
	}}})
	c.Assert(err, jc.ErrorIsNil)
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
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	credential, tag := cloudCredentialTag(credParams{name: "two", owner: "bruce", cloudName: "meep", authType: jujucloud.UserPassAuthType,
		attrs: map[string]string{
			"username": "admin",
			"password": "adm1n",
		}})

	creds := map[string]jujucloud.Credential{
		tag.Id(): credential,
	}
	cloud := jujucloud.Cloud{
		Name:      "meep",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	s.cloudService.EXPECT().Get(gomock.Any(), "meep").Return(&cloud, nil)
	s.credService.EXPECT().CloudCredentialsForOwner(gomock.Any(), "bruce", "meep").Return(creds, nil)

	results, err := s.api.Credential(stdcontext.Background(), params.Entities{Entities: []params.Entity{{
		Tag: "cloudcred-meep_bruce_two",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	// admin can access others' credentials
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *cloudSuite) TestModifyCloudAccess(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	cloud := jujucloud.Cloud{
		Name:      "fluffy",
		Type:      "fluffy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	s.cloudService.EXPECT().Get(gomock.Any(), "fluffy").Return(&cloud, nil).Times(2)
	cloudPermissionService := s.cloudPermissionService.EXPECT()
	fred := names.NewUserTag("fred")
	mary := names.NewUserTag("mary")
	cloudPermissionService.CreateCloudAccess("fluffy", fred,
		permission.AddModelAccess).Return(nil)
	cloudPermissionService.RemoveCloudAccess("fluffy", mary).Return(nil)

	results, err := s.api.ModifyCloudAccess(stdcontext.Background(), params.ModifyCloudAccessRequest{
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
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{
		{}, {},
	})
}

func (s *cloudSuite) TestModifyCloudUpdateAccess(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	cloud := jujucloud.Cloud{
		Name:      "fluffy",
		Type:      "fluffy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	fredTag := names.NewUserTag("fred")

	s.cloudService.EXPECT().Get(gomock.Any(), "fluffy").Return(&cloud, nil)
	cloudPermissionService := s.cloudPermissionService.EXPECT()
	cloudPermissionService.CreateCloudAccess("fluffy", fredTag,
		permission.AdminAccess).Return(errors.AlreadyExistsf("access %s", permission.AdminAccess))
	cloudPermissionService.GetCloudAccess("fluffy", fredTag).Return(permission.AddModelAccess,
		nil)
	cloudPermissionService.UpdateCloudAccess("fluffy", fredTag,
		permission.AdminAccess).Return(nil)

	results, err := s.api.ModifyCloudAccess(stdcontext.Background(), params.ModifyCloudAccessRequest{
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
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{
		{},
	})
}

func (s *cloudSuite) TestModifyCloudAlreadyHasAccess(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	cloud := jujucloud.Cloud{
		Name:      "fluffy",
		Type:      "fluffy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	fredTag := names.NewUserTag("fred")

	s.cloudService.EXPECT().Get(gomock.Any(), "fluffy").Return(&cloud, nil)
	cloudPermissionService := s.cloudPermissionService.EXPECT()
	cloudPermissionService.CreateCloudAccess("fluffy", fredTag,
		permission.AdminAccess).Return(errors.AlreadyExistsf("access %s", permission.AdminAccess))
	cloudPermissionService.GetCloudAccess("fluffy", fredTag).Return(permission.AdminAccess,
		nil)

	results, err := s.api.ModifyCloudAccess(stdcontext.Background(), params.ModifyCloudAccessRequest{
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
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{
		{Error: &params.Error{Message: `could not grant cloud access: user already has "admin" access or greater`}},
	})
}

func (s *cloudSuite) TestCredentialContentsAllNoSecrets(c *gc.C) {
	bruceTag := names.NewUserTag("bruce")
	defer s.setup(c, bruceTag).Finish()

	credentialOne, tagOne := cloudCredentialTag(credParams{name: "one", owner: "bruce", cloudName: "meep", authType: jujucloud.EmptyAuthType,
		attrs: map[string]string{}})

	credentialTwo, tagTwo := cloudCredentialTag(credParams{name: "two", owner: "bruce", cloudName: "meep", authType: jujucloud.UserPassAuthType,
		attrs: map[string]string{
			"username": "admin",
		}})

	credentialTwo.Invalid = true
	creds := []credential.CloudCredential{
		{Credential: credentialOne, CloudName: "meep"},
		{Credential: credentialTwo, CloudName: "meep"},
	}
	cloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	s.credService.EXPECT().AllCloudCredentialsForOwner(gomock.Any(), bruceTag.Id()).Return(creds, nil)

	s.cloudService.EXPECT().Get(gomock.Any(), "meep").Return(&cloud, nil)
	modelCredentialService := s.modelCredentialService.EXPECT()
	modelCredentialService.CredentialModelsAndOwnerAccess(tagOne).Return([]jujucloud.CredentialOwnerModelAccess{}, nil)
	modelCredentialService.CredentialModelsAndOwnerAccess(tagTwo).Return([]jujucloud.CredentialOwnerModelAccess{}, nil)

	results, err := s.api.CredentialContents(stdcontext.Background(), params.CloudCredentialArgs{})
	c.Assert(err, jc.ErrorIsNil)

	_true := true
	_false := false
	expected := map[string]params.CredentialContent{
		"one": {
			Name:       "one",
			Cloud:      "meep",
			AuthType:   "empty",
			Valid:      &_true,
			Attributes: map[string]string{},
		},
		"two": {
			Name:     "two",
			Cloud:    "meep",
			AuthType: "userpass",
			Valid:    &_false,
			Attributes: map[string]string{
				"username": "admin",
			},
		},
	}

	c.Assert(results.Results, gc.HasLen, len(expected))
	for _, one := range results.Results {
		c.Assert(one.Result.Content, gc.DeepEquals, expected[one.Result.Content.Name])
	}
}

func cloudCredentialTag(params credParams) (jujucloud.Credential, names.CloudCredentialTag) {
	cred := jujucloud.NewNamedCredential(params.name, params.authType, params.attrs, false)
	id := fmt.Sprintf("%s/%s/%s", params.cloudName, params.owner, params.name)
	return cred, names.NewCloudCredentialTag(id)
}

type credParams struct {
	name      string
	owner     string
	cloudName string
	authType  jujucloud.AuthType
	attrs     map[string]string
}
