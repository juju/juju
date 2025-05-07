// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"context"
	"fmt"
	"regexp"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/client/cloud"
	"github.com/juju/juju/apiserver/facades/client/cloud/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/access"
	credentialservice "github.com/juju/juju/domain/credential/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	_ "github.com/juju/juju/internal/provider/dummy"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type cloudSuite struct {
	jujutesting.LoggingCleanupSuite

	cloudAccessService *mocks.MockCloudAccessService
	cloudService       *mocks.MockCloudService
	credService        *mocks.MockCredentialService
	api                *cloud.CloudAPI
	authorizer         *apiservertesting.FakeAuthorizer

	credentialValidator credentialservice.CredentialValidator
}

func (s *cloudSuite) setup(c *tc.C, userTag names.UserTag) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: userTag,
	}

	s.cloudAccessService = mocks.NewMockCloudAccessService(ctrl)
	s.cloudService = mocks.NewMockCloudService(ctrl)
	s.credService = mocks.NewMockCredentialService(ctrl)
	s.credentialValidator = mocks.NewMockCredentialValidator(ctrl)

	api, err := cloud.NewCloudAPI(
		context.Background(),
		coretesting.ControllerTag, "dummy",
		s.cloudService, s.cloudAccessService, s.credService,
		s.authorizer, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	s.api = api
	return ctrl
}

var _ = tc.Suite(&cloudSuite{})

func (s *cloudSuite) TestCloud(c *tc.C) {
	defer s.setup(c, names.NewUserTag("admin")).Finish()

	backend := s.cloudService.EXPECT()
	backend.Cloud(gomock.Any(), "my-cloud").Return(&jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}, nil)

	results, err := s.api.Cloud(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "cloud-my-cloud"}, {Tag: "machine-0"}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Assert(results.Results[0].Cloud, tc.DeepEquals, &params.Cloud{
		Type:      "dummy",
		AuthTypes: []string{"empty", "userpass"},
		Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
	})
	c.Assert(results.Results[1].Error, tc.DeepEquals, &params.Error{
		Message: `"machine-0" is not a valid cloud tag`,
	})
}

func (s *cloudSuite) TestCloudNotFound(c *tc.C) {
	defer s.setup(c, names.NewUserTag("admin")).Finish()

	backend := s.cloudService.EXPECT()
	backend.Cloud(gomock.Any(), "no-dice").Return(&jujucloud.Cloud{}, errors.NotFoundf("cloud \"no-dice\""))

	results, err := s.api.Cloud(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "cloud-no-dice"}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, "cloud \"no-dice\" not found")
}

func (s *cloudSuite) TestClouds(c *tc.C) {
	bruce := names.NewUserTag("bruce")
	defer s.setup(c, bruce).Finish()

	cloudPermissionService := s.cloudAccessService.EXPECT()

	cloudPermissionService.ReadUserAccessLevelForTarget(gomock.Any(),
		user.NameFromTag(bruce), permission.ID{ObjectType: permission.Cloud, Key: "my-cloud"}).Return(permission.AddModelAccess, nil)
	cloudPermissionService.ReadUserAccessLevelForTarget(gomock.Any(),
		user.NameFromTag(bruce), permission.ID{ObjectType: permission.Cloud, Key: "your-cloud"}).Return(permission.NoAccess, nil)

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

	result, err := s.api.Clouds(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Clouds, tc.DeepEquals, map[string]params.Cloud{
		"cloud-my-cloud": {
			Type:      "dummy",
			AuthTypes: []string{"empty", "userpass"},
			Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
		},
	})
}

