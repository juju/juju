// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"net/url"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/keymanager"
	keyserrors "github.com/juju/juju/domain/keymanager/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/ssh"
	importererrors "github.com/juju/juju/internal/ssh/importer/errors"
)

type serviceSuite struct {
	testing.IsolationSuite

	keyImporter *MockPublicKeyImporter
	state       *MockState
	userID      user.UUID
	subjectURI  *url.URL
	modelUUID   model.UUID
}

var (
	_ = gc.Suite(&serviceSuite{})

	existingUserPublicKeys = []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
	}

	// testingPublicKeys represents a set of keys that can be used and are valid.
	testingPublicKeys = []string{
		// ecdsa testing public key
		"ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBG00bYFLb/sxPcmVRMg8NXZK/ldefElAkC9wD41vABdHZiSRvp+2y9BMNVYzE/FnzKObHtSvGRX65YQgRn7k5p0= juju@example.com",

		// ed25519 testing public key
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju@example.com",

		// rsa testing public key
		"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDvplNOK3UBpULZKvZf/I5JHci/DufpSxj8yR4yKE2grescJxu6754jPT3xztSeLGD31/oJApJZGkMUAMRenvDqIaq+taRfOUo/l19AlGZc+Edv4bTlJzZ1Lzwex1vvL1doaLb/f76IIUHClGUgIXRceQH1ovHiIWj6nGltuLanG8YTWxlzzK33yhitmZt142DmpX1VUVF5c/Hct6Rav5lKmwej1TDed1KmHzXVoTHEsmWhKsOK27ue5yTuq0GX6LrAYDucF+2MqZCsuddXsPAW1tj5GNZSR7RrKW5q1CI0G7k9gSomuCsRMlCJ3BqID/vUSs/0qOWg4he0HUsYKQSrXIhckuZu+jYP8B80MoXT50ftRidoG/zh/PugBdXTk46FloVClQopG5A2fbqrphADcUUbRUxZ2lWQN+OVHKfEsfV2b8L2aSqZUGlryfW1cirB5JCTDvtv7rUy9/ny9iKA+8tAyKSDF0I901RDDqKc9dSkrHCg2bLnJZDoiRoWczE= juju@example.com",
	}

	// reservedPublicKeys are keys with reserved comments that can not be added
	// or removed via the service.
	reservedPublicKeys = []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju-system-key",
	}
)

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.keyImporter = NewMockPublicKeyImporter(ctrl)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *serviceSuite) SetUpTest(c *gc.C) {
	s.userID = usertesting.GenUserUUID(c)

	uri, err := url.Parse("gh:tlm")
	c.Check(err, jc.ErrorIsNil)
	s.subjectURI = uri
	s.modelUUID = modeltesting.GenModelUUID(c)
}

// TestAddKeysForInvalidUser is asserting that if we pass in an invalid user id
// to [Service.AddKeysForUser] we get back a [errors.NotValid] error.
func (s *serviceSuite) TestAddKeysForInvalidUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.modelUUID, s.state).
		AddPublicKeysForUser(context.Background(), user.UUID("notvalid"), "key")
	c.Check(err, jc.ErrorIs, coreerrors.NotValid)
}

// TestAddKeysForNonExistentUser is testing that if a user id doesn't exist that
// the services layer correctly passes back the [accesserrors.UserNotFound]
// error.
func (s *serviceSuite) TestAddKeysForNonExistentUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	keyInfo, err := ssh.ParsePublicKey(testingPublicKeys[0])
	c.Assert(err, jc.ErrorIsNil)
	expectedKeys := []keymanager.PublicKey{
		{
			Comment:         keyInfo.Comment,
			FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
			Fingerprint:     keyInfo.Fingerprint(),
			Key:             testingPublicKeys[0],
		},
	}

	s.state.EXPECT().AddPublicKeysForUser(
		gomock.Any(), s.modelUUID, s.userID, expectedKeys,
	).Return(accesserrors.UserNotFound)

	svc := NewService(s.modelUUID, s.state)
	err = svc.AddPublicKeysForUser(context.Background(), s.userID, testingPublicKeys[0])
	c.Check(err, jc.ErrorIs, accesserrors.UserNotFound)
}

