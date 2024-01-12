// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/user"
	usererrors "github.com/juju/juju/domain/user/errors"
	"github.com/juju/juju/internal/auth"
)

type serviceSuite struct {
	state *MockState
}

type stateUser struct {
	activationKey []byte
	creatorUUID   user.UUID
	createdAt     time.Time
	name          string
	displayName   string
	passwordHash  string
	passwordSalt  []byte
	removed       bool
	lastLogin     time.Time
	disabled      bool
}

var _ = gc.Suite(&serviceSuite{})

var (
	invalidUsernames = []string{
		"😱",  // We don't support emoji's
		"+蓮", // Cannot start with a +
		"-蓮", // Cannot start with a -
		".蓮", // Cannot start with a .
		"蓮+", // Cannot end with a +
		"蓮-", // Cannot end with a -
		"蓮.", // Cannot end with a .

		// long username that is valid for the regex but too long.
		"A1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890.+-1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890.+-1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890.+-1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRa",
	}

	validUsernames = []string{
		"蓮", // Ren in Japanese
		"wallyworld",
		"r", // username for Rob Pike, fixes lp1620444
		"Jürgen.test",
		"Günter+++test",
		"王",      // Wang in Chinese
		"杨-test", // Yang in Chinese
		"اقتدار",
		"f00-Bar.ram77",
		// long username that is pushing the boundaries of 255 chars.
		"1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890.+-1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890.+-1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890.+-1234567890",

		// Some Romanian usernames. Thanks Dora!!!
		"Alinuța",
		"Bulișor",
		"Gheorghiță",
		"Mărioara",
		"Vasilică",

		// Some Turkish usernames, Thanks Caner!!!
		"rüştü",
		"özlem",
		"yağız",
	}
)

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *serviceSuite) service() *Service {
	return NewService(s.state)
}

// IsUserNameAlreadyExists is a helper function that checks if a username is
// already in use.
func IsUserNameAlreadyExists(name string, m map[user.UUID]stateUser) bool {
	for _, v := range m {
		if v.name == name && !v.removed {
			return true
		}
	}
	return false
}

