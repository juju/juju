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
	uuid          user.UUID
	activationKey []byte
	creatorUUID   user.UUID
	createdAt     time.Time
	name          string
	displayName   string
	passwordHash  string
	passwordSalt  []byte
	removed       bool
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

func (s *serviceSuite) setMockState(c *gc.C) map[string]stateUser {
	mockState := map[string]stateUser{}

	s.state.EXPECT().GetUser(
		gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context,
		id user.UUID) (user.User, error) {
		stUser, exists := mockState[id.String()]
		if !exists || stUser.removed {
			return user.User{}, usererrors.NotFound
		}
		return user.User{
			UUID:        stUser.uuid,
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
			if usr.name == name {
				return user.User{
					UUID:        usr.uuid,
					CreatorUUID: usr.creatorUUID,
					CreatedAt:   usr.createdAt,
					DisplayName: usr.displayName,
					Name:        usr.name,
				}, nil
			}
		}
		return user.User{}, usererrors.NotFound
	}).AnyTimes()

	s.state.EXPECT().AddUser(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context,
		uuid user.UUID,
		user user.User,
		creatorUUID user.UUID) error {
		usr, exists := mockState[user.Name]
		if exists && !usr.removed {
			return usererrors.AlreadyExists
		}
		mockState[user.Name] = stateUser{
			creatorUUID: creatorUUID,
			createdAt:   time.Now(),
			displayName: user.DisplayName,
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
		usr, exists := mockState[user.Name]
		if exists && !usr.removed {
			return usererrors.AlreadyExists
		}
		mockState[user.Name] = stateUser{
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
		usr, exists := mockState[user.Name]
		if exists && !usr.removed {
			return usererrors.AlreadyExists
		}
		mockState[user.Name] = stateUser{
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
		user, exists := mockState[uuid.String()]
		if !exists || user.removed {
			return usererrors.NotFound
		}
		user.removed = true
		user.activationKey = nil
		user.passwordHash = ""
		user.passwordSalt = nil
		mockState[uuid.String()] = user
		return nil
	}).AnyTimes()

	s.state.EXPECT().SetActivationKey(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context,
		uuid user.UUID,
		key []byte) error {
		user, exists := mockState[uuid.String()]
		if !exists || user.removed {
			return usererrors.NotFound
		}
		user.passwordHash = ""
		user.passwordSalt = nil
		user.activationKey = key
		mockState[uuid.String()] = user
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
		user, exists := mockState[uuid.String()]
		if !exists || user.removed {
			return usererrors.NotFound
		}
		user.passwordHash = hash
		user.passwordSalt = salt
		user.activationKey = nil
		mockState[uuid.String()] = user
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
	mockState[adminUUID.String()] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}

	usr := user.User{
		Name:        "f00-Bar.ram77",
		DisplayName: "Display",
	}

	err = s.service().AddUser(context.Background(), usr, adminUUID)
	c.Assert(err, jc.ErrorIsNil)

	userState := mockState["f00-Bar.ram77"]
	c.Assert(userState.displayName, gc.Equals, "Display")

	// We want to check now that we can add a user with the same name as one
	// that has already been removed.
	mockState["grace"] = stateUser{
		createdAt:   time.Now(),
		displayName: "Grace",
		removed:     true,
	}

	usr = user.User{
		Name:        "grace",
		DisplayName: "test",
	}

	err = s.service().AddUser(context.Background(), usr, adminUUID)
	c.Assert(err, jc.ErrorIsNil)

	userState = mockState["grace"]
	c.Assert(userState.displayName, gc.Equals, "test")
	c.Assert(userState.creatorUUID, gc.Equals, user.UUID(""))
}

// TestAddUserCreatorUUIDNotFound is testing that if we try and add a user with the
// creator UUID field set and the creator UUID does not exist we get a error back that
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

	err = s.service().AddUser(context.Background(), usr, adminUUID)
	c.Assert(err, jc.ErrorIs, usererrors.UserCreatorUUIDNotFound)

	// We need to check that there were no side effects to state.
	c.Assert(len(mockState), gc.Equals, 0)
}

