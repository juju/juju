// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/cloud"
	"github.com/juju/juju/apiserver/facades/client/cloud/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/context"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

type cloudSuite struct {
	gitjujutesting.LoggingCleanupSuite
	backend           *mocks.MockBackend
	ctrlBackend       *mocks.MockBackend
	pool              *mocks.MockModelPoolBackend
	api               *cloud.CloudAPI
	authorizer        *apiservertesting.FakeAuthorizer
	ctrlConfigService *mocks.MockControllerConfigGetter
}

func (s *cloudSuite) setup(c *gc.C, userTag names.UserTag) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.backend = mocks.NewMockBackend(ctrl)
	s.backend.EXPECT().ControllerTag().Return(coretesting.ControllerTag).AnyTimes()

	s.pool = mocks.NewMockModelPoolBackend(ctrl)
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: userTag,
	}

	s.ctrlBackend = mocks.NewMockBackend(ctrl)
	s.ctrlBackend.EXPECT().ControllerTag().Return(coretesting.ControllerTag).AnyTimes()

	s.ctrlConfigService = mocks.NewMockControllerConfigGetter(ctrl)
	api, err := cloud.NewCloudAPI(s.backend, s.ctrlBackend, s.pool, s.authorizer, loggo.GetLogger("juju.apiserver.cloud"), s.ctrlConfigService)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
	return ctrl
}

var _ = gc.Suite(&cloudSuite{})

func newModelBackend(c *gc.C, aCloud jujucloud.Cloud, uuid string) *mockModelBackend {
	return &mockModelBackend{
		uuid: uuid,
	}
}

