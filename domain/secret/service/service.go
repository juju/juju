// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/clock"

	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	backenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/uuid"
)

// NewSecretService returns a new secret service wrapping the specified state.
func NewSecretService(
	secretState State,
	secretBackendState SecretBackendState,
	leaderEnsurer leadership.Ensurer,
	logger logger.Logger,
) *SecretService {
	return &SecretService{
		secretState:        secretState,
		secretBackendState: secretBackendState,
		providerGetter:     provider.Provider,
		leaderEnsurer:      leaderEnsurer,
		uuidGenerator:      uuid.NewUUID,
		clock:              clock.WallClock,
		logger:             logger,
	}
}

// ProviderGetter is a func used to get a secret backend provider for a specified type.
type ProviderGetter func(backendType string) (provider.SecretBackendProvider, error)

// SecretService provides the API for working with secrets.
type SecretService struct {
	secretState        State
	secretBackendState SecretBackendState

	providerGetter ProviderGetter

	activeBackendID string
	backends        map[string]provider.SecretsBackend
	uuidGenerator   func() (uuid.UUID, error)

	leaderEnsurer leadership.Ensurer

	clock  clock.Clock
	logger logger.Logger
}

// CreateSecretURIs returns the specified number of new secret URIs.
func (s *SecretService) CreateSecretURIs(ctx context.Context, count int) ([]*secrets.URI, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if count <= 0 {
		return nil, errors.Errorf("secret URi count %d %w", count, coreerrors.NotValid)
	}

	modelUUID, err := s.secretState.GetModelUUID(ctx)
	if err != nil {
		return nil, errors.Errorf("getting model uuid: %w", err)
	}
	result := make([]*secrets.URI, count)
	for i := 0; i < count; i++ {
		result[i] = secrets.NewURI().WithSource(modelUUID.String())
	}
	return result, nil
}

func (s *SecretService) getBackend(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
	p, err := s.providerGetter(cfg.BackendType)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return p.NewBackend(cfg)
}

func (s *SecretService) getBackendForUserSecrets(ctx context.Context, accessor SecretAccessor) (provider.SecretsBackend, string, error) {
	modelUUID, err := s.secretState.GetModelUUID(ctx)
	if err != nil {
		return nil, "", errors.Errorf("getting model UUID: %w", err)
	}

	activeBackendID, modelBackendCfg, err := s.secretBackendState.GetActiveModelSecretBackend(ctx, modelUUID)
	if err != nil {
		return nil, "", errors.Errorf("getting model secret backend: %w", err)
	}

	p, err := s.providerGetter(modelBackendCfg.BackendType)
	if err != nil {
		return nil, "", errors.Capture(err)
	}
	err = p.Initialise(modelBackendCfg)
	if err != nil {
		return nil, "", errors.Errorf("initialising secrets provider: %w", err)
	}

	revInfo, err := s.ListGrantedSecretsForBackend(ctx, activeBackendID, secrets.RoleManage, accessor)
	if err != nil {
		return nil, "", errors.Errorf("listing granted secrets: %w", err)
	}
	ownedRevisions := provider.SecretRevisions{}
	for _, r := range revInfo {
		ownedRevisions.Add(r.URI, r.RevisionID)
	}
	s.logger.Debugf(ctx, "secrets for %s:\nowned: %v", accessor, ownedRevisions)

	// Get the restricted config for the provided accessor.
	restrictedConfig, err := p.RestrictedConfig(ctx, modelBackendCfg, true, false, secrets.Accessor{
		Kind: secrets.ModelAccessor,
		ID:   accessor.ID,
	}, ownedRevisions, provider.SecretRevisions{})
	if err != nil {
		return nil, "", errors.Capture(err)
	}

	info := &provider.ModelBackendConfig{
		ControllerUUID: modelBackendCfg.ControllerUUID,
		ModelUUID:      modelBackendCfg.ModelUUID,
		ModelName:      modelBackendCfg.ModelName,
		BackendConfig:  *restrictedConfig,
	}
	sb, err := p.NewBackend(info)
	if err != nil {
		return nil, "", errors.Capture(err)
	}
	return sb, activeBackendID, nil
}