func (s *cloudSuite) TestCloudInfoAdmin(c *tc.C) {
	ctrl := s.setup(c, names.NewUserTag("admin"))
	defer ctrl.Finish()

	cloudPermissionService := s.cloudAccessService.EXPECT()
	userPerm := []permission.UserAccess{
		{UserName: usertesting.GenNewName(c, "fred"), DisplayName: "display-fred", Access: permission.AddModelAccess},
		{UserName: usertesting.GenNewName(c, "mary"), DisplayName: "display-mary", Access: permission.AdminAccess},
	}
	target := permission.ID{
		ObjectType: permission.Cloud,
		Key:        "my-cloud",
	}
	cloudPermissionService.ReadAllUserAccessForTarget(gomock.Any(), target).Return(userPerm,
		nil)

	cloudService := s.cloudService.EXPECT()
	cloudService.Cloud(gomock.Any(), "my-cloud").Return(&jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}, nil)

	result, err := s.api.CloudInfo(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: "cloud-my-cloud",
	}, {
		Tag: "machine-0",
	}}})
	c.Assert(err, tc.ErrorIsNil)
	// Make sure that the slice is sorted in a predictable manor
	sort.Slice(result.Results[0].Result.Users, func(i, j int) bool {
		return result.Results[0].Result.Users[i].UserName < result.Results[0].Result.Users[j].UserName
	})
	c.Assert(result.Results, tc.DeepEquals, []params.CloudInfoResult{
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

func (s *cloudSuite) TestCloudInfoNonAdmin(c *tc.C) {
	fredTag := names.NewUserTag("fred")
	ctrl := s.setup(c, fredTag)
	defer ctrl.Finish()

	cloudPermissionService := s.cloudAccessService.EXPECT()
	permID := permission.ID{
		ObjectType: permission.Cloud,
		Key:        "my-cloud",
	}
	cloudPermissionService.ReadUserAccessLevelForTarget(gomock.Any(), user.NameFromTag(fredTag),
		permID).Return(permission.AddModelAccess, nil)
	userPerm := []permission.UserAccess{
		{UserName: usertesting.GenNewName(c, "fred"), DisplayName: "display-fred", Access: permission.AddModelAccess},
		{UserName: usertesting.GenNewName(c, "mary"), DisplayName: "display-mary", Access: permission.AdminAccess},
	}
	cloudPermissionService.ReadAllUserAccessForTarget(gomock.Any(), permID).Return(userPerm,
		nil)

	s.cloudService.EXPECT().Cloud(gomock.Any(), "my-cloud").Return(&jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}, nil)

	result, err := s.api.CloudInfo(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: "cloud-my-cloud",
	}, {
		Tag: "machine-0",
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 2)
	c.Assert(result.Results, tc.DeepEquals, []params.CloudInfoResult{
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

func (s *cloudSuite) TestAddCloud(c *tc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	cloudservice := s.cloudService.EXPECT()
	cloud := jujucloud.Cloud{
		Name: "newcloudname",
		Type: "maas",
	}
	cloudservice.Cloud(gomock.Any(), "dummy").Return(&cloud, nil)
	newCloud := jujucloud.Cloud{
		Name:      "newcloudname",
		Type:      "maas",
		Endpoint:  "fake-endpoint",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "nether-endpoint"}},
	}
	cloudservice.CreateCloud(gomock.Any(), user.NameFromTag(adminTag), newCloud).Return(nil)
	paramsCloud := params.AddCloudArgs{
		Name: "newcloudname",
		Cloud: params.Cloud{
			Type:      "maas",
			AuthTypes: []string{"empty", "userpass"},
			Endpoint:  "fake-endpoint",
			Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "nether-endpoint"}},
		}}

	err := s.api.AddCloud(context.Background(), paramsCloud)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *cloudSuite) TestAddCloudAsExternalUser(c *tc.C) {
	// In this test we attempt to add a cloud as an authorized external user.

	// User `superuser-alice@external` is an external user. We need the `superuser` prefix
	// because of the fake authorized used in this test - means alice is has the
	// `superuser` access level on the controller.
	aliceTag := names.NewUserTag("superuser-alice@external")
	defer s.setup(c, aliceTag).Finish()

	cloudservice := s.cloudService.EXPECT()
	cloud := jujucloud.Cloud{
		Name: "newcloudname",
		Type: "maas",
	}
	cloudservice.Cloud(gomock.Any(), "dummy").Return(&cloud, nil)
	newCloud := jujucloud.Cloud{
		Name:      "newcloudname",
		Type:      "maas",
		Endpoint:  "fake-endpoint",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "nether-endpoint"}},
	}
	cloudservice.CreateCloud(gomock.Any(), user.NameFromTag(aliceTag), newCloud).Return(nil)
	paramsCloud := params.AddCloudArgs{
		Name: "newcloudname",
		Cloud: params.Cloud{
			Type:      "maas",
			AuthTypes: []string{"empty", "userpass"},
			Endpoint:  "fake-endpoint",
			Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "nether-endpoint"}},
		}}

	err := s.api.AddCloud(context.Background(), paramsCloud)
	c.Assert(err, tc.ErrorIsNil)
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

