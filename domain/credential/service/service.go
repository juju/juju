// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	corecredential "github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/credential"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
)

// WatcherFactory instances return a watcher for a specified credential UUID,
type WatcherFactory interface {
	NewValueWatcher(
		namespace, uuid string, changeMask changestream.ChangeType,
	) (watcher.NotifyWatcher, error)
}

// State describes retrieval and persistence methods for credentials.
type State interface {
	ProviderState

	// UpsertCloudCredential adds or updates a cloud credential with the given name, cloud, owner.
	// If the credential already exists, the existing credential's value of Invalid is returned.
	UpsertCloudCredential(ctx context.Context, id corecredential.ID, credential credential.CloudCredentialInfo) (*bool, error)

	// InvalidateCloudCredential marks the cloud credential for the given name, cloud, owner as invalid.
	InvalidateCloudCredential(ctx context.Context, id corecredential.ID, reason string) error

	// CloudCredentialsForOwner returns the owner's cloud credentials for a given cloud,
	// keyed by credential name.
	CloudCredentialsForOwner(ctx context.Context, owner, cloudName string) (map[string]credential.CloudCredentialResult, error)

	// AllCloudCredentialsForOwner returns all cloud credentials stored on the controller
	// for a given owner.
	AllCloudCredentialsForOwner(ctx context.Context, owner string) (map[corecredential.ID]credential.CloudCredentialResult, error)

	// RemoveCloudCredential removes a cloud credential with the given name, cloud, owner.
	RemoveCloudCredential(ctx context.Context, id corecredential.ID) error

	// ModelsUsingCloudCredential returns a map of uuid->name for models which use the credential.
	ModelsUsingCloudCredential(ctx context.Context, id corecredential.ID) (map[coremodel.UUID]string, error)
}

// ValidationContextGetter returns the artefacts for a specified model, used to make credential validation calls.
type ValidationContextGetter func(ctx context.Context, modelUUID coremodel.UUID) (CredentialValidationContext, error)

// Logger facilitates emitting log messages.
type Logger interface {
	Debugf(string, ...interface{})
}

// Service provides the API for working with credentials.
type Service struct {
	st     State
	logger Logger

	// These are set via options after the service is created.

	validationContextGetter ValidationContextGetter
	validator               CredentialValidator

	// TODO(wallyworld) - remove when models are out of mongo
	legacyUpdater func(tag names.CloudCredentialTag) error
	legacyRemover func(tag names.CloudCredentialTag) error
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State, logger Logger) *Service {
	return &Service{
		st:        st,
		logger:    logger,
		validator: NewCredentialValidator(),
	}
}

// WithValidationContextGetter configures the service to use the specified function
// to get a context used to validate a credential for a specified model.
// TODO(wallyworld) - remove when models are out of mongo
func (s *Service) WithValidationContextGetter(validationContextGetter ValidationContextGetter) *Service {
	s.validationContextGetter = validationContextGetter
	return s
}

// WithCredentialValidator configures the service to use the specified
// credential validator.
func (s *Service) WithCredentialValidator(validator CredentialValidator) *Service {
	s.validator = validator
	return s
}

// WithLegacyUpdater configures the service to use the specified function
// to update credential details in mongo.
// TODO(wallyworld) - remove when models are out of mongo
func (s *Service) WithLegacyUpdater(updater func(tag names.CloudCredentialTag) error) *Service {
	s.legacyUpdater = updater
	return s
}

// WithLegacyRemover configures the service to use the specified function
// to remove credential details from mongo.
// TODO(wallyworld) - remove when models are out of mongo
func (s *Service) WithLegacyRemover(remover func(tag names.CloudCredentialTag) error) *Service {
	s.legacyRemover = remover
	return s
}

// CloudCredential returns the cloud credential for the given tag.
func (s *Service) CloudCredential(ctx context.Context, id corecredential.ID) (cloud.Credential, error) {
	if err := id.Validate(); err != nil {
		return cloud.Credential{}, errors.Annotate(err, "invalid id getting cloud credential")
	}
	credInfo, err := s.st.CloudCredential(ctx, id)
	if err != nil {
		return cloud.Credential{}, errors.Trace(err)
	}
	cred := cloud.NewNamedCredential(credInfo.Label, cloud.AuthType(credInfo.AuthType), credInfo.Attributes, credInfo.Revoked)
	cred.Invalid = credInfo.Invalid
	cred.InvalidReason = credInfo.InvalidReason
	return cred, nil
}

// AllCloudCredentialsForOwner returns all cloud credentials stored on the controller
// for a given owner.
func (s *Service) AllCloudCredentialsForOwner(ctx context.Context, owner string) (map[corecredential.ID]cloud.Credential, error) {
	creds, err := s.st.AllCloudCredentialsForOwner(ctx, owner)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make(map[corecredential.ID]cloud.Credential)
	for id, c := range creds {
		result[id] = cloudCredentialFromCredentialResult(c)
	}
	return result, nil
}

