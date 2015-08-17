// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type EnvironSuite struct {
	ConnSuite
}

var _ = gc.Suite(&EnvironSuite{})

func (s *EnvironSuite) TestEnvironment(c *gc.C) {
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)

	expectedTag := names.NewEnvironTag(env.UUID())
	c.Assert(env.Tag(), gc.Equals, expectedTag)
	c.Assert(env.ServerTag(), gc.Equals, expectedTag)
	c.Assert(env.Name(), gc.Equals, "testenv")
	c.Assert(env.Owner(), gc.Equals, s.Owner)
	c.Assert(env.Life(), gc.Equals, state.Alive)
}

func (s *EnvironSuite) TestNewEnvironmentNonExistentLocalUser(c *gc.C) {
	cfg, _ := s.createTestEnvConfig(c)
	owner := names.NewUserTag("non-existent@local")

	_, _, err := s.State.NewEnvironment(cfg, owner)
	c.Assert(err, gc.ErrorMatches, `cannot create environment: user "non-existent" not found`)
}

func (s *EnvironSuite) TestNewEnvironmentSameUserSameNameFails(c *gc.C) {
	cfg, _ := s.createTestEnvConfig(c)
	owner := s.Factory.MakeUser(c, nil).UserTag()

	// Create the first environment.
	_, st1, err := s.State.NewEnvironment(cfg, owner)
	c.Assert(err, jc.ErrorIsNil)
	defer st1.Close()

	// Attempt to create another environment with a different UUID but the
	// same owner and name as the first.
	newUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	cfg2 := testing.CustomEnvironConfig(c, testing.Attrs{
		"name": cfg.Name(),
		"uuid": newUUID.String(),
	})
	_, _, err = s.State.NewEnvironment(cfg2, owner)
	errMsg := fmt.Sprintf("environment %q for %s already exists", cfg2.Name(), owner.Username())
	c.Assert(err, gc.ErrorMatches, errMsg)
	c.Assert(errors.IsAlreadyExists(err), jc.IsTrue)

	// Remove the first environment.
	env1, err := st1.Environment()
	c.Assert(err, jc.ErrorIsNil)
	err = env1.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = st1.RemoveAllEnvironDocs()
	c.Assert(err, jc.ErrorIsNil)

	// We should now be able to create the other environment.
	env2, st2, err := s.State.NewEnvironment(cfg2, owner)
	c.Assert(err, jc.ErrorIsNil)
	defer st2.Close()
	c.Assert(env2, gc.NotNil)
	c.Assert(st2, gc.NotNil)
}

func (s *EnvironSuite) TestNewEnvironment(c *gc.C) {
	cfg, uuid := s.createTestEnvConfig(c)
	owner := names.NewUserTag("test@remote")

	env, st, err := s.State.NewEnvironment(cfg, owner)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	envTag := names.NewEnvironTag(uuid)
	assertEnvMatches := func(env *state.Environment) {
		c.Assert(env.UUID(), gc.Equals, envTag.Id())
		c.Assert(env.Tag(), gc.Equals, envTag)
		c.Assert(env.ServerTag(), gc.Equals, s.envTag)
		c.Assert(env.Owner(), gc.Equals, owner)
		c.Assert(env.Name(), gc.Equals, "testing")
		c.Assert(env.Life(), gc.Equals, state.Alive)
	}
	assertEnvMatches(env)

	// Since the environ tag for the State connection is different,
	// asking for this environment through FindEntity returns a not found error.
	env, err = s.State.GetEnvironment(envTag)
	c.Assert(err, jc.ErrorIsNil)
	assertEnvMatches(env)

	env, err = st.Environment()
	c.Assert(err, jc.ErrorIsNil)
	assertEnvMatches(env)

	_, err = s.State.FindEntity(envTag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	entity, err := st.FindEntity(envTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.Tag(), gc.Equals, envTag)

	// Ensure the environment is functional by adding a machine
	_, err = st.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EnvironSuite) TestStateServerEnvironment(c *gc.C) {
	env, err := s.State.StateServerEnvironment()
	c.Assert(err, jc.ErrorIsNil)

	expectedTag := names.NewEnvironTag(env.UUID())
	c.Assert(env.Tag(), gc.Equals, expectedTag)
	c.Assert(env.ServerTag(), gc.Equals, expectedTag)
	c.Assert(env.Name(), gc.Equals, "testenv")
	c.Assert(env.Owner(), gc.Equals, s.Owner)
	c.Assert(env.Life(), gc.Equals, state.Alive)
}

func (s *EnvironSuite) TestStateServerEnvironmentAccessibleFromOtherEnvironments(c *gc.C) {
	cfg, _ := s.createTestEnvConfig(c)
	_, st, err := s.State.NewEnvironment(cfg, names.NewUserTag("test@remote"))
	defer st.Close()

	env, err := st.StateServerEnvironment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Tag(), gc.Equals, s.envTag)
	c.Assert(env.Name(), gc.Equals, "testenv")
	c.Assert(env.Owner(), gc.Equals, s.Owner)
	c.Assert(env.Life(), gc.Equals, state.Alive)
}

