// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type CloudUserSuite struct {
	ConnSuite
}

var _ = gc.Suite(&CloudUserSuite{})

func (s *CloudUserSuite) makeCloud(c *gc.C, access permission.Access) (cloud.Cloud, names.UserTag) {
	cloud := cloud.Cloud{
		Name:      "fluffy",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.UserPassAuthType},
	}
	err := s.State.AddCloud(cloud, "test-admin")
	c.Assert(err, jc.ErrorIsNil)
	user := s.Factory.MakeUser(c,
		&factory.UserParams{Name: "validusername"})

	// Initially no access.
	_, err = s.State.UserPermission(user.UserTag(), names.NewCloudTag(cloud.Name))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	err = s.State.CreateCloudAccess(cloud.Name, user.UserTag(), access)
	c.Assert(err, jc.ErrorIsNil)
	return cloud, user.UserTag()
}

func (s *CloudUserSuite) assertAddCloud(c *gc.C, wantedAccess permission.Access) string {
	cloud, user := s.makeCloud(c, wantedAccess)

	access, err := s.State.GetCloudAccess(cloud.Name, user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, wantedAccess)

	// Creator of cloud has admin.
	access, err = s.State.GetCloudAccess(cloud.Name, names.NewUserTag("test-admin"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.AdminAccess)

	// Everyone else has no access.
	_, err = s.State.GetCloudAccess(cloud.Name, names.NewUserTag("everyone@external"))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	return cloud.Name
}

func (s *CloudUserSuite) TestAddModelUser(c *gc.C) {
	s.assertAddCloud(c, permission.AddModelAccess)
}

func (s *CloudUserSuite) TestGetCloudAccess(c *gc.C) {
	cloud := s.assertAddCloud(c, permission.AddModelAccess)
	users, err := s.State.GetCloudUsers(cloud)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(users, jc.DeepEquals, map[string]permission.Access{
		"test-admin":    permission.AdminAccess,
		"validusername": permission.AddModelAccess,
	})
}

func (s *CloudUserSuite) TestUpdateCloudAccess(c *gc.C) {
	cloud, user := s.makeCloud(c, permission.AdminAccess)
	err := s.State.UpdateCloudAccess(cloud.Name, user, permission.AddModelAccess)
	c.Assert(err, jc.ErrorIsNil)

	access, err := s.State.GetCloudAccess(cloud.Name, user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.AddModelAccess)
}

func (s *CloudUserSuite) TestCreateCloudAccessNoUserFails(c *gc.C) {
	cloud := cloud.Cloud{
		Name:      "fluffy",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.UserPassAuthType},
	}
	err := s.State.AddCloud(cloud, "test-admin")
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.CreateCloudAccess(
		"fluffy",
		names.NewUserTag("validusername"), permission.AddModelAccess)
	c.Assert(err, gc.ErrorMatches, `user "validusername" does not exist locally: user "validusername" not found`)
}

func (s *CloudUserSuite) TestRemoveCloudAccess(c *gc.C) {
	cloud, user := s.makeCloud(c, permission.AddModelAccess)

	err := s.State.RemoveCloudAccess(cloud.Name, user)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.GetCloudAccess(cloud.Name, user)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CloudUserSuite) TestRemoveCloudAccessNoUser(c *gc.C) {
	cloud, _ := s.makeCloud(c, permission.AddModelAccess)
	err := s.State.RemoveCloudAccess(cloud.Name, names.NewUserTag("fred"))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CloudUserSuite) TestCloudsForUser(c *gc.C) {
	cloudName := s.assertAddCloud(c, permission.AddModelAccess)
	info, err := s.State.CloudsForUser(names.NewUserTag("validusername"), false)
	c.Assert(err, jc.ErrorIsNil)
	cloud, err := s.State.Cloud(cloudName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, []state.CloudInfo{
		{
			Cloud:  cloud,
			Access: permission.AddModelAccess,
		},
	})
}

func (s *CloudUserSuite) TestCloudsForUserAll(c *gc.C) {
	cloudName := s.assertAddCloud(c, permission.AddModelAccess)
	info, err := s.State.CloudsForUser(names.NewUserTag("test-admin"), true)
	c.Assert(err, jc.ErrorIsNil)
	cloud, err := s.State.Cloud(cloudName)
	c.Assert(err, jc.ErrorIsNil)
	controllerInfo, err := s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	controllerCloud, err := s.State.Cloud(controllerInfo.CloudName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, []state.CloudInfo{
		{
			Cloud:  controllerCloud,
			Access: permission.AdminAccess,
		}, {
			Cloud:  cloud,
			Access: permission.AdminAccess,
		},
	})
}