func (s *cloudSuite) TestCloud(c *gc.C) {
	defer s.setup(c, names.NewUserTag("admin")).Finish()

	backend := s.backend.EXPECT()
	backend.Cloud("my-cloud").Return(jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}, nil)

	results, err := s.api.Cloud(params.Entities{
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

	backend := s.backend.EXPECT()
	backend.Cloud("no-dice").Return(jujucloud.Cloud{}, errors.NotFoundf("cloud \"no-dice\""))

	results, err := s.api.Cloud(params.Entities{
		Entities: []params.Entity{{Tag: "cloud-no-dice"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, "cloud \"no-dice\" not found")
}

func (s *cloudSuite) TestClouds(c *gc.C) {
	bruce := names.NewUserTag("bruce")
	defer s.setup(c, bruce).Finish()

	ctrlBackend := s.ctrlBackend.EXPECT()

	ctrlBackend.GetCloudAccess("my-cloud",
		bruce).Return(permission.AddModelAccess, nil)
	ctrlBackend.GetCloudAccess("your-cloud",
		bruce).Return(permission.NoAccess, nil)

	backend := s.backend.EXPECT()
	backend.Clouds().Return(map[names.CloudTag]jujucloud.Cloud{
		names.NewCloudTag("my-cloud"): {
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
			Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
		},
		names.NewCloudTag("your-cloud"): {
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
			Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
		},
	}, nil)

	result, err := s.api.Clouds()
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

	ctrlBackend := s.ctrlBackend.EXPECT()
	userPerm := map[string]permission.Access{"fred": permission.AddModelAccess,
		"mary": permission.AdminAccess}
	ctrlBackend.GetCloudUsers("my-cloud").Return(userPerm,
		nil)

	backend := s.backend.EXPECT()
	backend.Cloud("my-cloud").Return(jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}, nil)

	mary := mocks.NewMockUser(ctrl)
	fred := mocks.NewMockUser(ctrl)
	mary.EXPECT().DisplayName().Return("display-mary")
	fred.EXPECT().DisplayName().Return("display-fred")

	maryTag := names.NewUserTag("mary")
	backend.User(maryTag).Return(mary, nil)
	fredTag := names.NewUserTag("fred")
	backend.User(fredTag).Return(fred, nil)

	result, err := s.api.CloudInfo(params.Entities{Entities: []params.Entity{{
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

	ctrlBackend := s.ctrlBackend.EXPECT()
	ctrlBackend.GetCloudAccess("my-cloud",
		fredTag).Return(permission.AddModelAccess, nil)
	userPerm := map[string]permission.Access{"fred": permission.AddModelAccess,
		"mary": permission.AdminAccess}
	ctrlBackend.GetCloudUsers("my-cloud").Return(userPerm,
		nil)

	backend := s.backend.EXPECT()
	backend.User(fredTag).Return(fred, nil)
	backend.Cloud("my-cloud").Return(jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}, nil)

	result, err := s.api.CloudInfo(params.Entities{Entities: []params.Entity{{
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

	backend := s.backend.EXPECT()
	backend.ControllerInfo().Return(&state.ControllerInfo{CloudName: "newcloudname"}, nil)
	backend.Cloud("newcloudname").Return(cloud, nil)
	backend.AddCloud(newCloud, adminTag.Name()).Return(nil)
	paramsCloud := params.AddCloudArgs{
		Name: "newcloudname",
		Cloud: params.Cloud{
			Type:      "maas",
			AuthTypes: []string{"empty", "userpass"},
			Endpoint:  "fake-endpoint",
			Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "nether-endpoint"}},
		}}

	err := s.api.AddCloud(paramsCloud)
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

	backend := s.backend.EXPECT()
	backend.ControllerInfo().Return(&state.ControllerInfo{CloudName: "dummy"}, nil)
	backend.Cloud("dummy").Return(cloud, nil)

	err := s.api.AddCloud(createAddCloudParam(""))
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

	backend := s.backend.EXPECT()
	backend.ControllerInfo().Return(&state.ControllerInfo{CloudName: "newcloudname"}, nil)
	backend.Cloud("newcloudname").Return(cloud, nil)
	backend.AddCloud(newCloud, adminTag.Name()).Return(nil)

	force := true
	addCloudArg := createAddCloudParam("")
	addCloudArg.Force = &force
	err := s.api.AddCloud(addCloudArg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cloudSuite) TestAddCloudControllerInfoErr(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	backend := s.backend.EXPECT()
	backend.ControllerInfo().Return(nil, errors.New("kaboom"))

	err := s.api.AddCloud(createAddCloudParam(""))
	c.Assert(err, gc.ErrorMatches, "kaboom")
}

func (s *cloudSuite) TestAddCloudControllerCloudErr(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	backend := s.backend.EXPECT()
	backend.ControllerInfo().Return(&state.ControllerInfo{CloudName: "kaboom"}, nil)
	backend.Cloud("kaboom").Return(jujucloud.Cloud{}, errors.New("kaboom"))

	err := s.api.AddCloud(createAddCloudParam(""))
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

	backend := s.backend.EXPECT()
	backend.AddCloud(cloud, adminTag.Name()).Return(nil).Times(2)

	addCloudArg := createAddCloudParam(string(k8sconstants.CAASProviderType))

	add := func() {
		err := s.api.AddCloud(addCloudArg)
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

	backend := s.backend.EXPECT()
	backend.ControllerInfo().Return(&state.ControllerInfo{CloudName: "newcloudname"}, nil)
	backend.Cloud("newcloudname").Return(cloud, nil)
	backend.AddCloud(newCloud, adminTag.Name()).Return(nil)
	paramsCloud := params.AddCloudArgs{
		Name: "newcloudname",
		Cloud: params.Cloud{
			Type:      "maas",
			AuthTypes: []string{"empty", "userpass"},
			Endpoint:  "fake-endpoint",
		}}
	err := s.api.AddCloud(paramsCloud)
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
	err := s.api.AddCloud(paramsCloud)
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

	backend := s.backend.EXPECT()
	backend.UpdateCloud(dummyCloud).Return(nil)

	updatedCloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether-updated", Endpoint: "endpoint-updated"}},
	}
	results, err := s.api.UpdateCloud(params.UpdateCloudArgs{
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
	results, err := s.api.UpdateCloud(params.UpdateCloudArgs{
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

	backend := s.backend.EXPECT()
	backend.UpdateCloud(dummyCloud).Return(errors.New("cloud \"nope\" not found"))

	updatedCloud := jujucloud.Cloud{
		Name:      "nope",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether-updated", Endpoint: "endpoint-updated"}},
	}

	results, err := s.api.UpdateCloud(params.UpdateCloudArgs{
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

	cloudInfo := []state.CloudInfo{
		{
			Cloud: jujucloud.Cloud{
				Name:      "dummy",
				Type:      "dummy",
				AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
				Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
			},
			Access: permission.AddModelAccess,
		},
	}
	ctrlBackend := s.ctrlBackend.EXPECT()
	ctrlBackend.CloudsForUser(fredTag, true).Return(cloudInfo, nil)

	result, err := s.api.ListCloudInfo(params.ListCloudsRequest{
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
				Access: "add-model",
			},
		},
	})
}

func (s *cloudSuite) TestUserCredentials(c *gc.C) {
	bruceTag := names.NewUserTag("bruce")
	defer s.setup(c, bruceTag).Finish()

	credentialOne, tagOne := cloudCredentialTag(credParams{name: "one", owner: "bruce", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)
	credentialTwo, tagTwo := cloudCredentialTag(credParams{name: "two", owner: "bruce", cloudName: "meep", permission: jujucloud.UserPassAuthType,
		attrs: map[string]string{
			"username": "admin",
			"password": "adm1n",
		}}, c)

	creds := map[string]state.Credential{
		tagOne.Id(): credentialOne,
		tagTwo.Id(): credentialTwo,
	}

	backend := s.backend.EXPECT()
	backend.CloudCredentials(bruceTag, "meep").Return(creds, nil)

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
	backend := s.backend.EXPECT()
	backend.CloudCredentials(julia, "meep").Return(map[string]state.Credential{}, nil)

	results, err := s.api.UserCredentials(params.UserClouds{UserClouds: []params.UserCloud{{
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

	_, tagOne := cloudCredentialTag(credParams{name: "three", owner: "bruce", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)
	_, tagTwo := cloudCredentialTag(credParams{name: "three", owner: "bruce", cloudName: "badcloud", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tagOne).Return(nil, nil)
	backend.UpdateCloudCredential(tagTwo, jujucloud.NewCredential(
		jujucloud.OAuth1AuthType,
		map[string]string{"token": "foo:bar:baz"},
	)).Return(errors.New("cannot update credential \"three\": controller does not manage cloud \"badcloud\""))
	backend.CredentialModels(tagTwo).Return(nil, nil)
	backend.UpdateCloudCredential(tagOne, jujucloud.NewCredential(
		jujucloud.OAuth1AuthType,
		map[string]string{"token": "foo:bar:baz"},
	)).Return(nil)

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

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(nil, nil)
	backend.UpdateCloudCredential(names.NewCloudCredentialTag("meep/julia/three"),
		jujucloud.Credential{}).Return(nil)

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{CredentialTag: "cloudcred-meep_julia_three"}}})
}

func (s *cloudSuite) TestUpdateCredentialsNoModelsFound(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(nil, errors.NotFoundf("how about it"))
	backend.UpdateCloudCredential(names.NewCloudCredentialTag("meep/julia/three"),
		jujucloud.Credential{}).Return(nil)

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{CredentialTag: "cloudcred-meep_julia_three"}}})
}

func (s *cloudSuite) TestUpdateCredentialsModelsError(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	_, tag := cloudCredentialTag(credParams{"three", "julia", "meep", jujucloud.EmptyAuthType,
		map[string]string{}}, c)
	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(nil, errors.New("cannot get models"))

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{
			{
				CredentialTag: "cloudcred-meep_julia_three",
				Error:         &params.Error{Message: "cannot get models"},
			},
		}})
}

func (s *cloudSuite) TestUpdateCredentialsModelsErrorForce(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(nil, errors.New("cannot get models"))
	backend.UpdateCloudCredential(tag,
		jujucloud.Credential{}).Return(nil)

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: true,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{
			{
				CredentialTag: "cloudcred-meep_julia_three",
			},
		}})
}

func (s *cloudSuite) TestUpdateCredentialsOneModelSuccess(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	s.PatchValue(cloud.ValidateNewCredentialForModelFunc,
		func(
			_ credentialcommon.PersistentBackend, _ context.ProviderCallContext,
			_ names.CloudCredentialTag, _ *jujucloud.Credential, _ bool,
		) (params.ErrorResults, error) {
			return params.ErrorResults{}, nil
		})

	aCloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(map[string]string{
		coretesting.ModelTag.Id(): "testModel1",
	}, nil)
	backend.UpdateCloudCredential(tag, jujucloud.Credential{}).Return(nil)

	pool := s.pool.EXPECT()
	pool.GetModelCallContext(coretesting.ModelTag.Id()).Return(newModelBackend(c, aCloud, coretesting.ModelTag.Id()),
		context.NewEmptyCloudCallContext(), nil)

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
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

func (s *cloudSuite) TestUpdateCredentialsModelGetError(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(map[string]string{
		coretesting.ModelTag.Id(): "testModel1",
	}, nil)

	pool := s.pool.EXPECT()
	pool.GetModelCallContext(coretesting.ModelTag.Id()).Return(nil, nil, errors.New("cannot get a model"))

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
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
					Errors:    []params.ErrorResult{{Error: &params.Error{Message: "cannot get a model", Code: ""}}},
				},
			},
		}},
	})
}

func (s *cloudSuite) TestUpdateCredentialsModelGetErrorForce(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag)

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(map[string]string{
		coretesting.ModelTag.Id(): "testModel1",
	}, nil)
	backend.UpdateCloudCredential(tag, jujucloud.Credential{}).Return(nil)

	pool := s.pool.EXPECT()
	pool.GetModelCallContext(coretesting.ModelTag.Id()).Return(nil, nil, errors.New("cannot get a model"))

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: true,
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
					Errors:    []params.ErrorResult{{Error: &params.Error{Message: "cannot get a model", Code: ""}}},
				},
			},
		}},
	})
}

