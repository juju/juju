// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"slices"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/keymanager"
	keyerrors "github.com/juju/juju/domain/keymanager/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/ssh"
)

type stateSuite struct {
	schematesting.ModelSuite

	userId user.UUID
}

var _ = gc.Suite(&stateSuite{})

var (
	testingPublicKeys = []string{
		// ecdsa testing public key
		"ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBG00bYFLb/sxPcmVRMg8NXZK/ldefElAkC9wD41vABdHZiSRvp+2y9BMNVYzE/FnzKObHtSvGRX65YQgRn7k5p0= juju1@example.com",

		// ed25519 testing public key
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju2@example.com",

		// rsa testing public key
		"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDvplNOK3UBpULZKvZf/I5JHci/DufpSxj8yR4yKE2grescJxu6754jPT3xztSeLGD31/oJApJZGkMUAMRenvDqIaq+taRfOUo/l19AlGZc+Edv4bTlJzZ1Lzwex1vvL1doaLb/f76IIUHClGUgIXRceQH1ovHiIWj6nGltuLanG8YTWxlzzK33yhitmZt142DmpX1VUVF5c/Hct6Rav5lKmwej1TDed1KmHzXVoTHEsmWhKsOK27ue5yTuq0GX6LrAYDucF+2MqZCsuddXsPAW1tj5GNZSR7RrKW5q1CI0G7k9gSomuCsRMlCJ3BqID/vUSs/0qOWg4he0HUsYKQSrXIhckuZu+jYP8B80MoXT50ftRidoG/zh/PugBdXTk46FloVClQopG5A2fbqrphADcUUbRUxZ2lWQN+OVHKfEsfV2b8L2aSqZUGlryfW1cirB5JCTDvtv7rUy9/ny9iKA+8tAyKSDF0I901RDDqKc9dSkrHCg2bLnJZDoiRoWczE= juju3@example.com",
	}
)

func generatePublicKeys(c *gc.C, publicKeys []string) []keymanager.PublicKey {
	rval := make([]keymanager.PublicKey, 0, len(publicKeys))
	for _, pk := range publicKeys {
		parsedKey, err := ssh.ParsePublicKey(pk)
		c.Assert(err, jc.ErrorIsNil)

		rval = append(rval, keymanager.PublicKey{
			Comment:         parsedKey.Comment,
			FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
			Fingerprint:     parsedKey.Fingerprint(),
			Key:             pk,
		})
	}

	return rval
}

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)
	s.userId = usertesting.GenUserUUID(c)
}

