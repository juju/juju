// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	accesserrors "github.com/juju/juju/domain/access/errors"
	accessstate "github.com/juju/juju/domain/access/state"
	"github.com/juju/juju/domain/keymanager"
	keyerrors "github.com/juju/juju/domain/keymanager/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	statemodeltesting "github.com/juju/juju/domain/model/state/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/ssh"
)

type stateSuite struct {
	schematesting.ControllerSuite

	userId   user.UUID
	userName user.Name
	modelId  model.UUID
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

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

func generatePublicKeys(c *tc.C, publicKeys []string) []keymanager.PublicKey {
	rval := make([]keymanager.PublicKey, 0, len(publicKeys))
	for _, pk := range publicKeys {
		parsedKey, err := ssh.ParsePublicKey(pk)
		c.Assert(err, tc.ErrorIsNil)

		rval = append(rval, keymanager.PublicKey{
			Comment:         parsedKey.Comment,
			FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
			Fingerprint:     parsedKey.Fingerprint(),
			Key:             pk,
		})
	}

	return rval
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.SeedControllerUUID(c)

	s.modelId = statemodeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "keys")

	var userUUID user.UUID
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context,
		tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM user where name = ?", "test-userkeys").Scan(&userUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(userUUID, tc.NotNil)
	s.userId = userUUID
	s.userName = usertesting.GenNewName(c, "test-userkeys")
}