func (s *serviceSuite) setMockState(c *gc.C) map[user.UUID]stateUser {
	mockState := map[user.UUID]stateUser{}

	s.state.EXPECT().GetUser(
		gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context,
		uuid user.UUID) (user.User, error) {
		stUser, exists := mockState[uuid]
		if !exists {
			return user.User{}, usererrors.NotFound
		}
		return user.User{
			CreatorUUID: stUser.creatorUUID,
			CreatedAt:   stUser.createdAt,
			DisplayName: stUser.displayName,
			Name:        stUser.name,
		}, nil
	}).AnyTimes()

	s.state.EXPECT().GetUserByName(
		gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context,
		name string) (user.User, error) {
		for _, usr := range mockState {
			if usr.name == name && !usr.removed {
				return user.User{
					CreatorUUID: usr.creatorUUID,
					CreatedAt:   usr.createdAt,
					DisplayName: usr.displayName,
					Name:        usr.name,
				}, nil
			}
		}
		return user.User{}, usererrors.NotFound
	}).AnyTimes()

	s.state.EXPECT().GetAllUsers(
		gomock.Any(),
	).DoAndReturn(func(
		_ context.Context) ([]user.User, error) {
		var users []user.User
		for _, usr := range mockState {
			if !usr.removed {
				users = append(users, user.User{
					CreatorUUID: usr.creatorUUID,
					CreatedAt:   usr.createdAt,
					DisplayName: usr.displayName,
					Name:        usr.name,
				})
			}
		}
		return users, nil
	}).AnyTimes()

	s.state.EXPECT().GetUserWithAuthInfo(
		gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context,
		uuid user.UUID) (user.UserWithAuthInfo, error) {
		stUser, exists := mockState[uuid]
		if !exists {
			return user.UserWithAuthInfo{}, usererrors.NotFound
		}
		return user.UserWithAuthInfo{
			User: user.User{
				CreatorUUID: stUser.creatorUUID,
				CreatedAt:   stUser.createdAt,
				DisplayName: stUser.displayName,
				Name:        stUser.name,
			},
			LastLogin: stUser.lastLogin,
			Disabled:  stUser.disabled,
		}, nil
	}).AnyTimes()

	s.state.EXPECT().AddUser(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context,
		uuid user.UUID,
		user user.User,
		creatorUUID user.UUID) error {
		if IsUserNameAlreadyExists(user.Name, mockState) {
			return usererrors.AlreadyExists
		}
		cusr, exists := mockState[creatorUUID]
		if !exists || cusr.removed {
			return usererrors.UserCreatorUUIDNotFound
		}
		mockState[uuid] = stateUser{
			creatorUUID: creatorUUID,
			createdAt:   time.Now(),
			displayName: user.DisplayName,
			name:        user.Name,
		}
		return nil
	}).AnyTimes()

	s.state.EXPECT().AddUserWithPasswordHash(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context,
		uuid user.UUID,
		user user.User,
		creatorUUID user.UUID,
		hash string,
		salt []byte) error {
		if IsUserNameAlreadyExists(user.Name, mockState) {
			return usererrors.AlreadyExists
		}
		cusr, exists := mockState[creatorUUID]
		if !exists || cusr.removed {
			return usererrors.UserCreatorUUIDNotFound
		}
		mockState[uuid] = stateUser{
			name:         user.Name,
			creatorUUID:  creatorUUID,
			createdAt:    time.Now(),
			displayName:  user.DisplayName,
			passwordHash: hash,
			passwordSalt: salt,
		}
		return nil
	}).AnyTimes()

	s.state.EXPECT().AddUserWithActivationKey(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context,
		uuid user.UUID,
		user user.User,
		creatorUUID user.UUID,
		key []byte) error {
		if IsUserNameAlreadyExists(user.Name, mockState) {
			return usererrors.AlreadyExists
		}
		cusr, exists := mockState[creatorUUID]
		if !exists || cusr.removed {
			return usererrors.UserCreatorUUIDNotFound
		}
		mockState[uuid] = stateUser{
			name:          user.Name,
			creatorUUID:   creatorUUID,
			createdAt:     time.Now(),
			displayName:   user.DisplayName,
			activationKey: key,
		}
		return nil
	}).AnyTimes()

	s.state.EXPECT().RemoveUser(
		gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context,
		uuid user.UUID) error {
		user, exists := mockState[uuid]
		if !exists {
			return usererrors.NotFound
		}
		user.removed = true
		user.activationKey = nil
		user.passwordHash = ""
		user.passwordSalt = nil
		mockState[uuid] = user
		return nil
	}).AnyTimes()

	s.state.EXPECT().SetActivationKey(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context,
		uuid user.UUID,
		key []byte) error {
		user, exists := mockState[uuid]
		if !exists || user.removed {
			return usererrors.NotFound
		}
		user.passwordHash = ""
		user.passwordSalt = nil
		user.activationKey = key
		mockState[uuid] = user
		return nil
	}).AnyTimes()

	// Implement the contract defined by SetPasswordHash
	s.state.EXPECT().SetPasswordHash(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context,
		uuid user.UUID,
		hash string,
		salt []byte) error {
		usr, exists := mockState[uuid]
		if !exists || usr.removed {
			return usererrors.NotFound
		}
		usr.passwordHash = hash
		usr.passwordSalt = salt
		usr.activationKey = nil
		mockState[uuid] = usr
		return nil
	}).AnyTimes()

	// Implement the contract defined by EnableUserAuthentication
	s.state.EXPECT().EnableUserAuthentication(
		gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context,
		uuid user.UUID) error {
		usr, exists := mockState[uuid]
		if !exists || usr.removed {
			return usererrors.NotFound
		}
		usr.disabled = false
		mockState[uuid] = usr
		return nil
	}).AnyTimes()

	// Implement the contract defined by DisableUserAuthentication
	s.state.EXPECT().DisableUserAuthentication(
		gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context,
		uuid user.UUID) error {
		usr, exists := mockState[uuid]
		if !exists || usr.removed {
			return usererrors.NotFound
		}
		usr.disabled = true
		mockState[uuid] = usr
		return nil
	}).AnyTimes()

	return mockState
}

