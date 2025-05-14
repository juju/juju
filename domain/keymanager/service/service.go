// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"net/url"

	"github.com/juju/collections/set"

	"github.com/juju/juju/core/model"
	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/controller"
	"github.com/juju/juju/domain/keymanager"
	keyerrors "github.com/juju/juju/domain/keymanager/errors"
	"github.com/juju/juju/internal/errors"
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
	// modelUUID is the model that this service is scoped to.
	modelUUID model.UUID

	// st provides the state access layer to this service.
	st State
}

// ImporterService provides the means for interacting with a users underlying
// public keys for a model while also offering a mechanism to import keys for a
// user from an external source.
type ImporterService struct {
	*Service

	// keyImporter is the [PublicKeyImporter] to use for fetching a users
	// public key's for subject.
	keyImporter PublicKeyImporter
}

// State provides the access layer the [Service] needs for persisting and
// retrieving a user's public keys on a model.
type State interface {
	// AddPublicKeyForUser is responsible for adding one or more ssh public keys
	// for a user to a given model.
	// The following errors can be expected:
	// - [keyerrors.PublicKeyAlreadyExists] - When one of the public keys being
	// added for a user already exists on the model.
	// - [github.com/juju/juju/domain/access/errors.UserNotFound] - When the user does not exist.
	// - [modelerrors.NotFound] - When the model does not exist.
	AddPublicKeysForUser(context.Context, model.UUID, user.UUID, []keymanager.PublicKey) error

	// EnsurePublicKeysForUser will attempt to add the given set of public
	// keys for the user to the specified model. If the user already has the
	// public key in the model it will be skipped and no
	// [keyerrors.PublicKeyAlreadyExists] error will be returned.
	// The following errors can be expected:
	// - [github.com/juju/juju/domain/access/errors.UserNotFound] - When the user does not exist.
	// - [modelerrors.NotFound] - When the model does not exist.
	EnsurePublicKeysForUser(context.Context, model.UUID, user.UUID, []keymanager.PublicKey) error

	// GetPublicKeysForUser is responsible for returning all of the public
	// keys for the user uuid on a model. If the user does not exist no error is
	// returned.
	// The following errors can be expected:
	// - [github.com/juju/juju/domain/access/errors.UserNotFound] - If the user does not exist.
	// - [modelerrors.NotFound] - If the model does not exist.
	GetPublicKeysForUser(context.Context, model.UUID, user.UUID) ([]coressh.PublicKey, error)

	// GetAllUsersPublicKeys returns all of the public keys that are in a model
	// and their respective username. This is useful for building a view during
	// model migration. The following errors can be expected:
	// - [modelerrors.NotFound] - When no model exists for the uuid.
	GetAllUsersPublicKeys(context.Context, model.UUID) (map[user.Name][]string, error)

	// DeletePublicKeysForUser is responsible for removing the keys from the
	// users list of public keys on the given model. keyIds represent one of the
	// keys fingerprint, public key data or comment.
	// The following errors can be expected:
	// - [github.com/juju/juju/domain/access/errors.UserNotFound] - When the user does not exist.
	// - [modelerrors.NotFound] - When the model does not exist.
	DeletePublicKeysForUser(context.Context, model.UUID, user.UUID, []string) error
}

var (
	// reservedPublicKeyComments is the set of comments that can not be
	// removed or added by a user.
	reservedPublicKeyComments = set.NewStrings(
		controller.ControllerSSHKeyComment,
	)
)

// NewService constructs a new [Service] for interacting with a user's public
// keys on a model.
func NewService(modelUUID model.UUID, state State) *Service {
	return &Service{
		modelUUID: modelUUID,
		st:        state,
	}
}

// NewImporterService constructs a new [ImporterService] that can both be used
// for interacting with a user's public keys and also importing new public keys
// from external sources.
func NewImporterService(
	modelUUID model.UUID,
	keyImporter PublicKeyImporter,
	state State,
) *ImporterService {
	return &ImporterService{
		keyImporter: keyImporter,
		Service:     NewService(modelUUID, state),
	}
}

// AddPublicKeysForUser is responsible for adding public keys for a user to a
// model. The following errors can be expected:
// - [errors.NotValid] when the user uuid is not valid
// - [github.com/juju/juju/domain/access/errors.UserNotFound] when the user does
// not exist.
// - [keyerrors.InvalidPublicKey] - When a public key fails validation.
// - [keyerrors.ReservedCommentViolation] - When a key being added contains a
// comment string that is reserved.
// - [keyerrors.PublicKeyAlreadyExists] - When a public key being added
// for a user already exists.
// - [github.com/juju/juju/domain/access/errors.UserNotFound] - When the provided
// user does not exist.
// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model does
// not exist.
func (s *Service) AddPublicKeysForUser(
	ctx context.Context,
	userUUID user.UUID,
	keys ...string,
) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := userUUID.Validate(); err != nil {
		return errors.Errorf("validating user uuid %q when adding public keys: %w", userUUID, err)
	}

	if len(keys) == 0 {
		return nil
	}

	toAdd := make([]keymanager.PublicKey, 0, len(keys))
	for i, keyToAdd := range keys {
		parsedKey, err := ssh.ParsePublicKey(keyToAdd)
		if err != nil {
			return errors.Errorf(
				"%w %q at index %d: %w",
				keyerrors.InvalidPublicKey, keyToAdd, i, err,
			)
		}

		if reservedPublicKeyComments.Contains(parsedKey.Comment) {
			return errors.Errorf(
				"public key %q at index %d contains a reserved comment %q that cannot be used",
				keyToAdd,
				i,
				parsedKey.Comment,
			).Add(keyerrors.ReservedCommentViolation)
		}

		toAdd = append(toAdd, keymanager.PublicKey{
			Comment:         parsedKey.Comment,
			FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
			Fingerprint:     parsedKey.Fingerprint(),
			Key:             keyToAdd,
		})
	}

	return s.st.AddPublicKeysForUser(ctx, s.modelUUID, userUUID, toAdd)
}

