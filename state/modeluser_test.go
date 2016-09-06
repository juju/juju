// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/description"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type ModelUserSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ModelUserSuite{})

func (s *ModelUserSuite) TestAddModelUser(c *gc.C) {
	now := state.NowToTheSecond()
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:        "validusername",
			NoModelUser: true,
		})
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	modelUser, err := s.State.AddModelUser(
		s.State.ModelUUID(),
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    description.WriteAccess,
		})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(modelUser.UserID, gc.Equals, fmt.Sprintf("%s:validusername@local", s.modelTag.Id()))
	c.Assert(modelUser.Object, gc.Equals, s.modelTag)
	c.Assert(modelUser.UserName, gc.Equals, "validusername@local")
	c.Assert(modelUser.DisplayName, gc.Equals, user.DisplayName())
	c.Assert(modelUser.Access, gc.Equals, description.WriteAccess)
	c.Assert(modelUser.CreatedBy.Id(), gc.Equals, "createdby@local")
	c.Assert(modelUser.DateCreated.Equal(now) || modelUser.DateCreated.After(now), jc.IsTrue)
	when, err := s.State.LastModelConnection(modelUser.UserTag)
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	c.Assert(when.IsZero(), jc.IsTrue)

	modelUser, err = s.State.UserAccess(user.UserTag(), s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserID, gc.Equals, fmt.Sprintf("%s:validusername@local", s.modelTag.Id()))
	c.Assert(modelUser.Object, gc.Equals, s.modelTag)
	c.Assert(modelUser.UserName, gc.Equals, "validusername@local")
	c.Assert(modelUser.DisplayName, gc.Equals, user.DisplayName())
	c.Assert(modelUser.Access, gc.Equals, description.WriteAccess)
	c.Assert(modelUser.CreatedBy.Id(), gc.Equals, "createdby@local")
	c.Assert(modelUser.DateCreated.Equal(now) || modelUser.DateCreated.After(now), jc.IsTrue)
	when, err = s.State.LastModelConnection(modelUser.UserTag)
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	c.Assert(when.IsZero(), jc.IsTrue)
}

func (s *ModelUserSuite) TestAddReadOnlyModelUser(c *gc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:        "validusername",
			NoModelUser: true,
		})
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	modelUser, err := s.State.AddModelUser(
		s.State.ModelUUID(),
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    description.ReadAccess,
		})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(modelUser.UserName, gc.Equals, "validusername@local")
	c.Assert(modelUser.DisplayName, gc.Equals, user.DisplayName())
	c.Assert(modelUser.Access, gc.Equals, description.ReadAccess)

	// Make sure that it is set when we read the user out.
	modelUser, err = s.State.UserAccess(user.UserTag(), s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName, gc.Equals, "validusername@local")
	c.Assert(modelUser.Access, gc.Equals, description.ReadAccess)
}

func (s *ModelUserSuite) TestAddReadWriteModelUser(c *gc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:        "validusername",
			NoModelUser: true,
		})
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	modelUser, err := s.State.AddModelUser(
		s.State.ModelUUID(),
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    description.WriteAccess,
		})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(modelUser.UserName, gc.Equals, "validusername@local")
	c.Assert(modelUser.DisplayName, gc.Equals, user.DisplayName())
	c.Assert(modelUser.Access, gc.Equals, description.WriteAccess)

	// Make sure that it is set when we read the user out.
	modelUser, err = s.State.UserAccess(user.UserTag(), s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName, gc.Equals, "validusername@local")
	c.Assert(modelUser.Access, gc.Equals, description.WriteAccess)
}

func (s *ModelUserSuite) TestAddAdminModelUser(c *gc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:        "validusername",
			NoModelUser: true,
		})
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	modelUser, err := s.State.AddModelUser(
		s.State.ModelUUID(),
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    description.AdminAccess,
		})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(modelUser.UserName, gc.Equals, "validusername@local")
	c.Assert(modelUser.DisplayName, gc.Equals, user.DisplayName())
	c.Assert(modelUser.Access, gc.Equals, description.AdminAccess)

	// Make sure that it is set when we read the user out.
	modelUser, err = s.State.UserAccess(user.UserTag(), s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName, gc.Equals, "validusername@local")
	c.Assert(modelUser.Access, gc.Equals, description.AdminAccess)
}

