// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/secrets"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	backenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/uuid"
)

// NewSecretService returns a new secret service wrapping the specified state.
func NewSecretService(
	secretState State, secretBackendReferenceMutator SecretBackendReferenceMutator, modelID coremodel.UUID,
	logger logger.Logger, adminConfigGetter BackendAdminConfigGetter,
) *SecretService {
	return &SecretService{
		modelID:                       modelID,
		secretState:                   secretState,
		secretBackendReferenceMutator: secretBackendReferenceMutator,
		logger:                        logger,
		clock:                         clock.WallClock,
		providerGetter:                provider.Provider,
		adminConfigGetter:             adminConfigGetter,
		uuidGenerator:                 uuid.NewUUID,
	}
}

// BackendAdminConfigGetter is a func used to get admin level secret backend config.
type BackendAdminConfigGetter func(context.Context) (*provider.ModelBackendConfigInfo, error)

// NotImplementedBackendConfigGetter is a not implemented secret backend getter.
// It is used by callers of the secret service that do not need any backend functionality.
var NotImplementedBackendConfigGetter = func(context.Context) (*provider.ModelBackendConfigInfo, error) {
	return nil, errors.NotImplemented
}

// SecretService provides the API for working with secrets.
type SecretService struct {
	secretState                   State
	secretBackendReferenceMutator SecretBackendReferenceMutator
	logger                        logger.Logger
	clock                         clock.Clock
	providerGetter                func(backendType string) (provider.SecretBackendProvider, error)
	adminConfigGetter             BackendAdminConfigGetter

	activeBackendID string
	modelID         coremodel.UUID
	backends        map[string]provider.SecretsBackend
	uuidGenerator   func() (uuid.UUID, error)
}

// CreateSecretURIs returns the specified number of new secret URIs.
func (s *SecretService) CreateSecretURIs(ctx context.Context, count int) ([]*secrets.URI, error) {
	if count <= 0 {
		return nil, errors.NotValidf("secret URi count %d", count)
	}

	modelUUID, err := s.secretState.GetModelUUID(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "getting model uuid")
	}
	result := make([]*secrets.URI, count)
	for i := 0; i < count; i++ {
		result[i] = secrets.NewURI().WithSource(modelUUID)
	}
	return result, nil
}