// TestAddUser is testing the happy path for adding a user.
func (s *serviceSuite) TestAddUser(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[adminUUID] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}

	usr := user.User{
		Name:        "f00-Bar.ram77",
		DisplayName: "Display",
	}

	usrUUID, err := s.service().AddUser(context.Background(), usr, adminUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mockState[usrUUID].displayName, gc.Equals, "Display")

	// We want to check now that we can add a user with the same name as one
	// that has already been removed.
	graceUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[graceUUID] = stateUser{
		name:        "grace",
		createdAt:   time.Now(),
		displayName: "Grace",
		removed:     true,
	}

	usr = user.User{
		Name:        "grace",
		DisplayName: "test",
	}

	_, err = s.service().AddUser(context.Background(), usr, adminUUID)
	c.Assert(err, jc.ErrorIsNil)

	resultUser, err := s.service().GetUserByName(context.Background(), usr.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(resultUser.DisplayName, gc.Equals, "test")
	c.Assert(resultUser.CreatorUUID, gc.Equals, adminUUID)
}

// TestAddUserCreatorUUIDNotFound is testing that if we try and add a user with the
// creator UUID field set and the creator UUID does not exist we get an error back that
// satisfies usererrors.UserCreatorUUIDNotFound.
func (s *serviceSuite) TestAddUserCreatorUUIDNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)

	usr := user.User{
		Name:        "f00-Bar.ram77",
		DisplayName: "Display",
	}

	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.service().AddUser(context.Background(), usr, adminUUID)
	c.Assert(err, jc.ErrorIs, usererrors.UserCreatorUUIDNotFound)

	// We need to check that there were no side effects to state.
	c.Assert(len(mockState), gc.Equals, 0)
}

// TestAddUserNameNotValid is testing that if we try and add a user with a
// username that is not valid we get an error that satisfies
// usererrors.UsernameNotValid back.
func (s *serviceSuite) TestAddUserNameNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[adminUUID] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}

	usr := user.User{
		Name:        invalidUsernames[0],
		DisplayName: "Display",
	}

	_, err = s.service().AddUser(context.Background(), usr, adminUUID)
	c.Assert(err, jc.ErrorIs, usererrors.UsernameNotValid)

	// We need to check that there were no side effects to state.
	c.Assert(len(mockState), gc.Equals, 1)
}

// TestAddUserAlreadyExists is testing that we cannot add a user with a username
// that already exists and is active. We expect that in this case we should
// receive an error back that satisfies usererrors.AlreadyExists.
func (s *serviceSuite) TestAddUserAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[adminUUID] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}

	fredUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	createdAt := time.Now()
	mockState[fredUUID] = stateUser{
		createdAt:   createdAt,
		name:        "fred",
		displayName: "Freddie",
	}

	usr := user.User{
		Name:        "fred",
		DisplayName: "Display",
	}

	_, err = s.service().AddUser(context.Background(), usr, adminUUID)
	c.Assert(err, jc.ErrorIs, usererrors.AlreadyExists)

	// Test no state changes occurred
	fredUser := mockState[fredUUID]
	c.Assert(fredUser, jc.DeepEquals, stateUser{
		createdAt:   createdAt,
		name:        "fred",
		displayName: "Freddie",
	})
}

// TestAddUserAlreadyExistsRemoved is testing that add a user with
// a creator uuid that has been removed in the system.
func (s *serviceSuite) TestAddUserWithRemovedCreator(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)

	// Add admin user to state.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[adminUUID] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}

	// Add a user that has been removed.
	fredUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	createdAt := time.Now()
	mockState[fredUUID] = stateUser{
		createdAt:   createdAt,
		name:        "fred",
		displayName: "Freddie",
		removed:     true,
	}

	// Add a user with the removed user as the creator.
	usr := user.User{
		Name:        "f00-Bar.ram77",
		DisplayName: "Display",
	}

	_, err = s.service().AddUser(context.Background(), usr, fredUUID)
	c.Assert(err, jc.ErrorIs, usererrors.UserCreatorUUIDNotFound)
}

// TestAddUserWithPassword is testing the happy path of AddUserWithPassword.
func (s *serviceSuite) TestAddUserWithPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[adminUUID] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}

	usr := user.User{
		Name:        "f00-Bar.ram77",
		DisplayName: "Display",
	}
	password := auth.NewPassword("password")

	usrUUID, err := s.service().AddUserWithPassword(context.Background(), usr, adminUUID, password)
	c.Assert(err, jc.ErrorIsNil)

	userState := mockState[usrUUID]
	c.Assert(password.IsDestroyed(), jc.IsTrue)
	c.Assert(userState.passwordHash == "", jc.IsFalse)
	c.Assert(len(userState.passwordSalt) == 0, jc.IsFalse)
	c.Assert(userState.activationKey, gc.IsNil)

	fionaUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[fionaUUID] = stateUser{
		name:        "fiona",
		displayName: "Fee",
		createdAt:   time.Now(),
		removed:     true,
	}

	usr = user.User{
		Name:        "fiona",
		DisplayName: "Fiona",
	}
	password = auth.NewPassword("password")

	usrUUID, err = s.service().AddUserWithPassword(context.Background(), usr, adminUUID, password)
	c.Assert(err, jc.ErrorIsNil)

	userState = mockState[usrUUID]
	c.Assert(password.IsDestroyed(), jc.IsTrue)
	c.Assert(userState.passwordHash == "", jc.IsFalse)
	c.Assert(userState.displayName, gc.Equals, "Fiona")
	c.Assert(len(userState.passwordSalt) == 0, jc.IsFalse)
	c.Assert(userState.activationKey, gc.IsNil)
}