func (s *SecretService) loadBackendInfo(ctx context.Context, activeOnly bool) error {
	modelUUID, err := s.secretState.GetModelUUID(ctx)
	if err != nil {
		return errors.Errorf("getting model UUID: %w", err)
	}

	modelBackend, err := s.secretBackendState.GetModelSecretBackendDetails(ctx, modelUUID)
	if err != nil {
		return errors.Errorf("getting model secret backend: %w", err)
	}
	s.activeBackendID = modelBackend.SecretBackendID

	backends, err := s.secretBackendState.ListSecretBackendsForModel(ctx, modelUUID, true)
	if err != nil {
		return errors.Errorf("listing secret backends: %w", err)
	}

	s.backends = make(map[string]provider.SecretsBackend)
	for _, b := range backends {
		if activeOnly && b.ID != s.activeBackendID {
			continue
		}

		cfg := provider.ModelBackendConfig{
			ControllerUUID: modelBackend.ControllerUUID,
			ModelUUID:      modelUUID.String(),
			ModelName:      modelBackend.ModelName,
			BackendConfig: provider.BackendConfig{
				BackendType: b.BackendType,
				Config:      b.Config,
			},
		}

		s.backends[b.ID], err = s.getBackend(&cfg)
		if err != nil {
			if b.ID != s.activeBackendID && cfg.BackendType == kubernetes.BackendType {
				// TODO(secrets) - on an iaas controller, attempting to get the "model" k8s backend fails
				// The root cause is not filtering backends to those that are in use.
				s.logger.Debugf(ctx, "failed to load backend info for id %s (%s): %v", b.ID, cfg.BackendType, err)
				continue
			}
			return errors.Errorf("acquiring secret backend %s: %w", b.ID, err)
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(errOut)
		span.End()
	}()

	if len(params.Data) == 0 {
		return errors.Errorf("empty secret value %w", coreerrors.NotValid)
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

	backend, backendID, err := s.getBackendForUserSecrets(ctx, params.Accessor)
	if err != nil {
		return errors.Capture(err)
	}

	revId, err := backend.SaveContent(ctx, uri, 1, secrets.NewSecretValue(params.Data))
	if err != nil && !errors.Is(err, coreerrors.NotSupported) {
		return errors.Errorf("saving secret content to backend: %w", err)
	}
	if err == nil {
		defer func() {
			if errOut != nil {
				// If we failed to create the secret, we should delete the
				// secret value from the backend.
				if err2 := backend.DeleteContent(ctx, revId); err2 != nil &&
					!errors.Is(err2, coreerrors.NotSupported) &&
					!errors.Is(err2, secreterrors.SecretRevisionNotFound) {
					s.logger.Warningf(ctx, "failed to delete secret %q: %v", revId, err2)
				}
			}
		}()
		p.Data = nil
		p.ValueRef = &secrets.ValueRef{
			BackendID:  backendID,
			RevisionID: revId,
		}
	}
	revisionID, err := s.uuidGenerator()
	if err != nil {
		return errors.Capture(err)
	}
	p.RevisionID = ptr(revisionID.String())

	modelID, err := s.secretState.GetModelUUID(ctx)
	if err != nil {
		return errors.Errorf("getting model uuid: %w", err)
	}
	rollBack, err := s.secretBackendState.AddSecretBackendReference(ctx, p.ValueRef, modelID, revisionID.String())
	if err != nil {
		return errors.Capture(err)
	}
	defer func() {
		if errOut != nil {
			if err := rollBack(); err != nil {
				s.logger.Warningf(ctx, "failed to roll back secret reference count: %v", err)
			}
		}
	}()

	if err = s.secretState.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return s.createSecret(ctx, params.Version, uri, secrets.Owner{Kind: secrets.ModelOwner}, p)
	}); err != nil {
		return errors.Errorf("creating user secret %q: %w", uri.ID, err)
	}
	return nil
}