// DeletePublicKeysForUser removes the keys associated with targets from the
// user's list of public keys. Targets can be an arbitrary list of a
// public key fingerprint (sha256), comment or full key value to be
// removed. Where a match is found the key will be removed. If no key exists for
// a target this will result in no operation. The following errors can be
// expected:
// - [errors.NotValid] when the user uuid is not valid
// - [github.com/juju/juju/domain/access/errors.UserNotFound] when the provided
// user does not exist.
// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model does
// not exist.
// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model does
// not exist.
func (s *Service) DeleteKeysForUser(
	ctx context.Context,
	userUUID user.UUID,
	targets ...string,
) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := userUUID.Validate(); err != nil {
		return errors.Errorf(
			"validating user uuid %q when deleting public keys: %w",
			userUUID, err,
		)
	}

	return s.st.DeletePublicKeysForUser(ctx, s.modelUUID, userUUID, targets)
}

// GetAllUserPublicKeys returns all of the public keys in the model for each
// user grouped by [user.Name].
// The following errors can be expected:
// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model does
// not exist.
func (s *Service) GetAllUsersPublicKeys(
	ctx context.Context,
) (_ map[user.Name][]string, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetAllUsersPublicKeys(ctx, s.modelUUID)
}

// ImportPublicKeysForUser will import all of the public keys available for a
// given subject and add them to the specified Juju user. If the user already
// has one or more of the public keys being imported they will safely be skipped
// with no errors being returned.
// The following errors can be expected:
// - [errors.NotValid] when the user uuid is not valid
// - [github.com/juju/juju/domain/access/errors.UserNotFound] when the user does
// not exist.
// - [keyerrors.InvalidPublicKey] when a key being imported fails validation.
// - [keyerrors.ReservedCommentViolation] when a key being added contains a
// comment string that is reserved.
// - [keyerrors.UnknownImportSource] when the source for the import operation
// is unknown to the service.
// - [keyerrors.ImportSubjectNotFound] when the source has indicated that the
// subject for the import operation does not exist.
// - [github.com/juju/juju/domain/access/errors.UserNotFound] when the provided
// user does not exist.
// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model does
// not exist.
func (s *ImporterService) ImportPublicKeysForUser(
	ctx context.Context,
	userUUID user.UUID,
	subject *url.URL,
) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := userUUID.Validate(); err != nil {
		return errors.Errorf(
			"validating user uuid %q when importing public keys from %q: %w",
			userUUID, subject.String(), err,
		)
	}

	keys, err := s.keyImporter.FetchPublicKeysForSubject(ctx, subject)

	switch {
	case errors.Is(err, importererrors.NoResolver):
		return errors.Errorf(
			"importing public keys for user %q, unknown public key source %q",
			userUUID, subject.Scheme,
		).Add(keyerrors.UnknownImportSource)
	case errors.Is(err, importererrors.SubjectNotFound):
		return errors.Errorf(
			"importing public keys for user %q, import subject %q not found",
			userUUID, subject.String(),
		).Add(keyerrors.ImportSubjectNotFound)
	case err != nil:
		return errors.Errorf(
			"importing public keys for user %q using subject %q: %w",
			userUUID, subject.String(), err,
		)
	}

	keysToAdd := make([]keymanager.PublicKey, 0, len(keys))
	for i, key := range keys {
		parsedKey, err := ssh.ParsePublicKey(key)
		if err != nil {
			return errors.Errorf(
				"parsing key %d for subject %q when importing keys for user %q: %w",
				i, subject.String(), userUUID, err,
			).Add(keyerrors.InvalidPublicKey)
		}

		if reservedPublicKeyComments.Contains(parsedKey.Comment) {
			return errors.Errorf(
				"importing key %d for user %q with subject %q because the comment %q is reserved",
				i,
				userUUID,
				subject.String(),
				parsedKey.Comment,
			).Add(keyerrors.ReservedCommentViolation)
		}

		keysToAdd = append(keysToAdd, keymanager.PublicKey{
			Comment:         parsedKey.Comment,
			Key:             key,
			FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
			Fingerprint:     parsedKey.Fingerprint(),
		})
	}

	return s.st.EnsurePublicKeysForUser(ctx, s.modelUUID, userUUID, keysToAdd)
}

// ListPublicKeysForUser is responsible for returning the public ssh keys for
// the specified user. The following errors can be expected:
// - [errors.NotValid] when the user uuid is not valid.
// - [github.com/juju/juju/domain/access/errors.UserNotFound] when the provided
// user does not exist.
// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model does
// not exist.
func (s *Service) ListPublicKeysForUser(
	ctx context.Context,
	userUUID user.UUID,
) (_ []coressh.PublicKey, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := userUUID.Validate(); err != nil {
		return nil, errors.Errorf(
			"validating user uuid %q when listing public keys: %w",
			userUUID, err,
		)
	}

	return s.st.GetPublicKeysForUser(ctx, s.modelUUID, userUUID)
}