// TestAddUserWithPasswordCreatorUUIDNotFound is testing that is we add a user with
// the creatorUUID field set and a user does not exist for the creatorUUID a error that
// satisfies usererrors.UserCreatorUUIDNotFound is returned.
func (s *serviceSuite) TestAddUserWithPasswordCreatorUUIDNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	usr := user.User{
		Name:        "f00-Bar.ram77",
		DisplayName: "Display",
	}

	password := auth.NewPassword("07fd670820925bad78a214c249379b")

	_, err = s.service().AddUserWithPassword(context.Background(), usr, adminUUID, password)
	c.Assert(err, jc.ErrorIs, usererrors.UserCreatorUUIDNotFound)

	// We want to assert no state changes occurred.
	c.Assert(len(mockState), gc.Equals, 0)
}

// TestAddUserWithPasswordInvalidUser is testing that if we call
// AddUserWithPassword and the username of the user we are trying to add is
// invalid we both get an error back that satisfies usererrors.UsernameNotValid
// and that no state changes occur.
func (s *serviceSuite) TestAddUserWithPasswordInvalidUser(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)

	// Add admin user to state.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[adminUUID] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}

	fakeUser := user.User{
		Name:        invalidUsernames[0],
		DisplayName: "Display",
	}

	fakePassword := auth.NewPassword("password")

	_, err = s.service().AddUserWithPassword(context.Background(), fakeUser, adminUUID, fakePassword)
	c.Assert(err, jc.ErrorIs, usererrors.UsernameNotValid)

	c.Assert(fakePassword.IsDestroyed(), jc.IsTrue)
	c.Assert(len(mockState), gc.Equals, 1)
}

// TestAddUserWithPasswordAlreadyExists is testing that if we try and add a user
// with the same name as one that already exists we get back an error that
// satisfies usererrors.AlreadyExists.
func (s *serviceSuite) TestAddUserWithPasswordAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[adminUUID] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}

	jimboUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[jimboUUID] = stateUser{
		createdAt:   time.Now(),
		name:        "jimbo",
		displayName: "Jimmy",
		removed:     false,
	}

	usr := user.User{
		DisplayName: "tlm",
		Name:        "jimbo",
	}
	password := auth.NewPassword("51b11eb2e6d094a62a489e40")

	_, err = s.service().AddUserWithPassword(context.Background(), usr, adminUUID, password)
	c.Assert(err, jc.ErrorIs, usererrors.AlreadyExists)

	// Let's check that the password was destroyed as per the func contract.
	c.Assert(password.IsDestroyed(), jc.IsTrue)

	// We now need to double-check no state change occurred.
	userState := mockState[jimboUUID]
	c.Assert(userState.displayName, gc.Equals, "Jimmy")
	c.Assert(userState.removed, jc.IsFalse)
}

// TestAddUserWithPasswordDestroyedPassword tests that when adding a new user
// with password we get an internal/auth.ErrPasswordDestroyed back when passing
// in a password that has already been destroyed.
//
// The reason we want to check this is because, there could exist circumstances
// where a call might fail for a user password and something else has zero'd the
// password. This is most commonly going to happen because of retry logic.
func (s *serviceSuite) TestAddUserWithPasswordDestroyedPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[adminUUID] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}

	usr := user.User{
		DisplayName: "tlm",
		Name:        "tlm",
	}
	password := auth.NewPassword("51b11eb2e6d094a62a489e40")
	password.Destroy()

	_, err = s.service().AddUserWithPassword(context.Background(), usr, adminUUID, password)
	c.Assert(err, jc.ErrorIs, auth.ErrPasswordDestroyed)

	// Let's check that the password was destroyed as per the func contract.
	c.Assert(password.IsDestroyed(), jc.IsTrue)

	// Check that no state changes occurred.
	c.Assert(len(mockState), gc.Equals, 1)
}

