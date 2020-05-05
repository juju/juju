// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type CloudSuite struct {
	ConnSuite
}

var _ = gc.Suite(&CloudSuite{})

var lowCloud = cloud.Cloud{
	Name:             "stratus",
	Type:             "low",
	AuthTypes:        cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	Endpoint:         "global-endpoint",
	IdentityEndpoint: "identity-endpoint",
	StorageEndpoint:  "storage-endpoint",
	Regions: []cloud.Region{{
		Name:             "region1",
		Endpoint:         "region1-endpoint",
		IdentityEndpoint: "region1-identity",
		StorageEndpoint:  "region1-storage",
	}, {
		Name:             "region2",
		Endpoint:         "region2-endpoint",
		IdentityEndpoint: "region2-identity",
		StorageEndpoint:  "region2-storage",
	}},
	CACertificates: []string{"cert1", "cert2"},
}

func (s *CloudSuite) TestCloudNotFound(c *gc.C) {
	cld, err := s.State.Cloud("unknown")
	c.Assert(err, gc.ErrorMatches, `cloud "unknown" not found`)
	c.Assert(cld, jc.DeepEquals, cloud.Cloud{})
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CloudSuite) TestClouds(c *gc.C) {
	dummyCloud, err := s.State.Cloud("dummy")
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AddCloud(lowCloud, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)

	clouds, err := s.State.Clouds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, jc.DeepEquals, map[names.CloudTag]cloud.Cloud{
		names.NewCloudTag("dummy"):   dummyCloud,
		names.NewCloudTag("stratus"): lowCloud,
	})
}

func (s *CloudSuite) TestAddCloud(c *gc.C) {
	cloudToAdd := lowCloud
	cloudToAdd.Config = map[string]interface{}{"foo": "bar"}
	cloudToAdd.RegionConfig = cloud.RegionConfig{"us-east": {"foo": "baz"}}
	err := s.State.AddCloud(cloudToAdd, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)
	cloud, err := s.State.Cloud("stratus")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloud, jc.DeepEquals, lowCloud)
	access, err := s.State.GetCloudAccess(lowCloud.Name, s.Owner)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.AdminAccess)

	settings, err := s.State.ReadSettings(state.GlobalSettingsC, state.CloudGlobalKey(lowCloud.Name))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings.Map(), jc.DeepEquals, cloudToAdd.Config)
	settings, err = s.State.ReadSettings(state.GlobalSettingsC, state.RegionSettingsGlobalKey(lowCloud.Name, "us-east"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings.Map(), jc.DeepEquals, map[string]interface{}{"foo": "baz"})
}

func (s *CloudSuite) TestAddCloudDuplicate(c *gc.C) {
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "stratus",
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	}, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AddCloud(cloud.Cloud{
		Name:      "stratus",
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	}, s.Owner.Name())
	c.Assert(err, gc.ErrorMatches, `cloud "stratus" already exists`)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *CloudSuite) TestAddCloudNoName(c *gc.C) {
	err := s.State.AddCloud(cloud.Cloud{
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	}, s.Owner.Name())
	c.Assert(err, gc.ErrorMatches, `invalid cloud: empty Name not valid`)
}

func (s *CloudSuite) TestAddCloudNoType(c *gc.C) {
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "stratus",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	}, s.Owner.Name())
	c.Assert(err, gc.ErrorMatches, `invalid cloud: empty Type not valid`)
}

func (s *CloudSuite) TestAddCloudNoAuthTypes(c *gc.C) {
	err := s.State.AddCloud(cloud.Cloud{
		Name: "stratus",
		Type: "foo",
	}, s.Owner.Name())
	c.Assert(err, gc.ErrorMatches, `invalid cloud: empty auth-types not valid`)
}

