// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/internal/secrets/provider"
)

// State describes retrieval and persistence methods needed for
// the secrets domain service.
type State interface {
	GetModelUUID(ctx context.Context) (string, error)
	CreateUserSecret(ctx context.Context, version int, uri *secrets.URI, secret domainsecret.UpsertSecretParams) error
	CreateCharmApplicationSecret(ctx context.Context, version int, uri *secrets.URI, appName string, secret domainsecret.UpsertSecretParams) error
	CreateCharmUnitSecret(ctx context.Context, version int, uri *secrets.URI, unitName string, secret domainsecret.UpsertSecretParams) error
	UpdateSecret(ctx context.Context, uri *secrets.URI, secret domainsecret.UpsertSecretParams) error
	GetSecret(ctx context.Context, uri *secrets.URI) (*secrets.SecretMetadata, error)
	GetSecretRevision(ctx context.Context, uri *secrets.URI, revision int) (*secrets.SecretRevisionMetadata, error)
	GetSecretValue(ctx context.Context, uri *secrets.URI, revision int) (secrets.SecretData, *secrets.ValueRef, error)
	ListSecrets(ctx context.Context, uri *secrets.URI,
		revision *int, labels domainsecret.Labels,
	) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)
	ListCharmSecrets(ctx context.Context,
		appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)
	ListUserSecrets(ctx context.Context) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)
	GetSecretConsumer(ctx context.Context, uri *secrets.URI, unitName string) (*secrets.SecretConsumerMetadata, int, error)
	SaveSecretConsumer(ctx context.Context, uri *secrets.URI, unitName string, md *secrets.SecretConsumerMetadata) error
	GetUserSecretURIByLabel(ctx context.Context, label string) (*secrets.URI, error)
	GetURIByConsumerLabel(ctx context.Context, label string, unitName string) (*secrets.URI, error)
	GetSecretRemoteConsumer(ctx context.Context, uri *secrets.URI, unitName string) (*secrets.SecretConsumerMetadata, int, error)
	SaveSecretRemoteConsumer(ctx context.Context, uri *secrets.URI, unitName string, md *secrets.SecretConsumerMetadata) error
	UpdateRemoteSecretRevision(ctx context.Context, uri *secrets.URI, latestRevision int) error
	GrantAccess(ctx context.Context, uri *secrets.URI, params domainsecret.GrantParams) error
	RevokeAccess(ctx context.Context, uri *secrets.URI, params domainsecret.AccessParams) error
	GetSecretAccess(ctx context.Context, uri *secrets.URI, params domainsecret.AccessParams) (string, error)
	GetSecretAccessScope(ctx context.Context, uri *secrets.URI, params domainsecret.AccessParams) (*domainsecret.AccessScope, error)
	GetSecretGrants(ctx context.Context, uri *secrets.URI, role secrets.SecretRole) ([]domainsecret.GrantParams, error)
	InitialWatchStatementForObsoleteRevision(
		ctx context.Context, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	) (tableName string, statement eventsource.NamespaceQuery)
	GetRevisionIDsForObsolete(
		ctx context.Context,
		appOwners domainsecret.ApplicationOwners,
		unitOwners domainsecret.UnitOwners,
		revisionUUID ...string,
	) ([]string, error)
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new namespace watcher
	// for events based on the input change mask.
	NewNamespaceWatcher(string, changestream.ChangeType, eventsource.NamespaceQuery) (watcher.StringsWatcher, error)
}

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

// CreateSecretURIs returns the specified number of new secret URIs.
func (s *SecretService) CreateSecretURIs(ctx context.Context, count int) ([]*secrets.URI, error) {
	if count <= 0 {
		return nil, errors.NotValidf("secret URi count %d", count)
	}

	modelUUID, err := s.st.GetModelUUID(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "getting model uuid")
	}
	result := make([]*secrets.URI, count)
	for i := 0; i < count; i++ {
		result[i] = secrets.NewURI().WithSource(modelUUID)
	}
	return result, nil
}

