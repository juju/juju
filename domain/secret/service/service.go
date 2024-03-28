// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/core/secrets"
	domainsecret "github.com/juju/juju/domain/secret"
	"github.com/juju/juju/internal/secrets/provider"
)

type State interface{}

// Logger facilitates emitting log messages.
type Logger interface {
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
}

// NewSecretService returns a new secret service wrapping the specified state.
func NewSecretService(st State, logger Logger, adminConfigGetter BackendAdminConfigGetter) *SecretService {
	return &SecretService{
		st:                st,
		logger:            logger,
		clock:             clock.WallClock,
		providerGetter:    provider.Provider,
		adminConfigGetter: adminConfigGetter,
	}
}

// BackendAdminConfigGetter is a func used to get admin level secret backend config.
type BackendAdminConfigGetter func(context.Context) (*provider.ModelBackendConfigInfo, error)

// NotImplementedBackendConfigGetter is a not implemented secret backend getter.
// TODO(secrets) - this is a placeholder
var NotImplementedBackendConfigGetter = func(context.Context) (*provider.ModelBackendConfigInfo, error) {
	return nil, errors.NotImplemented
}

// SecretService provides the API for working with secrets.
type SecretService struct {
	st                State
	logger            Logger
	clock             clock.Clock
	providerGetter    func(backendType string) (provider.SecretBackendProvider, error)
	adminConfigGetter BackendAdminConfigGetter
}

func (s *SecretService) CreateSecretURIs(ctx context.Context, count int) ([]*secrets.URI, error) {
	if count <= 0 {
		return nil, errors.NotValidf("secret URi count %d", count)
	}

	// TODO(secrets)
	modelUUID := ""
	result := make([]*secrets.URI, count)
	for i := 0; i < count; i++ {
		result[i] = secrets.NewURI().WithSource(modelUUID)
	}
	return result, nil
}
func (s *SecretService) CreateSecret(ctx context.Context, uri *secrets.URI, params CreateSecretParams) (*secrets.SecretMetadata, error) {
	return nil, errors.NotImplemented
	/*
		var nextRotateTime *time.Time
		if params.RotatePolicy.WillRotate() {
			nextRotateTime = params.RotatePolicy.NextRotateTime(s.clock.Now())
		}
	*/
	// also grant manage access to owner
}

func (s *SecretService) UpdateSecret(ctx context.Context, uri *secrets.URI, params UpdateSecretParams) (*secrets.SecretMetadata, error) {
	return nil, errors.NotFound

	// TODO(secrets)
	/*
		md, err := s.secretsState.GetSecret(uri)
		if err != nil {
			return errors.Trace(err)
		}
		var nextRotateTime *time.Time
		if !md.RotatePolicy.WillRotate() && arg.RotatePolicy.WillRotate() {
			nextRotateTime = arg.RotatePolicy.NextRotateTime(s.clock.Now())
		}

	*/

	//var md *secrets.SecretMetadata
	//if !md.AutoPrune {
	//	return md, nil
	//}
	//// If the secret was updated, we need to delete the old unused secret revisions.
	//revsToDelete, err := s.ListUnusedSecretRevisions(ctx, uri)
	//if err != nil {
	//	return nil, errors.Trace(err)
	//}
	//var revisions []int
	//for _, rev := range revsToDelete {
	//	if rev == md.LatestRevision {
	//		// We don't want to delete the latest revision.
	//		continue
	//	}
	//	revisions = append(revisions, rev)
	//}
	//if len(revisions) == 0 {
	//	return md, nil
	//}
	//err = s.DeleteUserSecret(ctx, uri, revisions, func(uri *secrets.URI) error { return nil })
	//if err != nil {
	//	// We don't want to fail the update if we can't prune the unused secret revisions because they will be picked up later
	//	// when the secret has any new obsolete revisions.
	//	s.logger.Warningf("failed to prune unused secret revisions for %q: %v", uri, err)
	//}
	//return md, nil
}

// ListSecrets returns the secrets matching the specified terms.
// If multiple values for a given term are specified, secrets matching any of the
// values for that term are included.
func (s *SecretService) ListSecrets(ctx context.Context, uri *secrets.URI,
	revisions domainsecret.Revisions,
	labels domainsecret.Labels, appOwners domainsecret.ApplicationOwners,
	unitOwners domainsecret.UnitOwners, modelOwners domainsecret.ModelOwners,
) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error) {
	return nil, nil, nil
}

// ListCharmSecrets returns the secret metadata and revision metadata for any secrets matching the specified owner.
// The result contains secrets owned by any of the non nil owner attributes.
// The count of secret and revisions in the result must match.
func (s *SecretService) ListCharmSecrets(ctx context.Context, owner CharmSecretOwners) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error) {
	// TODO(secrets)
	return nil, nil, nil
}

// GetSecret returns the secret with the specified URI.
// If returns [secreterrors.SecretNotFound] is there's no such secret.
func (s *SecretService) GetSecret(ctx context.Context, uri *secrets.URI) (*secrets.SecretMetadata, error) {
	return nil, errors.NotFound
}

// GetSecretRevision returns the secret revision for the specified URI.
// If returns [secreterrors.SecretNotFound] is there's no such secret.
// If returns [secreterrors.SecretRevisionNotFound] is there's no such secret revision.
func (s *SecretService) GetSecretRevision(ctx context.Context, uri *secrets.URI, revision int) (*secrets.SecretRevisionMetadata, error) {
	return nil, errors.NotFound
}

// GetUserSecretByLabel returns the user secret with the specified label.
// If returns [secreterrors.SecretNotFound] is there's no such secret.
func (s *SecretService) GetUserSecretByLabel(ctx context.Context, label string) (*secrets.SecretMetadata, error) {
	return nil, errors.NotFound
}

// ListUserSecrets returns the secret metadata and revision metadata for any user secrets in the current model.
// The count of secret and revisions in the result must match.
func (s *SecretService) ListUserSecrets(ctx context.Context) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error) {
	// TODO(secrets)
	return nil, nil, nil
}

// GetSecretValue returns the value of the specified secret revision.
// If returns [secreterrors.SecretRevisionNotFound] is there's no such secret revision.
func (s *SecretService) GetSecretValue(ctx context.Context, uri *secrets.URI, rev int) (secrets.SecretValue, *secrets.ValueRef, error) {
	return nil, nil, errors.NotFound
}

// ChangeSecretBackend sets the secret backend where the specified secret revision is stored.
// If returns [secreterrors.SecretRevisionNotFound] is there's no such secret revision.
func (s *SecretService) ChangeSecretBackend(ctx context.Context, uri *secrets.URI, revision int, params ChangeSecretBackendParams) error {
	return nil
}