func (s *SecretService) getBackend(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
	p, err := s.providerGetter(cfg.BackendType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return p.NewBackend(cfg)
}

func (s *SecretService) loadBackendInfo(ctx context.Context, activeOnly bool) error {
	s.backends = make(map[string]provider.SecretsBackend)
	info, err := s.adminConfigGetter(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	s.activeBackendID = info.ActiveID
	for id, cfg := range info.Configs {
		if id != info.ActiveID && activeOnly {
			continue
		}
		s.backends[id], err = s.getBackend(&cfg)
		if err != nil {
			if id != info.ActiveID && cfg.BackendType == kubernetes.BackendType {
				// TODO(secrets) - on an iaas controller, attempting to get the "model" k8s backend fails
				//The root cause is not filtering backends to those that are in use.
				s.logger.Debugf("failed to load backend info for id %s (%s): %v", id, cfg.BackendType, err)
				continue
			}
			return errors.Trace(err)
		}
	}
	if activeOnly && len(s.backends) == 0 {
		// Should never happen.
		return errors.New("no active secret backend")
	}
	return nil
}

// CreateUserSecret creates a user secret with the specified parameters, returning an error
// satisfying [secreterrors.SecretLabelAlreadyExists] if the secret owner already has
// a secret with the same label.
func (s *SecretService) CreateUserSecret(ctx context.Context, uri *secrets.URI, params CreateUserSecretParams) (errOut error) {
	if len(params.Data) == 0 {
		return errors.NotValidf("empty secret value")
	}

	p := domainsecret.UpsertSecretParams{
		Description: params.Description,
		Label:       params.Label,
		AutoPrune:   params.AutoPrune,
		Checksum:    params.Checksum,
	}
	// Take a copy as we may set it to nil below
	// if the content is saved to a backend.
	p.Data = make(map[string]string)
	for k, v := range params.Data {
		p.Data[k] = v
	}

	err := s.loadBackendInfo(ctx, true)
	if err != nil {
		return errors.Trace(err)
	}
	// loadBackendInfo will error is there's no active backend.
	backend := s.backends[s.activeBackendID]

	revId, err := backend.SaveContent(ctx, uri, 1, secrets.NewSecretValue(params.Data))
	if err != nil && !errors.Is(err, errors.NotSupported) {
		return errors.Annotatef(err, "saving secret content to backend")
	}
	if err == nil {
		defer func() {
			if errOut != nil {
				// If we failed to create the secret, we should delete the
				// secret value from the backend.
				if err2 := backend.DeleteContent(ctx, revId); err2 != nil &&
					!errors.Is(err2, errors.NotSupported) &&
					!errors.Is(err2, secreterrors.SecretRevisionNotFound) {
					s.logger.Warningf("failed to delete secret %q: %v", revId, err2)
				}
			}
		}()
		p.Data = nil
		p.ValueRef = &secrets.ValueRef{
			BackendID:  s.activeBackendID,
			RevisionID: revId,
		}
	}
	revisionID, err := s.uuidGenerator()
	if err != nil {
		return errors.Trace(err)
	}
	p.RevisionID = ptr(revisionID.String())

	rollBack, err := s.secretBackendReferenceMutator.AddSecretBackendReference(ctx, p.ValueRef, s.modelID, revisionID)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		if errOut != nil {
			if err := rollBack(); err != nil {
				s.logger.Warningf("failed to roll back secret reference count: %v", err)
			}
		}
	}()

	if err = s.secretState.CreateUserSecret(ctx, params.Version, uri, p); err != nil {
		return errors.Annotatef(err, "cannot create user secret %q", uri.ID)
	}
	return nil
}

func ptr[T any](s T) *T {
	return &s
}

// CreateCharmSecret creates a charm secret with the specified parameters, returning an error
// satisfying [secreterrors.SecretLabelAlreadyExists] if the secret owner already has
// a secret with the same label.
func (s *SecretService) CreateCharmSecret(ctx context.Context, uri *secrets.URI, params CreateCharmSecretParams) (errOut error) {
	if len(params.Data) > 0 && params.ValueRef != nil {
		return errors.New("must specify either content or a value reference but not both")
	}

	p := domainsecret.UpsertSecretParams{
		Description: params.Description,
		Label:       params.Label,
		ValueRef:    params.ValueRef,
		Checksum:    params.Checksum,
	}
	if len(params.Data) > 0 {
		p.Data = make(map[string]string)
		for k, v := range params.Data {
			p.Data[k] = v
		}
	}

	rotatePolicy := domainsecret.MarshallRotatePolicy(params.RotatePolicy)
	p.RotatePolicy = &rotatePolicy
	if params.RotatePolicy.WillRotate() {
		p.NextRotateTime = params.RotatePolicy.NextRotateTime(s.clock.Now())
	}
	p.ExpireTime = params.ExpireTime

	revisionID, err := s.uuidGenerator()
	if err != nil {
		return errors.Trace(err)
	}
	p.RevisionID = ptr(revisionID.String())

	rollBack, err := s.secretBackendReferenceMutator.AddSecretBackendReference(ctx, p.ValueRef, s.modelID, revisionID)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		if errOut != nil {
			if err := rollBack(); err != nil {
				s.logger.Warningf("failed to roll back secret reference count: %v", err)
			}
		}
	}()
	if params.CharmOwner.Kind == ApplicationOwner {
		// Only unit leaders can create application secrets.
		if params.LeaderToken == nil {
			return secreterrors.PermissionDenied
		}
		if err := params.LeaderToken.Check(); err != nil {
			if leadership.IsNotLeaderError(err) {
				return secreterrors.PermissionDenied
			}
			return errors.Trace(err)
		}
		err = s.secretState.CreateCharmApplicationSecret(ctx, params.Version, uri, params.CharmOwner.ID, p)
	} else {
		err = s.secretState.CreateCharmUnitSecret(ctx, params.Version, uri, params.CharmOwner.ID, p)
	}
	if errors.Is(err, secreterrors.SecretLabelAlreadyExists) {
		return errors.Errorf("secret with label %q is already being used", *params.Label)
	}
	if err != nil {
		return errors.Annotatef(err, "cannot create charm secret %q", uri.ID)
	}
	return nil
}