// TestAddPublicKeyForUser is asserting the happy path of adding a public key
// for a user. Specifically we want to see that inserting the same key across
// multiple models doesn't result in constraint violations for the users public
// ssh keys.
func (s *stateSuite) TestAddPublicKeyForUser(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeysForUser(c.Context(), s.modelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	keys, err := state.GetPublicKeysDataForUser(c.Context(), s.modelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(keys, tc.DeepEquals, testingPublicKeys)

	// Create a second model to add keys onto
	modelId := statemodeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "second-model")

	// Confirm that the users public ssh keys don't show up on the second model
	// yet
	keys, err = state.GetPublicKeysDataForUser(c.Context(), modelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(keys), tc.Equals, 0)

	// Add the users keys onto the second model. We want to see here that this
	// is a successful operation with no errors.
	err = state.AddPublicKeysForUser(c.Context(), modelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	// Confirm the keys exists on the second model
	keys, err = state.GetPublicKeysDataForUser(c.Context(), modelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(keys, tc.DeepEquals, testingPublicKeys)
}

// TestAddPublicKeysForUserAlreadyExists is asserting that if we try and add the
// same public key for a user more then once to a model we get back an error
// that satisfies [keyerrors.PublicKeyAlreadyExists].
func (s *stateSuite) TestAddPublicKeyForUserAlreadyExists(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeysForUser(c.Context(), s.modelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	keys, err := state.GetPublicKeysDataForUser(c.Context(), s.modelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(keys, tc.DeepEquals, testingPublicKeys)

	// Add the users keys onto the second model. We want to see here that this
	// is a successful operation with no errors.
	err = state.AddPublicKeysForUser(c.Context(), s.modelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIs, keyerrors.PublicKeyAlreadyExists)

	// Confirm the key still exists on the model
	keys, err = state.GetPublicKeysDataForUser(c.Context(), s.modelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(keys, tc.DeepEquals, testingPublicKeys)
}

// TestAddPublicKeyForUserNotFound is asserting that if we attempt to add a
// public key to a model for a user that doesn't exist we get back a
// [accesserrors.UserNotFound] error.
func (s *stateSuite) TestAddPublicKeyForUserNotFound(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	badUserId := usertesting.GenUserUUID(c)

	err := state.AddPublicKeysForUser(c.Context(), s.modelId, badUserId, keysToAdd)
	c.Check(err, tc.ErrorIs, accesserrors.UserNotFound)
}

// TestAddPublicKeyForUserOnNotFoundModel is asserting that if we attempt to add
// a public key for a user on a model that does not exist we get back a
// [modelerrors.NotFound] error.
func (s *stateSuite) TestAddPublicKeyForUserOnNotFoundModel(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	badModelId := tc.Must0(c, model.NewUUID)

	err := state.AddPublicKeysForUser(c.Context(), badModelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestEnsurePublicKeysForUser is asserting the happy path of
// [State.EnsurePublicKeysForUser].
func (s *stateSuite) TestEnsurePublicKeysForUser(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.EnsurePublicKeysForUser(c.Context(), s.modelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	keys, err := state.GetPublicKeysDataForUser(c.Context(), s.modelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(keys, tc.DeepEquals, testingPublicKeys)

	// Run all of the operations again and confirm that there exists no errors.
	err = state.EnsurePublicKeysForUser(c.Context(), s.modelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	keys, err = state.GetPublicKeysDataForUser(c.Context(), s.modelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(keys, tc.DeepEquals, testingPublicKeys)
}

// TestEnsurePublicKeysForUser is asserting the ensure user after keys have
// been stripped of the comments. This should ensure that we're checking against
// the fingerprint and not the public key.
// [State.EnsurePublicKeysForUser].
func (s *stateSuite) TestEnsurePublicKeysForUserForStrippedComments(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.EnsurePublicKeysForUser(c.Context(), s.modelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	keys, err := state.GetPublicKeysDataForUser(c.Context(), s.modelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(keys, tc.DeepEquals, testingPublicKeys)

	// Run all of the operations again and confirm that there exists no errors.

	stripped := make([]keymanager.PublicKey, len(keysToAdd))
	for i, key := range keysToAdd {

		newKey := key.Key
		if parts := strings.Split(key.Key, " "); len(parts) > 2 {
			newKey = fmt.Sprintf("%s %s", parts[0], parts[1])
		}

		stripped[i] = keymanager.PublicKey{
			Comment:         key.Comment,
			FingerprintHash: key.FingerprintHash,
			Fingerprint:     key.Fingerprint,
			Key:             newKey,
		}
	}

	err = state.EnsurePublicKeysForUser(c.Context(), s.modelId, s.userId, stripped)
	c.Check(err, tc.ErrorIsNil)

	keys, err = state.GetPublicKeysDataForUser(c.Context(), s.modelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(keys, tc.DeepEquals, testingPublicKeys)
}

// TestEnsurePublicKeyForUserNotFound is asserting that if we attempt to add a
// public key to a model for a user that doesn't exist we get back a
// [accesserrors.UserNotFound] error.
func (s *stateSuite) TestEnsurePublicKeyForUserNotFound(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	badUserId := usertesting.GenUserUUID(c)

	err := state.EnsurePublicKeysForUser(c.Context(), s.modelId, badUserId, keysToAdd)
	c.Check(err, tc.ErrorIs, accesserrors.UserNotFound)
}

// TestEnsurePublicKeyForUserOnNotFoundModel is asserting that if we attempt to
// add a public key for a user on a model that does not exist we get back a
// [modelerrors.NotFound] error.
func (s *stateSuite) TestEnsurePublicKeyForUserOnNotFoundModel(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	badModelId := tc.Must0(c, model.NewUUID)

	err := state.EnsurePublicKeysForUser(c.Context(), badModelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestDeletePublicKeysForNonExistentUser is asserting that if we try and
// delete public keys for a user that doesn't exist we get an
// [accesserrors.UserNotFound] error
func (s *stateSuite) TestDeletePublicKeysForNonExistentUser(c *tc.C) {
	userId := usertesting.GenUserUUID(c)
	state := NewState(s.TxnRunnerFactory())
	err := state.DeletePublicKeysForUser(c.Context(), s.modelId, userId, []string{"comment"})
	c.Check(err, tc.ErrorIs, accesserrors.UserNotFound)
}

// TestDeletePublicKeysForComment is testing that we can remove a users public
// keys via the comment string.
func (s *stateSuite) TestDeletePublicKeysForComment(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeysForUser(c.Context(), s.modelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	err = state.DeletePublicKeysForUser(c.Context(), s.modelId, s.userId, []string{
		keysToAdd[0].Comment,
	})
	c.Assert(err, tc.ErrorIsNil)

	keys, err := state.GetPublicKeysDataForUser(c.Context(), s.modelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(testingPublicKeys[1:], tc.DeepEquals, keys)
}

// TestDeletePublicKeysForComment is testing that we can remove a users public
// keys via the fingerprint.
func (s *stateSuite) TestDeletePublicKeysForFingerprint(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeysForUser(c.Context(), s.modelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	err = state.DeletePublicKeysForUser(c.Context(), s.modelId, s.userId, []string{
		keysToAdd[0].Fingerprint,
	})
	c.Assert(err, tc.ErrorIsNil)

	keys, err := state.GetPublicKeysDataForUser(c.Context(), s.modelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(testingPublicKeys[1:], tc.DeepEquals, keys)
}

// TestDeletePublicKeysForComment is testing that we can remove a users public
// keys via the keys data.
func (s *stateSuite) TestDeletePublicKeysForKeyData(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeysForUser(c.Context(), s.modelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	err = state.DeletePublicKeysForUser(c.Context(), s.modelId, s.userId, []string{
		keysToAdd[0].Key,
	})
	c.Assert(err, tc.ErrorIsNil)

	keys, err := state.GetPublicKeysDataForUser(c.Context(), s.modelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(testingPublicKeys[1:], tc.DeepEquals, keys)
}

// TestDeletePublicKeysForCombination is asserting that we can remove a users
// public keys via a combination of fingerprint and comment.
func (s *stateSuite) TestDeletePublicKeysForCombination(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeysForUser(c.Context(), s.modelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	err = state.DeletePublicKeysForUser(c.Context(), s.modelId, s.userId, []string{
		keysToAdd[0].Comment,
		keysToAdd[1].Fingerprint,
	})
	c.Assert(err, tc.ErrorIsNil)

	keys, err := state.GetPublicKeysDataForUser(c.Context(), s.modelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(testingPublicKeys[2:], tc.DeepEquals, keys)
}

// TestDeleteSamePublicKeyByTwoMethods is here to assert that if we call one
// delete operation with both a fingerprint and a comment for the same key only
// that key is removed and no other keys are removed and no other errors happen.
func (s *stateSuite) TestDeleteSamePublicKeyByTwoMethods(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeysForUser(c.Context(), s.modelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	err = state.DeletePublicKeysForUser(c.Context(), s.modelId, s.userId, []string{
		keysToAdd[0].Comment,
		keysToAdd[0].Fingerprint,
	})
	c.Assert(err, tc.ErrorIsNil)

	keys, err := state.GetPublicKeysDataForUser(c.Context(), s.modelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(testingPublicKeys[1:], tc.DeepEquals, keys)
}

// TestDeletePublicKeysForNonExistentModel is asserting the if we try and delete
// user keys off of a model that doesn't exist we get back a
// [modelerrors.NotFound] error.
func (s *stateSuite) TestDeletePublicKeysForNonExistentModel(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	badModelId := tc.Must0(c, model.NewUUID)

	err := state.DeletePublicKeysForUser(c.Context(), badModelId, s.userId, []string{
		keysToAdd[0].Comment,
		keysToAdd[0].Fingerprint,
	})
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *stateSuite) TestDeletePublicKeysForModel(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeysForUser(c.Context(), s.modelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	err = state.DeletePublicKeysForModel(c.Context(), s.modelId)
	c.Assert(err, tc.ErrorIsNil)

	keys, err := state.GetAllUsersPublicKeys(c.Context(), s.modelId)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(keys), tc.Equals, 0)
}

func (s *stateSuite) TestDeletePublicKeysForModelNonExistentModel(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())

	badModelId := tc.Must0(c, model.NewUUID)

	err := state.DeletePublicKeysForModel(c.Context(), badModelId)
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *stateSuite) TestDeletePublicKeysForModelNoKeys(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())

	err := state.DeletePublicKeysForModel(c.Context(), s.modelId)
	c.Check(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestDeletePublicKeysForModelKeepsOtherModels(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeysForUser(c.Context(), s.modelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	// Create a second model to add keys onto
	secondModelId := statemodeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "second-model")

	err = state.AddPublicKeysForUser(c.Context(), secondModelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	err = state.DeletePublicKeysForModel(c.Context(), s.modelId)
	c.Assert(err, tc.ErrorIsNil)

	keys, err := state.GetAllUsersPublicKeys(c.Context(), secondModelId)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(keys), tc.Equals, 1)
	c.Check(keys[s.userName], tc.DeepEquals, testingPublicKeys)
}

// TestGetAllUsersPublicKeys is responsible for testing the happy path of
// getting all user keys in the model.
func (s *stateSuite) TestGetAllUsersPublicKeys(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	err := state.AddPublicKeysForUser(
		c.Context(),
		s.modelId,
		s.userId,
		keysToAdd,
	)
	c.Assert(err, tc.ErrorIsNil)

	secondUserId := usertesting.GenUserUUID(c)
	secondUserName := usertesting.GenNewName(c, "tlm")
	userSt := accessstate.NewUserState(s.TxnRunnerFactory())
	err = userSt.AddUser(
		c.Context(),
		secondUserId,
		secondUserName,
		"tlm",
		false,
		s.userId,
	)
	c.Assert(err, tc.ErrorIsNil)

	err = state.AddPublicKeysForUser(
		c.Context(),
		s.modelId,
		secondUserId,
		keysToAdd,
	)
	c.Assert(err, tc.ErrorIsNil)

	allKeys, err := state.GetAllUsersPublicKeys(c.Context(), s.modelId)
	c.Check(err, tc.ErrorIsNil)

	for k := range allKeys {
		slices.Sort(allKeys[k])
	}
	expected := []string{
		"ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBG00bYFLb/sxPcmVRMg8NXZK/ldefElAkC9wD41vABdHZiSRvp+2y9BMNVYzE/FnzKObHtSvGRX65YQgRn7k5p0= juju1@example.com",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju2@example.com",
		"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDvplNOK3UBpULZKvZf/I5JHci/DufpSxj8yR4yKE2grescJxu6754jPT3xztSeLGD31/oJApJZGkMUAMRenvDqIaq+taRfOUo/l19AlGZc+Edv4bTlJzZ1Lzwex1vvL1doaLb/f76IIUHClGUgIXRceQH1ovHiIWj6nGltuLanG8YTWxlzzK33yhitmZt142DmpX1VUVF5c/Hct6Rav5lKmwej1TDed1KmHzXVoTHEsmWhKsOK27ue5yTuq0GX6LrAYDucF+2MqZCsuddXsPAW1tj5GNZSR7RrKW5q1CI0G7k9gSomuCsRMlCJ3BqID/vUSs/0qOWg4he0HUsYKQSrXIhckuZu+jYP8B80MoXT50ftRidoG/zh/PugBdXTk46FloVClQopG5A2fbqrphADcUUbRUxZ2lWQN+OVHKfEsfV2b8L2aSqZUGlryfW1cirB5JCTDvtv7rUy9/ny9iKA+8tAyKSDF0I901RDDqKc9dSkrHCg2bLnJZDoiRoWczE= juju3@example.com",
	}
	slices.Sort(expected)

	c.Check(allKeys, tc.DeepEquals, map[user.Name][]string{
		s.userName:     expected,
		secondUserName: expected,
	})
}

// TestGetAllUserPublicKeysEmpty is asserting that if there exists no public
// keys for any user in the model and we call [State.GetAllUsersPublicKeys] we
// get back an empty map and no errors.
func (s *stateSuite) TestGetAllUserPublicKeysEmpty(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	allKeys, err := state.GetAllUsersPublicKeys(c.Context(), s.modelId)
	c.Check(err, tc.ErrorIsNil)
	c.Check(len(allKeys), tc.Equals, 0)
}

// TestGetAllUserPublicKeysModelNotFound is asserting that is we ask for all the
// user public keys on a model that does not exist we get back a
// [modelerrors.NotFound] error.
func (s *stateSuite) TestGetAllUserPublicKeysModelNotFound(c *tc.C) {
	badModelUUID := tc.Must0(c, model.NewUUID)
	_, err := NewState(s.TxnRunnerFactory()).GetAllUsersPublicKeys(
		c.Context(),
		badModelUUID,
	)
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestAddPublicKeysForUserOnNonActivatedModel is asserting that we can add
// public keys for a user on a model that has not been activated yet. This is
// important for the model creation process where keys need to be added before
// the model is fully activated, this is the case in migrations.
func (s *stateSuite) TestAddPublicKeysForUserOnNonActivatedModel(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	// Create a non-activated model using the testing helper
	nonActivatedModelId := statemodeltesting.CreateTestModelWithoutActivation(
		c, s.TxnRunnerFactory(), "non-activated-model",
	)

	// Should be able to add keys to a non-activated model
	err := state.AddPublicKeysForUser(c.Context(), nonActivatedModelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	// Verify the keys were added
	keys, err := state.GetPublicKeysDataForUser(c.Context(), nonActivatedModelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(keys, tc.DeepEquals, testingPublicKeys)
}

// TestEnsurePublicKeysForUserOnNonActivatedModel is asserting that we can
// ensure public keys for a user on a model that has not been activated yet.
func (s *stateSuite) TestEnsurePublicKeysForUserOnNonActivatedModel(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	// Create a non-activated model using the testing helper
	nonActivatedModelId := statemodeltesting.CreateTestModelWithoutActivation(
		c, s.TxnRunnerFactory(), "non-activated-ensure-model",
	)

	// Should be able to ensure keys on a non-activated model
	err := state.EnsurePublicKeysForUser(c.Context(), nonActivatedModelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	// Verify the keys were added
	keys, err := state.GetPublicKeysDataForUser(c.Context(), nonActivatedModelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(keys, tc.DeepEquals, testingPublicKeys)

	// Run again to verify idempotency
	err = state.EnsurePublicKeysForUser(c.Context(), nonActivatedModelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)
}

// TestGetPublicKeysForUserOnNonActivatedModel is asserting that we can get
// public keys for a user on a model that has not been activated yet.
func (s *stateSuite) TestGetPublicKeysForUserOnNonActivatedModel(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	// Create a non-activated model
	nonActivatedModelId := statemodeltesting.CreateTestModelWithoutActivation(
		c, s.TxnRunnerFactory(), "non-activated-get-model",
	)

	// Add keys
	err := state.AddPublicKeysForUser(c.Context(), nonActivatedModelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	// Should be able to get keys from a non-activated model
	keys, err := state.GetPublicKeysForUser(c.Context(), nonActivatedModelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(keys), tc.Equals, len(testingPublicKeys))
}

// TestGetPublicKeysDataForUserOnNonActivatedModel is asserting that we can get
// public keys data for a user on a model that has not been activated yet.
func (s *stateSuite) TestGetPublicKeysDataForUserOnNonActivatedModel(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	// Create a non-activated model
	nonActivatedModelId := statemodeltesting.CreateTestModelWithoutActivation(
		c, s.TxnRunnerFactory(), "non-activated-getdata-model",
	)

	// Add keys
	err := state.AddPublicKeysForUser(c.Context(), nonActivatedModelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	// Should be able to get keys data from a non-activated model
	keys, err := state.GetPublicKeysDataForUser(c.Context(), nonActivatedModelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(keys, tc.DeepEquals, testingPublicKeys)
}

// TestDeletePublicKeysForUserOnNonActivatedModel is asserting that we can
// delete public keys for a user on a model that has not been activated yet.
func (s *stateSuite) TestDeletePublicKeysForUserOnNonActivatedModel(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	// Create a non-activated model
	nonActivatedModelId := statemodeltesting.CreateTestModelWithoutActivation(
		c, s.TxnRunnerFactory(), "non-activated-delete-model",
	)

	// Add keys
	err := state.AddPublicKeysForUser(c.Context(), nonActivatedModelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	// Should be able to delete keys from a non-activated model
	err = state.DeletePublicKeysForUser(c.Context(), nonActivatedModelId, s.userId, []string{
		keysToAdd[0].Comment,
	})
	c.Assert(err, tc.ErrorIsNil)

	// Verify the key was deleted
	keys, err := state.GetPublicKeysDataForUser(c.Context(), nonActivatedModelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	slices.Sort(keys)
	slices.Sort(testingPublicKeys)
	c.Check(testingPublicKeys[1:], tc.DeepEquals, keys)
}

// TestGetAllUsersPublicKeysOnNonActivatedModel is asserting that we can get
// all users' public keys on a model that has not been activated yet.
func (s *stateSuite) TestGetAllUsersPublicKeysOnNonActivatedModel(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	keysToAdd := generatePublicKeys(c, testingPublicKeys)

	// Create a non-activated model
	nonActivatedModelId := statemodeltesting.CreateTestModelWithoutActivation(
		c, s.TxnRunnerFactory(), "non-activated-getall-model",
	)

	// Add keys
	err := state.AddPublicKeysForUser(c.Context(), nonActivatedModelId, s.userId, keysToAdd)
	c.Check(err, tc.ErrorIsNil)

	// Should be able to get all keys from a non-activated model
	allKeys, err := state.GetAllUsersPublicKeys(c.Context(), nonActivatedModelId)
	c.Check(err, tc.ErrorIsNil)
	c.Check(len(allKeys), tc.Equals, 1)
	c.Check(allKeys[s.userName], tc.HasLen, len(testingPublicKeys))
}

// TestSameKeyDifferentModelsWithDifferentComments is asserting that the same SSH
// key can be added to multiple models with different comments.
func (s *stateSuite) TestSameKeyDifferentModelsWithDifferentComments(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())

	parsedKey, err := ssh.ParsePublicKey(testingPublicKeys[0])
	c.Assert(err, tc.ErrorIsNil)

	keyWithCommentA := keymanager.PublicKey{
		Comment:         "comment-a",
		FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
		Fingerprint:     parsedKey.Fingerprint(),
		Key:             testingPublicKeys[0],
	}

	keyWithCommentB := keymanager.PublicKey{
		Comment:         "comment-b",
		FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
		Fingerprint:     parsedKey.Fingerprint(),
		Key:             testingPublicKeys[0],
	}

	// Add key with comment A to model 1.
	err = state.AddPublicKeysForUser(c.Context(), s.modelId, s.userId, []keymanager.PublicKey{keyWithCommentA})
	c.Assert(err, tc.ErrorIsNil)

	// Create a second model.
	modelId2 := statemodeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "keys2")

	// Add the same key with comment B to model 2.
	err = state.AddPublicKeysForUser(c.Context(), modelId2, s.userId, []keymanager.PublicKey{keyWithCommentB})
	c.Assert(err, tc.ErrorIsNil)

	// Verify model 1 has the key with comment A.
	keys1, err := state.GetPublicKeysDataForUser(c.Context(), s.modelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(keys1, tc.HasLen, 1)
	c.Check(strings.HasSuffix(keys1[0], "comment-a"), tc.IsTrue, tc.Commentf("Expected key to end with 'comment-a', got: %s", keys1[0]))

	// Verify model 2 has the key with comment B.
	keys2, err := state.GetPublicKeysDataForUser(c.Context(), modelId2, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	// Expect len 1.
	c.Assert(keys2, tc.HasLen, 1)
	c.Check(strings.HasSuffix(keys2[0], "comment-b"), tc.IsTrue, tc.Commentf("Expected key to end with 'comment-b', got: %s", keys2[0]))
}

// TestSameKeyDifferentModelsWithAndWithoutComment is asserting that the same SSH
// key can be added to one model with a comment and another model without a comment.
func (s *stateSuite) TestSameKeyDifferentModelsWithAndWithoutComment(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())

	parsedKey, err := ssh.ParsePublicKey(testingPublicKeys[0])
	c.Assert(err, tc.ErrorIsNil)

	keyWithComment := keymanager.PublicKey{
		Comment:         "my-comment",
		FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
		Fingerprint:     parsedKey.Fingerprint(),
		Key:             testingPublicKeys[0],
	}

	keyWithoutComment := keymanager.PublicKey{
		Comment:         "",
		FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
		Fingerprint:     parsedKey.Fingerprint(),
		Key:             testingPublicKeys[0],
	}

	// Add key with comment to model 1.
	err = state.AddPublicKeysForUser(c.Context(), s.modelId, s.userId, []keymanager.PublicKey{keyWithComment})
	c.Assert(err, tc.ErrorIsNil)

	// Create a second model.
	modelId2 := statemodeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "keys2")

	// Add the same key without comment to model 2.
	err = state.AddPublicKeysForUser(c.Context(), modelId2, s.userId, []keymanager.PublicKey{keyWithoutComment})
	c.Assert(err, tc.ErrorIsNil)

	// Verify model 1 has the key with comment.
	keys1, err := state.GetPublicKeysDataForUser(c.Context(), s.modelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(keys1, tc.HasLen, 1)
	c.Check(strings.HasSuffix(keys1[0], "my-comment"), tc.IsTrue)

	// Verify model 2 has the key without comment (should just be the clean key).
	keys2, err := state.GetPublicKeysDataForUser(c.Context(), modelId2, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(keys2, tc.HasLen, 1)
	c.Check(strings.HasSuffix(keys2[0], "my-comment"), tc.IsFalse)
}

// TestSameKeyNoComment is asserting that a key can be added without any comment.
func (s *stateSuite) TestSameKeyNoComment(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())

	parsedKey, err := ssh.ParsePublicKey(testingPublicKeys[0])
	c.Assert(err, tc.ErrorIsNil)

	keyWithoutComment := keymanager.PublicKey{
		Comment:         "",
		FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
		Fingerprint:     parsedKey.Fingerprint(),
		Key:             testingPublicKeys[0],
	}

	err = state.AddPublicKeysForUser(c.Context(), s.modelId, s.userId, []keymanager.PublicKey{keyWithoutComment})
	c.Assert(err, tc.ErrorIsNil)

	// Verify the key was added without comment.
	keys, err := state.GetPublicKeysDataForUser(c.Context(), s.modelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(keys, tc.HasLen, 1)
	// The key should not have any extra comment appended.
	c.Check(strings.Contains(keys[0], "ecdsa-sha2-nistp256"), tc.IsTrue)
}

// TestSameKeySameModelSameCommentTwice is asserting that adding the same key
// with the same comment to the same model twice results in a
// PublicKeyAlreadyExists error.
func (s *stateSuite) TestSameKeySameModelSameCommentTwice(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())

	parsedKey, err := ssh.ParsePublicKey(testingPublicKeys[0])
	c.Assert(err, tc.ErrorIsNil)

	keyWithComment := keymanager.PublicKey{
		Comment:         "my-comment",
		FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
		Fingerprint:     parsedKey.Fingerprint(),
		Key:             testingPublicKeys[0],
	}

	// Add key first time.
	err = state.AddPublicKeysForUser(c.Context(), s.modelId, s.userId, []keymanager.PublicKey{keyWithComment})
	c.Assert(err, tc.ErrorIsNil)

	// Try to add the same key with same comment again.
	err = state.AddPublicKeysForUser(c.Context(), s.modelId, s.userId, []keymanager.PublicKey{keyWithComment})
	c.Check(err, tc.ErrorIs, keyerrors.PublicKeyAlreadyExists)
}

// TestSameKeySameModelDifferentComments is asserting that adding the same key
// with different comments to the same model results in a PublicKeyAlreadyExists error.
// This is the critical test: comments are per-model metadata, not part of the key identity.
func (s *stateSuite) TestSameKeySameModelDifferentComments(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())

	parsedKey, err := ssh.ParsePublicKey(testingPublicKeys[0])
	c.Assert(err, tc.ErrorIsNil)

	keyWithCommentA := keymanager.PublicKey{
		Comment:         "comment-a",
		FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
		Fingerprint:     parsedKey.Fingerprint(),
		Key:             testingPublicKeys[0],
	}

	keyWithCommentB := keymanager.PublicKey{
		Comment:         "comment-b",
		FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
		Fingerprint:     parsedKey.Fingerprint(),
		Key:             testingPublicKeys[0],
	}

	// Add key with comment A.
	err = state.AddPublicKeysForUser(c.Context(), s.modelId, s.userId, []keymanager.PublicKey{keyWithCommentA})
	c.Assert(err, tc.ErrorIsNil)

	// Try to add the same key with comment B to the same model.
	err = state.AddPublicKeysForUser(c.Context(), s.modelId, s.userId, []keymanager.PublicKey{keyWithCommentB})
	// Expect this to fail as we don't wanna allow adding the same key twice
	// even if the comment id different.
	c.Check(err, tc.ErrorIs, keyerrors.PublicKeyAlreadyExists)

	// Verify the original comment A is still there.
	keys, err := state.GetPublicKeysDataForUser(c.Context(), s.modelId, s.userId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(keys, tc.HasLen, 1)
	c.Check(strings.HasSuffix(keys[0], "comment-a"), tc.IsTrue)
}
