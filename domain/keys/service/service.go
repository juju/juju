// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/user"
	keyserrors "github.com/juju/juju/domain/keys/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/ssh"
)

// keyDataForDelete represents all of the current authorised keys for a user
// indexed by they key data itself, fingerprint and comment.
type keyDataForDelete struct {
	allKeys       map[string]keyInfo
	byFingerprint map[string]string
	byComment     map[string]string
}

// keyInfo holds the reverse index keys for raw key data.
type keyInfo struct {
	comment     string
	fingerprint string
}

// Service provides the means for interacting with a users underlying
// authorised keys for a model.
type Service struct {
	st State
}

// State provides the access layer the [Service] needs for persisting and
// retrieving a user authorised keys on a model.
type State interface {
	// AddAuthorisedKeysForUser adds a set of keys as authorised for a user on
	// this model.
	AddAuthorisedKeysForUser(context.Context, user.UUID, ...string) error

	// GetAuthorisedKeysForUser is responsible for returning all of the
	// authorised keys for the current model.
	GetAuthorisedKeysForUser(context.Context, user.UUID) ([]string, error)

	// DeleteAuthorisedKeysForUser is responsible for removing the keys form the
	// users list of authorised keys.
	DeleteAuthorisedKeysForUser(context.Context, user.UUID, ...string) error
}

var (
	// reservedAuthorisedKeyComments is the set of comments that can not be
	// removed or added by a user.
	reservedAuthorisedKeyComments = set.NewStrings(
		"juju-client-key",
		config.JujuSystemKey,
	)
)

// NewService constructs a new [Service] for interfacting with a users
// authorised keys.
func NewService(state State) *Service {
	return &Service{
		st: state,
	}
}

// AddKeysForUser is responsible for adding authorised keys for a user to a
// model. Keys being added for a user will be de-duped. The following errors can be expected:
// - [errors.NotValid] when the user id is not valid
// - [github.com/juju/juju/domain/access/errors.UserNotFound] when the user does
// not exist.
// - [keyserrors.InvalidAuthorisedKey] when an authorised key fails validation.
// - [keyserrors.ReservedCommentViolation] when a key being added contains a
// comment string that is reserved.
// - [keyserrors.AuthorisedKeyAlreadyExists] when an authorised key being added
// for a user already exists.
func (s *Service) AddKeysForUser(ctx context.Context, userID user.UUID, keys ...string) error {
	if err := userID.Validate(); err != nil {
		return fmt.Errorf("validating user id %q when adding authorised keys: %w", userID, err)
	}

	if len(keys) == 0 {
		return nil
	}

	fingerprints, err := s.getExistingFingerprints(ctx, userID)
	if err != nil {
		return fmt.Errorf(
			"adding %d authorised keys for user %q: %w",
			len(keys), userID, err,
		)
	}

	// alreadySeen holds a list of fingerprints already seen during the add
	// operation. This is to ensure that on the off chance the caller passes the
	// the same key twice to add we only add it once.
	alreadySeen := map[string]struct{}{}
	toAdd := make([]string, 0, len(keys))
	for _, key := range keys {
		parsedKey, err := ssh.ParseAuthorisedKey(key)
		if err != nil {
			return fmt.Errorf("%w %q: %w", keyserrors.InvalidAuthorisedKey, key, err)
		}

		fingerprint := parsedKey.Fingerprint()
		if fingerprints.Contains(fingerprint) {
			return fmt.Errorf(
				"authorised key %q already exists for user %q: %w",
				key, userID, errors.Hide(keyserrors.AuthorisedKeyAlreadyExists),
			)
		}

		if reservedAuthorisedKeyComments.Contains(parsedKey.Comment) {
			return fmt.Errorf(
				"authorised key %q contains a reserved comment %q that cannot be used: %w",
				key, parsedKey.Comment, errors.Hide(keyserrors.ReservedCommentViolation),
			)
		}

		if _, seen := alreadySeen[fingerprint]; seen {
			continue
		}
		toAdd = append(toAdd, key)
	}

	return s.st.AddAuthorisedKeysForUser(ctx, userID, toAdd...)
}

