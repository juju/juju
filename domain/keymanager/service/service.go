// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"net/url"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/keymanager"
	keyserrors "github.com/juju/juju/domain/keymanager/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/ssh"
)

// PublicKeyImporter describes a service that is capable of fetching and
// providing public keys for a subject from a set of well known sources that
// don't need to be understood by this service.
type PublicKeyImporter interface {
	// FetchPublicKeysForSubject is responsible for gathering all of the
	// public keys available for a specified subject.
	FetchPublicKeysForSubject(context.Context, *url.URL) ([]string, error)
}

// Service provides the means for interacting with a users underlying
// public keys for a model.
type Service struct {
	// keyImporter is the [PublicKeyImporter] to use for fetching a users
	// public key's for subject.
	keyImporter PublicKeyImporter

	// st is the provides the state access layer to this service.
	st State
}

// State provides the access layer the [Service] needs for persisting and
// retrieving a users public keys on a model.
type State interface {
	// AddPublicKeysForUser adds a set of public keys for a user on
	// this model. If one or more of the public keys to add for the user already
	// exists a [keyserrors.PublicKeyAlreadyExists] error will be returned.
	AddPublicKeysForUser(context.Context, user.UUID, []keymanager.PublicKey) error

	// AddPublicKeyForUserIfNotFound will attempt to add the given set of public
	// keys to the user. If the user already contains the public key it will be
	// skipped and no [keyserrors.PublicKeyAlreadyExists] error will be returned.
	AddPublicKeyForUserIfNotFound(context.Context, user.UUID, []keymanager.PublicKey) error

	// GetPublicKeysForUser is responsible for returning all of the
	// public keys for the current user in this model.
	GetPublicKeysForUser(context.Context, user.UUID) ([]string, error)

	// DeletePublicKeysForUser is responsible for removing the keys form the
	// users list of public keys where the string list represents one of
	// the keys fingerprint, public key data or comment.
	DeletePublicKeysForUser(context.Context, user.UUID, []string) error
}

var (
	// reservedPublicKeyComments is the set of comments that can not be
	// removed or added by a user.
	reservedPublicKeyComments = set.NewStrings(
		"juju-client-key",
		config.JujuSystemKey,
	)
)

// NewService constructs a new [Service] for interfacting with a users
// public keys.
func NewService(keyImporter PublicKeyImporter, state State) *Service {
	return &Service{
		keyImporter: keyImporter,
		st:          state,
	}
}

// AddPublicKeysForUser is responsible for adding public keys for a user to a
// model. The following errors can be expected:
// - [errors.NotValid] when the user id is not valid
// - [github.com/juju/juju/domain/access/errors.UserNotFound] when the user does
// not exist.
// - [keyserrors.InvalidPublicKey] when a public key fails validation.
// - [keyserrors.ReservedCommentViolation] when a key being added contains a
// comment string that is reserved.
// - [keyserrors.PublicKeyAlreadyExists] when a public key being added
// for a user already exists.
func (s *Service) AddPublicKeysForUser(
	ctx context.Context,
	userID user.UUID,
	keys ...string,
) error {
	if err := userID.Validate(); err != nil {
		return fmt.Errorf("validating user id %q when adding public keys: %w", userID, err)
	}

	if len(keys) == 0 {
		return nil
	}

	toAdd := make([]keymanager.PublicKey, 0, len(keys))
	for i, keyToAdd := range keys {
		parsedKey, err := ssh.ParsePublicKey(keyToAdd)
		if err != nil {
			return fmt.Errorf(
				"%w %q at index %d: %w",
				keyserrors.InvalidPublicKey, keyToAdd, i, err,
			)
		}

		if reservedPublicKeyComments.Contains(parsedKey.Comment) {
			return fmt.Errorf(
				"public key %q at index %d contains a reserved comment %q that cannot be used: %w",
				keyToAdd,
				i,
				parsedKey.Comment,
				errors.Hide(keyserrors.ReservedCommentViolation),
			)
		}

		toAdd = append(toAdd, keymanager.PublicKey{
			Comment:     parsedKey.Comment,
			Fingerprint: parsedKey.Fingerprint(),
			Key:         keyToAdd,
		})
	}

	return s.st.AddPublicKeysForUser(ctx, userID, toAdd)
}

// DeletePublicKeysForUser removes the keys associated with targets from the
// users list of public keys. Targets can be an arbitary list of a
// public key fingerprint (sha256), comment or full key value to be
// removed. Where a match is found the key will be removed. If no key exists for
// a target this will result in no operation. The following errors can be
// expected:
// - [errors.NotValid] when the user id is not valid
// - [accesserrors.UserNotFound] when the provided user does not exist.
func (s *Service) DeleteKeysForUser(
	ctx context.Context,
	userID user.UUID,
	targets ...string,
) error {
	if err := userID.Validate(); err != nil {
		return fmt.Errorf(
			"validating user id %q when deleting public keys: %w",
			userID, err,
		)
	}

	return s.st.DeletePublicKeysForUser(ctx, userID, targets)
}

// ImportPublicKeysForUser will import all of the public keys available for a
// given subject and add them to the specified Juju user. If the user already
// has one or more of the public keys being imported they will safely be skipped
// with no errors being returned.
// The following errors can be expected:
// - [errors.NotValid] when the user id is not valid
// - [github.com/juju/juju/domain/access/errors.UserNotFound] when the user does
// not exist.
// - [keyserrors.InvalidPublicKey] when a key being imported fails validation.
// - [keyserrors.ReservedCommentViolation] when a key being added contains a
// comment string that is reserved.
func (s *Service) ImportPublicKeysForUser(
	ctx context.Context,
	userID user.UUID,
	subject *url.URL,
) error {
	return nil
}

// ListPublicKeysForUser is responsible for returning the public ssh keys for
// the specified user. The following errors can be expected:
// - [errors.NotValid] when the user id is not valid.
// - [usererrors.NotFound] when the given user does not exist.
func (s *Service) ListPublicKeysForUser(
	ctx context.Context,
	userID user.UUID,
) ([]string, error) {
	if err := userID.Validate(); err != nil {
		return nil, fmt.Errorf(
			"validating user id %q when listing public keys: %w",
			userID, err,
		)
	}

	return s.st.GetPublicKeysForUser(ctx, userID)
}