func (s *CloudSuite) TestUpdateCloud(c *gc.C) {
	err := s.State.AddCloud(lowCloud, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)

	updatedCloud := lowCloud
	updatedCloud.Endpoint = "updated-global-endpoint"
	updatedCloud.Regions[0] = cloud.Region{
		Name:             "updatedregion1",
		Endpoint:         "updatedregion1-endpoint",
		IdentityEndpoint: "updatedregion1-identity",
		StorageEndpoint:  "updatedregion1-storage",
	}
	updatedCloud.Regions[1] = cloud.Region{
		Name:             "updatedregion2",
		Endpoint:         "updatedregion2-endpoint",
		IdentityEndpoint: "updatedregion2-identity",
		StorageEndpoint:  "updatedregion2-storage",
	}
	updatedCloud.CACertificates[0] = "updatedcert1"
	updatedCloud.Config = map[string]interface{}{"foo": "bar"}
	updatedCloud.RegionConfig = cloud.RegionConfig{"us-east": {"foo": "baz"}}

	err = s.State.UpdateCloud(updatedCloud)
	c.Assert(err, jc.ErrorIsNil)

	updatedCloud.Config = nil
	updatedCloud.RegionConfig = nil
	cloud, err := s.State.Cloud("stratus")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloud, jc.DeepEquals, updatedCloud)
	access, err := s.State.GetCloudAccess(updatedCloud.Name, s.Owner)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.AdminAccess)

	settings, err := s.State.ReadSettings(state.GlobalSettingsC, state.CloudGlobalKey(updatedCloud.Name))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings.Map(), jc.DeepEquals, map[string]interface{}{"foo": "bar"})
	settings, err = s.State.ReadSettings(state.GlobalSettingsC, state.RegionSettingsGlobalKey(updatedCloud.Name, "us-east"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings.Map(), jc.DeepEquals, map[string]interface{}{"foo": "baz"})
}