func (s *ModelUserSuite) TestDefaultAccessModelUser(c *gc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:        "validusername",
			NoModelUser: true,
		})
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	modelUser, err := s.State.AddModelUser(
		s.State.ModelUUID(),
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    description.ReadAccess,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.Access, gc.Equals, description.ReadAccess)
}

func (s *ModelUserSuite) TestSetAccessModelUser(c *gc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:        "validusername",
			NoModelUser: true,
		})
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	modelUser, err := s.State.AddModelUser(
		s.State.ModelUUID(),
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    description.AdminAccess,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.Access, gc.Equals, description.AdminAccess)

	s.State.SetUserAccess(modelUser.UserTag, s.State.ModelTag(), description.ReadAccess)

	modelUser, err = s.State.UserAccess(user.UserTag(), s.State.ModelTag())
	c.Assert(modelUser.Access, gc.Equals, description.ReadAccess)
}

func (s *ModelUserSuite) TestCaseUserNameVsId(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	user, err := s.State.AddModelUser(
		s.State.ModelUUID(),
		state.UserAccessSpec{
			User:      names.NewUserTag("Bob@RandomProvider"),
			CreatedBy: model.Owner(),
			Access:    description.ReadAccess,
		})
	c.Assert(err, gc.IsNil)
	c.Assert(user.UserName, gc.Equals, "Bob@RandomProvider")
	c.Assert(user.UserID, gc.Equals, state.DocID(s.State, "bob@randomprovider"))
}

func (s *ModelUserSuite) TestCaseSensitiveModelUserErrors(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.Factory.MakeModelUser(c, &factory.ModelUserParams{User: "Bob@ubuntuone"})

	_, err = s.State.AddModelUser(
		s.State.ModelUUID(),
		state.UserAccessSpec{
			User:      names.NewUserTag("boB@ubuntuone"),
			CreatedBy: model.Owner(),
			Access:    description.ReadAccess,
		})
	c.Assert(err, gc.ErrorMatches, `user access "boB@ubuntuone" already exists`)
	c.Assert(errors.IsAlreadyExists(err), jc.IsTrue)
}

func (s *ModelUserSuite) TestCaseInsensitiveLookupInMultiEnvirons(c *gc.C) {
	assertIsolated := func(st1, st2 *state.State, usernames ...string) {
		f := factory.NewFactory(st1)
		expectedUser := f.MakeModelUser(c, &factory.ModelUserParams{User: usernames[0]})

		// assert case insensitive lookup for each username
		for _, username := range usernames {
			userTag := names.NewUserTag(username)
			obtainedUser, err := st1.UserAccess(userTag, st1.ModelTag())
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(obtainedUser, gc.DeepEquals, expectedUser)

			_, err = st2.UserAccess(userTag, st2.ModelTag())
			c.Assert(errors.IsNotFound(err), jc.IsTrue)
		}
	}

	otherSt := s.Factory.MakeModel(c, nil)
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

func (s *ModelUserSuite) TestAddModelDisplayName(c *gc.C) {
	modelUserDefault := s.Factory.MakeModelUser(c, nil)
	c.Assert(modelUserDefault.DisplayName, gc.Matches, "display name-[0-9]*")

	modelUser := s.Factory.MakeModelUser(c, &factory.ModelUserParams{DisplayName: "Override user display name"})
	c.Assert(modelUser.DisplayName, gc.Equals, "Override user display name")
}

func (s *ModelUserSuite) TestAddModelNoUserFails(c *gc.C) {
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	_, err := s.State.AddModelUser(
		s.State.ModelUUID(),
		state.UserAccessSpec{
			User:      names.NewLocalUserTag("validusername"),
			CreatedBy: createdBy.UserTag(),
			Access:    description.ReadAccess,
		})
	c.Assert(err, gc.ErrorMatches, `user "validusername" does not exist locally: user "validusername" not found`)
}

func (s *ModelUserSuite) TestAddModelNoCreatedByUserFails(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "validusername"})
	_, err := s.State.AddModelUser(
		s.State.ModelUUID(),
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: names.NewLocalUserTag("createdby"),
			Access:    description.ReadAccess,
		})
	c.Assert(err, gc.ErrorMatches, `createdBy user "createdby" does not exist locally: user "createdby" not found`)
}