func ptr[T any](s T) *T {
	return &s
}

// CreateCharmSecret creates a charm secret with the specified parameters,
// returning an error satisfying [secreterrors.SecretLabelAlreadyExists] if the
// secret owner already has a secret with the same label.
func (s *SecretService) CreateCharmSecret(ctx context.Context, uri *secrets.URI, params CreateCharmSecretParams) (errOut error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(errOut)
		span.End()
	}()

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
		return errors.Capture(err)
	}
	p.RevisionID = ptr(revisionID.String())

	modelID, err := s.secretState.GetModelUUID(ctx)
	if err != nil {
		return errors.Errorf("getting model uuid: %w", err)
	}
	rollBack, err := s.secretBackendState.AddSecretBackendReference(ctx, p.ValueRef, modelID, revisionID.String())
	if err != nil {
		return errors.Capture(err)
	}
	defer func() {
		if errOut != nil {
			if err := rollBack(); err != nil {
				s.logger.Warningf(ctx, "failed to roll back secret reference count: %v", err)
			}
		}
	}()
	if params.CharmOwner.Kind == ApplicationOwner {
		unitName, err := coreunit.NewName(params.Accessor.ID)
		if err != nil {
			return errors.Capture(err)
		}
		appName := unitName.Application()
		if err := s.leaderEnsurer.LeadershipCheck(appName, params.Accessor.ID).Check(); err != nil {
			if leadership.IsNotLeaderError(err) {
				return secreterrors.PermissionDenied
			}
			return errors.Capture(err)
		}
	}

	err = s.secretState.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		owner := secrets.Owner{
			ID:   params.CharmOwner.ID,
			Kind: secrets.OwnerKind(params.CharmOwner.Kind),
		}
		return s.createSecret(ctx, params.Version, uri, owner, p)
	})
	if err != nil {
		return errors.Errorf("cannot create charm secret %q: %w", uri.ID, err)
	}
	return nil
}

// UpdateUserSecret updates a user secret with the specified parameters, returning an error
// satisfying [secreterrors.SecretNotFound] if the secret does not exist.
// It also returns an error satisfying [secreterrors.SecretLabelAlreadyExists] if
// the secret owner already has a secret with the same label.
// It returns [secreterrors.PermissionDenied] if the secret cannot be managed by the accessor.
func (s *SecretService) UpdateUserSecret(ctx context.Context, uri *secrets.URI, params UpdateUserSecretParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	withCaveat, err := s.getManagementCaveat(ctx, uri, params.Accessor)
	if err != nil {
		return errors.Capture(err)
	}

	p := domainsecret.UpsertSecretParams{
		Description: params.Description,
		Label:       params.Label,
		AutoPrune:   params.AutoPrune,
		Checksum:    params.Checksum,
	}

	return withCaveat(ctx, func(innerCtx context.Context) (errOut error) {
		// Take a copy as we may set it to nil below
		// if the content is saved to a backend.
		if len(params.Data) > 0 {
			p.Data = make(map[string]string)
			for k, v := range params.Data {
				p.Data[k] = v
			}

			backend, backendID, err := s.getBackendForUserSecrets(innerCtx, params.Accessor)
			if err != nil {
				return errors.Capture(err)
			}

			latestRevision, err := s.secretState.GetLatestRevision(innerCtx, uri)
			if err != nil {
				// Check if the uri exists or not.
				return errors.Capture(err)
			}
			revId, err := backend.SaveContent(innerCtx, uri, latestRevision+1, secrets.NewSecretValue(params.Data))
			if err != nil && !errors.Is(err, coreerrors.NotSupported) {
				return errors.Errorf("saving secret content to backend: %w", err)
			}
			if err == nil {
				defer func() {
					if errOut != nil {
						// If we failed to update the secret, we should delete the
						// secret value from the backend for the new revision.
						if err2 := backend.DeleteContent(innerCtx, revId); err2 != nil &&
							!errors.Is(err2, coreerrors.NotSupported) &&
							!errors.Is(err2, secreterrors.SecretRevisionNotFound) {
							s.logger.Warningf(ctx, "failed to delete secret %q: %v", revId, err2)
						}
					}
				}()
				p.Data = nil
				p.ValueRef = &secrets.ValueRef{
					BackendID:  backendID,
					RevisionID: revId,
				}
			}
		}

		if p.ValueRef != nil || len(p.Data) != 0 {
			revisionID, err := s.uuidGenerator()
			if err != nil {
				return errors.Capture(err)
			}
			p.RevisionID = ptr(revisionID.String())

			modelID, err := s.secretState.GetModelUUID(innerCtx)
			if err != nil {
				return errors.Errorf("getting model uuid: %w", err)
			}
			rollBack, err := s.secretBackendState.AddSecretBackendReference(
				innerCtx, p.ValueRef, modelID, revisionID.String())
			if err != nil {
				return errors.Capture(err)
			}
			defer func() {
				if errOut != nil {
					if err := rollBack(); err != nil {
						s.logger.Warningf(ctx, "failed to roll back secret reference count: %v", err)
					}
				}
			}()
		}

		// TODO (manadart 2024-11-29): This context naming is nasty,
		// but will be removed with RunAtomic.
		err := s.secretState.RunAtomic(innerCtx, func(innerInnerCtx domain.AtomicContext) error {
			return s.updateSecret(innerInnerCtx, uri, p)
		})
		if err != nil {
			return errors.Errorf("updating user secret %q: %w", uri.ID, err)
		}
		return nil
	})
}

