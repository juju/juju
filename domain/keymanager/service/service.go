// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"net/url"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/keymanager"
	keyserrors "github.com/juju/juju/domain/keymanager/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/ssh"
	importererrors "github.com/juju/juju/internal/ssh/importer/errors"
)

// PublicKeyImporter describes a service that is capable of fetching and
// providing public keys for a subject from a set of well known sources that
// don't need to be understood by this service.
type PublicKeyImporter interface {
	// FetchPublicKeysForSubject is responsible for gathering all of the
	// public keys available for a specified subject.
	// The following errors can be expected:
	// - [importererrors.NoResolver] when there is import resolver the subject
	// schema.
	// - [importerrors.SubjectNotFound] when the resolver has reported that no
	// subject exists.
	FetchPublicKeysForSubject(context.Context, *url.URL) ([]string, error)
}

// Service provides the means for interacting with a users underlying
// public keys for a model.
type Service struct {
	// keyImporter is the [PublicKeyImporter] to use for fetching a users
	// public key's for subject.
	keyImporter PublicKeyImporter

	// st provides the state access layer to this service.
	st State
}

// State provides the access layer the [Service] needs for persisting and
// retrieving a user's public keys on a model.
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
	GetPublicKeysForUser(context.Context, user.UUID) ([]coressh.PublicKey, error)

	// DeletePublicKeysForUser is responsible for removing the keys from the
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

// NewService constructs a new [Service] for interacting with a users
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
			Comment:         parsedKey.Comment,
			FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
			Fingerprint:     parsedKey.Fingerprint(),
			Key:             keyToAdd,
		})
	}

	return s.st.AddPublicKeysForUser(ctx, userID, toAdd)
}

// DeletePublicKeysForUser removes the keys associated with targets from the
// user's list of public keys. Targets can be an arbitrary list of a
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
// - [keyserrors.UnknownImportSource] when the source for the import operation
// is unknown to the service.
// - [keyserrors.ImportSubjectNotFound] when the source has indicated that the
// subject for the import operation does not exist.
func (s *Service) ImportPublicKeysForUser(
	ctx context.Context,
	userID user.UUID,
	subject *url.URL,
) error {
	if err := userID.Validate(); err != nil {
		return fmt.Errorf(
			"validating user id %q when importing public keys from %q: %w",
			userID, subject.String(), err,
		)
	}

	keys, err := s.keyImporter.FetchPublicKeysForSubject(ctx, subject)

	switch {
	case errors.Is(err, importererrors.NoResolver):
		return fmt.Errorf(
			"cannot import public keys for user %q, unknown public key source %q%w",
			userID, subject.Scheme, errors.Hide(keyserrors.UnknownImportSource),
		)
	case errors.Is(err, importererrors.SubjectNotFound):
		return fmt.Errorf(
			"cannot import public keys for user %q, import subject %q not found%w",
			userID, subject.String(), errors.Hide(keyserrors.ImportSubjectNotFound),
		)
	case err != nil:
		return fmt.Errorf(
			"cannot import public keys for user %q using subject %q: %w",
			userID, subject.String(), err,
		)
	}

	keysToAdd := make([]keymanager.PublicKey, 0, len(keys))
	for i, key := range keys {
		parsedKey, err := ssh.ParsePublicKey(key)
		if err != nil {
			return fmt.Errorf(
				"cannot parse key %d for subject %q when importing keys for user %q: %w%w",
				i, subject.String(), userID, err, errors.Hide(keyserrors.InvalidPublicKey),
			)
		}

		if reservedPublicKeyComments.Contains(parsedKey.Comment) {
			return fmt.Errorf(
				"cannot import key %d for user %q with subject %q because the comment %q is reserved%w",
				i,
				userID,
				subject.String(),
				parsedKey.Comment,
				errors.Hide(keyserrors.ReservedCommentViolation),
			)
		}

		keysToAdd = append(keysToAdd, keymanager.PublicKey{
			Comment:         parsedKey.Comment,
			Key:             key,
			FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
			Fingerprint:     parsedKey.Fingerprint(),
		})
	}

	return s.st.AddPublicKeyForUserIfNotFound(ctx, userID, keysToAdd)
}

// ListPublicKeysForUser is responsible for returning the public ssh keys for
// the specified user. The following errors can be expected:
// - [errors.NotValid] when the user id is not valid.
// - [usererrors.NotFound] when the given user does not exist.
func (s *Service) ListPublicKeysForUser(
	ctx context.Context,
	userID user.UUID,
) ([]coressh.PublicKey, error) {
	if err := userID.Validate(); err != nil {
		return nil, fmt.Errorf(
			"validating user id %q when listing public keys: %w",
			userID, err,
		)
	}

	return s.st.GetPublicKeysForUser(ctx, userID)
}