// TestAddUserUsernameNotValid is testing that if we try and add a user with a
// username that is not valid we get a error that satisfies
// usererrors.UsernameNotValid back.
func (s *serviceSuite) TestAddUserUsernameNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	mockState["admin"] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}

	usr := user.User{
		Name:        invalidUsernames[0],
		DisplayName: "Display",
	}

	err := s.service().AddUser(context.Background(), usr, "admin")
	c.Assert(err, jc.ErrorIs, usererrors.UsernameNotValid)

	// We need to check that there were no side effects to state.
	c.Assert(len(mockState), gc.Equals, 1)
	_, exists := mockState[invalidUsernames[0]]
	c.Assert(exists, jc.IsFalse)
}

// TestAddUserAlreadyExists is testing that we cannot add a user with a username
// that already exists and is active. We expect that in this case we should
// receive an error back that satisfies usererrors.AlreadyExists.
func (s *serviceSuite) TestAddUserAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[adminUUID.String()] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}
	createdAt := time.Now()
	mockState["fred"] = stateUser{
		createdAt:   createdAt,
		displayName: "Freddie",
	}

	usr := user.User{
		Name:        "fred",
		DisplayName: "Display",
	}

	err = s.service().AddUser(context.Background(), usr, adminUUID)
	c.Assert(err, jc.ErrorIs, usererrors.AlreadyExists)

	// Test no state changes occurred
	c.Assert(mockState["fred"], jc.DeepEquals, stateUser{
		createdAt:   createdAt,
		displayName: "Freddie",
	})
}

// TestAddUserWithPassword is testing the happy path of AddUserWithPassword.
func (s *serviceSuite) TestAddUserWithPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[adminUUID.String()] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}

	usr := user.User{
		Name:        "f00-Bar.ram77",
		DisplayName: "Display",
	}
	password := auth.NewPassword("password")

	err = s.service().AddUserWithPassword(context.Background(), usr, adminUUID, password)
	c.Assert(err, jc.ErrorIsNil)

	userState := mockState[usr.Name]
	c.Assert(password.IsDestroyed(), jc.IsTrue)
	c.Assert(userState.passwordHash == "", jc.IsFalse)
	c.Assert(len(userState.passwordSalt) == 0, jc.IsFalse)
	c.Assert(userState.activationKey, gc.IsNil)

	mockState["fiona"] = stateUser{
		displayName: "Fee",
		createdAt:   time.Now(),
		removed:     true,
	}

	usr = user.User{
		Name:        "fiona",
		DisplayName: "Fiona",
	}
	password = auth.NewPassword("password")

	err = s.service().AddUserWithPassword(context.Background(), usr, adminUUID, password)
	c.Assert(err, jc.ErrorIsNil)

	userState = mockState[usr.Name]
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

	err = s.service().AddUserWithPassword(context.Background(), usr, adminUUID, password)
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

	fakeUser := user.User{
		Name:        invalidUsernames[0],
		DisplayName: "Display",
	}

	fakePassword := auth.NewPassword("password")

	err := s.service().AddUserWithPassword(context.Background(), fakeUser, "admin", fakePassword)
	c.Assert(err, jc.ErrorIs, usererrors.UsernameNotValid)

	c.Assert(fakePassword.IsDestroyed(), jc.IsTrue)
	c.Assert(len(mockState), gc.Equals, 0)
}

// TestAddUserWithPasswordAlreadyExists is testing that if we try and add a user
// with the same name as one that already exists we get back an error that
// satisfies usererrors.AlreadyExists.
func (s *serviceSuite) TestAddUserWithPasswordAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[adminUUID.String()] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}

	mockState["jimbo"] = stateUser{
		createdAt:   time.Now(),
		displayName: "Jimmy",
		removed:     false,
	}

	usr := user.User{
		DisplayName: "tlm",
		Name:        "jimbo",
	}
	password := auth.NewPassword("51b11eb2e6d094a62a489e40")

	err = s.service().AddUserWithPassword(context.Background(), usr, adminUUID, password)
	c.Assert(err, jc.ErrorIs, usererrors.AlreadyExists)

	// Let's check that the password was destroyed as per the func contract.
	c.Assert(password.IsDestroyed(), jc.IsTrue)

	// We now need to double-check no state change occurred.
	userState := mockState["jimbo"]
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

	usr := user.User{
		DisplayName: "tlm",
		Name:        "tlm",
	}
	password := auth.NewPassword("51b11eb2e6d094a62a489e40")
	password.Destroy()

	err = s.service().AddUserWithPassword(context.Background(), usr, adminUUID, password)
	c.Assert(err, jc.ErrorIs, auth.ErrPasswordDestroyed)

	// Let's check that the password was destroyed as per the func contract.
	c.Assert(password.IsDestroyed(), jc.IsTrue)

	// Check that no state changes occurred.
	c.Assert(len(mockState), gc.Equals, 0)
}