func (s *cloudSuite) TestUpdateCredentialsModelFailedValidation(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag)

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(map[string]string{
		coretesting.ModelTag.Id(): "testModel1",
	}, nil)

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
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
					ModelUUID: coretesting.ModelTag.Id(),
					ModelName: "testModel1",
					Errors:    []params.ErrorResult{{Error: &params.Error{Message: "not valid for model", Code: ""}}},
				},
			},
		}},
	})
}

func (s *cloudSuite) TestUpdateCredentialsModelFailedValidationForce(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	s.PatchValue(cloud.ValidateNewCredentialForModelFunc,
		func(backend credentialcommon.PersistentBackend, _ context.ProviderCallContext,
			_ names.CloudCredentialTag, _ *jujucloud.Credential, _ bool,
		) (params.ErrorResults, error) {
			return params.ErrorResults{Results: []params.ErrorResult{{Error: &params.Error{Message: "not valid for model"}}}}, nil
		})

	aCloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(map[string]string{
		coretesting.ModelTag.Id(): "testModel1",
	}, nil)
	backend.UpdateCloudCredential(tag, jujucloud.Credential{}).Return(nil)

	pool := s.pool.EXPECT()
	pool.GetModelCallContext(gomock.Any()).DoAndReturn(func(modelUUID string) (
		credentialcommon.PersistentBackend, context.ProviderCallContext, error,
	) {
		return newModelBackend(c, aCloud,
			modelUUID), context.NewEmptyCloudCallContext(), nil
	}).MinTimes(1)

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: true,
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
					ModelUUID: coretesting.ModelTag.Id(),
					ModelName: "testModel1",
					Errors:    []params.ErrorResult{{Error: &params.Error{Message: "not valid for model", Code: ""}}},
				},
			},
		}},
	})
}