// UpdateCharmSecret updates a charm secret with the specified parameters, returning an error
// satisfying [secreterrors.SecretNotFound] if the secret does not exist.
// It also returns an error satisfying [secreterrors.SecretLabelAlreadyExists] if
// the secret owner already has a secret with the same label.
// It returns [secreterrors.PermissionDenied] if the secret cannot be managed by the accessor.
func (s *SecretService) UpdateCharmSecret(ctx context.Context, uri *secrets.URI, params UpdateCharmSecretParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(params.Data) > 0 && params.ValueRef != nil {
		return errors.New("must specify either content or a value reference but not both")
	}

	withCaveat, err := s.getManagementCaveat(ctx, uri, params.Accessor)
	if err != nil {
		return errors.Capture(err)
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
			return errors.Capture(err)
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

	return withCaveat(ctx, func(innerCtx context.Context) (errOut error) {
		if p.ValueRef != nil || len(p.Data) != 0 {
			revisionID, err := s.uuidGenerator()
			if err != nil {
				return errors.Capture(err)
			}
			p.RevisionID = ptr(revisionID.String())

			modelID, err := s.secretState.GetModelUUID(innerCtx)
			if err != nil {
				return errors.Errorf("getting model uuid: %w", err)
			}
			rollBack, err := s.secretBackendState.AddSecretBackendReference(
				innerCtx, p.ValueRef, modelID, revisionID.String())
			if err != nil {
				return errors.Capture(err)
			}
			defer func() {
				if errOut != nil {
					if err := rollBack(); err != nil {
						s.logger.Warningf(ctx, "failed to roll back secret reference count: %v", err)
					}
				}
			}()
		}

		// TODO (manadart 2024-11-29): This context naming is nasty,
		// but will be removed with RunAtomic.
		err := s.secretState.RunAtomic(innerCtx, func(innerInnerCtx domain.AtomicContext) error {
			return s.updateSecret(innerInnerCtx, uri, p)
		})
		if err != nil {
			return errors.Errorf("cannot update charm secret %q: %w", uri.ID, err)
		}
		return nil
	})
}

func (s *SecretService) createSecret(
	ctx domain.AtomicContext, version int, uri *secrets.URI, owner secrets.Owner, params domainsecret.UpsertSecretParams,
) (err error) {
	defer func() {
		if err != nil {
			if errors.Is(err, secreterrors.SecretLabelAlreadyExists) {
				err = errors.Errorf("secret with label %q is already being used: %w", *params.Label, secreterrors.SecretLabelAlreadyExists)
			}
		}
	}()

	var createSecret func() error
	var (
		labelExists bool
		labelErr    error
	)
	switch kind := owner.Kind; kind {
	case secrets.ApplicationOwner:
		appUUID, err := s.secretState.GetApplicationUUID(ctx, owner.ID)
		if err != nil {
			return errors.Capture(err)
		}
		if params.Label != nil && *params.Label != "" {
			labelExists, labelErr = s.secretState.CheckApplicationSecretLabelExists(ctx, appUUID, *params.Label)
		}
		createSecret = func() error { return s.secretState.CreateCharmApplicationSecret(ctx, version, uri, appUUID, params) }
	case secrets.UnitOwner:
		unitName, err := coreunit.NewName(owner.ID)
		if err != nil {
			return errors.Capture(err)
		}
		unitUUID, err := s.secretState.GetUnitUUID(ctx, unitName)
		if err != nil {
			return errors.Capture(err)
		}
		if params.Label != nil && *params.Label != "" {
			labelExists, labelErr = s.secretState.CheckUnitSecretLabelExists(ctx, unitUUID, *params.Label)
		}
		createSecret = func() error { return s.secretState.CreateCharmUnitSecret(ctx, version, uri, unitUUID, params) }
	case secrets.ModelOwner:
		if params.Label != nil && *params.Label != "" {
			labelExists, labelErr = s.secretState.CheckUserSecretLabelExists(ctx, *params.Label)
		}
		createSecret = func() error { return s.secretState.CreateUserSecret(ctx, version, uri, params) }
	default:
		// Should never happen.
		return errors.Errorf("unexpected secret owner kind %q for secret %q", kind, uri.ID)
	}

	if labelErr != nil {
		return errors.Capture(err)
	}
	if labelExists {
		return errors.Errorf("secret with label %q is already being used: %w", *params.Label, secreterrors.SecretLabelAlreadyExists)
	}
	return errors.Capture(createSecret())
}

func (s *SecretService) updateSecret(ctx domain.AtomicContext, uri *secrets.URI, params domainsecret.UpsertSecretParams) error {
	if params.Label != nil && *params.Label != "" {
		// Check to be sure a duplicate label won't be used.
		owner, err := s.secretState.GetSecretOwner(ctx, uri)
		if err != nil {
			return errors.Capture(err)
		}
		var labelExists bool
		switch kind := owner.Kind; kind {
		case domainsecret.ApplicationOwner:
			labelExists, err = s.secretState.CheckApplicationSecretLabelExists(ctx, coreapplication.ID(owner.UUID), *params.Label)
		case domainsecret.UnitOwner:
			labelExists, err = s.secretState.CheckUnitSecretLabelExists(ctx, coreunit.UUID(owner.UUID), *params.Label)
		case domainsecret.ModelOwner:
			labelExists, err = s.secretState.CheckUserSecretLabelExists(ctx, *params.Label)
		default:
			// Should never happen.
			return errors.Errorf("unexpected secret owner kind %q for secret %q", kind, uri.ID)
		}
		if err != nil {
			return errors.Capture(err)
		}
		if labelExists {
			return errors.Errorf("secret with label %q is already being used: %w", *params.Label, secreterrors.SecretLabelAlreadyExists)
		}
	}
	err := s.secretState.UpdateSecret(ctx, uri, params)
	return errors.Capture(err)
}

// ListSecrets returns the secrets matching the specified terms.
// If multiple values for a given term are specified, secrets matching any of the
// values for that term are included.
func (s *SecretService) ListSecrets(ctx context.Context, uri *secrets.URI,
	revision *int,
	labels domainsecret.Labels,
) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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
func (s *SecretService) ListCharmSecrets(
	ctx context.Context, owners ...CharmSecretOwner,
) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appOwners, unitOwners := splitCharmSecretOwners(owners...)
	return s.secretState.ListCharmSecrets(ctx, appOwners, unitOwners)
}