// TestAddUserWithPasswordNotValid is checking that if we try and add a user
// with password that is not valid we get back a error that satisfies
// internal/auth.ErrorPasswordNotValid.
func (s *serviceSuite) TestAddUserWithPasswordNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	usr := user.User{
		DisplayName: "tlm",
		Name:        "tlm",
	}
	password := auth.NewPassword("")
	password.Destroy()

	err = s.service().AddUserWithPassword(context.Background(), usr, adminUUID, password)
	c.Assert(err, jc.ErrorIs, auth.ErrPasswordNotValid)

	// Check that no state changes occurred.
	c.Assert(len(mockState), gc.Equals, 0)
}

// TestAddUserWithPasswordInvalidUsername is testing the happy path for adding a
// user and generating an activation key for the new user.
func (s *serviceSuite) TestAddUserWithActivationKey(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	mockState[adminUUID.String()] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}

	u := user.User{
		Name:        "f00-Bar.ram77",
		DisplayName: "Display",
	}

	activationKey, err := s.service().AddUserWithActivationKey(context.Background(), u, adminUUID)
	c.Assert(err, jc.ErrorIsNil)

	userState := mockState[u.Name]
	c.Assert(len(activationKey) > 0, jc.IsTrue)
	c.Assert(userState.activationKey, gc.DeepEquals, activationKey)
	c.Assert(userState.displayName, gc.Equals, "Display")

	// We want to check now that we can add a user with the same name as one
	// that has already been removed.
	mockState["adam"] = stateUser{
		createdAt: time.Now(),
		removed:   true,
	}

	u = user.User{
		Name:        "adam",
		DisplayName: "Adam",
	}

	activationKey, err = s.service().AddUserWithActivationKey(context.Background(), u, adminUUID)
	c.Assert(err, jc.ErrorIsNil)

	userState = mockState[u.Name]
	c.Assert(len(activationKey) > 0, jc.IsTrue)
	c.Assert(userState.activationKey, gc.DeepEquals, activationKey)
	c.Assert(userState.displayName, gc.Equals, "Adam")
	c.Assert(userState.creatorUUID, gc.Equals, user.UUID(""))
}

// TestAddUserWithActivationKeyUsernameNotValid is testing that if we add a user
// with an invalid username that we get back a error that satisfies
// usererrors.UsernameNotValid.
func (s *serviceSuite) TestAddUserWithActivationKeyUsernameNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)

	usr := user.User{
		Name:        invalidUsernames[0],
		DisplayName: "Display",
	}

	activationKey, err := s.service().AddUserWithActivationKey(context.Background(), usr, "admin")
	c.Assert(err, jc.ErrorIs, usererrors.UsernameNotValid)
	c.Assert(len(mockState), gc.Equals, 0)
	c.Assert(len(activationKey), gc.Equals, 0)
}

// TestAddUserWithActivationKeyAlreadyExists is testing that is we try to add a
// user that already exists we get back a error that satisfies
// usererrors.AlreadyExists.
func (s *serviceSuite) TestAddUserWithActivationKeyAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	adminUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	mockState[adminUUID.String()] = stateUser{
		createdAt:   time.Now(),
		displayName: "Admin",
	}

	mockState["gazza"] = stateUser{
		displayName: "Garry",
		createdAt:   time.Now(),
	}

	usr := user.User{
		Name:        "gazza",
		DisplayName: "Garry",
	}

	activationKey, err := s.service().AddUserWithActivationKey(context.Background(), usr, adminUUID)
	c.Assert(err, jc.ErrorIs, usererrors.AlreadyExists)
	c.Assert(len(mockState), gc.Equals, 2)
	c.Assert(len(activationKey), gc.Equals, 0)
}