// CreateSecret creates a secret with the specified parameters, returning an error
// satisfying [secreterrors.SecretLabelAlreadyExists] if the secret owner already has
// a secret with the same label.
func (s *SecretService) CreateSecret(ctx context.Context, uri *secrets.URI, params CreateSecretParams) error {
	if !params.UserSecret && params.CharmOwner == nil {
		return errors.New("must specify a charm secret owner or user secret")
	}
	if params.UserSecret && params.CharmOwner != nil {
		return errors.New("cannot specify both a charm secret owner and a user secret")
	}
	if params.LeaderToken != nil {
		if err := params.LeaderToken.Check(); err != nil {
			return errors.Trace(err)
		}
	}

	p := domainsecret.UpsertSecretParams{
		Description: params.Description,
		Label:       params.Label,
		ValueRef:    params.ValueRef,
		AutoPrune:   params.AutoPrune,
	}
	if len(params.Data) > 0 {
		p.Data = make(map[string]string)
		for k, v := range params.Data {
			p.Data[k] = v
		}
	}
	if params.UserSecret {
		err := s.st.CreateUserSecret(ctx, params.Version, uri, p)
		return errors.Annotatef(err, "creating user secret %q", uri.ID)
	}

	rotatePolicy := domainsecret.MarshallRotatePolicy(params.RotatePolicy)
	p.RotatePolicy = &rotatePolicy
	if params.RotatePolicy.WillRotate() {
		p.NextRotateTime = params.RotatePolicy.NextRotateTime(s.clock.Now())
	}
	p.ExpireTime = params.ExpireTime
	var err error
	if params.CharmOwner.Kind == ApplicationOwner {
		err = s.st.CreateCharmApplicationSecret(ctx, params.Version, uri, params.CharmOwner.ID, p)
	} else {
		err = s.st.CreateCharmUnitSecret(ctx, params.Version, uri, params.CharmOwner.ID, p)
	}
	if errors.Is(err, secreterrors.SecretLabelAlreadyExists) {
		return errors.Errorf("secret with label %q is already being used", *params.Label)
	}
	return errors.Annotatef(err, "creating charm secret %q", uri.ID)
}

// UpdateSecret creates a secret with the specified parameters, returning an error
// satisfying [secreterrors.SecretNotFound] if the secret does not exist.
// It also returns an error satisfying [secreterrors.SecretLabelAlreadyExists] if
// the secret owner already has a secret with the same label.
func (s *SecretService) UpdateSecret(ctx context.Context, uri *secrets.URI, params UpdateSecretParams) error {
	if len(params.Data) > 0 && params.ValueRef != nil {
		return errors.New("must specify either content or a value reference but not both")
	}

	if params.LeaderToken != nil {
		if err := params.LeaderToken.Check(); err != nil {
			return errors.Trace(err)
		}
	}

	p := domainsecret.UpsertSecretParams{
		Description: params.Description,
		Label:       params.Label,
		ValueRef:    params.ValueRef,
		AutoPrune:   params.AutoPrune,
		ExpireTime:  params.ExpireTime,
	}
	rotatePolicy := domainsecret.MarshallRotatePolicy(params.RotatePolicy)
	p.RotatePolicy = &rotatePolicy
	if len(params.Data) > 0 {
		p.Data = make(map[string]string)
		for k, v := range params.Data {
			p.Data[k] = v
		}
	}
	err := s.st.UpdateSecret(ctx, uri, p)
	if errors.Is(err, secreterrors.SecretLabelAlreadyExists) {
		return errors.Errorf("secret with label %q is already being used", *params.Label)
	}
	return errors.Annotatef(err, "updating charm secret %q", uri.ID)
}

// ListSecrets returns the secrets matching the specified terms.
// If multiple values for a given term are specified, secrets matching any of the
// values for that term are included.
func (s *SecretService) ListSecrets(ctx context.Context, uri *secrets.URI,
	revision *int,
	labels domainsecret.Labels,
) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error) {
	return s.st.ListSecrets(ctx, uri, revision, labels)
}

func splitCharmSecretOwners(owners ...CharmSecretOwner) (domainsecret.ApplicationOwners, domainsecret.UnitOwners) {
	var (
		appOwners  domainsecret.ApplicationOwners
		unitOwners domainsecret.UnitOwners
	)
	for _, owner := range owners {
		if owner.Kind == ApplicationOwner {
			appOwners = append(appOwners, owner.ID)
		} else {
			unitOwners = append(unitOwners, owner.ID)
		}
	}
	return appOwners, unitOwners
}

// ListCharmSecrets returns the secret metadata and revision metadata for any secrets matching the specified owner.
// The result contains secrets owned by any of the non nil owner attributes.
// The count of secret and revisions in the result must match.
func (s *SecretService) ListCharmSecrets(ctx context.Context, owners ...CharmSecretOwner) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error) {
	appOwners, unitOwners := splitCharmSecretOwners(owners...)
	return s.st.ListCharmSecrets(ctx, appOwners, unitOwners)
}

// GetSecret returns the secret with the specified URI.
// If returns [secreterrors.SecretNotFound] is there's no such secret.
func (s *SecretService) GetSecret(ctx context.Context, uri *secrets.URI) (*secrets.SecretMetadata, error) {
	return s.st.GetSecret(ctx, uri)
}

// GetUserSecretURIByLabel returns the user secret URI with the specified label.
// If returns [secreterrors.SecretNotFound] is there's no such secret.
func (s *SecretService) GetUserSecretURIByLabel(ctx context.Context, label string) (*secrets.URI, error) {
	return s.st.GetUserSecretURIByLabel(ctx, label)
}