// TestAddUserWithPasswordNotValid is checking that if we try and add a user
// with password that is not valid we get back a error that satisfies
// internal/auth.ErrorPasswordNotValid.
func (s *serviceSuite) TestAddUserWithPasswordNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[adminUUID] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}

	usr := user.User{
		DisplayName: "tlm",
		Name:        "tlm",
	}
	password := auth.NewPassword("")
	password.Destroy()

	_, err = s.service().AddUserWithPassword(context.Background(), usr, adminUUID, password)
	c.Assert(err, jc.ErrorIs, auth.ErrPasswordNotValid)

	// Check that no state changes occurred.
	c.Assert(len(mockState), gc.Equals, 1)
}

// TestAddUserWithPasswordInvalidUsername is testing the happy path for adding a
// user and generating an activation key for the new user.
func (s *serviceSuite) TestAddUserWithActivationKey(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	mockState[adminUUID] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}

	usr := user.User{
		Name:        "f00-Bar.ram77",
		DisplayName: "Display",
	}

	activationKey, usrUUID, err := s.service().AddUserWithActivationKey(context.Background(), usr, adminUUID)
	c.Assert(err, jc.ErrorIsNil)

	userState := mockState[usrUUID]
	c.Assert(len(activationKey) > 0, jc.IsTrue)
	c.Assert(userState.activationKey, gc.DeepEquals, activationKey)
	c.Assert(userState.displayName, gc.Equals, "Display")

	// We want to check now that we can add a user with the same name as one
	// that has already been removed.
	adamUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[adamUUID] = stateUser{
		name:      "adam",
		createdAt: time.Now(),
		removed:   true,
	}

	usr = user.User{
		Name:        "adam",
		DisplayName: "Adam",
	}

	activationKey, usrUUID, err = s.service().AddUserWithActivationKey(context.Background(), usr, adminUUID)
	c.Assert(err, jc.ErrorIsNil)

	userState = mockState[usrUUID]
	c.Assert(len(activationKey) > 0, jc.IsTrue)
	c.Assert(userState.activationKey, gc.DeepEquals, activationKey)
	c.Assert(userState.displayName, gc.Equals, "Adam")
}

// TestAddUserWithActivationKeyUsernameNotValid is testing that if we add a user
// with an invalid username that we get back an error that satisfies
// usererrors.UsernameNotValid.
func (s *serviceSuite) TestAddUserWithActivationKeyUsernameNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)

	// Add admin user to state.
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[adminUUID] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}

	usr := user.User{
		Name:        invalidUsernames[0],
		DisplayName: "Display",
	}

	activationKey, _, err := s.service().AddUserWithActivationKey(context.Background(), usr, adminUUID)
	c.Assert(err, jc.ErrorIs, usererrors.UsernameNotValid)
	c.Assert(len(mockState), gc.Equals, 1)
	c.Assert(len(activationKey), gc.Equals, 0)
}

// TestAddUserWithActivationKeyAlreadyExists is testing that is we try to add a
// user that already exists we get back an error that satisfies
// usererrors.AlreadyExists.
func (s *serviceSuite) TestAddUserWithActivationKeyAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	mockState[adminUUID] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}

	gazzaUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[gazzaUUID] = stateUser{
		name:        "gazza",
		displayName: "Garry",
		createdAt:   time.Now(),
	}

	usr := user.User{
		Name:        "gazza",
		DisplayName: "Garry",
	}

	activationKey, _, err := s.service().AddUserWithActivationKey(context.Background(), usr, adminUUID)
	c.Assert(err, jc.ErrorIs, usererrors.AlreadyExists)
	c.Assert(len(mockState), gc.Equals, 2)
	c.Assert(len(activationKey), gc.Equals, 0)

	// check that no state change occurred for the already established user
	resultUser, err := s.service().GetUser(context.Background(), gazzaUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultUser.DisplayName, gc.Equals, "Garry")

}