func (s *EnvironSuite) TestConfigForStateServerEnv(c *gc.C) {
	otherState := s.Factory.MakeEnvironment(c, &factory.EnvParams{Name: "other"})
	defer otherState.Close()

	env, err := otherState.GetEnvironment(s.envTag)
	c.Assert(err, jc.ErrorIsNil)

	conf, err := env.Config()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conf.Name(), gc.Equals, "testenv")
	uuid, ok := conf.UUID()
	c.Assert(ok, jc.IsTrue)
	c.Assert(uuid, gc.Equals, s.envTag.Id())
}

func (s *EnvironSuite) TestConfigForOtherEnv(c *gc.C) {
	otherState := s.Factory.MakeEnvironment(c, &factory.EnvParams{Name: "other"})
	defer otherState.Close()
	otherEnv, err := otherState.Environment()
	c.Assert(err, jc.ErrorIsNil)

	// By getting the environment through a different state connection,
	// the underlying state pointer in the *state.Environment struct has
	// a different environment tag.
	env, err := s.State.GetEnvironment(otherEnv.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)

	conf, err := env.Config()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conf.Name(), gc.Equals, "other")
	uuid, ok := conf.UUID()
	c.Assert(ok, jc.IsTrue)
	c.Assert(uuid, gc.Equals, otherEnv.UUID())
}

// createTestEnvConfig returns a new environment config and its UUID for testing.
func (s *EnvironSuite) createTestEnvConfig(c *gc.C) (*config.Config, string) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return testing.CustomEnvironConfig(c, testing.Attrs{
		"name": "testing",
		"uuid": uuid.String(),
	}), uuid.String()
}

func (s *EnvironSuite) TestEnvironmentConfigSameEnvAsState(c *gc.C) {
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := env.Config()
	c.Assert(err, jc.ErrorIsNil)
	uuid, exists := cfg.UUID()
	c.Assert(exists, jc.IsTrue)
	c.Assert(uuid, gc.Equals, s.State.EnvironUUID())
}

func (s *EnvironSuite) TestEnvironmentConfigDifferentEnvThanState(c *gc.C) {
	otherState := s.Factory.MakeEnvironment(c, nil)
	defer otherState.Close()
	env, err := otherState.Environment()
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := env.Config()
	c.Assert(err, jc.ErrorIsNil)
	uuid, exists := cfg.UUID()
	c.Assert(exists, jc.IsTrue)
	c.Assert(uuid, gc.Equals, env.UUID())
	c.Assert(uuid, gc.Not(gc.Equals), s.State.EnvironUUID())
}

func (s *EnvironSuite) TestDestroyStateServerEnvironment(c *gc.C) {
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EnvironSuite) TestDestroyOtherEnvironment(c *gc.C) {
	st2 := s.Factory.MakeEnvironment(c, nil)
	defer st2.Close()
	env, err := st2.Environment()
	c.Assert(err, jc.ErrorIsNil)
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EnvironSuite) TestDestroyStateServerEnvironmentFails(c *gc.C) {
	st2 := s.Factory.MakeEnvironment(c, nil)
	defer st2.Close()
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Destroy(), gc.ErrorMatches, "failed to destroy environment: hosting 1 other environments")
}

func (s *EnvironSuite) TestDestroyStateServerEnvironmentRace(c *gc.C) {
	// Simulate an environment being added just before the remove txn is
	// called.
	defer state.SetBeforeHooks(c, s.State, func() {
		blocker := s.Factory.MakeEnvironment(c, nil)
		err := blocker.Close()
		c.Check(err, jc.ErrorIsNil)
	}).Check()

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Destroy(), gc.ErrorMatches, "failed to destroy environment: hosting 1 other environments")
}

func (s *EnvironSuite) TestDestroyStateServerAlreadyDyingRaceNoOp(c *gc.C) {
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)

	// Simulate an environment being destroyed by another client just before
	// the remove txn is called.
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(env.Destroy(), jc.ErrorIsNil)
	}).Check()

	c.Assert(env.Destroy(), jc.ErrorIsNil)
}

func (s *EnvironSuite) TestDestroyStateServerAlreadyDyingNoOp(c *gc.C) {
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(env.Destroy(), jc.ErrorIsNil)
	c.Assert(env.Destroy(), jc.ErrorIsNil)
}