func (s *CloudSuite) TestUpdateNonExistentCloud(c *gc.C) {
	missNamed := lowCloud
	missNamed.Name = "nope"
	err := s.State.UpdateCloud(missNamed)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CloudSuite) TestRemoveNonExistentCloud(c *gc.C) {
	err := s.State.RemoveCloud("foo")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CloudSuite) TestRemoveCloud(c *gc.C) {
	cloudToAdd := lowCloud
	cloudToAdd.Config = map[string]interface{}{"foo": "bar"}
	err := s.State.AddCloud(cloudToAdd, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)

	// Add some region serrings to ensure they're removed also.
	settings := state.NewStateSettingsForCollection(s.State, state.GlobalSettingsC)
	err = settings.CreateSettings(
		state.RegionSettingsGlobalKey(lowCloud.Name, "someregion"),
		map[string]interface{}{"fred": "mary"})
	c.Assert(err, jc.ErrorIsNil)
	err = settings.CreateSettings(
		state.CloudGlobalKey("another"),
		map[string]interface{}{"fred": "mary2"})
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.RemoveCloud(lowCloud.Name)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.Cloud(lowCloud.Name)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.State.ReadSettings(state.GlobalSettingsC, state.CloudGlobalKey(lowCloud.Name))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.State.ReadSettings(state.GlobalSettingsC, state.RegionSettingsGlobalKey(lowCloud.Name, "someregion"))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	stateSettings, err := s.State.ReadSettings(state.GlobalSettingsC, state.CloudGlobalKey("another"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stateSettings.Map(), jc.DeepEquals, map[string]interface{}{"fred": "mary2"})
}

func (s *CloudSuite) TestRemoveCloudAlsoRemovesCredentials(c *gc.C) {
	err := s.State.AddCloud(lowCloud, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)

	credTag := names.NewCloudCredentialTag(lowCloud.Name + "/admin/cred")
	err = s.State.UpdateCloudCredential(credTag, cloud.NewCredential(cloud.UserPassAuthType, nil))
	c.Assert(err, jc.ErrorIsNil)
	credTag = names.NewCloudCredentialTag(lowCloud.Name + "/bob/cred")
	err = s.State.UpdateCloudCredential(credTag, cloud.NewCredential(cloud.UserPassAuthType, nil))
	c.Assert(err, jc.ErrorIsNil)

	// Add credential for a different cloud, shouldn't be touched.
	otherCredTag := names.NewCloudCredentialTag("dummy/mary/cred")
	err = s.State.UpdateCloudCredential(otherCredTag, cloud.NewEmptyCredential())
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.RemoveCloud(lowCloud.Name)
	c.Assert(err, jc.ErrorIsNil)

	coll, closer := state.GetCollection(s.State, "cloudCredentials")
	defer closer()

	// Creds for removed cloud are gone.
	n, err := coll.Find(bson.D{{"_id", bson.D{{"$regex", "^" + lowCloud.Name + "#"}}}}).Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 0)

	// Creds for other clouds are not affected.
	n, err = coll.Find(bson.D{{"_id", bson.D{{"$regex", "^dummy#"}}}}).Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1)
	_, err = s.State.CloudCredential(otherCredTag)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CloudSuite) TestRemoveCloudAlsoRemovesPermissions(c *gc.C) {
	err := s.State.AddCloud(lowCloud, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.UpdateCloudAccess(lowCloud.Name, s.Owner, permission.AddModelAccess)
	c.Assert(err, jc.ErrorIsNil)

	// Add permission for a different cloud, shouldn't be touched.
	err = s.State.CreateCloudAccess("othercloud", s.Owner, permission.AddModelAccess)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.RemoveCloud(lowCloud.Name)
	c.Assert(err, jc.ErrorIsNil)

	coll, closer := state.GetCollection(s.State, "permissions")
	defer closer()

	// Permissions for removed cloud are gone.
	n, err := coll.Find(bson.D{{"_id", bson.D{{"$regex", "^cloud#" + lowCloud.Name + "#"}}}}).Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 0)

	// Permissions for other clouds are not affected.
	n, err = coll.Find(bson.D{{"_id", bson.D{{"$regex", "^cloud#othercloud#"}}}}).Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1)
	_, err = s.State.GetCloudAccess("othercloud", s.Owner)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CloudSuite) TestRemoveControllerModelCloudNotAllowed(c *gc.C) {
	err := s.State.RemoveCloud("dummy")
	c.Assert(err, gc.ErrorMatches, "cloud is used by 1 model")
	_, err = s.State.Cloud("dummy")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CloudSuite) TestRemoveInUseCloudNotAllowed(c *gc.C) {
	err := s.State.AddCloud(lowCloud, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)
	otherModelOwner := s.Factory.MakeModelUser(c, nil)
	credTag := names.NewCloudCredentialTag(lowCloud.Name + "/" + otherModelOwner.UserName + "/cred")
	err = s.State.UpdateCloudCredential(credTag, cloud.NewCredential(cloud.UserPassAuthType, nil))
	c.Assert(err, jc.ErrorIsNil)

	otherSt := s.Factory.MakeCAASModel(c, &factory.ModelParams{
		CloudName:       lowCloud.Name,
		CloudRegion:     "region1",
		CloudCredential: credTag,
		Owner:           otherModelOwner.UserTag,
		ConfigAttrs: testing.Attrs{
			"controller": false,
		},
	})
	defer otherSt.Close()

	err = otherSt.RemoveCloud(lowCloud.Name)
	c.Assert(err, gc.ErrorMatches, "cloud is used by 1 model")
	_, err = s.State.Cloud(lowCloud.Name)
	c.Assert(err, jc.ErrorIsNil)

}

func (s *CloudSuite) TestRemoveCloudNewModelRace(c *gc.C) {
	err := s.State.AddCloud(lowCloud, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)
	otherModelOwner := s.Factory.MakeModelUser(c, nil)
	credTag := names.NewCloudCredentialTag(lowCloud.Name + "/" + otherModelOwner.UserName + "/cred")
	err = s.State.UpdateCloudCredential(credTag, cloud.NewCredential(cloud.UserPassAuthType, nil))
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		otherSt := s.Factory.MakeCAASModel(c, &factory.ModelParams{
			CloudName:       lowCloud.Name,
			CloudRegion:     "region1",
			CloudCredential: credTag,
			Owner:           otherModelOwner.UserTag,
			ConfigAttrs: testing.Attrs{
				"controller": false,
			},
		})
		defer otherSt.Close()
	}).Check()

	err = s.State.RemoveCloud(lowCloud.Name)
	c.Assert(err, gc.ErrorMatches, "cloud is used by 1 model")
}

func (s *CloudSuite) TestRemoveCloudRace(c *gc.C) {
	err := s.State.AddCloud(lowCloud, s.Owner.Name())
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		err = s.State.RemoveCloud(lowCloud.Name)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err = s.State.RemoveCloud(lowCloud.Name)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.Cloud(lowCloud.Name)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