// TestAddKeysForNonExistentModel is testing that if we add keys for a model
// that doesn't exist we get back a [modelerrors.NotFound] error.
func (s *serviceSuite) TestAddKeysForNonExistentModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	badModelId := modeltesting.GenModelUUID(c)

	keyInfo, err := ssh.ParsePublicKey(testingPublicKeys[0])
	c.Assert(err, jc.ErrorIsNil)
	expectedKeys := []keymanager.PublicKey{
		{
			Comment:         keyInfo.Comment,
			FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
			Fingerprint:     keyInfo.Fingerprint(),
			Key:             testingPublicKeys[0],
		},
	}

	s.state.EXPECT().AddPublicKeysForUser(
		gomock.Any(), badModelId, s.userID, expectedKeys,
	).Return(modelerrors.NotFound)

	svc := NewService(badModelId, s.state)
	err = svc.AddPublicKeysForUser(context.Background(), s.userID, testingPublicKeys[0])
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestAddInvalidPublicKeys is testing that if we try and add one or more keys
// that are invalid we get back a [keyserrors.InvalidPublicKey] key error
// and no modificiation to state is performed.
func (s *serviceSuite) TestAddInvalidPublicKeys(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.modelUUID, s.state)

	err := svc.AddPublicKeysForUser(context.Background(), s.userID, "notvalid")
	c.Check(err, jc.ErrorIs, keyserrors.InvalidPublicKey)

	err = svc.AddPublicKeysForUser(context.Background(), s.userID, "notvalid", testingPublicKeys[0])
	c.Check(err, jc.ErrorIs, keyserrors.InvalidPublicKey)

	err = svc.AddPublicKeysForUser(context.Background(), s.userID, testingPublicKeys[0], "notvalid")
	c.Check(err, jc.ErrorIs, keyserrors.InvalidPublicKey)

	err = svc.AddPublicKeysForUser(
		context.Background(),
		s.userID,
		testingPublicKeys[0],
		"notvalid",
		testingPublicKeys[1],
	)
	c.Check(err, jc.ErrorIs, keyserrors.InvalidPublicKey)
}

// TestAddReservedKeys is testing that if we try and add public keys with
// reserved comments we get back an error that satisfies
// [keyserrors.ReservedCommentViolation]
func (s *serviceSuite) TestAddReservedPublicKeys(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.modelUUID, s.state)

	err := svc.AddPublicKeysForUser(context.Background(), s.userID, reservedPublicKeys...)
	c.Check(err, jc.ErrorIs, keyserrors.ReservedCommentViolation)

	err = svc.AddPublicKeysForUser(
		context.Background(),
		s.userID,
		testingPublicKeys[0],
		reservedPublicKeys[0],
	)
	c.Check(err, jc.ErrorIs, keyserrors.ReservedCommentViolation)
}

// TestAddExistingKeysForUser is asserting that when we try and add a key for a
// user that already exists we get back a
// [keyserrors.PublicKeyAlreadyExists] error.
func (s *serviceSuite) TestAddExistingKeysForUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.modelUUID, s.state)

	keyInfo1, err := ssh.ParsePublicKey(existingUserPublicKeys[1])
	c.Assert(err, jc.ErrorIsNil)
	keyInfo2, err := ssh.ParsePublicKey(testingPublicKeys[1])
	c.Assert(err, jc.ErrorIsNil)
	expectedKeys := []keymanager.PublicKey{
		{
			Comment:         keyInfo1.Comment,
			FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
			Fingerprint:     keyInfo1.Fingerprint(),
			Key:             existingUserPublicKeys[1],
		},
	}

	s.state.EXPECT().AddPublicKeysForUser(
		gomock.Any(), s.modelUUID, s.userID, expectedKeys,
	).Return(keyserrors.PublicKeyAlreadyExists)

	err = svc.AddPublicKeysForUser(
		context.Background(),
		s.userID,
		existingUserPublicKeys[1],
	)
	c.Check(err, jc.ErrorIs, keyserrors.PublicKeyAlreadyExists)

	expectedKeys = []keymanager.PublicKey{
		{
			Comment:         keyInfo2.Comment,
			FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
			Fingerprint:     keyInfo2.Fingerprint(),
			Key:             testingPublicKeys[1],
		},
		{
			Comment:         keyInfo1.Comment,
			FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
			Fingerprint:     keyInfo1.Fingerprint(),
			Key:             existingUserPublicKeys[1],
		},
	}

	s.state.EXPECT().AddPublicKeysForUser(
		gomock.Any(),
		s.modelUUID,
		s.userID,
		expectedKeys,
	).Return(keyserrors.PublicKeyAlreadyExists)

	err = svc.AddPublicKeysForUser(
		context.Background(),
		s.userID,
		testingPublicKeys[1],
		existingUserPublicKeys[1],
	)
	c.Check(err, jc.ErrorIs, keyserrors.PublicKeyAlreadyExists)
}