// CloudCredentialsForOwner returns the owner's cloud credentials for a given cloud,
// keyed by credential name.
func (s *Service) CloudCredentialsForOwner(ctx context.Context, owner, cloudName string) (map[string]cloud.Credential, error) {
	creds, err := s.st.CloudCredentialsForOwner(ctx, owner, cloudName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make(map[string]cloud.Credential)
	for name, credInfoResult := range creds {
		result[name] = cloudCredentialFromCredentialResult(credInfoResult)
	}
	return result, nil
}

// UpdateCloudCredential adds or updates a cloud credential with the given tag.
func (s *Service) UpdateCloudCredential(ctx context.Context, id corecredential.ID, cred cloud.Credential) error {
	if err := id.Validate(); err != nil {
		return errors.Annotatef(err, "invalid id updating cloud credential")
	}
	_, err := s.st.UpsertCloudCredential(ctx, id, credentialInfoFromCloudCredential(cred))
	return err
}

// RemoveCloudCredential removes a cloud credential with the given tag.
func (s *Service) RemoveCloudCredential(ctx context.Context, id corecredential.ID) error {
	if err := id.Validate(); err != nil {
		return errors.Annotatef(err, "invalid id removing cloud credential")
	}
	return s.st.RemoveCloudCredential(ctx, id)
}

// InvalidateCredential marks the cloud credential for the given name, cloud, owner as invalid.
func (s *Service) InvalidateCredential(ctx context.Context, id corecredential.ID, reason string) error {
	if err := id.Validate(); err != nil {
		return errors.Annotatef(err, "invalid id invalidating cloud credential")
	}
	return s.st.InvalidateCloudCredential(ctx, id, reason)
}

func (s *Service) modelsUsingCredential(ctx context.Context, id corecredential.ID) (map[coremodel.UUID]string, error) {
	models, err := s.st.ModelsUsingCloudCredential(ctx, id)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	return models, nil
}

func (s *Service) validateCredentialForModel(ctx context.Context, modelUUID coremodel.UUID, id corecredential.ID, cred *cloud.Credential) ([]error, error) {
	if s.validator == nil || s.validationContextGetter == nil {
		return nil, errors.Errorf("missing validation helpers")
	}
	validationCtx, err := s.validationContextGetter(ctx, modelUUID)
	if err != nil {
		return []error{errors.Trace(err)}, nil
	}

	modelErrors, err := s.validator.Validate(ctx, validationCtx, id, cred, false)
	if err != nil {
		return []error{errors.Trace(err)}, nil
	}
	return modelErrors, nil
}

// CheckAndUpdateCredential updates the credential after first checking that any models which use the credential
// can still access the cloud resources. If force is true, update the credential even if there are issues
// validating the credential.
// TODO(wallyworld) - the validation getter can be set during service construction once dqlite is used everywhere.
// Note - it is expected that `WithValidationContextGetter` is called to set up the service to have a non-nil
// validationContextGetter prior to calling this function, or else an error will be returned.
// TODO(wallyworld) - we need a strategy to handle changes which occur after the affected models have been read
// but before validation can complete.
func (s *Service) CheckAndUpdateCredential(ctx context.Context, id corecredential.ID, cred cloud.Credential, force bool) ([]UpdateCredentialModelResult, error) {
	if err := id.Validate(); err != nil {
		return nil, errors.Annotatef(err, "invalid id updating cloud credential")
	}

	if s.validationContextGetter == nil {
		return nil, errors.New("cannot validate credential with nil context getter")
	}

	models, err := s.modelsUsingCredential(ctx, id)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var (
		modelsErred  bool
		modelsResult []UpdateCredentialModelResult
	)
	for uuid, name := range models {
		result := UpdateCredentialModelResult{
			ModelUUID: uuid,
			ModelName: name,
		}
		result.Errors, err = s.validateCredentialForModel(ctx, uuid, id, &cred)
		if err != nil {
			return nil, errors.Trace(err)
		}
		modelsResult = append(modelsResult, result)
		if len(result.Errors) > 0 {
			modelsErred = true
		}
	}
	// Since we get a map above, for consistency ensure that models are added
	// sorted by model uuid.
	sort.Slice(modelsResult, func(i, j int) bool {
		return modelsResult[i].ModelUUID < modelsResult[j].ModelUUID
	})

	if modelsErred && !force {
		return modelsResult, credentialerrors.CredentialModelValidation
	}

	existingInvalid, err := s.st.UpsertCloudCredential(ctx, id, credentialInfoFromCloudCredential(cred))
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			err = fmt.Errorf("%w %q for credential %q", credentialerrors.UnknownCloud, id.Name, id.Cloud)
		}
		return nil, errors.Trace(err)
	}
	if s.legacyUpdater == nil || cred.Invalid {
		return modelsResult, nil
	}

	// Credential is valid - revoke the suspended status of any relevant models.

	// TODO(wallyworld) - we still manage models in mongo.
	// This can be removed after models are in dqlite.
	// Existing credential will become valid after this call, and
	// the model status of all models that use it will be reverted.
	if existingInvalid != nil && *existingInvalid {
		tag, err := id.Tag()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if err := s.legacyUpdater(tag); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return modelsResult, nil
}