// UpdateUserSecret updates a user secret with the specified parameters, returning an error
// satisfying [secreterrors.SecretNotFound] if the secret does not exist.
// It also returns an error satisfying [secreterrors.SecretLabelAlreadyExists] if
// the secret owner already has a secret with the same label.
// It returns [secreterrors.PermissionDenied] if the secret cannot be managed by the accessor.
func (s *SecretService) UpdateUserSecret(ctx context.Context, uri *secrets.URI, params UpdateUserSecretParams) (errOut error) {
	if err := s.canManage(ctx, uri, params.Accessor, nil); err != nil {
		return errors.Trace(err)
	}

	p := domainsecret.UpsertSecretParams{
		Description: params.Description,
		Label:       params.Label,
		AutoPrune:   params.AutoPrune,
		Checksum:    params.Checksum,
	}
	// Take a copy as we may set it to nil below
	// if the content is saved to a backend.
	if len(params.Data) > 0 {
		p.Data = make(map[string]string)
		for k, v := range params.Data {
			p.Data[k] = v
		}
		err := s.loadBackendInfo(ctx, true)
		if err != nil {
			return errors.Trace(err)
		}
		// loadBackendInfo will error is there's no active backend.
		backend := s.backends[s.activeBackendID]

		latestRevision, err := s.secretState.GetLatestRevision(ctx, uri)
		if err != nil {
			// Check if the uri exists or not.
			return errors.Trace(err)
		}
		revId, err := backend.SaveContent(ctx, uri, latestRevision+1, secrets.NewSecretValue(params.Data))
		if err != nil && !errors.Is(err, errors.NotSupported) {
			return errors.Annotatef(err, "saving secret content to backend")
		}
		if err == nil {
			defer func() {
				if errOut != nil {
					// If we failed to update the secret, we should delete the
					// secret value from the backend for the new revision.
					if err2 := backend.DeleteContent(ctx, revId); err2 != nil &&
						!errors.Is(err2, errors.NotSupported) &&
						!errors.Is(err2, secreterrors.SecretRevisionNotFound) {
						s.logger.Warningf("failed to delete secret %q: %v", revId, err2)
					}
				}
			}()
			p.Data = nil
			p.ValueRef = &secrets.ValueRef{
				BackendID:  s.activeBackendID,
				RevisionID: revId,
			}
		}
	}

	if p.ValueRef != nil || len(p.Data) != 0 {
		revisionID, err := s.uuidGenerator()
		if err != nil {
			return errors.Trace(err)
		}
		p.RevisionID = ptr(revisionID.String())
		rollBack, err := s.secretBackendReferenceMutator.AddSecretBackendReference(ctx, p.ValueRef, s.modelID, revisionID)
		if err != nil {
			return errors.Trace(err)
		}
		defer func() {
			if errOut != nil {
				if err := rollBack(); err != nil {
					s.logger.Warningf("failed to roll back secret reference count: %v", err)
				}
			}
		}()
	}

	if err := s.secretState.UpdateSecret(ctx, uri, p); err != nil {
		return errors.Annotatef(err, "cannot update user secret %q", uri.ID)
	}
	return nil
}

