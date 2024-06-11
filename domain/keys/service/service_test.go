// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	accesserrors "github.com/juju/juju/domain/access/errors"
	keyserrors "github.com/juju/juju/domain/keys/errors"
	"github.com/juju/juju/internal/ssh"
)

type serviceSuite struct {
	testing.IsolationSuite

	state  *MockState
	userID user.UUID
}

var (
	_ = gc.Suite(&serviceSuite{})

	existingUserKeys = []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
	}

	// testingKeys represents a set of keys that can be used and are valid.
	testingKeys = []string{
		// ecdsa testing public key
		"ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBG00bYFLb/sxPcmVRMg8NXZK/ldefElAkC9wD41vABdHZiSRvp+2y9BMNVYzE/FnzKObHtSvGRX65YQgRn7k5p0= juju@example.com",

		// ed25519 testing public key
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju@example.com",

		// rsa testing public key
		"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDvplNOK3UBpULZKvZf/I5JHci/DufpSxj8yR4yKE2grescJxu6754jPT3xztSeLGD31/oJApJZGkMUAMRenvDqIaq+taRfOUo/l19AlGZc+Edv4bTlJzZ1Lzwex1vvL1doaLb/f76IIUHClGUgIXRceQH1ovHiIWj6nGltuLanG8YTWxlzzK33yhitmZt142DmpX1VUVF5c/Hct6Rav5lKmwej1TDed1KmHzXVoTHEsmWhKsOK27ue5yTuq0GX6LrAYDucF+2MqZCsuddXsPAW1tj5GNZSR7RrKW5q1CI0G7k9gSomuCsRMlCJ3BqID/vUSs/0qOWg4he0HUsYKQSrXIhckuZu+jYP8B80MoXT50ftRidoG/zh/PugBdXTk46FloVClQopG5A2fbqrphADcUUbRUxZ2lWQN+OVHKfEsfV2b8L2aSqZUGlryfW1cirB5JCTDvtv7rUy9/ny9iKA+8tAyKSDF0I901RDDqKc9dSkrHCg2bLnJZDoiRoWczE= juju@example.com",
	}

	// reservedKeys are keys with reserved comments that can not be added or
	// removed via the service.
	reservedKeys = []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju-client-key",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju-system-key",
	}
)

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *serviceSuite) SetUpTest(c *gc.C) {
	s.userID = usertesting.GenUserUUID(c)
}

// TestAddKeysForInvalidUser is asserting that if we pass in an invalid user id
// to [Service.AddKeysForUser] we get back a [errors.NotValid] error.
func (s *serviceSuite) TestAddKeysForInvalidUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state).
		AddKeysForUser(context.Background(), user.UUID("notvalid"), "key")
	c.Check(err, jc.ErrorIs, errors.NotValid)
}

// TestAddKeysForNonExistentUser is testing that if a user id doesn't exist that
// the services layer correctly passes back the [accesserrors.UserNotFound]
// error.
func (s *serviceSuite) TestAddKeysForNonExistentUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAuthorisedKeysForUser(gomock.Any(), s.userID).Return(nil, accesserrors.UserNotFound)
	svc := NewService(s.state)
	err := svc.AddKeysForUser(context.Background(), s.userID, testingKeys[0])
	c.Check(err, jc.ErrorIs, accesserrors.UserNotFound)
}

// TestAddInvalidKeys is testing that if we try and add one or more keys that
// are invalid we get back a [keyserrors.InvalidAuthorisedKey] key error and no
// modificiation to state is performed.
func (s *serviceSuite) TestAddInvalidKeys(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAuthorisedKeysForUser(gomock.Any(), s.userID).
		Return(existingUserKeys, nil).AnyTimes()
	svc := NewService(s.state)

	err := svc.AddKeysForUser(context.Background(), s.userID, "notvalid")
	c.Check(err, jc.ErrorIs, keyserrors.InvalidAuthorisedKey)

	err = svc.AddKeysForUser(context.Background(), s.userID, "notvalid", testingKeys[0])
	c.Check(err, jc.ErrorIs, keyserrors.InvalidAuthorisedKey)

	err = svc.AddKeysForUser(context.Background(), s.userID, testingKeys[0], "notvalid")
	c.Check(err, jc.ErrorIs, keyserrors.InvalidAuthorisedKey)

	err = svc.AddKeysForUser(context.Background(), s.userID, testingKeys[0], "notvalid", testingKeys[1])
	c.Check(err, jc.ErrorIs, keyserrors.InvalidAuthorisedKey)
}

