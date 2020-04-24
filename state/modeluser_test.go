// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type ModelUserSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ModelUserSuite{})

func (s *ModelUserSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
}

func (s *ModelUserSuite) TestAddModelUser(c *gc.C) {
	now := state.NowToTheSecond(s.State)
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:        "validusername",
			NoModelUser: true,
		})
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	modelUser, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    permission.WriteAccess,
		})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(modelUser.UserID, gc.Equals, fmt.Sprintf("%s:validusername", s.modelTag.Id()))
	c.Assert(modelUser.Object, gc.Equals, s.modelTag)
	c.Assert(modelUser.UserName, gc.Equals, "validusername")
	c.Assert(modelUser.DisplayName, gc.Equals, user.DisplayName())
	c.Assert(modelUser.Access, gc.Equals, permission.WriteAccess)
	c.Assert(modelUser.CreatedBy.Id(), gc.Equals, "createdby")
	c.Assert(modelUser.DateCreated.Equal(now) || modelUser.DateCreated.After(now), jc.IsTrue)
	when, err := s.Model.LastModelConnection(modelUser.UserTag)
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	c.Assert(when.IsZero(), jc.IsTrue)

	modelUser, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserID, gc.Equals, fmt.Sprintf("%s:validusername", s.modelTag.Id()))
	c.Assert(modelUser.Object, gc.Equals, s.modelTag)
	c.Assert(modelUser.UserName, gc.Equals, "validusername")
	c.Assert(modelUser.DisplayName, gc.Equals, user.DisplayName())
	c.Assert(modelUser.Access, gc.Equals, permission.WriteAccess)
	c.Assert(modelUser.CreatedBy.Id(), gc.Equals, "createdby")
	c.Assert(modelUser.DateCreated.Equal(now) || modelUser.DateCreated.After(now), jc.IsTrue)
	when, err = s.Model.LastModelConnection(modelUser.UserTag)
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
	modelUser, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    permission.ReadAccess,
		})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(modelUser.UserName, gc.Equals, "validusername")
	c.Assert(modelUser.DisplayName, gc.Equals, user.DisplayName())
	c.Assert(modelUser.Access, gc.Equals, permission.ReadAccess)

	// Make sure that it is set when we read the user out.
	modelUser, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName, gc.Equals, "validusername")
	c.Assert(modelUser.Access, gc.Equals, permission.ReadAccess)
}

func (s *ModelUserSuite) TestAddReadWriteModelUser(c *gc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:        "validusername",
			NoModelUser: true,
		})
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	modelUser, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    permission.WriteAccess,
		})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(modelUser.UserName, gc.Equals, "validusername")
	c.Assert(modelUser.DisplayName, gc.Equals, user.DisplayName())
	c.Assert(modelUser.Access, gc.Equals, permission.WriteAccess)

	// Make sure that it is set when we read the user out.
	modelUser, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName, gc.Equals, "validusername")
	c.Assert(modelUser.Access, gc.Equals, permission.WriteAccess)
}

func (s *ModelUserSuite) TestAddAdminModelUser(c *gc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:        "validusername",
			NoModelUser: true,
		})
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	modelUser, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    permission.AdminAccess,
		})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(modelUser.UserName, gc.Equals, "validusername")
	c.Assert(modelUser.DisplayName, gc.Equals, user.DisplayName())
	c.Assert(modelUser.Access, gc.Equals, permission.AdminAccess)

	// Make sure that it is set when we read the user out.
	modelUser, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName, gc.Equals, "validusername")
	c.Assert(modelUser.Access, gc.Equals, permission.AdminAccess)
}

func (s *ModelUserSuite) TestDefaultAccessModelUser(c *gc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:        "validusername",
			NoModelUser: true,
		})
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	modelUser, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    permission.ReadAccess,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.Access, gc.Equals, permission.ReadAccess)
}

func (s *ModelUserSuite) TestSetAccessModelUser(c *gc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name:        "validusername",
			NoModelUser: true,
		})
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	modelUser, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    permission.AdminAccess,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.Access, gc.Equals, permission.AdminAccess)

	s.State.SetUserAccess(modelUser.UserTag, s.Model.ModelTag(), permission.ReadAccess)

	modelUser, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.Access, gc.Equals, permission.ReadAccess)
}

func (s *ModelUserSuite) TestCaseUserNameVsId(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	user, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      names.NewUserTag("Bob@RandomProvider"),
			CreatedBy: model.Owner(),
			Access:    permission.ReadAccess,
		})
	c.Assert(err, gc.IsNil)
	c.Assert(user.UserName, gc.Equals, "Bob@RandomProvider")
	c.Assert(user.UserID, gc.Equals, state.DocID(s.State, "bob@randomprovider"))
}

