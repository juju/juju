// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type EnvUserSuite struct {
	ConnSuite
}

var _ = gc.Suite(&EnvUserSuite{})

func (s *EnvUserSuite) TestAddEnvironmentUser(c *gc.C) {
	now := state.NowToTheSecond()
	user := s.factory.MakeUser(c, &factory.UserParams{Name: "validusername", NoEnvUser: true})
	createdBy := s.factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	envUser, err := s.State.AddEnvironmentUser(user.UserTag(), createdBy.UserTag(), "")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(envUser.ID(), gc.Equals, fmt.Sprintf("%s:validusername@local", s.envTag.Id()))
	c.Assert(envUser.EnvironmentTag(), gc.Equals, s.envTag)
	c.Assert(envUser.UserName(), gc.Equals, "validusername@local")
	c.Assert(envUser.DisplayName(), gc.Equals, user.DisplayName())
	c.Assert(envUser.CreatedBy(), gc.Equals, "createdby@local")
	c.Assert(envUser.DateCreated().Equal(now) || envUser.DateCreated().After(now), jc.IsTrue)
	c.Assert(envUser.LastConnection(), gc.IsNil)

	envUser, err = s.State.EnvironmentUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envUser.ID(), gc.Equals, fmt.Sprintf("%s:validusername@local", s.envTag.Id()))
	c.Assert(envUser.EnvironmentTag(), gc.Equals, s.envTag)
	c.Assert(envUser.UserName(), gc.Equals, "validusername@local")
	c.Assert(envUser.DisplayName(), gc.Equals, user.DisplayName())
	c.Assert(envUser.CreatedBy(), gc.Equals, "createdby@local")
	c.Assert(envUser.DateCreated().Equal(now) || envUser.DateCreated().After(now), jc.IsTrue)
	c.Assert(envUser.LastConnection(), gc.IsNil)
}

func (s *EnvUserSuite) TestCaseSensitiveEnvUserErrors(c *gc.C) {
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	s.factory.MakeEnvUser(c, &factory.EnvUserParams{User: "Bob@ubuntuone"})

	_, err = s.State.AddEnvironmentUser(names.NewUserTag("boB@ubuntuone"), env.Owner(), "")
	c.Assert(err, gc.ErrorMatches, `environment user "boB@ubuntuone" already exists`)
	c.Assert(errors.IsAlreadyExists(err), jc.IsTrue)
}

func (s *EnvUserSuite) TestCaseInsensitiveLookupInMultiEnvirons(c *gc.C) {
	assertIsolated := func(st1, st2 *state.State, usernames ...string) {
		f := factory.NewFactory(st1)
		expectedUser := f.MakeEnvUser(c, &factory.EnvUserParams{User: usernames[0]})

		// assert case insensitive lookup for each username
		for _, username := range usernames {
			userTag := names.NewUserTag(username)
			obtainedUser, err := st1.EnvironmentUser(userTag)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(obtainedUser, gc.DeepEquals, expectedUser)

			_, err = st2.EnvironmentUser(userTag)
			c.Assert(errors.IsNotFound(err), jc.IsTrue)
		}
	}

	otherSt := s.factory.MakeEnvironment(c, nil)
	defer otherSt.Close()
	assertIsolated(s.State, otherSt,
		"Bob@UbuntuOne",
		"bob@ubuntuone",
		"BOB@UBUNTUONE",
	)
	assertIsolated(otherSt, s.State,
		"Sam@UbuntuOne",
		"sam@ubuntuone",
		"SAM@UBUNTUONE",
	)
}

func (s *EnvUserSuite) TestAddEnvironmentDisplayName(c *gc.C) {
	envUserDefault := s.factory.MakeEnvUser(c, nil)
	c.Assert(envUserDefault.DisplayName(), gc.Matches, "display name-[0-9]*")

	envUser := s.factory.MakeEnvUser(c, &factory.EnvUserParams{DisplayName: "Override user display name"})
	c.Assert(envUser.DisplayName(), gc.Equals, "Override user display name")
}

func (s *EnvUserSuite) TestAddEnvironmentNoUserFails(c *gc.C) {
	createdBy := s.factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	_, err := s.State.AddEnvironmentUser(names.NewLocalUserTag("validusername"), createdBy.UserTag(), "")
	c.Assert(err, gc.ErrorMatches, `user "validusername" does not exist locally: user "validusername" not found`)
}

func (s *EnvUserSuite) TestAddEnvironmentNoCreatedByUserFails(c *gc.C) {
	user := s.factory.MakeUser(c, &factory.UserParams{Name: "validusername"})
	_, err := s.State.AddEnvironmentUser(user.UserTag(), names.NewLocalUserTag("createdby"), "")
	c.Assert(err, gc.ErrorMatches, `createdBy user "createdby" does not exist locally: user "createdby" not found`)
}

