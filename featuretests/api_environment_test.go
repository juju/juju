// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type apiEnvironmentSuite struct {
	testing.JujuConnSuite
	client *api.Client
}

func (s *apiEnvironmentSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var err error
	s.client, err = juju.NewAPIClientFromName("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.client, gc.NotNil)
}

func (s *apiEnvironmentSuite) TearDownTest(c *gc.C) {
	s.client.ClientFacade.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *apiEnvironmentSuite) TestEnvironmentShare(c *gc.C) {
	user := names.NewUserTag("foo@ubuntuone")

	err := s.client.ShareEnvironment(user)
	c.Assert(err, jc.ErrorIsNil)

	envUser, err := s.State.EnvironmentUser(user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envUser.UserName(), gc.Equals, user.Username())
	c.Assert(envUser.CreatedBy(), gc.Equals, s.AdminUserTag(c).Username())
	c.Assert(envUser.LastConnection(), gc.IsNil)
}

func (s *apiEnvironmentSuite) TestEnvironmentUnshare(c *gc.C) {
	// Firt share an environment with a user.
	user := names.NewUserTag("foo@ubuntuone")
	err := s.client.ShareEnvironment(user)
	c.Assert(err, jc.ErrorIsNil)

	envUser, err := s.State.EnvironmentUser(user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envUser, gc.NotNil)

	// Then test unsharing the environment.
	err = s.client.UnshareEnvironment(user)
	c.Assert(err, jc.ErrorIsNil)

	envUser, err = s.State.EnvironmentUser(user)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
	c.Assert(envUser, gc.IsNil)
}

func (s *apiEnvironmentSuite) TestEnvironmentUserInfo(c *gc.C) {
	envUser := s.Factory.MakeEnvUser(c, &factory.EnvUserParams{User: "bobjohns@ubuntuone", DisplayName: "Bob Johns"})
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	owner, err := s.State.EnvironmentUser(env.Owner())
	c.Assert(err, jc.ErrorIsNil)

	obtained, err := s.client.EnvironmentUserInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, []params.EnvUserInfo{
		{
			UserName:       owner.UserName(),
			DisplayName:    owner.DisplayName(),
			CreatedBy:      owner.UserName(),
			DateCreated:    owner.DateCreated(),
			LastConnection: owner.LastConnection(),
		}, {
			UserName:       "bobjohns@ubuntuone",
			DisplayName:    "Bob Johns",
			CreatedBy:      owner.UserName(),
			DateCreated:    envUser.DateCreated(),
			LastConnection: envUser.LastConnection(),
		},
	})
}