func (s *ModelUserSuite) TestRemoveModelUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "validUsername"})
	_, err := s.State.UserAccess(user.UserTag(), s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.RemoveUserAccess(user.UserTag(), s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.UserAccess(user.UserTag(), s.State.ModelTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ModelUserSuite) TestRemoveModelUserFails(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	err := s.State.RemoveUserAccess(user.UserTag(), s.State.ModelTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ModelUserSuite) TestUpdateLastConnection(c *gc.C) {
	now := state.NowToTheSecond()
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "validusername", Creator: createdBy.Tag()})
	modelUser, err := s.State.UserAccess(user.UserTag(), s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.UpdateLastModelConnection(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	when, err := s.State.LastModelConnection(modelUser.UserTag)
	c.Assert(err, jc.ErrorIsNil)
	// It is possible that the update is done over a second boundary, so we need
	// to check for after now as well as equal.
	c.Assert(when.After(now) || when.Equal(now), jc.IsTrue)
}

func (s *ModelUserSuite) TestUpdateLastConnectionTwoModelUsers(c *gc.C) {
	now := state.NowToTheSecond()

	// Create a user and add them to the inital model.
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "validusername", Creator: createdBy.Tag()})
	modelUser, err := s.State.UserAccess(user.UserTag(), s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	// Create a second model and add the same user to this.
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	modelUser2, err := st2.AddModelUser(
		st2.ModelUUID(),
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    description.ReadAccess,
		})
	c.Assert(err, jc.ErrorIsNil)

	// Now we have two model users with the same username. Ensure we get
	// separate last connections.

	// Connect modelUser and get last connection.
	err = s.State.UpdateLastModelConnection(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	when, err := s.State.LastModelConnection(modelUser.UserTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(when.After(now) || when.Equal(now), jc.IsTrue)

	// Try to get last connection for modelUser2. As they have never connected,
	// we expect to get an error.
	_, err = st2.LastModelConnection(modelUser2.UserTag)
	c.Assert(err, gc.ErrorMatches, `never connected: "validusername@local"`)

	// Connect modelUser2 and get last connection.
	err = s.State.UpdateLastModelConnection(modelUser2.UserTag)
	c.Assert(err, jc.ErrorIsNil)
	when, err = s.State.LastModelConnection(modelUser2.UserTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(when.After(now) || when.Equal(now), jc.IsTrue)
}

func (s *ModelUserSuite) TestModelsForUserNone(c *gc.C) {
	tag := names.NewUserTag("non-existent@remote")
	models, err := s.State.ModelsForUser(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, gc.HasLen, 0)
}

func (s *ModelUserSuite) TestModelsForUserNewLocalUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	models, err := s.State.ModelsForUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, gc.HasLen, 0)
}

func (s *ModelUserSuite) TestModelsForUser(c *gc.C) {
	user := s.Factory.MakeUser(c, nil)
	models, err := s.State.ModelsForUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, gc.HasLen, 1)
	c.Assert(models[0].UUID(), gc.Equals, s.State.ModelUUID())
	st, err := s.State.ForModel(models[0].ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	modelUser, err := s.State.UserAccess(user.UserTag(), models[0].ModelTag())
	when, err := st.LastModelConnection(modelUser.UserTag)
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	c.Assert(when.IsZero(), jc.IsTrue)
	c.Assert(st.Close(), jc.ErrorIsNil)
}

func (s *ModelUserSuite) newEnvWithOwner(c *gc.C, name string, owner names.UserTag) *state.Model {
	// Don't use the factory to call MakeModel because it may at some
	// time in the future be modified to do additional things.  Instead call
	// the state method directly to create an model to make sure that
	// the owner is able to access the model.
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": name,
		"uuid": uuid.String(),
	})
	model, st, err := s.State.NewModel(state.ModelArgs{
		CloudName: "dummy", CloudRegion: "dummy-region", Config: cfg, Owner: owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	return model
}

func (s *ModelUserSuite) TestModelsForUserEnvOwner(c *gc.C) {
	owner := names.NewUserTag("external@remote")
	model := s.newEnvWithOwner(c, "test-model", owner)

	models, err := s.State.ModelsForUser(owner)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, gc.HasLen, 1)
	s.checkSameModel(c, models[0].Model, model)
}

func (s *ModelUserSuite) checkSameModel(c *gc.C, env1, env2 *state.Model) {
	c.Check(env1.Name(), gc.Equals, env2.Name())
	c.Check(env1.UUID(), gc.Equals, env2.UUID())
}

func (s *ModelUserSuite) newEnvWithUser(c *gc.C, name string, user names.UserTag) *state.Model {
	envState := s.Factory.MakeModel(c, &factory.ModelParams{Name: name})
	defer envState.Close()
	newEnv, err := envState.Model()
	c.Assert(err, jc.ErrorIsNil)

	_, err = envState.AddModelUser(
		envState.ModelUUID(),
		state.UserAccessSpec{
			User: user, CreatedBy: newEnv.Owner(),
			Access: description.ReadAccess,
		})
	c.Assert(err, jc.ErrorIsNil)
	return newEnv
}

func (s *ModelUserSuite) TestModelsForUserOfNewEnv(c *gc.C) {
	userTag := names.NewUserTag("external@remote")
	model := s.newEnvWithUser(c, "test-model", userTag)

	models, err := s.State.ModelsForUser(userTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, gc.HasLen, 1)
	s.checkSameModel(c, models[0].Model, model)
}

func (s *ModelUserSuite) TestModelsForUserMultiple(c *gc.C) {
	userTag := names.NewUserTag("external@remote")
	expected := []*state.Model{
		s.newEnvWithUser(c, "user1", userTag),
		s.newEnvWithUser(c, "user2", userTag),
		s.newEnvWithUser(c, "user3", userTag),
		s.newEnvWithOwner(c, "owner1", userTag),
		s.newEnvWithOwner(c, "owner2", userTag),
	}
	sort.Sort(UUIDOrder(expected))

	models, err := s.State.ModelsForUser(userTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, gc.HasLen, len(expected))
	sort.Sort(userUUIDOrder(models))
	for i := range expected {
		s.checkSameModel(c, models[i].Model, expected[i])
	}
}

func (s *ModelUserSuite) TestIsControllerAdmin(c *gc.C) {
	isAdmin, err := s.State.IsControllerAdmin(s.Owner)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isAdmin, jc.IsTrue)

	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	isAdmin, err = s.State.IsControllerAdmin(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isAdmin, jc.IsFalse)

	s.State.SetUserAccess(user.UserTag(), s.State.ControllerTag(), description.SuperuserAccess)
	isAdmin, err = s.State.IsControllerAdmin(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isAdmin, jc.IsTrue)

	readonly := s.Factory.MakeModelUser(c, &factory.ModelUserParams{Access: description.ReadAccess})
	isAdmin, err = s.State.IsControllerAdmin(readonly.UserTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isAdmin, jc.IsFalse)
}

func (s *ModelUserSuite) TestIsControllerAdminFromOtherState(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})

	otherState := s.Factory.MakeModel(c, &factory.ModelParams{Owner: user.UserTag()})
	defer otherState.Close()

	isAdmin, err := otherState.IsControllerAdmin(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isAdmin, jc.IsFalse)

	isAdmin, err = otherState.IsControllerAdmin(s.Owner)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isAdmin, jc.IsTrue)
}

// UUIDOrder is used to sort the models into a stable order
type UUIDOrder []*state.Model

func (a UUIDOrder) Len() int           { return len(a) }
func (a UUIDOrder) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a UUIDOrder) Less(i, j int) bool { return a[i].UUID() < a[j].UUID() }

// userUUIDOrder is used to sort the UserModels into a stable order
type userUUIDOrder []*state.UserModel

func (a userUUIDOrder) Len() int           { return len(a) }
func (a userUUIDOrder) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a userUUIDOrder) Less(i, j int) bool { return a[i].UUID() < a[j].UUID() }