// GetSecret returns the secret with the specified URI.
// If returns [secreterrors.SecretNotFound] is there's no such secret.
func (s *SecretService) GetSecret(ctx context.Context, uri *secrets.URI) (*secrets.SecretMetadata, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.secretState.GetSecret(ctx, uri)
}

// GetUserSecretURIByLabel returns the user secret URI with the specified label.
// If returns [secreterrors.SecretNotFound] is there's no such secret.
func (s *SecretService) GetUserSecretURIByLabel(ctx context.Context, label string) (*secrets.URI, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.secretState.GetUserSecretURIByLabel(ctx, label)
}

// ListCharmSecretsToDrain returns secret drain revision info for
// the secrets owned by the specified apps and units.
func (s *SecretService) ListCharmSecretsToDrain(
	ctx context.Context,
	owners ...CharmSecretOwner,
) ([]*secrets.SecretMetadataForDrain, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appOwners, unitOwners := splitCharmSecretOwners(owners...)
	return s.secretState.ListCharmSecretsToDrain(ctx, appOwners, unitOwners)
}

// ListUserSecretsToDrain returns secret drain revision info for any user secrets.
func (s *SecretService) ListUserSecretsToDrain(ctx context.Context) ([]*secrets.SecretMetadataForDrain, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.secretState.ListUserSecretsToDrain(ctx)
}