// TestRemoveUser is testing the happy path for removing a user.
func (s *serviceSuite) TestRemoveUser(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[uuid] = stateUser{
		activationKey: []byte{0x1, 0x2, 0x3},
		passwordHash:  "secrethash",
		passwordSalt:  []byte{0x1, 0x2, 0x3},
	}

	err = s.service().RemoveUser(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	userState := mockState[uuid]
	c.Assert(userState.removed, jc.IsTrue)
	c.Assert(userState.passwordHash, gc.Equals, "")
	c.Assert(userState.passwordSalt, gc.IsNil)
	c.Assert(userState.activationKey, gc.IsNil)
}

// TestRemoveUserInvalidUUID is testing that if we supply RemoveUser with
// invalid UUIDs we get back an error.
func (s *serviceSuite) TestRemoveUserInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid := user.UUID("invalid-UUID")

	err := s.service().RemoveUser(context.Background(), uuid)
	c.Assert(err, jc.ErrorIs, usererrors.UUIDNotValid)
	c.Assert(len(mockState), gc.Equals, 0)
}

// TestRemoveUserNotFound is testing that trying to remove a user that does not
// exist results in an error that satisfies usererrors.UserNotFound. We also
// check that no state changes occur.
func (s *serviceSuite) TestRemoveUserNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	err = s.service().RemoveUser(context.Background(), uuid)
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
	c.Assert(len(mockState), gc.Equals, 0)
}

// TestSetPassword is testing the happy path for SetPassword.
func (s *serviceSuite) TestSetPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	mockState[uuid] = stateUser{
		name: "username",
	}

	password := auth.NewPassword("password")
	err = s.service().SetPassword(context.Background(), uuid, password)
	c.Assert(err, jc.ErrorIsNil)

	userState := mockState[uuid]
	c.Assert(password.IsDestroyed(), jc.IsTrue)
	c.Assert(userState.passwordHash == "", jc.IsFalse)
	c.Assert(len(userState.passwordSalt) == 0, jc.IsFalse)
	c.Assert(userState.activationKey, gc.IsNil)
}

// TestSetPasswordInvalidUUID is testing that if we supply SetPassword with
// invalid UUIDs we get back an error.
func (s *serviceSuite) TestSetPasswordInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid := user.UUID("invalid-UUID")

	password := auth.NewPassword("password")
	err := s.service().SetPassword(context.Background(), uuid, password)
	c.Assert(err, jc.ErrorIs, usererrors.UUIDNotValid)
	c.Assert(len(mockState), gc.Equals, 0)
	c.Assert(password.IsDestroyed(), jc.IsTrue)
}

// TestSetPasswordUserNotFound is testing that when setting a password for a
// user that doesn't exist we get a user.NotFound error and that the password
// gets destroyed.
func (s *serviceSuite) TestSetPasswordUserNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	password := auth.NewPassword("password")
	err = s.service().SetPassword(context.Background(), uuid, password)
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
	c.Assert(len(mockState), gc.Equals, 0)
	c.Assert(password.IsDestroyed(), jc.IsTrue)
}

// TestSetPasswordInvalid is asserting that if pass invalid passwords to
// SetPassword the correct errors are returned.
func (s *serviceSuite) TestSetPasswordInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	mockState[uuid] = stateUser{}

	// Empty password is a no no, well at least it should be.
	password := auth.NewPassword("")
	err = s.service().SetPassword(context.Background(), uuid, password)
	c.Assert(err, jc.ErrorIs, auth.ErrPasswordNotValid)
	userState := mockState[uuid]
	c.Assert(userState.passwordHash, gc.Equals, "")
	c.Assert(len(userState.passwordSalt), gc.Equals, 0)

	password = auth.NewPassword("password")
	password.Destroy()
	err = s.service().SetPassword(context.Background(), uuid, password)
	c.Assert(err, jc.ErrorIs, auth.ErrPasswordDestroyed)
	userState = mockState[uuid]
	c.Assert(userState.passwordHash, gc.Equals, "")
	c.Assert(len(userState.passwordSalt), gc.Equals, 0)
}

// TestResetPassword tests the happy path for resetting a user's password.
func (s *serviceSuite) TestResetPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[uuid] = stateUser{
		name:         "name",
		passwordHash: "12345",
		passwordSalt: []byte{0x1, 0x2, 0x3, 0x4},
	}

	key, err := s.service().ResetPassword(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	userState := mockState[uuid]
	c.Assert(len(key) > 0, jc.IsTrue)
	c.Assert(userState.activationKey, gc.DeepEquals, key)
	c.Assert(userState.passwordHash, gc.DeepEquals, "")
	c.Assert(userState.passwordSalt, gc.IsNil)
}