func (s *ModelUserSuite) TestCaseSensitiveModelUserErrors(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.Factory.MakeModelUser(c, &factory.ModelUserParams{User: "Bob@ubuntuone"})

	_, err = s.Model.AddUser(
		state.UserAccessSpec{
			User:      names.NewUserTag("boB@ubuntuone"),
			CreatedBy: model.Owner(),
			Access:    permission.ReadAccess,
		})
	c.Assert(err, gc.ErrorMatches, `user access "boB@ubuntuone" already exists`)
	c.Assert(errors.IsAlreadyExists(err), jc.IsTrue)
}

func (s *ModelUserSuite) TestCaseInsensitiveLookupInMultiEnvirons(c *gc.C) {
	assertIsolated := func(st1, st2 *state.State, usernames ...string) {
		f := factory.NewFactory(st1, s.StatePool)
		expectedUser := f.MakeModelUser(c, &factory.ModelUserParams{User: usernames[0]})

		m1, err := st1.Model()
		c.Assert(err, jc.ErrorIsNil)

		m2, err := st2.Model()
		c.Assert(err, jc.ErrorIsNil)

		// assert case insensitive lookup for each username
		for _, username := range usernames {
			userTag := names.NewUserTag(username)
			obtainedUser, err := st1.UserAccess(userTag, m1.ModelTag())
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(obtainedUser, gc.DeepEquals, expectedUser)

			_, err = st2.UserAccess(userTag, m2.ModelTag())
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
	_, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      names.NewLocalUserTag("validusername"),
			CreatedBy: createdBy.UserTag(),
			Access:    permission.ReadAccess,
		})
	c.Assert(err, gc.ErrorMatches, `user "validusername" does not exist locally: user "validusername" not found`)
}

func (s *ModelUserSuite) TestAddModelNoCreatedByUserFails(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "validusername"})
	_, err := s.Model.AddUser(
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: names.NewLocalUserTag("createdby"),
			Access:    permission.ReadAccess,
		})
	c.Assert(err, gc.ErrorMatches, `createdBy user "createdby" does not exist locally: user "createdby" not found`)
}

func (s *ModelUserSuite) TestRemoveModelUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "validUsername"})
	_, err := s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.RemoveUserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ModelUserSuite) TestRemoveModelUserFails(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	err := s.State.RemoveUserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ModelUserSuite) TestUpdateLastConnection(c *gc.C) {
	now := state.NowToTheSecond(s.State)
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "validusername", Creator: createdBy.Tag()})
	modelUser, err := s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.Model.UpdateLastModelConnection(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	when, err := s.Model.LastModelConnection(modelUser.UserTag)
	c.Assert(err, jc.ErrorIsNil)
	// It is possible that the update is done over a second boundary, so we need
	// to check for after now as well as equal.
	c.Assert(when.After(now) || when.Equal(now), jc.IsTrue)
}

func (s *ModelUserSuite) TestUpdateLastConnectionTwoModelUsers(c *gc.C) {
	now := state.NowToTheSecond(s.State)

	// Create a user and add them to the initial model.
	createdBy := s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "validusername", Creator: createdBy.Tag()})
	modelUser, err := s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	// Create a second model and add the same user to this.
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	model2, err := st2.Model()
	c.Assert(err, jc.ErrorIsNil)
	modelUser2, err := model2.AddUser(
		state.UserAccessSpec{
			User:      user.UserTag(),
			CreatedBy: createdBy.UserTag(),
			Access:    permission.ReadAccess,
		})
	c.Assert(err, jc.ErrorIsNil)

	// Now we have two model users with the same username. Ensure we get
	// separate last connections.

	// Connect modelUser and get last connection.
	err = s.Model.UpdateLastModelConnection(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	when, err := s.Model.LastModelConnection(modelUser.UserTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(when.After(now) || when.Equal(now), jc.IsTrue)

	// Try to get last connection for modelUser2. As they have never connected,
	// we expect to get an error.
	_, err = model2.LastModelConnection(modelUser2.UserTag)
	c.Assert(err, gc.ErrorMatches, `never connected: "validusername"`)

	// Connect modelUser2 and get last connection.
	err = s.Model.UpdateLastModelConnection(modelUser2.UserTag)
	c.Assert(err, jc.ErrorIsNil)
	when, err = s.Model.LastModelConnection(modelUser2.UserTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(when.After(now) || when.Equal(now), jc.IsTrue)
}

func (s *ModelUserSuite) TestModelUUIDsForUserNone(c *gc.C) {
	tag := names.NewUserTag("non-existent@remote")
	models, err := s.State.ModelUUIDsForUser(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, gc.HasLen, 0)
}

func (s *ModelUserSuite) TestModelUUIDsForUserNewLocalUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	models, err := s.State.ModelUUIDsForUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, gc.HasLen, 0)
}