// GetSecretValue returns the value of the specified secret revision.
// If returns [secreterrors.SecretRevisionNotFound] is there's no such secret revision.
func (s *SecretService) GetSecretValue(ctx context.Context, uri *secrets.URI, rev int, accessor SecretAccessor) (secrets.SecretValue, *secrets.ValueRef, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.canRead(ctx, uri, accessor); err != nil {
		return nil, nil, errors.Capture(err)
	}
	data, ref, err := s.secretState.GetSecretValue(ctx, uri, rev)
	return secrets.NewSecretValue(data), ref, errors.Capture(err)
}

// GetSecretContentFromBackend retrieves the content for the specified secret revision.
// If the content is not found, it may be that the secret has been drained so it tries
// again using the new active backend.
func (s *SecretService) GetSecretContentFromBackend(ctx context.Context, uri *secrets.URI, rev int) (secrets.SecretValue, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if s.activeBackendID == "" {
		err := s.loadBackendInfo(ctx, false)
		if err != nil {
			return nil, errors.Capture(err)
		}
	}
	lastBackendID := ""
	for {
		data, ref, err := s.secretState.GetSecretValue(ctx, uri, rev)
		val := secrets.NewSecretValue(data)
		if err != nil {
			notFound := errors.Is(err, secreterrors.SecretNotFound) || errors.Is(err, secreterrors.SecretRevisionNotFound)
			if notFound {
				return nil, errors.Errorf("secret %s revision %d not found", uri.ID, rev).Add(secreterrors.SecretRevisionNotFound)
			}
			return nil, errors.Capture(err)
		}
		if ref == nil {
			return val, nil
		}

		backendID := ref.BackendID
		backend, ok := s.backends[backendID]
		if !ok {
			return nil, errors.Errorf("external secret backend %q not found, have %q", backendID, s.backends).Add(backenderrors.NotFound)
		}
		val, err = backend.GetContent(ctx, ref.RevisionID)
		notFound := errors.Is(err, secreterrors.SecretNotFound) || errors.Is(err, secreterrors.SecretRevisionNotFound)
		if err == nil || !notFound || lastBackendID == backendID {
			if notFound {
				return nil, errors.Errorf("secret %s revision %d not found", uri.ID, rev).Add(secreterrors.SecretRevisionNotFound)
			}
			return val, errors.Capture(err)
		}
		lastBackendID = backendID
		// Secret may have been drained to the active backend.
		if backendID != s.activeBackendID {
			continue
		}
		// The active backend may have changed.
		if initErr := s.loadBackendInfo(ctx, false); initErr != nil {
			return nil, errors.Capture(initErr)
		}
		if s.activeBackendID == backendID {
			return nil, errors.Capture(err)
		}
	}
}