func (s *EnvUserSuite) TestRemoveEnvironmentUser(c *gc.C) {
	user := s.factory.MakeUser(c, &factory.UserParams{Name: "validUsername"})
	_, err := s.State.EnvironmentUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.RemoveEnvironmentUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.EnvironmentUser(user.UserTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *EnvUserSuite) TestRemoveEnvironmentUserFails(c *gc.C) {
	user := s.factory.MakeUser(c, &factory.UserParams{NoEnvUser: true})
	err := s.State.RemoveEnvironmentUser(user.UserTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *EnvUserSuite) TestUpdateLastConnection(c *gc.C) {
	now := state.NowToTheSecond()
	createdBy := s.factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	user := s.factory.MakeUser(c, &factory.UserParams{Name: "validusername", Creator: createdBy.Tag()})
	envUser, err := s.State.EnvironmentUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	err = envUser.UpdateLastConnection()
	c.Assert(err, jc.ErrorIsNil)
	// It is possible that the update is done over a second boundary, so we need
	// to check for after now as well as equal.
	c.Assert(envUser.LastConnection().After(now) ||
		envUser.LastConnection().Equal(now), jc.IsTrue)
}

func (s *EnvUserSuite) TestEnvironmentsForUserNone(c *gc.C) {
	tag := names.NewUserTag("non-existent@remote")
	environments, err := s.State.EnvironmentsForUser(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(environments, gc.HasLen, 0)
}

func (s *EnvUserSuite) TestEnvironmentsForUserNewLocalUser(c *gc.C) {
	user := s.factory.MakeUser(c, &factory.UserParams{NoEnvUser: true})
	environments, err := s.State.EnvironmentsForUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(environments, gc.HasLen, 0)
}

func (s *EnvUserSuite) TestEnvironmentsForUser(c *gc.C) {
	user := s.factory.MakeUser(c, nil)
	environments, err := s.State.EnvironmentsForUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(environments, gc.HasLen, 1)
	c.Assert(environments[0].UUID(), gc.Equals, s.State.EnvironUUID())
}

func (s *EnvUserSuite) newEnvWithOwner(c *gc.C, name string, owner names.UserTag) *state.Environment {
	// Don't use the factory to call MakeEnvironment because it may at some
	// time in the future be modified to do additional things.  Instead call
	// the state method directly to create an environment to make sure that
	// the owner is able to access the environment.
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	cfg := testing.CustomEnvironConfig(c, testing.Attrs{
		"name": name,
		"uuid": uuid.String(),
	})
	env, st, err := s.State.NewEnvironment(cfg, owner)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	return env
}

func (s *EnvUserSuite) TestEnvironmentsForUserEnvOwner(c *gc.C) {
	owner := names.NewUserTag("external@remote")
	env := s.newEnvWithOwner(c, "test-env", owner)

	environments, err := s.State.EnvironmentsForUser(owner)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(environments, gc.HasLen, 1)
	s.checkSameEnvironment(c, environments[0], env)
}

func (s *EnvUserSuite) checkSameEnvironment(c *gc.C, env1, env2 *state.Environment) {
	c.Check(env1.Name(), gc.Equals, env2.Name())
	c.Check(env1.UUID(), gc.Equals, env2.UUID())
}

func (s *EnvUserSuite) newEnvWithUser(c *gc.C, name string, user names.UserTag) *state.Environment {
	envState := s.factory.MakeEnvironment(c, &factory.EnvParams{Name: name})
	defer envState.Close()
	newEnv, err := envState.Environment()
	c.Assert(err, jc.ErrorIsNil)

	_, err = envState.AddEnvironmentUser(user, newEnv.Owner(), "")
	c.Assert(err, jc.ErrorIsNil)
	return newEnv
}

func (s *EnvUserSuite) TestEnvironmentsForUserOfNewEnv(c *gc.C) {
	userTag := names.NewUserTag("external@remote")
	env := s.newEnvWithUser(c, "test-env", userTag)

	environments, err := s.State.EnvironmentsForUser(userTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(environments, gc.HasLen, 1)
	s.checkSameEnvironment(c, environments[0], env)
}

func (s *EnvUserSuite) TestEnvironmentsForUserMultiple(c *gc.C) {
	userTag := names.NewUserTag("external@remote")
	expected := []*state.Environment{
		s.newEnvWithUser(c, "user1", userTag),
		s.newEnvWithUser(c, "user2", userTag),
		s.newEnvWithUser(c, "user3", userTag),
		s.newEnvWithOwner(c, "owner1", userTag),
		s.newEnvWithOwner(c, "owner2", userTag),
	}
	sort.Sort(UUIDOrder(expected))

	environments, err := s.State.EnvironmentsForUser(userTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(environments, gc.HasLen, len(expected))
	sort.Sort(UUIDOrder(environments))
	for i := range expected {
		s.checkSameEnvironment(c, environments[i], expected[i])
	}
}

// UUIDOrder is used to sort the environments into a stable order
type UUIDOrder []*state.Environment

func (a UUIDOrder) Len() int           { return len(a) }
func (a UUIDOrder) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a UUIDOrder) Less(i, j int) bool { return a[i].UUID() < a[j].UUID() }