// TestAddKeysForUser is testing the happy path of [Service.AddKeysForUser]
func (s *serviceSuite) TestAddKeysForUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	expectedKeys := make([]keymanager.PublicKey, 0, len(testingPublicKeys))
	for _, key := range testingPublicKeys {
		keyInfo, err := ssh.ParsePublicKey(key)
		c.Assert(err, jc.ErrorIsNil)
		expectedKeys = append(expectedKeys, keymanager.PublicKey{
			Comment:         keyInfo.Comment,
			FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
			Fingerprint:     keyInfo.Fingerprint(),
			Key:             key,
		})
	}

	s.state.EXPECT().AddPublicKeysForUser(
		gomock.Any(),
		s.modelUUID,
		s.userID,
		expectedKeys,
	).Return(nil)

	svc := NewService(s.modelUUID, s.state)

	err := svc.AddPublicKeysForUser(context.Background(), s.userID, testingPublicKeys...)
	c.Check(err, jc.ErrorIsNil)
}

// TestListKeysForInvalidUserId is testing that if we pass in a junk non valid
// user id to [Service.ListKeysForUser] we get back a [errors.NotValid] error.
func (s *serviceSuite) TestListKeysForInvalidUserId(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.modelUUID, s.state)

	_, err := svc.ListPublicKeysForUser(context.Background(), user.UUID("not-valid"))
	c.Check(err, jc.ErrorIs, coreerrors.NotValid)
}

// TestListKeysForNonExistentUser is testing that if we ask for the keys for a
// non existent user we get back an error that satisfies
// [accesserrors.UserNotFound].
func (s *serviceSuite) TestListKeysForNonExistentUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetPublicKeysForUser(gomock.Any(), s.modelUUID, s.userID).
		Return(nil, accesserrors.UserNotFound).AnyTimes()
	svc := NewService(s.modelUUID, s.state)

	_, err := svc.ListPublicKeysForUser(context.Background(), s.userID)
	c.Check(err, jc.ErrorIs, accesserrors.UserNotFound)
}

// TestListKeysForUser is testing the happy path for
// [Service.ListPublicKeysForUser].
func (s *serviceSuite) TestListKeysForUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	publicKeys := make([]coressh.PublicKey, 0, len(existingUserPublicKeys))
	for _, existingKey := range existingUserPublicKeys {
		publicKeys = append(publicKeys, coressh.PublicKey{
			Fingerprint: "fingerprint",
			Key:         existingKey,
		})
	}

	s.state.EXPECT().GetPublicKeysForUser(gomock.Any(), s.modelUUID, s.userID).
		Return(publicKeys, nil)
	svc := NewService(s.modelUUID, s.state)

	keys, err := svc.ListPublicKeysForUser(context.Background(), s.userID)
	c.Check(err, jc.ErrorIsNil)
	c.Check(keys, jc.DeepEquals, publicKeys)
}

// TestDeleteKeysForInvalidUser is asserting that if we pass an invalid user id
// to [Service.DeleteKeysForUser] we get back an [errors.NotValid] error.
func (s *serviceSuite) TestDeleteKeysForInvalidUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.modelUUID, s.state).
		DeleteKeysForUser(context.Background(), user.UUID("notvalid"), "key")
	c.Check(err, jc.ErrorIs, coreerrors.NotValid)
}