// TestAddReservedKeys is testing that if we try and add authorised keys with
// reserved comments we get back an error that satisfies
// [keyserrors.ReservedCommentViolation]
func (s *serviceSuite) TestAddReservedKeys(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAuthorisedKeysForUser(gomock.Any(), s.userID).
		Return(existingUserKeys, nil).AnyTimes()
	svc := NewService(s.state)

	err := svc.AddKeysForUser(context.Background(), s.userID, reservedKeys...)
	c.Check(err, jc.ErrorIs, keyserrors.ReservedCommentViolation)

	err = svc.AddKeysForUser(context.Background(), s.userID, testingKeys[0], reservedKeys[0])
	c.Check(err, jc.ErrorIs, keyserrors.ReservedCommentViolation)
}

// TestAddExistingKeysForUser is asserting that when we try and add a key for a
// user that already exists we get back a
// [keyserrors.AuthorisedKeyAlreadyExists] error.
func (s *serviceSuite) TestAddExistingKeysForUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAuthorisedKeysForUser(gomock.Any(), s.userID).
		Return(existingUserKeys, nil).AnyTimes()
	svc := NewService(s.state)

	err := svc.AddKeysForUser(context.Background(), s.userID, existingUserKeys[0])
	c.Check(err, jc.ErrorIs, keyserrors.AuthorisedKeyAlreadyExists)

	err = svc.AddKeysForUser(context.Background(), s.userID, testingKeys[1], existingUserKeys[1])
	c.Check(err, jc.ErrorIs, keyserrors.AuthorisedKeyAlreadyExists)
}

// TestAddKeysForUserDedupe is asserting that adding the same keys many times
// the result is de-duplicated.
func (s *serviceSuite) TestAddKeysForUserDedupe(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAuthorisedKeysForUser(gomock.Any(), s.userID).
		Return(existingUserKeys, nil).AnyTimes()
	svc := NewService(s.state)

	s.state.EXPECT().AddAuthorisedKeysForUser(gomock.Any(), s.userID, testingKeys[0])

	err := svc.AddKeysForUser(context.Background(), s.userID, testingKeys[0], testingKeys[0])
	c.Check(err, jc.ErrorIsNil)
}

// TestAddKeysForUser is testing the happy path of [Service.AddKeysForUser]
func (s *serviceSuite) TestAddKeysForUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAuthorisedKeysForUser(gomock.Any(), s.userID).
		Return(existingUserKeys, nil).AnyTimes()
	svc := NewService(s.state)

	s.state.EXPECT().AddAuthorisedKeysForUser(gomock.Any(), s.userID, testingKeys)

	err := svc.AddKeysForUser(context.Background(), s.userID, testingKeys...)
	c.Check(err, jc.ErrorIsNil)
}

// TestListKeysForInvalidUserId is testing that if we pass in a junk non valid
// user id to [Service.ListKeysForUser] we get back a [errors.NotValid] error.
func (s *serviceSuite) TestListKeysForInvalidUserId(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.state)

	_, err := svc.ListKeysForUser(context.Background(), user.UUID("not-valid"))
	c.Check(err, jc.ErrorIs, errors.NotValid)
}

// TestListKeysForNonExistentUser is testing that if we ask for the keys for a
// non existent user we get back an error that satisfies
// [accesserrors.UserNotFound].
func (s *serviceSuite) TestListKeysForNonExistentUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAuthorisedKeysForUser(gomock.Any(), s.userID).
		Return(nil, accesserrors.UserNotFound).AnyTimes()
	svc := NewService(s.state)

	_, err := svc.ListKeysForUser(context.Background(), s.userID)
	c.Check(err, jc.ErrorIs, accesserrors.UserNotFound)
}

