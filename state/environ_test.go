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
	"github.com/juju/juju/provider/ec2"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider/registry"
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
	owner := s.factory.MakeUser(c, nil).UserTag()

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
	otherState := s.factory.MakeEnvironment(c, nil)
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
	st2 := s.factory.MakeEnvironment(c, nil)
	defer st2.Close()
	env, err := st2.Environment()
	c.Assert(err, jc.ErrorIsNil)
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EnvironSuite) TestDestroyStateServerEnvironmentFails(c *gc.C) {
	st2 := s.factory.MakeEnvironment(c, nil)
	defer st2.Close()
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Destroy(), gc.ErrorMatches, "failed to destroy environment: state server environment cannot be destroyed before all other environments are destroyed")
}

func (s *EnvironSuite) TestDestroyStateServerEnvironmentRace(c *gc.C) {
	// Simulate an environment being added just before the remove txn is
	// called.
	defer state.SetBeforeHooks(c, s.State, func() {
		st := s.State
		c.Assert(state.HostedEnvironCount(c, st), gc.Equals, 0)
		state.IncHostedEnvironCount(c, st)
		c.Assert(state.HostedEnvironCount(c, st), gc.Equals, 1)
	}).Check()

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Destroy(), gc.ErrorMatches, "failed to destroy environment: transaction aborted")
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

func (s *EnvironSuite) TestDestroyEnvironmentWithPersistentVolumesFails(c *gc.C) {
	// Create a persistent volume.
	// TODO(wallyworld) - consider moving this to factory
	registry.RegisterEnvironStorageProviders("someprovider", ec2.EBS_ProviderType)
	pm := poolmanager.New(state.NewStateSettings(s.State))
	_, err := pm.Create("persistent-block", ec2.EBS_ProviderType, map[string]interface{}{"persistent": "true"})
	c.Assert(err, jc.ErrorIsNil)

	ch := s.AddTestingCharm(c, "storage-block2")
	storage := map[string]state.StorageConstraints{
		"multi1to10": makeStorageCons("persistent-block", 1024, 1),
	}
	service := s.AddTestingServiceWithStorage(c, "storage-block2", ch, storage)
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	volume1, err := s.State.StorageInstanceVolume(names.NewStorageTag("multi1to10/0"))
	c.Assert(err, jc.ErrorIsNil)
	volumeInfoSet := state.VolumeInfo{Size: 123, Persistent: true, VolumeId: "vol-ume"}
	err = s.State.SetVolumeInfo(volume1.VolumeTag(), volumeInfoSet)
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	// TODO(wallyworld) when we can destroy/remove volume, ensure env can then be destroyed
	c.Assert(errors.Cause(env.Destroy()), gc.Equals, state.ErrPersistentVolumesExist)
}

func (s *EnvironSuite) TestHostedEnvironCount(c *gc.C) {
	c.Assert(state.HostedEnvironCount(c, s.State), gc.Equals, 0)

	st1 := s.factory.MakeEnvironment(c, nil)
	defer st1.Close()
	c.Assert(state.HostedEnvironCount(c, s.State), gc.Equals, 1)

	st2 := s.factory.MakeEnvironment(c, nil)
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