// UpdateCharmSecret updates a charm secret with the specified parameters, returning an error
// satisfying [secreterrors.SecretNotFound] if the secret does not exist.
// It also returns an error satisfying [secreterrors.SecretLabelAlreadyExists] if
// the secret owner already has a secret with the same label.
// It returns [secreterrors.PermissionDenied] if the secret cannot be managed by the accessor.
func (s *SecretService) UpdateCharmSecret(ctx context.Context, uri *secrets.URI, params UpdateCharmSecretParams) (errOut error) {
	if len(params.Data) > 0 && params.ValueRef != nil {
		return errors.New("must specify either content or a value reference but not both")
	}

	if err := s.canManage(ctx, uri, params.Accessor, params.LeaderToken); err != nil {
		return errors.Trace(err)
	}

	p := domainsecret.UpsertSecretParams{
		Description: params.Description,
		Label:       params.Label,
		ValueRef:    params.ValueRef,
		ExpireTime:  params.ExpireTime,
		Checksum:    params.Checksum,
	}
	rotatePolicy := domainsecret.MarshallRotatePolicy(params.RotatePolicy)
	p.RotatePolicy = &rotatePolicy
	if params.RotatePolicy.WillRotate() {
		policy, err := s.secretState.GetRotatePolicy(ctx, uri)
		if err != nil {
			return errors.Trace(err)
		}
		if !policy.WillRotate() {
			p.NextRotateTime = params.RotatePolicy.NextRotateTime(s.clock.Now())
		}
	}
	if len(params.Data) > 0 {
		p.Data = make(map[string]string)
		for k, v := range params.Data {
			p.Data[k] = v
		}
	}

	if p.ValueRef != nil || len(p.Data) != 0 {
		revisionID, err := s.uuidGenerator()
		if err != nil {
			return errors.Trace(err)
		}
		p.RevisionID = ptr(revisionID.String())
		rollBack, err := s.secretBackendReferenceMutator.AddSecretBackendReference(ctx, p.ValueRef, s.modelID, revisionID)
		if err != nil {
			return errors.Trace(err)
		}
		defer func() {
			if errOut != nil {
				if err := rollBack(); err != nil {
					s.logger.Warningf("failed to roll back secret reference count: %v", err)
				}
			}
		}()
	}

	err := s.secretState.UpdateSecret(ctx, uri, p)
	if errors.Is(err, secreterrors.SecretLabelAlreadyExists) {
		return errors.Errorf("secret with label %q is already being used", *params.Label)
	}
	if err != nil {
		return errors.Annotatef(err, "cannot update charm secret %q", uri.ID)
	}
	return nil
}

// ListSecrets returns the secrets matching the specified terms.
// If multiple values for a given term are specified, secrets matching any of the
// values for that term are included.
func (s *SecretService) ListSecrets(ctx context.Context, uri *secrets.URI,
	revision *int,
	labels domainsecret.Labels,
) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error) {
	return s.secretState.ListSecrets(ctx, uri, revision, labels)
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
	return s.secretState.ListCharmSecrets(ctx, appOwners, unitOwners)
}

// GetSecret returns the secret with the specified URI.
// If returns [secreterrors.SecretNotFound] is there's no such secret.
func (s *SecretService) GetSecret(ctx context.Context, uri *secrets.URI) (*secrets.SecretMetadata, error) {
	return s.secretState.GetSecret(ctx, uri)
}

// GetUserSecretURIByLabel returns the user secret URI with the specified label.
// If returns [secreterrors.SecretNotFound] is there's no such secret.
func (s *SecretService) GetUserSecretURIByLabel(ctx context.Context, label string) (*secrets.URI, error) {
	return s.secretState.GetUserSecretURIByLabel(ctx, label)
}

// ListCharmSecretsToDrain returns secret drain revision info for
// the secrets owned by the specified apps and units.
func (s *SecretService) ListCharmSecretsToDrain(
	ctx context.Context,
	owners ...CharmSecretOwner,
) ([]*secrets.SecretMetadataForDrain, error) {
	appOwners, unitOwners := splitCharmSecretOwners(owners...)
	return s.secretState.ListCharmSecretsToDrain(ctx, appOwners, unitOwners)
}