// ProcessCharmSecretConsumerLabel takes a secret consumer, a uri and label
// which have been used to consume the secret. If the uri is empty, the label
// and consumer are used to look up the consumed secret uri.
// This method returns the resulting uri, and optionally the label to update for
// the consumer.
func (s *SecretService) ProcessCharmSecretConsumerLabel(
	ctx context.Context, unitName coreunit.Name, uri *secrets.URI, label string,
) (*secrets.URI, *string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	modelUUID, err := s.secretState.GetModelUUID(ctx)
	if err != nil {
		return nil, nil, errors.Errorf("getting model uuid: %w", err)
	}

	// label could be the consumer label for consumers or the owner label for owners.
	var labelToUpdate *string
	if label != "" && uri != nil {
		labelToUpdate = &label
	}

	// For local secrets, check those which may be owned by the caller.
	if uri == nil || uri.IsLocal(modelUUID.String()) {
		md, err := s.getAppOwnedOrUnitOwnedSecretMetadata(ctx, uri, unitName, label)
		if err != nil && !errors.Is(err, secreterrors.SecretNotFound) {
			return nil, nil, errors.Capture(err)
		}
		if md != nil {
			// If the label has is to be changed by the secret owner, update the secret metadata.
			// TODO(wallyworld) - the label staying the same should be asserted in a txn.
			if labelToUpdate != nil && *labelToUpdate != md.Label {
				isOwner, err := checkUnitOwner(unitName.String(), md.Owner, s.leaderEnsurer)
				if err != nil {
					return nil, nil, errors.Capture(err)
				}
				if isOwner {
					// TODO(secrets) - this should be updated when the consumed revision is looked up
					// but if the secret is a cross model secret, we get the content from the other
					// model and don't do the update. The logic should be reworked so local lookups
					// can ge done in a single txn.
					// Update the label.
					err := s.UpdateCharmSecret(ctx, uri, UpdateCharmSecretParams{
						Label: &label,
						Accessor: SecretAccessor{
							Kind: UnitAccessor,
							ID:   unitName.String(),
						},
					})
					if err != nil {
						return nil, nil, errors.Capture(err)
					}
				}
			}
			// 1. secrets can be accessed by the owner;
			// 2. application owned secrets can be accessed by all the units of
			//    the application using owner label or URI.
			uri = md.URI
			// We don't update the consumer label in this case since the label
			// comes from the owner metadata, and we don't want to violate
			// uniqueness checks.
			// 1. owners use owner label;
			// 2. the leader and peer units use the owner label for
			//    application-owned secrets.
			// So, no need to update the consumer label.
			labelToUpdate = nil
		}
	}

	if uri == nil {
		var err error
		uri, err = s.GetURIByConsumerLabel(ctx, label, unitName)
		if errors.Is(err, secreterrors.SecretNotFound) {
			return nil, nil, errors.Errorf("secret URI for consumer label %q %w", label, coreerrors.NotFound)
		}
		if err != nil {
			return nil, nil, errors.Capture(err)
		}
	}
	return uri, labelToUpdate, nil
}

func checkUnitOwner(unitName string, owner secrets.Owner, ensurer leadership.Ensurer) (bool, error) {
	if owner.Kind == secrets.UnitOwner && owner.ID == unitName {
		return true, nil
	}

	// Only unit leaders can "own" application secrets.
	if ensurer == nil {
		return false, secreterrors.PermissionDenied
	}

	if err := ensurer.LeadershipCheck(owner.ID, unitName).Check(); err != nil {
		if leadership.IsNotLeaderError(err) {
			return false, nil
		}
		return false, errors.Capture(err)
	}
	return true, nil
}