// DeleteKeysForUser removes the keys associated with targets from the users
// list of authorised keys. Targets can be an arbitary list of a authorised
// key's fingerprint (sha256), comment or full key value to be removed. Where a
// match is found the key will be removed. If no key exists for a target this
// will result in no operation. The following errors can be expected:
// - [errors.NotValid] when the user id is not valid
// - [accesserrors.UserNotFound] when the provided user does not exist.
func (s *Service) DeleteKeysForUser(
	ctx context.Context,
	userID user.UUID,
	targets ...string,
) error {
	if err := userID.Validate(); err != nil {
		return fmt.Errorf(
			"validating user id %q when deleting authorised keys: %w",
			userID, err,
		)
	}

	keyData, err := s.getExistingKeysForDelete(ctx, userID)
	if err != nil {
		return fmt.Errorf(
			"getting authorised keys for user %q to delete keys: %w",
			userID, err,
		)
	}

	// Deffensive programming to make sure we don't allocate a bunch of memory
	// from a destructive call.
	maxToRemoveCount := len(targets)
	if len(keyData.allKeys) < maxToRemoveCount {
		maxToRemoveCount = len(keyData.allKeys)
	}

	var (
		key          string
		keysToRemove = make([]string, 0, maxToRemoveCount)
		exists       bool
	)
	// Range over the list of targets and see if it actually matches a key we
	// have for the user.
	for _, target := range targets {
		if key, exists = keyData.byComment[target]; exists {
		} else if key, exists = keyData.byFingerprint[target]; exists {
		} else {
			key = target
		}

		if _, exists = keyData.allKeys[key]; !exists {
			// Break out if the key doesn't exist.
			continue
		}

		keysToRemove = append(keysToRemove, key)
		delete(keyData.byComment, keyData.allKeys[key].comment)
		delete(keyData.byFingerprint, keyData.allKeys[key].fingerprint)
		delete(keyData.allKeys, key)
	}

	if len(keysToRemove) == 0 {
		return nil
	}

	return s.st.DeleteAuthorisedKeysForUser(ctx, userID, keysToRemove...)
}

// getExistingFingerprints returns the currently set authorised keys and their
// associated fingerprints for the current user on this model.
func (s *Service) getExistingFingerprints(
	ctx context.Context,
	userID user.UUID,
) (set.Strings, error) {
	keys, err := s.st.GetAuthorisedKeysForUser(ctx, userID)
	if err != nil {
		return set.Strings{}, fmt.Errorf(
			"cannot get authorised keys for user %q: %w",
			userID, err,
		)
	}

	fingerprints := set.Strings{}
	for i, key := range keys {
		authKey, err := ssh.ParseAuthorisedKey(key)
		if err != nil {
			return set.Strings{}, fmt.Errorf(
				"parsing user %q authorised key %d: %w",
				userID, i, err,
			)
		}
		fingerprints.Add(authKey.Fingerprint())
	}

	return fingerprints, nil
}

// getExistingKeysForDelete returns the authorised keys for the user indexed on
// the key data, fingerprint and comment. This function exists to help provide
// the information necessary to assist delete operations.
func (s *Service) getExistingKeysForDelete(
	ctx context.Context,
	userID user.UUID,
) (keyDataForDelete, error) {
	keys, err := s.st.GetAuthorisedKeysForUser(ctx, userID)
	if err != nil {
		return keyDataForDelete{}, fmt.Errorf(
			"cannot get authorised keys for user %q: %w",
			userID, err,
		)
	}

	rval := keyDataForDelete{
		allKeys:       make(map[string]keyInfo, len(keys)),
		byFingerprint: make(map[string]string, len(keys)),
		byComment:     make(map[string]string, len(keys)),
	}
	for i, key := range keys {
		authKey, err := ssh.ParseAuthorisedKey(key)
		if err != nil {
			return keyDataForDelete{}, fmt.Errorf(
				"parsing user %q authorised key %d: %w",
				userID, i, err,
			)
		}

		fingerprint := authKey.Fingerprint()
		rval.byFingerprint[fingerprint] = key
		rval.byComment[authKey.Comment] = key
		rval.allKeys[key] = keyInfo{
			fingerprint: fingerprint,
			comment:     authKey.Comment,
		}
	}
	return rval, nil
}

// ListKeysForUser is responsible for returning the authorised ssh keys for the
// specified user. The following errors can be expected:
// - [errors.NotValid] when the user id is not valid.
// - [usererrors.NotFound] when the given user does not exist.
func (s *Service) ListKeysForUser(ctx context.Context, userID user.UUID) ([]string, error) {
	if err := userID.Validate(); err != nil {
		return nil, fmt.Errorf(
			"validating user id %q when listing authorised keys: %w",
			userID, err,
		)
	}

	return s.st.GetAuthorisedKeysForUser(ctx, userID)
}