func (s *cloudSuite) TestUpdateCredentialsSomeModelsFailedValidation(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	aCloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	s.PatchValue(cloud.ValidateNewCredentialForModelFunc,
		func(backend credentialcommon.PersistentBackend, _ context.ProviderCallContext,
			_ names.CloudCredentialTag, _ *jujucloud.Credential, _ bool,
		) (params.ErrorResults, error) {
			if backend.(*mockModelBackend).uuid == "deadbeef-0bad-400d-8000-4b1d0d06f00d" {
				return params.ErrorResults{Results: []params.ErrorResult{{Error: &params.Error{Message: "not valid for model"}}}}, nil
			}
			return params.ErrorResults{Results: []params.ErrorResult{}}, nil
		})

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(map[string]string{
		coretesting.ModelTag.Id():              "testModel1",
		"deadbeef-2f18-4fd2-967d-db9663db7bea": "testModel2",
	}, nil)

	pool := s.pool.EXPECT()
	pool.GetModelCallContext(coretesting.ModelTag.Id()).Return(newModelBackend(c, aCloud,
		coretesting.ModelTag.Id()), context.NewEmptyCloudCallContext(), nil)
	pool.GetModelCallContext("deadbeef-2f18-4fd2-967d-db9663db7bea").Return(newModelBackend(c, aCloud,
		"deadbeef-2f18-4fd2-967d-db9663db7bea"), context.NewEmptyCloudCallContext(), nil)

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: false,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{
			CredentialTag: "cloudcred-meep_julia_three",
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
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	aCloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	s.PatchValue(cloud.ValidateNewCredentialForModelFunc,
		func(
			backend credentialcommon.PersistentBackend, _ context.ProviderCallContext,
			_ names.CloudCredentialTag, _ *jujucloud.Credential, _ bool,
		) (params.ErrorResults, error) {
			if backend.(*mockModelBackend).uuid == "deadbeef-0bad-400d-8000-4b1d0d06f00d" {
				return params.ErrorResults{Results: []params.ErrorResult{{Error: &params.Error{Message: "not valid for model"}}}}, nil
			}
			return params.ErrorResults{Results: []params.ErrorResult{}}, nil
		})

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(map[string]string{
		coretesting.ModelTag.Id():              "testModel1",
		"deadbeef-2f18-4fd2-967d-db9663db7bea": "testModel2",
	}, nil)
	backend.UpdateCloudCredential(tag, jujucloud.Credential{}).Return(nil)

	pool := s.pool.EXPECT()
	pool.GetModelCallContext(coretesting.ModelTag.Id()).Return(newModelBackend(c, aCloud, coretesting.ModelTag.Id()),
		context.NewEmptyCloudCallContext(), nil)
	pool.GetModelCallContext("deadbeef-2f18-4fd2-967d-db9663db7bea").Return(newModelBackend(c, aCloud,
		"deadbeef-2f18-4fd2-967d-db9663db7bea"), nil, nil)

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: true,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{
			{
				CredentialTag: "cloudcred-meep_julia_three",
				Models: []params.UpdateCredentialModelResult{
					{
						ModelUUID: "deadbeef-0bad-400d-8000-4b1d0d06f00d",
						ModelName: "testModel1",
						Errors: []params.ErrorResult{{Error: &params.Error{Message: "not valid for model",
							Code: ""}}},
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
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	s.PatchValue(cloud.ValidateNewCredentialForModelFunc,
		func(_ credentialcommon.PersistentBackend, _ context.ProviderCallContext,
			_ names.CloudCredentialTag, _ *jujucloud.Credential, _ bool,
		) (params.ErrorResults, error) {
			return params.ErrorResults{Results: []params.ErrorResult{{Error: &params.Error{Message: "not valid for model"}}}}, nil
		})

	aCloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(map[string]string{
		coretesting.ModelTag.Id():              "testModel1",
		"deadbeef-2f18-4fd2-967d-db9663db7bea": "testModel2",
	}, nil)

	pool := s.pool.EXPECT()
	pool.GetModelCallContext(gomock.Any()).Return(newModelBackend(c, aCloud, coretesting.ModelTag.Id()),
		context.NewEmptyCloudCallContext(), nil).Times(2)

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
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
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	s.PatchValue(cloud.ValidateNewCredentialForModelFunc,
		func(_ credentialcommon.PersistentBackend, _ context.ProviderCallContext,
			_ names.CloudCredentialTag, _ *jujucloud.Credential, migrating bool) (params.ErrorResults,
			error) {
			return params.ErrorResults{Results: []params.ErrorResult{{Error: &params.Error{Message: "not valid for model"}}}}, nil
		})

	aCloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(map[string]string{
		coretesting.ModelTag.Id():              "testModel1",
		"deadbeef-2f18-4fd2-967d-db9663db7bea": "testModel2",
	}, nil)
	backend.UpdateCloudCredential(tag, jujucloud.Credential{}).Return(nil)

	pool := s.pool.EXPECT()
	pool.GetModelCallContext(gomock.Any()).Return(newModelBackend(c, aCloud, coretesting.ModelTag.Id()),
		context.NewEmptyCloudCallContext(), nil)
	pool.GetModelCallContext(gomock.Any()).Return(newModelBackend(c, aCloud, "deadbeef-2f18-4fd2-967d-db9663db7bea"),
		context.NewEmptyCloudCallContext(), nil)

	results, err := s.api.UpdateCredentialsCheckModels(params.UpdateCredentialArgs{
		Force: true,
		Credentials: []params.TaggedCredential{{
			Tag:        "cloudcred-meep_julia_three",
			Credential: params.CloudCredential{},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.UpdateCredentialResults{
		Results: []params.UpdateCredentialResult{{
			CredentialTag: "cloudcred-meep_julia_three",
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
	bruceTag := names.NewUserTag("bruce")
	defer s.setup(c, bruceTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "bruce", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(nil, nil)
	backend.RemoveCloudCredential(tag).Return(nil)
	backend.RemoveModelsCredential(tag).Return(nil)

	results, err := s.api.RevokeCredentialsCheckModels(params.RevokeCredentialArgs{
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

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(nil, nil)
	backend.RemoveCloudCredential(tag).Return(nil)
	backend.RemoveModelsCredential(tag).Return(nil)

	results, err := s.api.RevokeCredentialsCheckModels(params.RevokeCredentialArgs{
		Credentials: []params.RevokeCredentialArg{
			{Tag: "cloudcred-meep_julia_three"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	// admin can revoke others' credentials
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *cloudSuite) TestRevokeCredentialsCantGetModels(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(nil, errors.New("no niet nope"))

	results, err := s.api.RevokeCredentialsCheckModels(params.RevokeCredentialArgs{Credentials: []params.RevokeCredentialArg{
		{Tag: "cloudcred-meep_julia_three"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservererrors.ServerError(errors.New("no niet nope"))},
		},
	})
	c.Assert(c.GetTestLog(), jc.Contains, "")
}

func (s *cloudSuite) TestRevokeCredentialsForceCantGetModels(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(nil, errors.New("no niet nope"))
	backend.RemoveCloudCredential(tag).Return(nil)
	backend.RemoveModelsCredential(tag).Return(nil)

	results, err := s.api.RevokeCredentialsCheckModels(params.RevokeCredentialArgs{Credentials: []params.RevokeCredentialArg{
		{Tag: "cloudcred-meep_julia_three", Force: true},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{}, // no error: credential deleted
		},
	})
	c.Assert(c.GetTestLog(), jc.Contains,
		" WARNING juju.apiserver.cloud could not get models that use credential cloudcred-meep_julia_three: no niet nope")
}

func (s *cloudSuite) TestRevokeCredentialsHasModel(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(map[string]string{
		coretesting.ModelTag.Id(): "modelName",
	}, nil)

	results, err := s.api.RevokeCredentialsCheckModels(params.RevokeCredentialArgs{Credentials: []params.RevokeCredentialArg{
		{Tag: "cloudcred-meep_julia_three"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservererrors.ServerError(errors.New("cannot revoke credential cloudcred-meep_julia_three: it is still used by 1 model"))},
		},
	})
	c.Assert(c.GetTestLog(), jc.Contains,
		" WARNING juju.apiserver.cloud credential cloudcred-meep_julia_three cannot be deleted as it is used by model deadbeef-0bad-400d-8000-4b1d0d06f00d")
}

func (s *cloudSuite) TestRevokeCredentialsHasModels(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(map[string]string{
		coretesting.ModelTag.Id():              "modelName",
		"deadbeef-1bad-511d-8000-4b1d0d06f00d": "anotherModelName",
	}, nil)

	results, err := s.api.RevokeCredentialsCheckModels(params.RevokeCredentialArgs{Credentials: []params.RevokeCredentialArg{
		{Tag: "cloudcred-meep_julia_three"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservererrors.ServerError(errors.New("cannot revoke credential cloudcred-meep_julia_three: it is still used by 2 models"))},
		},
	})
	c.Assert(c.GetTestLog(), jc.Contains,
		` WARNING juju.apiserver.cloud credential cloudcred-meep_julia_three cannot be deleted as it is used by models:
- deadbeef-0bad-400d-8000-4b1d0d06f00d
- deadbeef-1bad-511d-8000-4b1d0d06f00d`)
}

func (s *cloudSuite) TestRevokeCredentialsForceHasModel(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(map[string]string{
		coretesting.ModelTag.Id(): "modelName",
	}, nil)
	backend.RemoveCloudCredential(tag).Return(nil)
	backend.RemoveModelsCredential(tag).Return(nil)

	results, err := s.api.RevokeCredentialsCheckModels(params.RevokeCredentialArgs{Credentials: []params.RevokeCredentialArg{
		{Tag: "cloudcred-meep_julia_three", Force: true},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{},
		},
	})
	c.Assert(c.GetTestLog(), jc.Contains,
		` WARNING juju.apiserver.cloud credential cloudcred-meep_julia_three will be deleted but it is used by model deadbeef-0bad-400d-8000-4b1d0d06f00d`)

}

func (s *cloudSuite) TestRevokeCredentialsForceMany(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	_, tagOne := cloudCredentialTag(credParams{name: "three", owner: "bruce", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)
	_, tagTwo := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tagOne).Return(map[string]string{
		coretesting.ModelTag.Id(): "modelName",
	}, nil)
	backend.CredentialModels(tagTwo).Return(map[string]string{
		coretesting.ModelTag.Id(): "modelName",
	}, nil)
	backend.RemoveCloudCredential(gomock.Any()).Return(nil)
	backend.RemoveModelsCredential(gomock.Any()).Return(nil)

	results, err := s.api.RevokeCredentialsCheckModels(params.RevokeCredentialArgs{Credentials: []params.RevokeCredentialArg{
		{Tag: "cloudcred-meep_julia_three", Force: true},
		{Tag: "cloudcred-meep_bruce_three"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{Error: apiservererrors.ServerError(errors.New("cannot revoke credential cloudcred-meep_bruce_three: it is still used by 1 model"))},
		},
	})
	c.Assert(c.GetTestLog(), jc.Contains,
		` WARNING juju.apiserver.cloud credential cloudcred-meep_julia_three will be deleted but it is used by model deadbeef-0bad-400d-8000-4b1d0d06f00d`)
	c.Assert(c.GetTestLog(), jc.Contains,
		` WARNING juju.apiserver.cloud credential cloudcred-meep_bruce_three cannot be deleted as it is used by model deadbeef-0bad-400d-8000-4b1d0d06f00d`)
}

func (s *cloudSuite) TestRevokeCredentialsClearModelCredentialsError(c *gc.C) {
	adminTag := names.NewUserTag("admin")
	defer s.setup(c, adminTag).Finish()

	_, tag := cloudCredentialTag(credParams{name: "three", owner: "julia", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	backend := s.backend.EXPECT()
	backend.CredentialModels(tag).Return(map[string]string{
		coretesting.ModelTag.Id(): "modelName",
	}, nil)
	backend.RemoveCloudCredential(tag).Return(nil)
	backend.RemoveModelsCredential(tag).Return(errors.New("kaboom"))

	results, err := s.api.RevokeCredentialsCheckModels(params.RevokeCredentialArgs{Credentials: []params.RevokeCredentialArg{
		{Tag: "cloudcred-meep_julia_three", Force: true},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservererrors.ServerError(errors.New("kaboom"))},
		},
	})
	c.Assert(c.GetTestLog(), jc.Contains,
		" WARNING juju.apiserver.cloud credential cloudcred-meep_julia_three will be deleted but it is used by model deadbeef-0bad-400d-8000-4b1d0d06f00d")
}

func (s *cloudSuite) TestCredential(c *gc.C) {
	bruceTag := names.NewUserTag("bruce")
	defer s.setup(c, bruceTag).Finish()

	credentialOne, tagOne := cloudCredentialTag(credParams{name: "foo", owner: "admin", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)
	credentialTwo, tagTwo := cloudCredentialTag(credParams{name: "two", owner: "bruce", cloudName: "meep", permission: jujucloud.UserPassAuthType,
		attrs: map[string]string{
			"username": "admin",
			"password": "adm1n",
		}}, c)

	creds := map[string]state.Credential{
		tagOne.Id(): credentialOne,
		tagTwo.Id(): credentialTwo,
	}

	cloud := jujucloud.Cloud{
		Name:      "meep",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	backend := s.backend.EXPECT()
	backend.Cloud("meep").Return(cloud, nil)
	backend.CloudCredentials(names.NewUserTag("bruce"), "meep").Return(creds, nil)

	results, err := s.api.Credential(params.Entities{Entities: []params.Entity{{
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

	credential, tag := cloudCredentialTag(credParams{name: "two", owner: "bruce", cloudName: "meep", permission: jujucloud.UserPassAuthType,
		attrs: map[string]string{
			"username": "admin",
			"password": "adm1n",
		}}, c)

	creds := map[string]state.Credential{
		tag.Id(): credential,
	}
	cloud := jujucloud.Cloud{
		Name:      "meep",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	backend := s.backend.EXPECT()
	backend.Cloud("meep").Return(cloud, nil)
	backend.CloudCredentials(names.NewUserTag("bruce"), "meep").Return(creds, nil)

	results, err := s.api.Credential(params.Entities{Entities: []params.Entity{{
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

	backend := s.backend.EXPECT()
	backend.Cloud("fluffy").Return(cloud, nil).Times(2)
	fred := names.NewUserTag("fred")
	mary := names.NewUserTag("mary")
	backend.CreateCloudAccess("fluffy", fred,
		permission.AddModelAccess).Return(nil)
	backend.RemoveCloudAccess("fluffy", mary).Return(nil)

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

	backend := s.backend.EXPECT()
	backend.Cloud("fluffy").Return(cloud, nil)
	backend.CreateCloudAccess("fluffy", fredTag,
		permission.AdminAccess).Return(errors.AlreadyExistsf("access %s", permission.AdminAccess))
	backend.GetCloudAccess("fluffy", fredTag).Return(permission.AddModelAccess,
		nil)
	backend.UpdateCloudAccess("fluffy", fredTag,
		permission.AdminAccess).Return(nil)

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

	backend := s.backend.EXPECT()
	backend.Cloud("fluffy").Return(cloud, nil)
	backend.CreateCloudAccess("fluffy", fredTag,
		permission.AdminAccess).Return(errors.AlreadyExistsf("access %s", permission.AdminAccess))
	backend.GetCloudAccess("fluffy", fredTag).Return(permission.AdminAccess,
		nil)

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
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{
		{Error: &params.Error{Message: `could not grant cloud access: user already has "admin" access or greater`}},
	})
}

func (s *cloudSuite) TestCredentialContentsAllNoSecrets(c *gc.C) {
	bruceTag := names.NewUserTag("bruce")
	defer s.setup(c, bruceTag).Finish()

	credentialOne, tagOne := cloudCredentialTag(credParams{name: "one", owner: "bruce", cloudName: "meep", permission: jujucloud.EmptyAuthType,
		attrs: map[string]string{}}, c)

	credentialTwo, tagTwo := cloudCredentialTag(credParams{name: "two", owner: "bruce", cloudName: "meep", permission: jujucloud.UserPassAuthType,
		attrs: map[string]string{
			"username": "admin",
		}}, c)

	credentialTwo.Invalid = true
	creds := []state.Credential{
		credentialOne,
		credentialTwo,
	}
	cloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}

	backend := s.backend.EXPECT()
	backend.AllCloudCredentials(bruceTag).Return(creds, nil)
	backend.Cloud("meep").Return(cloud, nil)
	backend.CredentialModelsAndOwnerAccess(tagOne).Return([]state.CredentialOwnerModelAccess{}, nil)
	backend.CredentialModelsAndOwnerAccess(tagTwo).Return([]state.CredentialOwnerModelAccess{}, nil)

	results, err := s.api.CredentialContents(params.CloudCredentialArgs{})
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

func cloudCredentialTag(params credParams,
	c *gc.C) (state.Credential, names.CloudCredentialTag) {
	cred := statetesting.CloudCredential(params.permission, params.attrs)
	cred.Name = params.name
	cred.Owner = params.owner
	cred.Cloud = params.cloudName

	tag, err := cred.CloudCredentialTag()
	c.Assert(err, jc.ErrorIsNil)

	return cred, tag
}

type credParams struct {
	name       string
	owner      string
	cloudName  string
	permission jujucloud.AuthType
	attrs      map[string]string
}

type mockModelBackend struct {
	credentialcommon.PersistentBackend
	uuid string
}
