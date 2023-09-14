// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type CloudUserSuite struct {
	ConnSuite
}

var _ = gc.Suite(&CloudUserSuite{})

func (s *CloudUserSuite) makeCloud(c *gc.C, access permission.Access) (string, names.UserTag) {
	cloudName := "fluffy"
	err := s.State.CreateCloudAccess(cloudName, names.NewUserTag("test-admin"), permission.AdminAccess)
	c.Assert(err, jc.ErrorIsNil)

	user := s.Factory.MakeUser(c,
		&factory.UserParams{Name: "validusername"})

	// Initially no access.
	_, err = s.State.UserPermission(user.UserTag(), names.NewCloudTag(cloudName))
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	err = s.State.CreateCloudAccess(cloudName, user.UserTag(), access)
	c.Assert(err, jc.ErrorIsNil)
	return cloudName, user.UserTag()
}

func (s *CloudUserSuite) assertAddCloud(c *gc.C, wantedAccess permission.Access) string {
	cloudName, user := s.makeCloud(c, wantedAccess)

	access, err := s.State.GetCloudAccess(cloudName, user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, wantedAccess)

	// Creator of cloud has admin.
	access, err = s.State.GetCloudAccess(cloudName, names.NewUserTag("test-admin"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.AdminAccess)

	// Everyone else has no access.
	_, err = s.State.GetCloudAccess(cloudName, names.NewUserTag("everyone@external"))
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	return cloudName
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
	cloudName, user := s.makeCloud(c, permission.AdminAccess)
	err := s.State.UpdateCloudAccess(cloudName, user, permission.AddModelAccess)
	c.Assert(err, jc.ErrorIsNil)

	access, err := s.State.GetCloudAccess(cloudName, user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.AddModelAccess)
}

func (s *CloudUserSuite) TestCreateCloudAccessNoUserFails(c *gc.C) {
	err := s.State.CreateCloudAccess(
		"fluffy",
		names.NewUserTag("validusername"), permission.AddModelAccess)
	c.Assert(err, gc.ErrorMatches, `user "validusername" does not exist locally: user "validusername" not found`)
}

func (s *CloudUserSuite) TestRemoveCloudAccess(c *gc.C) {
	cloudName, user := s.makeCloud(c, permission.AddModelAccess)

	err := s.State.RemoveCloudAccess(cloudName, user)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.GetCloudAccess(cloudName, user)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *CloudUserSuite) TestRemoveCloudAccessNoUser(c *gc.C) {
	cloudName, _ := s.makeCloud(c, permission.AddModelAccess)
	err := s.State.RemoveCloudAccess(cloudName, names.NewUserTag("fred"))
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *CloudUserSuite) TestCloudsForUser(c *gc.C) {
	cloudName := s.assertAddCloud(c, permission.AddModelAccess)
	info, err := s.State.CloudsForUser(names.NewUserTag("validusername"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, []state.CloudInfo{
		{
			Name:   cloudName,
			Access: permission.AddModelAccess,
		},
	})
}