func (s *ModelUserSuite) TestModelUUIDsForUser(c *gc.C) {
	user := s.Factory.MakeUser(c, nil)
	models, err := s.State.ModelUUIDsForUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.DeepEquals, []string{s.State.ModelUUID()})

	modelTag := names.NewModelTag(models[0])
	access, err := s.State.UserAccess(user.UserTag(), modelTag)
	c.Assert(err, jc.ErrorIsNil)
	when, err := s.Model.LastModelConnection(access.UserTag)
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	c.Assert(when.IsZero(), jc.IsTrue)
}

func (s *ModelUserSuite) TestImportingModelUUIDsForUser(c *gc.C) {
	user := s.Factory.MakeUser(c, nil)
	models, err := s.State.ModelUUIDsForUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.DeepEquals, []string{s.State.ModelUUID()})

	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, jc.ErrorIsNil)

	models, err = s.State.ModelUUIDsForUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, gc.HasLen, 0)
}

func (s *ModelUserSuite) TestModelUUIDsForUserModelOwner(c *gc.C) {
	owner := names.NewUserTag("external@remote")
	model := s.newModelWithOwner(c, owner)

	models, err := s.State.ModelUUIDsForUser(owner)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.DeepEquals, []string{model.UUID()})
}

func (s *ModelUserSuite) TestModelUUIDsForUserOfNewModel(c *gc.C) {
	userTag := names.NewUserTag("external@remote")
	model := s.newModelWithUser(c, userTag, state.ModelTypeIAAS)

	models, err := s.State.ModelUUIDsForUser(userTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.DeepEquals, []string{model.UUID()})
}

func (s *ModelUserSuite) TestModelUUIDsForUserMultiple(c *gc.C) {
	userTag := names.NewUserTag("external@remote")
	expected := []string{
		s.newModelWithUser(c, userTag, state.ModelTypeIAAS).UUID(),
		s.newModelWithUser(c, userTag, state.ModelTypeIAAS).UUID(),
		s.newModelWithUser(c, userTag, state.ModelTypeIAAS).UUID(),
		s.newModelWithOwner(c, userTag).UUID(),
		s.newModelWithOwner(c, userTag).UUID(),
	}

	models, err := s.State.ModelUUIDsForUser(userTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.SameContents, expected)
}

func (s *ModelUserSuite) TestModelBasicInfoForUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	model := s.newModelWithUser(c, user.UserTag(), state.ModelTypeIAAS)
	model2 := s.newModelWithUser(c, user.UserTag(), state.ModelTypeCAAS)

	models, err := s.State.ModelBasicInfoForUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.SameContents, []state.ModelAccessInfo{
		{
			Name:  model.Name(),
			Type:  model.Type(),
			UUID:  model.UUID(),
			Owner: "test-admin",
		}, {
			Name:  model2.Name(),
			Type:  model2.Type(),
			UUID:  model2.UUID(),
			Owner: "test-admin",
		},
	})
}

func (s *ModelUserSuite) TestIsControllerAdmin(c *gc.C) {
	isAdmin, err := s.State.IsControllerAdmin(s.Owner)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isAdmin, jc.IsTrue)

	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	isAdmin, err = s.State.IsControllerAdmin(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isAdmin, jc.IsFalse)

	s.State.SetUserAccess(user.UserTag(), s.State.ControllerTag(), permission.SuperuserAccess)
	isAdmin, err = s.State.IsControllerAdmin(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isAdmin, jc.IsTrue)

	readonly := s.Factory.MakeModelUser(c, &factory.ModelUserParams{Access: permission.ReadAccess})
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

func (s *ModelUserSuite) newModelWithOwner(c *gc.C, owner names.UserTag) *state.Model {
	// Don't use the factory to call MakeModel because it may at some
	// time in the future be modified to do additional things.  Instead call
	// the state method directly to create an model to make sure that
	// the owner is able to access the model.
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	uuidStr := uuid.String()

	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": uuidStr[:8],
		"uuid": uuidStr,
	})
	model, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	return model
}

func (s *ModelUserSuite) newModelWithUser(c *gc.C, user names.UserTag, modelType state.ModelType) *state.Model {
	params := &factory.ModelParams{Type: modelType}
	st := s.Factory.MakeModel(c, params)
	defer st.Close()
	newModel, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	_, err = newModel.AddUser(
		state.UserAccessSpec{
			User: user, CreatedBy: newModel.Owner(),
			Access: permission.ReadAccess,
		})
	c.Assert(err, jc.ErrorIsNil)
	return newModel
}