// ListUserSecretsToDrain returns secret drain revision info for any user secrets.
func (s *SecretService) ListUserSecretsToDrain(ctx context.Context) ([]*secrets.SecretMetadataForDrain, error) {
	return s.secretState.ListUserSecretsToDrain(ctx)
}

// GetSecretValue returns the value of the specified secret revision.
// If returns [secreterrors.SecretRevisionNotFound] is there's no such secret revision.
func (s *SecretService) GetSecretValue(ctx context.Context, uri *secrets.URI, rev int, accessor SecretAccessor) (secrets.SecretValue, *secrets.ValueRef, error) {
	if err := s.canRead(ctx, uri, accessor); err != nil {
		return nil, nil, errors.Trace(err)
	}
	data, ref, err := s.secretState.GetSecretValue(ctx, uri, rev)
	return secrets.NewSecretValue(data), ref, errors.Trace(err)
}

// GetSecretContentFromBackend retrieves the content for the specified secret revision.
// If the content is not found, it may be that the secret has been drained so it tries
// again using the new active backend.
func (s *SecretService) GetSecretContentFromBackend(ctx context.Context, uri *secrets.URI, rev int) (secrets.SecretValue, error) {
	if s.activeBackendID == "" {
		err := s.loadBackendInfo(ctx, false)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	lastBackendID := ""
	for {
		data, ref, err := s.secretState.GetSecretValue(ctx, uri, rev)
		val := secrets.NewSecretValue(data)
		if err != nil {
			notFound := errors.Is(err, secreterrors.SecretNotFound) || errors.Is(err, secreterrors.SecretRevisionNotFound)
			if notFound {
				return nil, fmt.Errorf("secret %s revision %d not found%w", uri.ID, rev, errors.Hide(secreterrors.SecretRevisionNotFound))
			}
			return nil, errors.Trace(err)
		}
		if ref == nil {
			return val, nil
		}

		backendID := ref.BackendID
		backend, ok := s.backends[backendID]
		if !ok {
			return nil, fmt.Errorf("external secret backend %q not found, have %q%w", backendID, s.backends, errors.Hide(backenderrors.NotFound))
		}
		val, err = backend.GetContent(ctx, ref.RevisionID)
		notFound := errors.Is(err, secreterrors.SecretNotFound) || errors.Is(err, secreterrors.SecretRevisionNotFound)
		if err == nil || !notFound || lastBackendID == backendID {
			if notFound {
				return nil, fmt.Errorf("secret %s revision %d not found%w", uri.ID, rev, errors.Hide(secreterrors.SecretRevisionNotFound))
			}
			return val, errors.Trace(err)
		}
		lastBackendID = backendID
		// Secret may have been drained to the active backend.
		if backendID != s.activeBackendID {
			continue
		}
		// The active backend may have changed.
		if initErr := s.loadBackendInfo(ctx, false); initErr != nil {
			return nil, errors.Trace(initErr)
		}
		if s.activeBackendID == backendID {
			return nil, errors.Trace(err)
		}
	}
}

// ProcessCharmSecretConsumerLabel takes a secret consumer and a uri and label which have been used to consumer the secret.
// If the uri is empty, the label and consumer are used to lookup the consumed secret uri.
// This method returns the resulting uri, and optionally the label to update for the consumer.
func (s *SecretService) ProcessCharmSecretConsumerLabel(
	ctx context.Context, unitName string, uri *secrets.URI, label string, token leadership.Token,
) (_ *secrets.URI, _ *string, err error) {
	modelUUID, err := s.secretState.GetModelUUID(ctx)
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
			if labelToUpdate != nil && *labelToUpdate != md.Label {
				isOwner, err := checkUnitOwner(unitName, md.Owner, token)
				if err != nil {
					return nil, nil, errors.Trace(err)
				}
				if isOwner {
					// TODO(secrets) - this should be updated when the consumed revision is looked up
					// but if the secret is a cross model secret, we get the content from the other
					// model and don't do the update. The logic should be reworked so local lookups
					// can ge done in a single txn.
					// Update the label.
					err := s.UpdateCharmSecret(ctx, uri, UpdateCharmSecretParams{
						LeaderToken: token,
						Label:       &label,
						Accessor: SecretAccessor{
							Kind: UnitAccessor,
							ID:   unitName,
						},
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
			// 1. owners use owner label;
			// 2. the leader and peer units use the owner label for application-owned secrets.
			// So, no need to update the consumer label.
			labelToUpdate = nil
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

func checkUnitOwner(unitName string, owner secrets.Owner, token leadership.Token) (bool, error) {
	if owner.Kind == secrets.UnitOwner && owner.ID == unitName {
		return true, nil
	}
	// Only unit leaders can "own" application secrets.
	if token == nil {
		return false, secreterrors.PermissionDenied
	}
	if err := token.Check(); err != nil {
		if leadership.IsNotLeaderError(err) {
			return false, nil
		}
		return false, errors.Trace(err)
	}
	return true, nil
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
// It returns [secreterrors.SecretNotFound] is there's no such secret.
// It returns [secreterrors.PermissionDenied] if the secret cannot be managed by the accessor.
func (s *SecretService) ChangeSecretBackend(ctx context.Context, uri *secrets.URI, revision int, params ChangeSecretBackendParams) (errOut error) {
	if err := s.canManage(ctx, uri, params.Accessor, params.LeaderToken); err != nil {
		return errors.Trace(err)
	}
	revisionIDStr, err := s.secretState.GetSecretRevisionID(ctx, uri, revision)
	if err != nil {
		return errors.Trace(err)
	}
	revisionID, err := uuid.UUIDFromString(revisionIDStr)
	if err != nil {
		return errors.Trace(err)
	}

	rollBack, err := s.secretBackendReferenceMutator.UpdateSecretBackendReference(ctx, params.ValueRef, s.modelID, revisionID)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		if errOut != nil {
			if err := rollBack(); err != nil {
				s.logger.Warningf("failed to roll back secret reference count: %v", err)
			}
		}
	}()

	err = s.secretState.ChangeSecretBackend(ctx, revisionID, params.ValueRef, params.Data)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// SecretRotated rotates the secret with the specified URI.
func (s *SecretService) SecretRotated(ctx context.Context, uri *secrets.URI, params SecretRotatedParams) error {
	if err := s.canManage(ctx, uri, params.Accessor, params.LeaderToken); err != nil {
		return errors.Trace(err)
	}

	info, err := s.secretState.GetRotationExpiryInfo(ctx, uri)
	if err != nil {
		return errors.Trace(err)
	}
	if !info.RotatePolicy.WillRotate() {
		s.logger.Debugf("secret %q was rotated but now is set to not rotate")
		return nil
	}
	lastRotateTime := info.NextRotateTime
	if lastRotateTime == nil {
		now := s.clock.Now()
		lastRotateTime = &now
	}
	nextRotateTime := *info.RotatePolicy.NextRotateTime(*lastRotateTime)
	s.logger.Debugf("secret %q was rotated: rev was %d, now %d", uri.ID, params.OriginalRevision, info.LatestRevision)
	// If the secret will expire before it is due to be next rotated, rotate sooner to allow
	// the charm a chance to update it before it expires.
	willExpire := info.LatestExpireTime != nil && info.LatestExpireTime.Before(nextRotateTime)
	forcedRotateTime := lastRotateTime.Add(secrets.RotateRetryDelay)
	if willExpire {
		s.logger.Warningf("secret %q rev %d will expire before next scheduled rotation", uri.ID, info.LatestRevision)
	}
	if willExpire && forcedRotateTime.Before(*info.LatestExpireTime) || !params.Skip && info.LatestRevision == params.OriginalRevision {
		nextRotateTime = forcedRotateTime
	}
	s.logger.Debugf("secret %q next rotate time is now: %s", uri.ID, nextRotateTime.UTC().Format(time.RFC3339))
	return s.secretState.SecretRotated(ctx, uri, nextRotateTime)
}
