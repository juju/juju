// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/credential"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	"github.com/juju/juju/internal/errors"
)

// WatcherFactory instances return a watcher for a specified credential UUID,
type WatcherFactory interface {
	// NewNotifyWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// State describes retrieval and persistence methods for credentials.
type State interface {
	ProviderState

	// UpsertCloudCredential adds or updates a cloud credential with the given name, cloud, owner.
	UpsertCloudCredential(ctx context.Context, key corecredential.Key, credential credential.CloudCredentialInfo) error

	// InvalidateCloudCredential marks the cloud credential for the given name, cloud, owner as invalid.
	InvalidateCloudCredential(ctx context.Context, key corecredential.Key, reason string) error

	// CloudCredentialsForOwner returns the owner's cloud credentials for a given cloud,
	// keyed by credential name.
	CloudCredentialsForOwner(ctx context.Context, owner user.Name, cloudName string) (map[string]credential.CloudCredentialResult, error)

	// AllCloudCredentialsForOwner returns all cloud credentials stored on the controller
	// for a given owner.
	AllCloudCredentialsForOwner(ctx context.Context, owner user.Name) (map[corecredential.Key]credential.CloudCredentialResult, error)

	// RemoveCloudCredential removes a cloud credential with the given name, cloud, owner.
	RemoveCloudCredential(ctx context.Context, key corecredential.Key) error

	// ModelsUsingCloudCredential returns a map of uuid->name for models which use the credential.
	ModelsUsingCloudCredential(ctx context.Context, key corecredential.Key) (map[coremodel.UUID]string, error)
}

// ValidationContextGetter returns the artefacts for a specified model, used to make credential validation calls.
type ValidationContextGetter func(ctx context.Context, modelUUID coremodel.UUID) (CredentialValidationContext, error)

// Service provides the API for working with credentials.
type Service struct {
	st     State
	logger logger.Logger
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State, logger logger.Logger) *Service {
	return &Service{
		st:     st,
		logger: logger,
	}
}

// CloudCredential returns the cloud credential for the given tag.
func (s *Service) CloudCredential(ctx context.Context, key corecredential.Key) (cloud.Credential, error) {
	if err := key.Validate(); err != nil {
		return cloud.Credential{}, errors.Errorf("invalid id getting cloud credential: %w", err)
	}
	credInfo, err := s.st.CloudCredential(ctx, key)
	if err != nil {
		return cloud.Credential{}, errors.Capture(err)
	}
	cred := cloud.NewNamedCredential(credInfo.Label, cloud.AuthType(credInfo.AuthType), credInfo.Attributes, credInfo.Revoked)
	cred.Invalid = credInfo.Invalid
	cred.InvalidReason = credInfo.InvalidReason
	return cred, nil
}

// AllCloudCredentialsForOwner returns all cloud credentials stored on the controller
// for a given owner.
func (s *Service) AllCloudCredentialsForOwner(ctx context.Context, owner user.Name) (map[corecredential.Key]cloud.Credential, error) {
	creds, err := s.st.AllCloudCredentialsForOwner(ctx, owner)
	if err != nil {
		return nil, errors.Capture(err)
	}
	result := make(map[corecredential.Key]cloud.Credential)
	for id, c := range creds {
		result[id] = cloudCredentialFromCredentialResult(c)
	}
	return result, nil
}

// CloudCredentialsForOwner returns the owner's cloud credentials for a given cloud,
// keyed by credential name.
func (s *Service) CloudCredentialsForOwner(ctx context.Context, owner user.Name, cloudName string) (map[string]cloud.Credential, error) {
	creds, err := s.st.CloudCredentialsForOwner(ctx, owner, cloudName)
	if err != nil {
		return nil, errors.Capture(err)
	}
	result := make(map[string]cloud.Credential)
	for name, credInfoResult := range creds {
		result[name] = cloudCredentialFromCredentialResult(credInfoResult)
	}
	return result, nil
}

// UpdateCloudCredential adds or updates a cloud credential with the given tag.
func (s *Service) UpdateCloudCredential(ctx context.Context, key corecredential.Key, cred cloud.Credential) error {
	if err := key.Validate(); err != nil {
		return errors.Errorf("invalid id updating cloud credential: %w", err)
	}
	return s.st.UpsertCloudCredential(ctx, key, credentialInfoFromCloudCredential(cred))
}

// RemoveCloudCredential removes a cloud credential with the given tag.
func (s *Service) RemoveCloudCredential(ctx context.Context, key corecredential.Key) error {
	if err := key.Validate(); err != nil {
		return errors.Errorf("invalid id removing cloud credential: %w", err)
	}
	return s.st.RemoveCloudCredential(ctx, key)
}

// InvalidateCredential marks the cloud credential for the given name, cloud, owner as invalid.
func (s *Service) InvalidateCredential(ctx context.Context, key corecredential.Key, reason string) error {
	if err := key.Validate(); err != nil {
		return errors.Errorf("invalid id invalidating cloud credential: %w", err)
	}
	return s.st.InvalidateCloudCredential(ctx, key, reason)
}

func (s *Service) modelsUsingCredential(ctx context.Context, key corecredential.Key) (map[coremodel.UUID]string, error) {
	models, err := s.st.ModelsUsingCloudCredential(ctx, key)
	if err != nil && !errors.Is(err, coreerrors.NotFound) {
		return nil, errors.Capture(err)
	}
	return models, nil
}

// CheckAndUpdateCredential updates the credential after first checking that any models which use the credential
// can still access the cloud resources. If force is true, update the credential even if there are issues
// validating the credential.
// TODO(wallyworld) - the validation getter can be set during service construction once dqlite is used everywhere.
// Note - it is expected that `WithValidationContextGetter` is called to set up the service to have a non-nil
// validationContextGetter prior to calling this function, or else an error will be returned.
// TODO(wallyworld) - we need a strategy to handle changes which occur after the affected models have been read
// but before validation can complete.
func (s *Service) CheckAndUpdateCredential(ctx context.Context, key corecredential.Key, cred cloud.Credential, force bool) ([]UpdateCredentialModelResult, error) {
	if err := key.Validate(); err != nil {
		return nil, errors.Errorf("invalid id updating cloud credential: %w", err)
	}

	models, err := s.modelsUsingCredential(ctx, key)
	if err != nil {
		return nil, errors.Capture(err)
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

	err = s.st.UpsertCloudCredential(ctx, key, credentialInfoFromCloudCredential(cred))
	if err != nil {
		if errors.Is(err, coreerrors.NotFound) {
			err = errors.Errorf("%w %q for credential %q", credentialerrors.UnknownCloud, key.Name, key.Cloud)
		}
		return nil, errors.Capture(err)
	}
	return modelsResult, nil
}

// CheckAndRevokeCredential removes the credential after first checking that any models which use the credential
// can still access the cloud resources. If force is true, update the credential even if there are issues
// validating the credential.
// TODO(wallyworld) - we need a strategy to handle changes which occur after the affected models have been read
// but before validation can complete.
func (s *Service) CheckAndRevokeCredential(ctx context.Context, key corecredential.Key, force bool) error {
	if err := key.Validate(); err != nil {
		return errors.Errorf("invalid id revoking cloud credential: %w", err)
	}

	models, err := s.modelsUsingCredential(ctx, key)
	if err != nil {
		return errors.Capture(err)
	}
	if len(models) != 0 {
		opMessage := "cannot be deleted as"
		if force {
			opMessage = "will be deleted but"
		}
		s.logger.Debugf(ctx, "credential %v %v it is used by model%v",
			key,
			opMessage,
			modelsPretty(models),
		)
		if !force {
			// Some models still use this credential - do not delete this credential...
			return errors.Errorf("cannot revoke credential %v: it is still used by %d model%v", key, len(models), plural(len(models)))
		}
	}
	err = s.st.RemoveCloudCredential(ctx, key)
	if err != nil {
		return errors.Capture(err)
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
func NewWatchableService(st State, watcherFactory WatcherFactory, logger logger.Logger) *WatchableService {
	return &WatchableService{
		Service: Service{
			st:     st,
			logger: logger,
		},
		watcherFactory: watcherFactory,
	}
}

// WatchCredential returns a watcher that observes changes to the specified
// credential.
func (s *WatchableService) WatchCredential(ctx context.Context, key corecredential.Key) (watcher.NotifyWatcher, error) {
	if err := key.Validate(); err != nil {
		return nil, errors.Errorf("invalid id watching cloud credential: %w", err)
	}
	return s.st.WatchCredential(ctx, s.watcherFactory.NewNotifyWatcher, key)
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