// ListUserSecrets returns the secret metadata and revision metadata for any user secrets in the current model.
// The count of secret and revisions in the result must match.
func (s *SecretService) ListUserSecrets(ctx context.Context) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error) {
	return s.st.ListUserSecrets(ctx)
}

// GetSecretValue returns the value of the specified secret revision.
// If returns [secreterrors.SecretRevisionNotFound] is there's no such secret revision.
func (s *SecretService) GetSecretValue(ctx context.Context, uri *secrets.URI, rev int) (secrets.SecretValue, *secrets.ValueRef, error) {
	data, ref, err := s.st.GetSecretValue(ctx, uri, rev)
	return secrets.NewSecretValue(data), ref, errors.Trace(err)
}

// ProcessSecretConsumerLabel takes a secret consumer and a uri and label which have been used to consumer the secret.
// If the uri is empty, the label and consumer are used to lookup the consumed secret uri.
// This method returns the resulting uri, and optionally the label to update for the consumer.
func (s *SecretService) ProcessSecretConsumerLabel(
	ctx context.Context, unitName string, uri *secrets.URI, label string, checkCallerOwner func(secretOwner secrets.Owner) (bool, leadership.Token, error),
) (*secrets.URI, *string, error) {
	modelUUID, err := s.st.GetModelUUID(ctx)
	if err != nil {
		return nil, nil, errors.Annotate(err, "getting model uuid")
	}

	// label could be the consumer label for consumers or the owner label for owners.
	var labelToUpdate *string
	if label != "" && uri != nil {
		labelToUpdate = &label
	}

	// For local secrets, check those which may be owned by the caller.
	if uri == nil || uri.IsLocal(modelUUID) {
		md, err := s.getAppOwnedOrUnitOwnedSecretMetadata(ctx, uri, unitName, label)
		if err != nil && !errors.Is(err, secreterrors.SecretNotFound) {
			return nil, nil, errors.Trace(err)
		}
		if md != nil {
			// If the label has is to be changed by the secret owner, update the secret metadata.
			// TODO(wallyworld) - the label staying the same should be asserted in a txn.
			isOwner := true
			if labelToUpdate != nil && *labelToUpdate != md.Label {
				var (
					token leadership.Token
					err   error
				)
				if isOwner, token, err = checkCallerOwner(md.Owner); err != nil {
					return nil, nil, errors.Trace(err)
				}
				if isOwner {
					// TODO(secrets) - this should be updated when the consumed revision is looked up
					// but if the secret is a cross model secret, we get the content from the other
					// model and don't do the update. The logic should be reworked so local lookups
					// can ge done in a single txn.
					// Update the label.
					err := s.UpdateSecret(ctx, uri, UpdateSecretParams{
						LeaderToken: token,
						Label:       &label,
					})
					if err != nil {
						return nil, nil, errors.Trace(err)
					}
				}
			}
			// 1. secrets can be accessed by the owner;
			// 2. application owned secrets can be accessed by all the units of the application using owner label or URI.
			uri = md.URI
			// We don't update the consumer label in this case since the label comes
			// from the owner metadata and we don't want to violate uniqueness checks.
			if isOwner {
				labelToUpdate = nil
			}
		}
	}

	if uri == nil {
		var err error
		uri, err = s.GetURIByConsumerLabel(ctx, label, unitName)
		if errors.Is(err, secreterrors.SecretNotFound) {
			return nil, nil, errors.NotFoundf("secret URI for consumer label %q", label)
		}
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
	}
	return uri, labelToUpdate, nil
}

func (s *SecretService) getAppOwnedOrUnitOwnedSecretMetadata(ctx context.Context, uri *secrets.URI, unitName, label string) (*secrets.SecretMetadata, error) {
	notFoundErr := fmt.Errorf("secret %q not found%w", uri, errors.Hide(secreterrors.SecretNotFound))
	if label != "" {
		notFoundErr = fmt.Errorf("secret with label %q not found%w", label, errors.Hide(secreterrors.SecretNotFound))
	}

	appName, err := names.UnitApplication(unitName)
	if err != nil {
		// Should never happen.
		return nil, errors.Trace(err)
	}
	owners := []CharmSecretOwner{{
		Kind: ApplicationOwner,
		ID:   appName,
	}, {
		Kind: UnitOwner,
		ID:   unitName,
	}}
	metadata, _, err := s.ListCharmSecrets(ctx, owners...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	for _, md := range metadata {
		if uri != nil && md.URI.ID == uri.ID {
			return md, nil
		}
		if label != "" && md.Label == label {
			return md, nil
		}
	}
	return nil, notFoundErr
}

// ChangeSecretBackend sets the secret backend where the specified secret revision is stored.
// If returns [secreterrors.SecretRevisionNotFound] is there's no such secret revision.
func (s *SecretService) ChangeSecretBackend(ctx context.Context, uri *secrets.URI, revision int, params ChangeSecretBackendParams) error {
	return nil
}