func (s *cloudSuite) TestAddCloudNotWhitelisted(c *tc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	cloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	s.cloudService.EXPECT().Cloud(gomock.Any(), "dummy").Return(&cloud, nil)

	err := s.api.AddCloud(context.Background(), createAddCloudParam(""))
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta(`
controller cloud type "dummy" is not whitelisted, current whitelist: 
 - controller cloud type "kubernetes" supports [lxd maas openstack]
 - controller cloud type "lxd" supports [lxd maas openstack]
 - controller cloud type "maas" supports [maas openstack]
 - controller cloud type "openstack" supports [openstack]`[1:]))
}

func (s *cloudSuite) TestAddCloudNotWhitelistedButForceAdded(c *tc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	cloudService := s.cloudService.EXPECT()
	cloud := jujucloud.Cloud{
		Name: "newcloudname",
		Type: "maas",
	}
	cloudService.Cloud(gomock.Any(), "dummy").Return(&cloud, nil)
	newCloud := jujucloud.Cloud{
		Name:      "newcloudname",
		Type:      "fake",
		Endpoint:  "fake-endpoint",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "nether-endpoint"}},
	}
	cloudService.CreateCloud(gomock.Any(), user.NameFromTag(adminTag), newCloud).Return(nil)

	force := true
	addCloudArg := createAddCloudParam("")
	addCloudArg.Force = &force
	err := s.api.AddCloud(context.Background(), addCloudArg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *cloudSuite) TestAddCloudControllerCloudErr(c *tc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	s.cloudService.EXPECT().Cloud(gomock.Any(), "dummy").Return(&jujucloud.Cloud{}, errors.New("kaboom"))

	err := s.api.AddCloud(context.Background(), createAddCloudParam(""))
	c.Assert(err, tc.ErrorMatches, "kaboom")
}

func (s *cloudSuite) TestAddCloudK8sForceIrrelevant(c *tc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	cloud := jujucloud.Cloud{
		Name:      "newcloudname",
		Type:      string(k8sconstants.CAASProviderType),
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Endpoint:  "fake-endpoint",
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "nether-endpoint"}},
	}
	s.cloudService.EXPECT().CreateCloud(gomock.Any(), user.NameFromTag(adminTag), cloud).Return(nil).Times(2)

	addCloudArg := createAddCloudParam(string(k8sconstants.CAASProviderType))

	add := func() {
		err := s.api.AddCloud(context.Background(), addCloudArg)
		c.Assert(err, tc.ErrorIsNil)
	}
	add()
	force := true
	addCloudArg.Force = &force
	add()
}