// TestDeleteKeysForUserNotFound is asserting that if the state layer
// propogrates a [accesserrors.UserNotFound] that the service layer returns the
// error back up.
func (s *serviceSuite) TestDeleteKeysForUserNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeletePublicKeysForUser(
		gomock.Any(),
		s.modelUUID,
		s.userID,
		[]string{testingPublicKeys[0]},
	).Return(accesserrors.UserNotFound)

	err := NewService(s.modelUUID, s.state).
		DeleteKeysForUser(context.Background(), s.userID, testingPublicKeys[0])
	c.Check(err, jc.ErrorIs, accesserrors.UserNotFound)
}

// TestDeleteKeysForUserWithFingerprint is asserting that we can remove keys for
// a user based on the key fingerprint.
func (s *serviceSuite) TestDeleteKeysForUserWithFingerprint(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key, err := ssh.ParsePublicKey(existingUserPublicKeys[0])
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().DeletePublicKeysForUser(
		gomock.Any(), s.modelUUID, s.userID, []string{key.Fingerprint()},
	).Return(nil)

	err = NewService(s.modelUUID, s.state).
		DeleteKeysForUser(context.Background(), s.userID, key.Fingerprint())
	c.Check(err, jc.ErrorIsNil)
}

// TestDeleteKeysForUserWithComment is asserting that we can remove keys for a
// user based on the key comment.
func (s *serviceSuite) TestDeleteKeysForUserWithComment(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key, err := ssh.ParsePublicKey(existingUserPublicKeys[0])
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().DeletePublicKeysForUser(
		gomock.Any(),
		s.modelUUID,
		s.userID,
		[]string{key.Comment},
	).Return(nil)

	err = NewService(s.modelUUID, s.state).
		DeleteKeysForUser(context.Background(), s.userID, key.Comment)
	c.Check(err, jc.ErrorIsNil)
}

// TestDeleteKeysForUserData is asserting that we can remove ssh keys for a user
// based on the raw key data.
func (s *serviceSuite) TestDeleteKeysForUserData(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeletePublicKeysForUser(
		gomock.Any(),
		s.modelUUID,
		s.userID,
		[]string{existingUserPublicKeys[0]},
	).Return(nil)

	err := NewService(s.modelUUID, s.state).
		DeleteKeysForUser(context.Background(), s.userID, existingUserPublicKeys[0])
	c.Check(err, jc.ErrorIsNil)
}

// TestDeleteKeysForUserCombination is asserting multiple deletes using
// different targets. We want to see that with a set of different target types
// all the correct keys are removed.
func (s *serviceSuite) TestDeleteKeysForUserCombination(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key, err := ssh.ParsePublicKey(existingUserPublicKeys[0])
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().DeletePublicKeysForUser(
		gomock.Any(),
		s.modelUUID,
		s.userID,
		[]string{key.Comment, existingUserPublicKeys[1]},
	).Return(nil)

	err = NewService(s.modelUUID, s.state).
		DeleteKeysForUser(
			context.Background(),
			s.userID,
			key.Comment,
			existingUserPublicKeys[1],
		)
	c.Check(err, jc.ErrorIsNil)
}

// TestImportKeyForUnknownSource is asserting that if we try and import keys for
// a subject where the source is unknown.
func (s *serviceSuite) TestImportKeysForUnknownSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.keyImporter.EXPECT().FetchPublicKeysForSubject(
		gomock.Any(),
		s.subjectURI,
	).Return(nil, importererrors.NoResolver)

	err := NewImporterService(s.modelUUID, s.keyImporter, s.state).
		ImportPublicKeysForUser(context.Background(), s.userID, s.subjectURI)
	c.Check(err, jc.ErrorIs, keyserrors.UnknownImportSource)
}

// TestImportKeyForUnknownSubject is asserting that if we ask to import keys for
// a subject that doesn't exist we get back a [keyserrors.ImportSubjectNotFound]
// error.
func (s *serviceSuite) TestImportKeysForUnknownSubject(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.keyImporter.EXPECT().FetchPublicKeysForSubject(
		gomock.Any(),
		s.subjectURI,
	).Return(nil, importererrors.SubjectNotFound)

	err := NewImporterService(s.modelUUID, s.keyImporter, s.state).
		ImportPublicKeysForUser(context.Background(), s.userID, s.subjectURI)
	c.Check(err, jc.ErrorIs, keyserrors.ImportSubjectNotFound)
}