// TestRemoveUser is testing the happy path for removing a user.
func (s *serviceSuite) TestRemoveUser(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[uuid.String()] = stateUser{
		activationKey: []byte{0x1, 0x2, 0x3},
		passwordHash:  "secrethash",
		passwordSalt:  []byte{0x1, 0x2, 0x3},
	}

	err = s.service().RemoveUser(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	userState := mockState[uuid.String()]
	c.Assert(userState.removed, jc.IsTrue)
	c.Assert(userState.passwordHash, gc.Equals, "")
	c.Assert(userState.passwordSalt, gc.IsNil)
	c.Assert(userState.activationKey, gc.IsNil)
}

// TestRemoveUser is testing that removing a user when they're already removed
// results in a usererrors.NotFound error.
func (s *serviceSuite) TestRemoveUserAlreadyRemoved(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[uuid.String()] = stateUser{
		removed: true,
	}

	err = s.service().RemoveUser(context.Background(), uuid)
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
	userState := mockState[uuid.String()]
	c.Assert(userState.removed, jc.IsTrue)
}

// TestRemoveUserInvalidUUID is testing that if we supply RemoveUser with
// invalid usernames we get back a error that satisfies
// usererrors.UsernameNotValid and not state changes occur.
func (s *serviceSuite) TestRemoveUserInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid := user.UUID("invalid-UUID")

	err := s.service().RemoveUser(context.Background(), uuid)
	c.Assert(err, gc.ErrorMatches, "removing user uuid \\\"invalid-UUID\\\": invalid uuid: \\\"invalid-UUID\\\"")
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
	mockState[uuid.String()] = stateUser{
		uuid: uuid,
		name: uuid.String(),
	}

	password := auth.NewPassword("password")
	err = s.service().SetPassword(context.Background(), uuid, password)
	c.Assert(err, jc.ErrorIsNil)

	userState := mockState[uuid.String()]
	c.Assert(password.IsDestroyed(), jc.IsTrue)
	c.Assert(userState.passwordHash == "", jc.IsFalse)
	c.Assert(len(userState.passwordSalt) == 0, jc.IsFalse)
	c.Assert(userState.activationKey, gc.IsNil)
}

// TestSetPasswordInvalidUsername is testing that if we throw junk usernames at
// set password we get username invalid errors and that the junk doesn't end up
// in state. We also want to assert that the password is destroyed no matter
// what.
func (s *serviceSuite) TestSetPasswordInvalidUsername(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid := user.UUID("invalid-UUID")

	password := auth.NewPassword("password")
	err := s.service().SetPassword(context.Background(), uuid, password)
	c.Assert(err, gc.ErrorMatches, "setting password for user uuid \\\"invalid-UUID\\\": invalid uuid: \\\"invalid-UUID\\\"")
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

	mockState[uuid.String()] = stateUser{}

	// Empty password is a no no, well at least it should be.
	password := auth.NewPassword("")
	err = s.service().SetPassword(context.Background(), uuid, password)
	c.Assert(err, jc.ErrorIs, auth.ErrPasswordNotValid)
	userState := mockState[uuid.String()]
	c.Assert(userState.passwordHash, gc.Equals, "")
	c.Assert(len(userState.passwordSalt), gc.Equals, 0)

	password = auth.NewPassword("password")
	password.Destroy()
	err = s.service().SetPassword(context.Background(), uuid, password)
	c.Assert(err, jc.ErrorIs, auth.ErrPasswordDestroyed)
	userState = mockState[uuid.String()]
	c.Assert(userState.passwordHash, gc.Equals, "")
	c.Assert(len(userState.passwordSalt), gc.Equals, 0)
}