func (s *cloudSuite) TestAddCloudNoRegion(c *tc.C) {
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

	cloudService := s.cloudService.EXPECT()
	cloud := jujucloud.Cloud{
		Name: "newcloudname",
		Type: "maas",
	}
	cloudService.Cloud(gomock.Any(), "dummy").Return(&cloud, nil)
	cloudService.CreateCloud(gomock.Any(), user.NameFromTag(adminTag), newCloud).Return(nil)
	paramsCloud := params.AddCloudArgs{
		Name: "newcloudname",
		Cloud: params.Cloud{
			Type:      "maas",
			AuthTypes: []string{"empty", "userpass"},
			Endpoint:  "fake-endpoint",
		}}
	err := s.api.AddCloud(context.Background(), paramsCloud)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *cloudSuite) TestAddCloudNoAdminPerms(c *tc.C) {
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
	err := s.api.AddCloud(context.Background(), paramsCloud)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *cloudSuite) TestUpdateCloud(c *tc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	dummyCloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether-updated", Endpoint: "endpoint-updated"}},
	}

	s.cloudService.EXPECT().UpdateCloud(gomock.Any(), dummyCloud).Return(nil)

	updatedCloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether-updated", Endpoint: "endpoint-updated"}},
	}
	results, err := s.api.UpdateCloud(context.Background(), params.UpdateCloudArgs{
		Clouds: []params.AddCloudArgs{{
			Name:  "dummy",
			Cloud: cloud.CloudToParams(updatedCloud),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
}

func (s *cloudSuite) TestUpdateCloudNonAdminPerm(c *tc.C) {
	frankTag := names.NewUserTag("frank")
	defer s.setup(c, frankTag).Finish()

	updatedCloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether-updated", Endpoint: "endpoint-updated"}},
	}
	results, err := s.api.UpdateCloud(context.Background(), params.UpdateCloudArgs{
		Clouds: []params.AddCloudArgs{{
			Name:  "dummy",
			Cloud: cloud.CloudToParams(updatedCloud),
		}},
	})
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
}

func (s *cloudSuite) TestUpdateNonExistentCloud(c *tc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	dummyCloud := jujucloud.Cloud{
		Name:      "nope",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether-updated", Endpoint: "endpoint-updated"}},
	}

	s.cloudService.EXPECT().UpdateCloud(gomock.Any(), dummyCloud).Return(errors.New("cloud \"nope\" not found"))

	updatedCloud := jujucloud.Cloud{
		Name:      "nope",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether-updated", Endpoint: "endpoint-updated"}},
	}

	results, err := s.api.UpdateCloud(context.Background(), params.UpdateCloudArgs{
		Clouds: []params.AddCloudArgs{{
			Name:  "nope",
			Cloud: cloud.CloudToParams(updatedCloud),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, fmt.Sprintf("cloud %q not found", updatedCloud.Name))
}

func (s *cloudSuite) TestListCloudInfo(c *tc.C) {
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

	result, err := s.api.ListCloudInfo(context.Background(), params.ListCloudsRequest{
		UserTag: "user-admin",
		All:     true,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.DeepEquals, []params.ListCloudInfoResult{
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

func (s *cloudSuite) TestUserCredentials(c *tc.C) {
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

	s.credService.EXPECT().CloudCredentialsForOwner(gomock.Any(), user.NameFromTag(bruceTag), "meep").Return(creds, nil)

	results, err := s.api.UserCredentials(context.Background(), params.UserClouds{UserClouds: []params.UserCloud{{
		UserTag:  "machine-0",
		CloudTag: "cloud-meep",
	}, {
		UserTag:  "user-admin",
		CloudTag: "cloud-meep",
	}, {
		UserTag:  "user-bruce",
		CloudTag: "cloud-meep",
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 3)
	c.Assert(results.Results[0].Error, tc.DeepEquals, &params.Error{
		Message: `"machine-0" is not a valid user tag`,
	})
	c.Assert(results.Results[1].Error, tc.DeepEquals, &params.Error{
		Message: "permission denied", Code: params.CodeUnauthorized,
	})
	c.Assert(results.Results[2].Error, tc.IsNil)
	c.Assert(results.Results[2].Result, tc.SameContents, []string{
		"cloudcred-meep_bruce_one",
		"cloudcred-meep_bruce_two",
	})
}

func (s *cloudSuite) TestUserCredentialsAdminAccess(c *tc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	julia := names.NewUserTag("julia")
	s.credService.EXPECT().CloudCredentialsForOwner(gomock.Any(), user.NameFromTag(julia), "meep").Return(map[string]jujucloud.Credential{}, nil)

	results, err := s.api.UserCredentials(context.Background(), params.UserClouds{UserClouds: []params.UserCloud{{
		UserTag:  "user-julia",
		CloudTag: "cloud-meep",
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	// admin can access others' credentials
	c.Assert(results.Results[0].Error, tc.IsNil)
}

func (s *cloudSuite) TestUpdateCredentials(c *tc.C) {
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
	s.credService.EXPECT().CheckAndUpdateCredential(gomock.Any(), credential.KeyFromTag(tagTwo), cred, false).Return(
		nil, errors.New("cannot update credential \"three\": controller does not manage cloud \"badcloud\""))
	s.credService.EXPECT().CheckAndUpdateCredential(gomock.Any(), credential.KeyFromTag(tagOne), cred, false).Return(
		[]credentialservice.UpdateCredentialModelResult{}, nil)

	results, err := s.api.UpdateCredentialsCheckModels(context.Background(), params.UpdateCredentialArgs{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.UpdateCredentialResults{
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

func (s *cloudSuite) TestUpdateCredentialsAdminAccess(c *tc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", authType: jujucloud.EmptyAuthType,
		attrs: map[string]string{}})

	cred := jujucloud.Credential{}
	s.credService.EXPECT().CheckAndUpdateCredential(gomock.Any(), credential.KeyFromTag(tag), cred, false).Return(
		[]credentialservice.UpdateCredentialModelResult{}, nil)

	results, err := s.api.UpdateCredentialsCheckModels(context.Background(), params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{CredentialTag: "cloudcred-meep_julia_three"}}})
}

func (s *cloudSuite) TestUpdateCredentialsOneModelSuccess(c *tc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", authType: jujucloud.EmptyAuthType,
		attrs: map[string]string{}})

	cred := jujucloud.Credential{}
	s.credService.EXPECT().CheckAndUpdateCredential(gomock.Any(), credential.KeyFromTag(tag), cred, false).Return(
		[]credentialservice.UpdateCredentialModelResult{{
			ModelUUID: "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			ModelName: "testModel1",
		}}, nil)

	results, err := s.api.UpdateCredentialsCheckModels(context.Background(), params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.UpdateCredentialResults{
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

func (s *cloudSuite) TestUpdateCredentialsModelFailedValidation(c *tc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag)

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", authType: jujucloud.EmptyAuthType,
		attrs: map[string]string{}})

	results, err := s.api.UpdateCredentialsCheckModels(context.Background(), params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag:        tag.String(),
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.UpdateCredentialResults{
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

func (s *cloudSuite) TestRevokeCredentials(c *tc.C) {
	bruceTag := names.NewUserTag("bruce")
	defer s.setup(c, bruceTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "bruce", cloudName: "meep", authType: jujucloud.EmptyAuthType,
		attrs: map[string]string{}})

	s.credService.EXPECT().CheckAndRevokeCredential(gomock.Any(), credential.KeyFromTag(tag), false).Return(nil)

	results, err := s.api.RevokeCredentialsCheckModels(context.Background(), params.RevokeCredentialArgs{
		Credentials: []params.RevokeCredentialArg{
			{Tag: "machine-0"},
			{Tag: "cloudcred-meep_admin_whatever"},
			{Tag: "cloudcred-meep_bruce_three"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 3)
	c.Assert(results.Results[0].Error, tc.DeepEquals, &params.Error{
		Message: `"machine-0" is not a valid cloudcred tag`,
	})
	c.Assert(results.Results[1].Error, tc.DeepEquals, &params.Error{
		Message: "permission denied", Code: params.CodeUnauthorized,
	})
	c.Assert(results.Results[2].Error, tc.IsNil)
}

func (s *cloudSuite) TestRevokeCredentialsAdminAccess(c *tc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", authType: jujucloud.EmptyAuthType,
		attrs: map[string]string{}})

	s.credService.EXPECT().CheckAndRevokeCredential(gomock.Any(), credential.KeyFromTag(tag), false).Return(nil)

	results, err := s.api.RevokeCredentialsCheckModels(context.Background(), params.RevokeCredentialArgs{
		Credentials: []params.RevokeCredentialArg{
			{Tag: "cloudcred-meep_julia_three"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	// admin can revoke others' credentials
	c.Assert(results.Results[0].Error, tc.IsNil)
}

func (s *cloudSuite) TestCredential(c *tc.C) {
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

	s.cloudService.EXPECT().Cloud(gomock.Any(), "meep").Return(&cloud, nil)
	s.credService.EXPECT().CloudCredentialsForOwner(gomock.Any(), usertesting.GenNewName(c, "bruce"), "meep").Return(creds, nil)

	results, err := s.api.Credential(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: "machine-0",
	}, {
		Tag: "cloudcred-meep_admin_foo",
	}, {
		Tag: "cloudcred-meep_bruce_two",
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 3)
	c.Assert(results.Results[0].Error, tc.DeepEquals, &params.Error{
		Message: `"machine-0" is not a valid cloudcred tag`,
	})
	c.Assert(results.Results[1].Error, tc.DeepEquals, &params.Error{
		Message: "permission denied", Code: params.CodeUnauthorized,
	})
	c.Assert(results.Results[2].Error, tc.IsNil)
	c.Assert(results.Results[2].Result, tc.DeepEquals, &params.CloudCredential{
		AuthType:   "userpass",
		Attributes: map[string]string{"username": "admin"},
		Redacted:   []string{"password"},
	})
}

func (s *cloudSuite) TestCredentialAdminAccess(c *tc.C) {
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

	s.cloudService.EXPECT().Cloud(gomock.Any(), "meep").Return(&cloud, nil)
	s.credService.EXPECT().CloudCredentialsForOwner(gomock.Any(), usertesting.GenNewName(c, "bruce"), "meep").Return(creds, nil)

	results, err := s.api.Credential(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: "cloudcred-meep_bruce_two",
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	// admin can access others' credentials
	c.Assert(results.Results[0].Error, tc.IsNil)
}

func (s *cloudSuite) TestModifyCloudAccess(c *tc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	cloudPermissionService := s.cloudAccessService.EXPECT()
	fredSpec := access.UpdatePermissionArgs{
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Cloud,
				Key:        "fluffy",
			},
			Access: permission.AddModelAccess,
		},
		Subject: usertesting.GenNewName(c, "fred"),
		Change:  permission.Grant,
	}
	cloudPermissionService.UpdatePermission(gomock.Any(), fredSpec).Return(nil)
	marySpec := access.UpdatePermissionArgs{
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Cloud,
				Key:        "fluffy",
			},
			Access: permission.AddModelAccess,
		},
		Subject: usertesting.GenNewName(c, "mary"),
		Change:  permission.Revoke,
	}
	cloudPermissionService.UpdatePermission(gomock.Any(), marySpec).Return(nil)

	results, err := s.api.ModifyCloudAccess(context.Background(), params.ModifyCloudAccessRequest{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.DeepEquals, []params.ErrorResult{
		{}, {},
	})
}

func (s *cloudSuite) TestCredentialContentsAllNoSecrets(c *tc.C) {
	bruceTag := names.NewUserTag("bruce")
	defer s.setup(c, bruceTag).Finish()

	credentialOne, tagOne := cloudCredentialTag(credParams{name: "one", owner: "bruce", cloudName: "meep", authType: jujucloud.EmptyAuthType,
		attrs: map[string]string{}})

	credentialTwo, tagTwo := cloudCredentialTag(credParams{name: "two", owner: "bruce", cloudName: "meep", authType: jujucloud.UserPassAuthType,
		attrs: map[string]string{
			"username": "admin",
		}})
	keyOne := credential.Key{
		Cloud: tagOne.Cloud().Id(),
		Owner: user.NameFromTag(tagOne.Owner()),
		Name:  tagOne.Name(),
	}
	keyTwo := credential.Key{
		Cloud: tagTwo.Cloud().Id(),
		Owner: user.NameFromTag(tagTwo.Owner()),
		Name:  tagTwo.Name(),
	}

	credentialTwo.Invalid = true
	creds := map[credential.Key]jujucloud.Credential{
		{Cloud: "meep", Owner: usertesting.GenNewName(c, "bruce"), Name: "one"}: credentialOne,
		{Cloud: "meep", Owner: usertesting.GenNewName(c, "bruce"), Name: "two"}: credentialTwo,
	}
	cloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	ctx := context.Background()
	s.credService.EXPECT().AllCloudCredentialsForOwner(gomock.Any(), user.NameFromTag(bruceTag)).Return(creds, nil)

	s.cloudService.EXPECT().Cloud(gomock.Any(), "meep").Return(&cloud, nil)
	modelCredentialService := s.cloudAccessService.EXPECT()
	modelCredentialService.AllModelAccessForCloudCredential(ctx, keyOne).Return([]access.CredentialOwnerModelAccess{}, nil)
	modelCredentialService.AllModelAccessForCloudCredential(ctx, keyTwo).Return([]access.CredentialOwnerModelAccess{}, nil)

	results, err := s.api.CredentialContents(ctx, params.CloudCredentialArgs{})
	c.Assert(err, tc.ErrorIsNil)

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

	c.Assert(results.Results, tc.HasLen, len(expected))
	for _, one := range results.Results {
		c.Assert(one.Result.Content, tc.DeepEquals, expected[one.Result.Content.Name])
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