// TestImportKeysInvalidPublicKeys is asserting that if the key importer returns
// invalid public keys a [keyserrors.InvalidPublicKey] error is returned.
func (s *serviceSuite) TestImportKeysInvalidPublicKeys(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.keyImporter.EXPECT().FetchPublicKeysForSubject(
		gomock.Any(),
		s.subjectURI,
	).Return([]string{"bad"}, nil)

	err := NewImporterService(s.modelUUID, s.keyImporter, s.state).
		ImportPublicKeysForUser(context.Background(), s.userID, s.subjectURI)
	c.Check(err, jc.ErrorIs, keyserrors.InvalidPublicKey)
}

// TestImportKeysWithReservedComment is asserting that if we import keys where
// one or more of the keys has a reserved comment we return an error that
// satisfies [keyserrors.ReservedCommentViolation].
func (s *serviceSuite) TestImportKeysWithReservedComment(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.keyImporter.EXPECT().FetchPublicKeysForSubject(
		gomock.Any(),
		s.subjectURI,
	).Return(reservedPublicKeys, nil)

	err := NewImporterService(s.modelUUID, s.keyImporter, s.state).
		ImportPublicKeysForUser(context.Background(), s.userID, s.subjectURI)
	c.Check(err, jc.ErrorIs, keyserrors.ReservedCommentViolation)
}

// TestImportPublicKeysForUser is asserting the happy path of
// [Service.ImportPublicKeysForUser].
func (s *serviceSuite) TestImportPublicKeysForUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.keyImporter.EXPECT().FetchPublicKeysForSubject(
		gomock.Any(),
		s.subjectURI,
	).Return(testingPublicKeys, nil)

	expectedKeys := make([]keymanager.PublicKey, 0, len(testingPublicKeys))
	for _, key := range testingPublicKeys {
		keyInfo, err := ssh.ParsePublicKey(key)
		c.Assert(err, jc.ErrorIsNil)
		expectedKeys = append(expectedKeys, keymanager.PublicKey{
			Comment:         keyInfo.Comment,
			FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
			Fingerprint:     keyInfo.Fingerprint(),
			Key:             key,
		})
	}

	s.state.EXPECT().EnsurePublicKeysForUser(
		gomock.Any(),
		s.modelUUID,
		s.userID,
		expectedKeys,
	).Return(nil)

	err := NewImporterService(s.modelUUID, s.keyImporter, s.state).
		ImportPublicKeysForUser(context.Background(), s.userID, s.subjectURI)
	c.Check(err, jc.ErrorIsNil)
}

// TestGetAllUsersPublicKeys is responsible for assuring that the happy path of
// getting all users and their public keys for a model returns the correct
// result.
func (s *serviceSuite) TestGetAllUsersPublicKeys(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllUsersPublicKeys(gomock.Any(), s.modelUUID).Return(
		map[user.Name][]string{
			usertesting.GenNewName(c, "tlm"): {
				"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
				"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
			},
			usertesting.GenNewName(c, "wallyworld"): {
				"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
				"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
			},
		},
		nil,
	)

	allKeys, err := NewService(s.modelUUID, s.state).GetAllUsersPublicKeys(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(allKeys, jc.DeepEquals, map[user.Name][]string{
		usertesting.GenNewName(c, "tlm"): {
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
		},
		usertesting.GenNewName(c, "wallyworld"): {
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
		},
	})
}

// TestGetAllUsersPublicKeysEmpty is responsible for testing that when a model
// has no public keys for any user in the system [Service.GetAllUsersPublicKeys]
// returns an empty map and no errors.
func (s *serviceSuite) TestGetAllUsersPublicKeysEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllUsersPublicKeys(gomock.Any(), s.modelUUID).Return(
		map[user.Name][]string{}, nil,
	)

	allKeys, err := NewService(s.modelUUID, s.state).GetAllUsersPublicKeys(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(len(allKeys), gc.Equals, 0)
}