// TestResetPasswordInvalidUserUUID is testing that if we supply ResetPassword
// with an invalid UUID we get back an error.
func (s *serviceSuite) TestResetPasswordInvalidUserUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid := user.UUID("invalid-UUID")

	_, err := s.service().ResetPassword(context.Background(), uuid)
	c.Assert(err, jc.ErrorIs, usererrors.UUIDNotValid)
	c.Assert(len(mockState), gc.Equals, 0)
}

// TestResetPassword is testing that resting a password for a user that doesn't
// exist returns a usererrors.NotFound error and that no state change occurs.
func (s *serviceSuite) TestResetPasswordUserNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.service().ResetPassword(context.Background(), uuid)
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
	c.Assert(len(mockState), gc.Equals, 0)
}

// TestGetUserNotFound is testing what the service does when we ask for a user
// that doesn't exist. The expected behaviour is that an error is returned that
// satisfies usererrors.NotFound.
func (s *serviceSuite) TestGetUserNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)

	nonExistingUserUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.service().GetUser(context.Background(), nonExistingUserUUID)
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
	c.Assert(len(mockState), gc.Equals, 0)
}

// TestGetUserRemoved tests that getting a user by name that has been removed
// results in an error that satisfies usererrors.NotFound. We also want to
// check that no state change occurs.
func (s *serviceSuite) TestGetUserRemoved(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)

	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	mockState[userUUID] = stateUser{
		name:    "removedUser",
		removed: true,
	}

	removedUser, err := s.service().GetUser(context.Background(), userUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(removedUser.Name, gc.Equals, "removedUser")

	_, err = s.service().GetUserByName(context.Background(), "removedUser")
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
}

// TestGetUser is asserting the safe path of GetUser in that if we supply a
// happy and good username and the username exists in state we get back a valid
// user object.
func (s *serviceSuite) TestGetUser(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid1, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[uuid1] = stateUser{
		name:        "Jürgen.test",
		createdAt:   time.Now().Add(-time.Minute * 5),
		displayName: "Old mate 👍",
	}
	uuid2, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[uuid2] = stateUser{
		name:        "杨-test",
		createdAt:   time.Now().Add(-time.Minute * 5),
		displayName: "test1",
	}

	for userUUID, userSt := range mockState {
		rval, err := s.service().GetUser(context.Background(), userUUID)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(rval.Name, gc.Equals, userSt.name)
		c.Assert(rval.DisplayName, gc.Equals, userSt.displayName)
	}
}

// TestGetUserByName tests the happy path for GetUserByName.
func (s *serviceSuite) TestGetUserByName(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid1, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[uuid1] = stateUser{
		name:        "Jürgen.test",
		createdAt:   time.Now().Add(-time.Minute * 5),
		displayName: "Old mate 👍",
	}
	uuid2, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[uuid2] = stateUser{
		name:        "杨-test",
		createdAt:   time.Now().Add(-time.Minute * 5),
		displayName: "test1",
	}

	for _, userSt := range mockState {
		rval, err := s.service().GetUserByName(context.Background(), userSt.name)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(rval.Name, gc.Equals, userSt.name)
		c.Assert(rval.DisplayName, gc.Equals, userSt.displayName)
	}
}

// TestGetUserByNameNotFound is testing that if we ask for a user by name that
// doesn't exist we get back an error that satisfies usererrors.NotFound.
func (s *serviceSuite) TestGetUserByNameNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)

	_, err := s.service().GetUserByName(context.Background(), "test")
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
	c.Assert(len(mockState), gc.Equals, 0)
}

// TestGetAllUsers tests the happy path for GetAllUsers.
func (s *serviceSuite) TestGetAllUsers(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid1, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[uuid1] = stateUser{
		name:        "Jürgen.test",
		createdAt:   time.Now().Add(-time.Minute * 5),
		displayName: "Old mate 👍",
	}
	uuid2, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[uuid2] = stateUser{
		name:        "杨-test",
		createdAt:   time.Now().Add(-time.Minute * 5),
		displayName: "test1",
	}

	users, err := s.service().GetAllUsers(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(users), gc.Equals, 2)
	c.Assert(users[0].Name, gc.Equals, "Jürgen.test")
	c.Assert(users[1].Name, gc.Equals, "杨-test")
}

