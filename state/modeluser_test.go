// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/permission"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
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
	c.Assert(modelUser.UserName, gc.Equals, usertesting.GenNewName(c, "validusername"))
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
	c.Assert(modelUser.UserName, gc.Equals, usertesting.GenNewName(c, "validusername"))
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

	c.Assert(modelUser.UserName, gc.Equals, usertesting.GenNewName(c, "validusername"))
	c.Assert(modelUser.DisplayName, gc.Equals, user.DisplayName())
	c.Assert(modelUser.Access, gc.Equals, permission.ReadAccess)

	// Make sure that it is set when we read the user out.
	modelUser, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName, gc.Equals, usertesting.GenNewName(c, "validusername"))
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

	c.Assert(modelUser.UserName, gc.Equals, usertesting.GenNewName(c, "validusername"))
	c.Assert(modelUser.DisplayName, gc.Equals, user.DisplayName())
	c.Assert(modelUser.Access, gc.Equals, permission.WriteAccess)

	// Make sure that it is set when we read the user out.
	modelUser, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName, gc.Equals, usertesting.GenNewName(c, "validusername"))
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

	c.Assert(modelUser.UserName, gc.Equals, usertesting.GenNewName(c, "validusername"))
	c.Assert(modelUser.DisplayName, gc.Equals, user.DisplayName())
	c.Assert(modelUser.Access, gc.Equals, permission.AdminAccess)

	// Make sure that it is set when we read the user out.
	modelUser, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName, gc.Equals, usertesting.GenNewName(c, "validusername"))
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
	c.Assert(user.UserName, gc.Equals, usertesting.GenNewName(c, "Bob@RandomProvider"))
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
	c.Assert(err, jc.ErrorIs, errors.AlreadyExists)
}

func (s *ModelUserSuite) TestCaseInsensitiveLookupInMultiEnvirons(c *gc.C) {
	assertIsolated := func(st1, st2 *state.State, usernames ...string) {
		f := factory.NewFactory(st1, s.StatePool, testing.FakeControllerConfig())
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
			c.Assert(err, jc.ErrorIs, errors.NotFound)
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
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *ModelUserSuite) TestRemoveModelUserFails(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	err := s.State.RemoveUserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
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