// CheckAndRevokeCredential removes the credential after first checking that any models which use the credential
// can still access the cloud resources. If force is true, update the credential even if there are issues
// validating the credential.
// TODO(wallyworld) - we need a strategy to handle changes which occur after the affected models have been read
// but before validation can complete.
func (s *Service) CheckAndRevokeCredential(ctx context.Context, id corecredential.ID, force bool) error {
	if err := id.Validate(); err != nil {
		return errors.Annotatef(err, "invalid id revoking cloud credential")
	}

	models, err := s.modelsUsingCredential(ctx, id)
	if err != nil {
		return errors.Trace(err)
	}
	if len(models) != 0 {
		opMessage := "cannot be deleted as"
		if force {
			opMessage = "will be deleted but"
		}
		s.logger.Debugf("credential %v %v it is used by model%v",
			id,
			opMessage,
			modelsPretty(models),
		)
		if !force {
			// Some models still use this credential - do not delete this credential...
			return errors.Errorf("cannot revoke credential %v: it is still used by %d model%v", id, len(models), plural(len(models)))
		}
	}
	err = s.st.RemoveCloudCredential(ctx, id)
	if err != nil || s.legacyRemover == nil {
		return errors.Trace(err)
	} else {
		// If credential was successfully removed, we also want to clear all references to it from the models.
		tag, err := id.Tag()
		if err != nil {
			return errors.Trace(err)
		}
		if err := s.legacyRemover(tag); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// WatchableService provides the API for working with credentials and the
// ability to create watchers.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new service reference wrapping the input state.
func NewWatchableService(st State, watcherFactory WatcherFactory, logger Logger) *WatchableService {
	return &WatchableService{
		Service: Service{
			st:        st,
			logger:    logger,
			validator: NewCredentialValidator(),
		},
		watcherFactory: watcherFactory,
	}
}

// WatchCredential returns a watcher that observes changes to the specified
// credential.
func (s *WatchableService) WatchCredential(ctx context.Context, id corecredential.ID) (watcher.NotifyWatcher, error) {
	if err := id.Validate(); err != nil {
		return nil, errors.Annotatef(err, "invalid id watching cloud credential")
	}
	return s.st.WatchCredential(ctx, s.watcherFactory.NewValueWatcher, id)
}

// WithValidationContextGetter configures the service to use the specified function
// to get a context used to validate a credential for a specified model.
// TODO(wallyworld) - remove when models are out of mongo
func (s *WatchableService) WithValidationContextGetter(validationContextGetter ValidationContextGetter) *WatchableService {
	s.validationContextGetter = validationContextGetter
	return s
}

// WithLegacyUpdater configures the service to use the specified function
// to update credential details in mongo.
// TODO(wallyworld) - remove when models are out of mongo
func (s *WatchableService) WithLegacyUpdater(updater func(tag names.CloudCredentialTag) error) *WatchableService {
	s.legacyUpdater = updater
	return s
}

// WithLegacyRemover configures the service to use the specified function
// to remove credential details from mongo.
// TODO(wallyworld) - remove when models are out of mongo
func (s *WatchableService) WithLegacyRemover(remover func(tag names.CloudCredentialTag) error) *WatchableService {
	s.legacyRemover = remover
	return s
}

func cloudCredentialFromCredentialResult(credInfo credential.CloudCredentialResult) cloud.Credential {
	cred := cloud.NewNamedCredential(credInfo.Label, cloud.AuthType(credInfo.AuthType), credInfo.Attributes, credInfo.Revoked)
	cred.Invalid = credInfo.Invalid
	cred.InvalidReason = credInfo.InvalidReason
	return cred
}

func credentialInfoFromCloudCredential(cred cloud.Credential) credential.CloudCredentialInfo {
	return credential.CloudCredentialInfo{
		AuthType:      string(cred.AuthType()),
		Attributes:    cred.Attributes(),
		Revoked:       cred.Revoked,
		Label:         cred.Label,
		Invalid:       cred.Invalid,
		InvalidReason: cred.InvalidReason,
	}
}

func plural(length int) string {
	if length == 1 {
		return ""
	}
	return "s"
}

func modelsPretty(in map[coremodel.UUID]string) string {
	// map keys are notoriously randomly ordered
	uuids := []string{}
	for uuid := range in {
		uuids = append(uuids, string(uuid))
	}
	sort.Strings(uuids)

	firstLine := ":\n- "
	if len(uuids) == 1 {
		firstLine = " "
	}

	return fmt.Sprintf("%v%v%v",
		plural(len(in)),
		firstLine,
		strings.Join(uuids, "\n- "),
	)
}