// TestListKeysForUser is testing the happy path for [Service.ListKeysForUser].
func (s *serviceSuite) TestListKeysForUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAuthorisedKeysForUser(gomock.Any(), s.userID).
		Return(existingUserKeys, nil)
	svc := NewService(s.state)

	keys, err := svc.ListKeysForUser(context.Background(), s.userID)
	c.Check(err, jc.ErrorIsNil)
	c.Check(keys, gc.DeepEquals, existingUserKeys)
}

// TestDeleteKeysForInvalidUser is asserting that if we pass an invalid user id
// to [Service.DeleteKeysForUser] we get back an [errors.NotValid] error.
func (s *serviceSuite) TestDeleteKeysForInvalidUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state).
		DeleteKeysForUser(context.Background(), user.UUID("notvalid"), "key")
	c.Check(err, jc.ErrorIs, errors.NotValid)
}

// TestDeleteKeysForUserNotFound is asserting that if the state layer
// propogrates a [accesserrors.UserNotFound] that the service layer returns the
// error back up.
func (s *serviceSuite) TestDeleteKeysForUserNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAuthorisedKeysForUser(gomock.Any(), s.userID).
		Return(nil, accesserrors.UserNotFound)

	err := NewService(s.state).
		DeleteKeysForUser(context.Background(), s.userID, testingKeys[0])
	c.Check(err, jc.ErrorIs, accesserrors.UserNotFound)
}

// TestDeleteKeysForUserWithFingerprint is asserting that we can remove keys for
// a user based on the key fingerprint.
func (s *serviceSuite) TestDeleteKeysForUserWithFingerprint(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAuthorisedKeysForUser(gomock.Any(), s.userID).
		Return(existingUserKeys, nil)
	s.state.EXPECT().DeleteAuthorisedKeysForUser(gomock.Any(), s.userID, existingUserKeys[0]).
		Return(nil)

	key, err := ssh.ParseAuthorisedKey(existingUserKeys[0])
	c.Assert(err, jc.ErrorIsNil)

	err = NewService(s.state).
		DeleteKeysForUser(context.Background(), s.userID, key.Fingerprint())
	c.Check(err, jc.ErrorIsNil)
}

// TestDeleteKeysForUserWithComment is asserting that we can remove keys for a
// user based on the key comment.
func (s *serviceSuite) TestDeleteKeysForUserWithComment(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAuthorisedKeysForUser(gomock.Any(), s.userID).
		Return(existingUserKeys, nil)
	s.state.EXPECT().DeleteAuthorisedKeysForUser(gomock.Any(), s.userID, existingUserKeys[0]).
		Return(nil)

	key, err := ssh.ParseAuthorisedKey(existingUserKeys[0])
	c.Assert(err, jc.ErrorIsNil)

	err = NewService(s.state).
		DeleteKeysForUser(context.Background(), s.userID, key.Comment)
	c.Check(err, jc.ErrorIsNil)
}

// TestDeleteKeysForUserData is asserting that we can remove ssh keys for a user
// based on the raw key data.
func (s *serviceSuite) TestDeleteKeysForUserData(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAuthorisedKeysForUser(gomock.Any(), s.userID).
		Return(existingUserKeys, nil)
	s.state.EXPECT().DeleteAuthorisedKeysForUser(gomock.Any(), s.userID, existingUserKeys[0]).
		Return(nil)

	err := NewService(s.state).
		DeleteKeysForUser(context.Background(), s.userID, existingUserKeys[0])
	c.Check(err, jc.ErrorIsNil)
}

// TestDeleteKeysForUserCombination is asserting multiple deletes using
// different targets. We want to see that with a set of different target types
// all the correct keys are removed.
func (s *serviceSuite) TestDeleteKeysForUserCombination(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAuthorisedKeysForUser(gomock.Any(), s.userID).
		Return(existingUserKeys, nil)
	s.state.EXPECT().DeleteAuthorisedKeysForUser(
		gomock.Any(),
		s.userID,
		existingUserKeys[0],
		existingUserKeys[1],
	).Return(nil)

	key, err := ssh.ParseAuthorisedKey(existingUserKeys[0])
	c.Assert(err, jc.ErrorIsNil)

	err = NewService(s.state).
		DeleteKeysForUser(
			context.Background(),
			s.userID,
			key.Comment,
			existingUserKeys[1],
		)
	c.Check(err, jc.ErrorIsNil)
}