func (s *SecretService) getAppOwnedOrUnitOwnedSecretMetadata(
	ctx context.Context, uri *secrets.URI, unitName coreunit.Name, label string,
) (*secrets.SecretMetadata, error) {
	notFoundErr := errors.Errorf("secret %q not found", uri).Add(secreterrors.SecretNotFound)
	if label != "" {
		notFoundErr = errors.Errorf("secret with label %q not found", label).Add(secreterrors.SecretNotFound)
	}

	appName := unitName.Application()
	owners := []CharmSecretOwner{{
		Kind: ApplicationOwner,
		ID:   appName,
	}, {
		Kind: UnitOwner,
		ID:   unitName.String(),
	}}
	metadata, _, err := s.ListCharmSecrets(ctx, owners...)
	if err != nil {
		return nil, errors.Capture(err)
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
func (s *SecretService) ChangeSecretBackend(
	ctx context.Context, uri *secrets.URI, revision int, params ChangeSecretBackendParams,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	withCaveat, err := s.getManagementCaveat(ctx, uri, params.Accessor)
	if err != nil {
		return errors.Capture(err)
	}

	revisionIDStr, err := s.secretState.GetSecretRevisionID(ctx, uri, revision)
	if err != nil {
		return errors.Capture(err)
	}
	revisionID, err := uuid.UUIDFromString(revisionIDStr)
	if err != nil {
		return errors.Capture(err)
	}

	modelID, err := s.secretState.GetModelUUID(ctx)
	if err != nil {
		return errors.Errorf("getting model uuid: %w", err)
	}

	return withCaveat(ctx, func(innerCtx context.Context) (errOut error) {
		rollBack, err := s.secretBackendState.UpdateSecretBackendReference(
			innerCtx, params.ValueRef, modelID, revisionID.String())
		if err != nil {
			return errors.Capture(err)
		}

		defer func() {
			if errOut != nil {
				if err := rollBack(); err != nil {
					s.logger.Warningf(ctx, "failed to roll back secret reference count: %v", err)
				}
			}
		}()

		err = s.secretState.ChangeSecretBackend(innerCtx, revisionID, params.ValueRef, params.Data)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
}

// SecretRotated rotates the secret with the specified URI.
func (s *SecretService) SecretRotated(ctx context.Context, uri *secrets.URI, params SecretRotatedParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	withCaveat, err := s.getManagementCaveat(ctx, uri, params.Accessor)
	if err != nil {
		return errors.Capture(err)
	}

	info, err := s.secretState.GetRotationExpiryInfo(ctx, uri)
	if err != nil {
		return errors.Capture(err)
	}
	if !info.RotatePolicy.WillRotate() {
		s.logger.Debugf(ctx, "secret %q was rotated but now is set to not rotate")
		return nil
	}
	lastRotateTime := info.NextRotateTime
	if lastRotateTime == nil {
		now := s.clock.Now()
		lastRotateTime = &now
	}

	nextRotateTime := *info.RotatePolicy.NextRotateTime(*lastRotateTime)
	s.logger.Debugf(ctx, "secret %q was rotated: rev was %d, now %d", uri.ID, params.OriginalRevision, info.LatestRevision)

	// If the secret will expire before it is due to be next rotated, rotate sooner to allow
	// the charm a chance to update it before it expires.
	willExpire := info.LatestExpireTime != nil && info.LatestExpireTime.Before(nextRotateTime)
	forcedRotateTime := lastRotateTime.Add(secrets.RotateRetryDelay)
	if willExpire {
		s.logger.Warningf(ctx, "secret %q rev %d will expire before next scheduled rotation", uri.ID, info.LatestRevision)
	}

	if willExpire && forcedRotateTime.Before(*info.LatestExpireTime) || !params.Skip && info.LatestRevision == params.OriginalRevision {
		nextRotateTime = forcedRotateTime
	}
	s.logger.Debugf(ctx, "secret %q next rotate time is now: %s", uri.ID, nextRotateTime.UTC().Format(time.RFC3339))

	return withCaveat(ctx, func(innerCtx context.Context) (errOut error) {
		return s.secretState.SecretRotated(innerCtx, uri, nextRotateTime)
	})
}