func (s *EnvironSuite) TestListEnvironmentUsers(c *gc.C) {
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)

	expected := addEnvUsers(c, s.State)
	obtained, err := env.Users()
	c.Assert(err, gc.IsNil)

	assertObtainedUsersMatchExpectedUsers(c, obtained, expected)
}

func (s *EnvironSuite) TestMisMatchedEnvs(c *gc.C) {
	// create another environment
	otherEnvState := s.Factory.MakeEnvironment(c, nil)
	defer otherEnvState.Close()
	otherEnv, err := otherEnvState.Environment()
	c.Assert(err, jc.ErrorIsNil)

	// get that environment from State
	env, err := s.State.GetEnvironment(otherEnv.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)

	// check that the Users method errors
	users, err := env.Users()
	c.Assert(users, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "cannot lookup environment users outside the current environment")
}

func (s *EnvironSuite) TestListUsersTwoEnvironments(c *gc.C) {
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)

	otherEnvState := s.Factory.MakeEnvironment(c, nil)
	defer otherEnvState.Close()
	otherEnv, err := otherEnvState.Environment()
	c.Assert(err, jc.ErrorIsNil)

	// Add users to both environments
	expectedUsers := addEnvUsers(c, s.State)
	expectedUsersOtherEnv := addEnvUsers(c, otherEnvState)

	// test that only the expected users are listed for each environment
	obtainedUsers, err := env.Users()
	c.Assert(err, jc.ErrorIsNil)
	assertObtainedUsersMatchExpectedUsers(c, obtainedUsers, expectedUsers)

	obtainedUsersOtherEnv, err := otherEnv.Users()
	c.Assert(err, jc.ErrorIsNil)
	assertObtainedUsersMatchExpectedUsers(c, obtainedUsersOtherEnv, expectedUsersOtherEnv)
}

func addEnvUsers(c *gc.C, st *state.State) (expected []*state.EnvironmentUser) {
	// get the environment owner
	testAdmin := names.NewUserTag("test-admin")
	owner, err := st.EnvironmentUser(testAdmin)
	c.Assert(err, jc.ErrorIsNil)

	f := factory.NewFactory(st)
	return []*state.EnvironmentUser{
		// we expect the owner to be an existing environment user
		owner,
		// add new users to the environment
		f.MakeEnvUser(c, nil),
		f.MakeEnvUser(c, nil),
		f.MakeEnvUser(c, nil),
	}
}

func assertObtainedUsersMatchExpectedUsers(c *gc.C, obtainedUsers, expectedUsers []*state.EnvironmentUser) {
	c.Assert(len(obtainedUsers), gc.Equals, len(expectedUsers))
	for i, obtained := range obtainedUsers {
		c.Assert(obtained.EnvironmentTag().Id(), gc.Equals, expectedUsers[i].EnvironmentTag().Id())
		c.Assert(obtained.UserName(), gc.Equals, expectedUsers[i].UserName())
		c.Assert(obtained.DisplayName(), gc.Equals, expectedUsers[i].DisplayName())
		c.Assert(obtained.CreatedBy(), gc.Equals, expectedUsers[i].CreatedBy())
	}
}

func (s *EnvironSuite) TestAllEnvironments(c *gc.C) {
	s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "test", Owner: names.NewUserTag("bob@remote")}).Close()
	s.Factory.MakeEnvironment(c, &factory.EnvParams{
		Name: "test", Owner: names.NewUserTag("mary@remote")}).Close()
	envs, err := s.State.AllEnvironments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envs, gc.HasLen, 3)
	var obtained []string
	for _, env := range envs {
		obtained = append(obtained, fmt.Sprintf("%s/%s", env.Owner().Username(), env.Name()))
	}
	expected := []string{
		"test-admin@local/testenv",
		"bob@remote/test",
		"mary@remote/test",
	}
	c.Assert(obtained, jc.SameContents, expected)
}

func (s *EnvironSuite) TestHostedEnvironCount(c *gc.C) {
	c.Assert(state.HostedEnvironCount(c, s.State), gc.Equals, 0)

	st1 := s.Factory.MakeEnvironment(c, nil)
	defer st1.Close()
	c.Assert(state.HostedEnvironCount(c, s.State), gc.Equals, 1)

	st2 := s.Factory.MakeEnvironment(c, nil)
	defer st2.Close()
	c.Assert(state.HostedEnvironCount(c, s.State), gc.Equals, 2)

	env1, err := st1.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env1.Destroy(), jc.ErrorIsNil)
	c.Assert(state.HostedEnvironCount(c, s.State), gc.Equals, 1)

	env2, err := st2.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env2.Destroy(), jc.ErrorIsNil)
	c.Assert(state.HostedEnvironCount(c, s.State), gc.Equals, 0)
}