// TestGetUserWithAuthInfo tests the happy path for GetUserWithAuthInfo.
func (s *serviceSuite) TestGetUserWithAuthInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	lastLogin := time.Now().Add(-time.Minute * 2)
	mockState[uuid] = stateUser{
		name:        "Jürgen.test",
		createdAt:   time.Now().Add(-time.Minute * 5),
		displayName: "Old mate 👍",
		lastLogin:   lastLogin,
		disabled:    true,
	}

	user, err := s.service().GetUserWithAuthInfo(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.Name, gc.Equals, "Jürgen.test")
	c.Assert(user.DisplayName, gc.Equals, "Old mate 👍")
	c.Assert(user.LastLogin, gc.Equals, lastLogin)
	c.Assert(user.Disabled, gc.Equals, true)
}

// TestGetUserWithAuthInfoNotFound is testing that if we ask for a user that
// doesn't exist we get back an error that satisfies usererrors.NotFound.
func (s *serviceSuite) TestGetUserWithAuthInfoNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)

	nonExistingUserUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.service().GetUserWithAuthInfo(context.Background(), nonExistingUserUUID)
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
	c.Assert(len(mockState), gc.Equals, 0)
}

// TestGetUserByNameInvalidUsername is here to assert that when we ask for a user with
// a username that is invalid we get a UsernameNotValid error. We also check
// here that the service doesn't let invalid usernames flow through to the state
// layer.
func (s *serviceSuite) TestGetUserByNameInvalidUsername(c *gc.C) {
	defer s.setupMocks(c).Finish()
	for _, invalid := range invalidUsernames {
		_, err := s.service().GetUserByName(context.Background(), invalid)
		c.Assert(err, jc.ErrorIs, usererrors.UsernameNotValid)
	}
}

// TestEnableUserAuthentication tests the happy path for EnableUserAuthentication.
func (s *serviceSuite) TestEnableUserAuthentication(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	mockState[uuid] = stateUser{
		name:     "username",
		disabled: true,
	}

	err = s.service().EnableUserAuthentication(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	userState := mockState[uuid]
	c.Assert(userState.disabled, jc.IsFalse)
}

// TestDisableUserAuthentication tests the happy path for DisableUserAuthentication.
func (s *serviceSuite) TestDisableUserAuthentication(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	mockState[uuid] = stateUser{
		name:     "username",
		disabled: false,
	}

	err = s.service().DisableUserAuthentication(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	userState := mockState[uuid]
	c.Assert(userState.disabled, jc.IsTrue)
}

// FuzzGetUser is a fuzz test for GetUser() that stresses the username input of
// the function to make sure that no panics occur and all input is handled
// gracefully.
func FuzzGetUser(f *testing.F) {
	for _, valid := range validUsernames {
		f.Add(valid)
	}

	f.Fuzz(func(t *testing.T, username string) {
		ctrl := gomock.NewController(t)
		state := NewMockState(ctrl)
		defer ctrl.Finish()

		state.EXPECT().GetUserByName(gomock.Any(), username).Return(
			user.User{
				Name: username,
			},
			nil,
		).AnyTimes()

		usr, err := NewService(state).GetUserByName(context.Background(), username)
		if err != nil && !errors.Is(err, usererrors.UsernameNotValid) {
			t.Errorf("unexpected error %v when fuzzing GetUser with %q",
				err, username,
			)
		} else if errors.Is(err, usererrors.UsernameNotValid) {
			return
		}

		if usr.Name != username {
			t.Errorf("GetUser() user.name %q != %q", usr.Name, username)
		}
	})
}

// TestUsernameValidation exists to assert the regex that is in use by
// ValidateUsername. We want to pass it a wide range of unicode names with weird
func (s *serviceSuite) TestUsernameValidation(c *gc.C) {
	tests := []struct {
		Username   string
		ShouldPass bool
	}{}

	for _, valid := range validUsernames {
		tests = append(tests, struct {
			Username   string
			ShouldPass bool
		}{valid, true})
	}

	for _, invalid := range invalidUsernames {
		tests = append(tests, struct {
			Username   string
			ShouldPass bool
		}{invalid, false})
	}

	for _, test := range tests {
		err := ValidateUsername(test.Username)
		if test.ShouldPass {
			c.Assert(err, jc.ErrorIsNil, gc.Commentf("test username %q", test.Username))
		} else {
			c.Assert(
				err, jc.ErrorIs, usererrors.UsernameNotValid,
				gc.Commentf("test username %q", test.Username),
			)
		}
	}
}