// TestAddPublicKeyForUser is asserting the happy path of adding a public key
// for a user.
func (s *stateSuite) TestAddPublicKeyForUser(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeysForUser(context.Background(), s.userId, keysToAdd)
	c.Check(err, jc.ErrorIsNil)

	keys, err := state.GetPublicKeysForUser(context.Background(), s.userId)
	c.Assert(err, jc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(testingPublicKeys, jc.DeepEquals, keys)
}

// TestAddPublicKeyForUserIfNotFound is asserting the happy path for adding a
// public key for a user.
func (s *stateSuite) TestAddPublicKeyForUserIfNotFound(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeyForUserIfNotFound(context.Background(), s.userId, keysToAdd)
	c.Check(err, jc.ErrorIsNil)

	keys, err := state.GetPublicKeysForUser(context.Background(), s.userId)
	c.Assert(err, jc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(testingPublicKeys, jc.DeepEquals, keys)
}

// TestAddExistingPublicKey is asserting that if we try and add a public key
// that already exists for the same user we get back a error that satisfies.
func (s *stateSuite) TestAddExistingPublicKey(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeysForUser(context.Background(), s.userId, keysToAdd)
	c.Check(err, jc.ErrorIsNil)

	err = state.AddPublicKeysForUser(context.Background(), s.userId, keysToAdd[:1])
	c.Check(err, jc.ErrorIs, keyerrors.PublicKeyAlreadyExists)

	keys, err := state.GetPublicKeysForUser(context.Background(), s.userId)
	c.Assert(err, jc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(testingPublicKeys, jc.DeepEquals, keys)
}

// TestAddExistingPublicKeyIfNotFound is asserting that we call
// [State.AddPublicKeyForUserIfNotFound] with an already existing public key
// that no operation takes place and no error is returned.
func (s *stateSuite) TestAddExistingPublicKeyIfNotFound(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeysForUser(context.Background(), s.userId, keysToAdd)
	c.Check(err, jc.ErrorIsNil)

	err = state.AddPublicKeyForUserIfNotFound(context.Background(), s.userId, keysToAdd[:1])
	c.Check(err, jc.ErrorIsNil)

	keys, err := state.GetPublicKeysForUser(context.Background(), s.userId)
	c.Assert(err, jc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(testingPublicKeys, jc.DeepEquals, keys)
}

// TestAddExistingPublicKeyDifferentUser is asserting that different users can
// have the same key registered with no resultant errors.
func (s *stateSuite) TestAddExistingPublicKeyDifferentUser(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeysForUser(context.Background(), s.userId, keysToAdd)
	c.Check(err, jc.ErrorIsNil)

	altUser := usertesting.GenUserUUID(c)
	err = state.AddPublicKeysForUser(context.Background(), altUser, keysToAdd[:1])
	c.Check(err, jc.ErrorIsNil)
}

// TestAddExistingKeySameFingerprint is a special test to assert the DDL for
// user public keys. Theorectically it is possible to store the same key for a
// user twice if the key data has a 1 byte difference. For example adding an
// extra space or somment character. With this test we want to make sure that
// even with different key data but the same fingerprint we get back a
// [keyerrors.PublicKeyAlreadyExists] error.
//
// This is a very contrived situation but it is worth checking.
func (s *stateSuite) TestAddExistingKeySameFingerprint(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())
	publicKeys := []string{
		testingPublicKeys[0],
		testingPublicKeys[0],
	}
	keysToAdd := generatePublicKeys(c, publicKeys)
	keysToAdd[1].Key = "different key data but same fingerprint"

	err := state.AddPublicKeysForUser(context.Background(), s.userId, keysToAdd)
	c.Check(err, jc.ErrorIs, keyerrors.PublicKeyAlreadyExists)
}

// TestAddExistingKeyDifferentFingerprint is another test to make sure that we
// can not add the same public key again for a user where small change could be
// observed. In this test we are changing the fingerprints of two identical keys
// to make sure that the unqiue index throws an error based on the key data it
// self being the same data.
//
// This is a very contrived situation but it is worth checking.
func (s *stateSuite) TestAddExistingKeyDifferentFingerprint(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())
	publicKeys := []string{
		testingPublicKeys[0],
		testingPublicKeys[0],
	}
	keysToAdd := generatePublicKeys(c, publicKeys)
	keysToAdd[1].Fingerprint = "different fingerprint"

	err := state.AddPublicKeysForUser(context.Background(), s.userId, keysToAdd)
	c.Check(err, jc.ErrorIs, keyerrors.PublicKeyAlreadyExists)
}

// TestGetPublicKeysForUserNoExist is asserting that if we ask for public keys
// for a user that doesn't exist no error is returned.
func (s *stateSuite) TestGetPublicKeysForUserNoExist(c *gc.C) {
	userId := usertesting.GenUserUUID(c)
	state := NewState(s.TxnRunnerFactory())
	keys, err := state.GetPublicKeysForUser(context.Background(), userId)
	c.Check(err, jc.ErrorIsNil)
	c.Check(len(keys), gc.Equals, 0)
}

// TestDeletePublicKeysForNonExistentUser is asserting that if we try and
// delete public keys for a user that doesn't exist we get back no error.
func (s *stateSuite) TestDeletePublicKeysForNonExistentUser(c *gc.C) {
	userId := usertesting.GenUserUUID(c)
	state := NewState(s.TxnRunnerFactory())
	err := state.DeletePublicKeysForUser(context.Background(), userId, []string{"comment"})
	c.Check(err, jc.ErrorIsNil)
}

// TestDeletePublicKeysForComment is testing that we can remove a users public
// keys via the comment string.
func (s *stateSuite) TestDeletePublicKeysForComment(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeysForUser(context.Background(), s.userId, keysToAdd)
	c.Check(err, jc.ErrorIsNil)

	err = state.DeletePublicKeysForUser(context.Background(), s.userId, []string{
		keysToAdd[0].Comment,
	})
	c.Assert(err, jc.ErrorIsNil)

	keys, err := state.GetPublicKeysForUser(context.Background(), s.userId)
	c.Assert(err, jc.ErrorIsNil)
	fmt.Println(keys)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(testingPublicKeys[1:], jc.DeepEquals, keys)
}

// TestDeletePublicKeysForComment is testing that we can remove a users public
// keys via the fingerprint.
func (s *stateSuite) TestDeletePublicKeysForFingerprint(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeysForUser(context.Background(), s.userId, keysToAdd)
	c.Check(err, jc.ErrorIsNil)

	err = state.DeletePublicKeysForUser(context.Background(), s.userId, []string{
		keysToAdd[0].Fingerprint,
	})
	c.Assert(err, jc.ErrorIsNil)

	keys, err := state.GetPublicKeysForUser(context.Background(), s.userId)
	c.Assert(err, jc.ErrorIsNil)
	fmt.Println(keys)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(testingPublicKeys[1:], jc.DeepEquals, keys)
}

// TestDeletePublicKeysForComment is testing that we can remove a users public
// keys via the keys data.
func (s *stateSuite) TestDeletePublicKeysForKeyData(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeysForUser(context.Background(), s.userId, keysToAdd)
	c.Check(err, jc.ErrorIsNil)

	err = state.DeletePublicKeysForUser(context.Background(), s.userId, []string{
		keysToAdd[0].Key,
	})
	c.Assert(err, jc.ErrorIsNil)

	keys, err := state.GetPublicKeysForUser(context.Background(), s.userId)
	c.Assert(err, jc.ErrorIsNil)
	fmt.Println(keys)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(testingPublicKeys[1:], jc.DeepEquals, keys)
}

// TestDeletePublicKeysForCombination is asserting that we can remove a users
// public keys via a combination of fingerprint and comment.
func (s *stateSuite) TestDeletePublicKeysForCombination(c *gc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeysForUser(context.Background(), s.userId, keysToAdd)
	c.Check(err, jc.ErrorIsNil)

	err = state.DeletePublicKeysForUser(context.Background(), s.userId, []string{
		keysToAdd[0].Comment,
		keysToAdd[1].Fingerprint,
	})
	c.Assert(err, jc.ErrorIsNil)

	keys, err := state.GetPublicKeysForUser(context.Background(), s.userId)
	c.Assert(err, jc.ErrorIsNil)
	fmt.Println(keys)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(testingPublicKeys[2:], jc.DeepEquals, keys)
}