// TestResetPassword tests the happy path for resetting a user's password.
func (s *serviceSuite) TestResetPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[uuid.String()] = stateUser{
		uuid:         uuid,
		name:         uuid.String(),
		passwordHash: "12345",
		passwordSalt: []byte{0x1, 0x2, 0x3, 0x4},
	}

	key, err := s.service().ResetPassword(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	userState := mockState[uuid.String()]
	c.Assert(len(key) > 0, jc.IsTrue)
	c.Assert(userState.activationKey, gc.DeepEquals, key)
	c.Assert(userState.passwordHash, gc.DeepEquals, "")
	c.Assert(userState.passwordSalt, gc.IsNil)
}

// TestResetPasswordInvalidUser is testing invalid usernames to reset password
// causes an usererrors.NotValid error to be returned and no state changes occurs.
func (s *serviceSuite) TestResetPasswordInvalidUser(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid := user.UUID("invalid-UUID")

	_, err := s.service().ResetPassword(context.Background(), uuid)
	c.Assert(err, gc.ErrorMatches, "resetting password for user uuid \\\"invalid-UUID\\\": invalid uuid: \\\"invalid-UUID\\\"")
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

// TestGetUserRemoved tests that getting a user that has been removed results in
// an error that satisfies usererrors.NotFound. We also want to check that no
// state change occurs
func (s *serviceSuite) TestGetUserRemoved(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)

	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	mockState[userUUID.String()] = stateUser{
		removed: true,
	}

	_, err = s.service().GetUser(context.Background(), userUUID)
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
	mockState[uuid1.String()] = stateUser{
		uuid:        uuid1,
		name:        "Jürgen.test",
		createdAt:   time.Now().Add(-time.Minute * 5),
		displayName: "Old mate 👍",
	}
	uuid2, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[uuid2.String()] = stateUser{
		uuid:        uuid2,
		name:        "杨-test",
		createdAt:   time.Now().Add(-time.Minute * 5),
		displayName: "test1",
	}

	for userId, userSt := range mockState {
		rval, err := s.service().GetUser(context.Background(), user.UUID(userId))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(rval.UUID, gc.Equals, user.UUID(userId))
		c.Assert(rval.DisplayName, gc.Equals, userSt.displayName)
	}
}

// TestGetUserByName tests the happy path for GetUserByName.
func (s *serviceSuite) TestGetUserByName(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mockState := s.setMockState(c)
	uuid1, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[uuid1.String()] = stateUser{
		uuid:        uuid1,
		name:        "Jürgen.test",
		createdAt:   time.Now().Add(-time.Minute * 5),
		displayName: "Old mate 👍",
	}
	uuid2, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	mockState[uuid2.String()] = stateUser{
		uuid:        uuid2,
		name:        "杨-test",
		createdAt:   time.Now().Add(-time.Minute * 5),
		displayName: "test1",
	}

	for _, userSt := range mockState {
		rval, err := s.service().GetUserByName(context.Background(), userSt.name)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(rval.UUID, gc.Equals, userSt.uuid)
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

// FuzzGetUser is a fuzz test for GetUser() that stresses the username input of
// the function to make sure that no panics occur and all input is handled
// gracefully.
func FuzzGetUser(f *testing.F) {
	for _, valid := range validUsernames {
		f.Add(valid)
	}

	f.Fuzz(func(t *testing.T, idStr string) {
		ctrl := gomock.NewController(t)
		state := NewMockState(ctrl)
		defer ctrl.Finish()

		uuid, err := user.NewUUID()
		if err != nil {
			t.Fatalf("unexpected error %v when fuzzing GetUser with %q",
				err, idStr,
			)
		}

		state.EXPECT().GetUser(gomock.Any(), uuid).Return(
			user.User{
				UUID: uuid,
			},
			nil,
		).AnyTimes()

		usr, err := NewService(state).GetUser(context.Background(), uuid)
		if err != nil && !errors.Is(err, usererrors.UsernameNotValid) {
			t.Errorf("unexpected error %v when fuzzing GetUser with %q",
				err, uuid,
			)
		} else if errors.Is(err, usererrors.UsernameNotValid) {
			return
		}

		if usr.UUID != uuid {
			t.Errorf("GetUser() user.name %q != %q", usr.Name, uuid.String())
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
