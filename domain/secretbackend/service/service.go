// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	internalsecrets "github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
)

// SecretProviderRegistry is a function that returns a secret backend provider for the given backend type.
type SecretProviderRegistry func(backendType string) (provider.SecretBackendProvider, error)

type Service struct {
	st             State
	logger         Logger
	clock          clock.Clock
	controllerUUID string
	registry       SecretProviderRegistry
}

// NewService creates a new Service for interacting with the secret backend state.
func NewService(
	st State, logger Logger,
	controllerUUID string,
	clk clock.Clock,
	registry SecretProviderRegistry,
) *Service {
	return &Service{
		st:             st,
		logger:         logger,
		controllerUUID: controllerUUID,
		clock:          clk,
		registry:       registry,
	}
}

// GetSecretBackendConfigForAdmin returns the secret backend configuration for the given backend ID for an admin user.
func (s *Service) GetSecretBackendConfigForAdmin(
	ctx context.Context, modelConfig *config.Config, cloud cloud.Cloud, cred cloud.Credential,
) (*provider.ModelBackendConfigInfo, error) {
	var info provider.ModelBackendConfigInfo
	info.Configs = make(map[string]provider.ModelBackendConfig)
	// We need to include builtin backends for secret draining and accessing those secrets while drain is in progress.
	// TODO(secrets) - only use those in use by model
	// For now, we'll return all backends on the controller.
	jujuBackendID := s.controllerUUID
	info.Configs[jujuBackendID] = provider.ModelBackendConfig{
		ControllerUUID: s.controllerUUID,
		ModelUUID:      modelConfig.UUID(),
		ModelName:      modelConfig.Name(),
		BackendConfig:  juju.BuiltInConfig(),
	}
	backendName := modelConfig.SecretBackend()
	if backendName == provider.Auto || backendName == provider.Internal {
		info.ActiveID = jujuBackendID
	}

	if modelConfig.Type() == "caas" {
		// TODO: "caas" const?
		spec, err := cloudspec.MakeCloudSpec(cloud, "", &cred)
		if err != nil {
			return nil, domain.CoerceError(err)
		}
		k8sConfig, err := kubernetes.BuiltInConfig(spec)
		if err != nil {
			return nil, domain.CoerceError(err)
		}
		k8sBackendID := modelConfig.UUID()
		info.Configs[k8sBackendID] = provider.ModelBackendConfig{
			ControllerUUID: s.controllerUUID,
			ModelUUID:      modelConfig.UUID(),
			ModelName:      modelConfig.Name(),
			BackendConfig:  *k8sConfig,
		}
		if backendName == provider.Auto {
			info.ActiveID = k8sBackendID
		}
	}

	backends, err := s.st.ListSecretBackends(ctx)
	if err != nil {
		return nil, domain.CoerceError(err)
	}
	for _, b := range backends {
		if b.Name == backendName {
			info.ActiveID = b.ID
		}
		info.Configs[b.ID] = provider.ModelBackendConfig{
			ControllerUUID: s.controllerUUID,
			ModelUUID:      modelConfig.UUID(),
			ModelName:      modelConfig.Name(),
			BackendConfig: provider.BackendConfig{
				BackendType: b.BackendType,
				Config:      b.Config,
			},
		}
	}
	if info.ActiveID == "" {
		return nil, errors.NotFoundf("secret backend %q", backendName)
	}
	return &info, nil
}

// GetSecretBackendConfigLegacy gets the config needed to create a client to secret backends.
// TODO - drop when we no longer support juju 3.1.x
func (s *Service) GetSecretBackendConfigLegacy(
	ctx context.Context, modelConfig *config.Config,
	cloud cloud.Cloud, cred cloud.Credential,
) (*provider.ModelBackendConfigInfo, error) {
	// TODO: implement once we have secret service in place.
	return nil, nil
}

// GetSecretBackendConfig returns the secret backend configuration for the given backend ID.
func (s *Service) GetSecretBackendConfig(
	ctx context.Context, modelConfig *config.Config,
	cloud cloud.Cloud, cred cloud.Credential,
) (*provider.ModelBackendConfigInfo, error) {
	// TODO: implement once we have secret service in place.
	return nil, nil
}

// GetSecretBackendConfigForDrain returns the secret backend configuration for the given backend ID for the drain worker.
func (s *Service) GetSecretBackendConfigForDrain(
	ctx context.Context, backendID string,
	modelConfig *config.Config, cloud cloud.Cloud, cred cloud.Credential,
) (*provider.ModelBackendConfigInfo, error) {
	// TODO: implement once we have secret service in place.
	return nil, nil
}

// CheckSecretBackend checks the secret backend for the given backend ID.
func (s *Service) CheckSecretBackend(ctx context.Context, backendID string) error {
	backend, err := s.st.GetSecretBackend(ctx, backendID)
	if err != nil {
		return domain.CoerceError(err)
	}
	p, err := s.registry(backend.BackendType)
	if err != nil {
		return domain.CoerceError(err)
	}
	return pingBackend(p, backend.Config)
}

// pingBackend instantiates a backend and pings it.
func pingBackend(p provider.SecretBackendProvider, cfg provider.ConfigAttrs) error {
	b, err := p.NewBackend(&provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{BackendType: p.Type(), Config: cfg},
	})
	if err != nil {
		return errors.Annotate(err, "checking backend")
	}
	return b.Ping()
}

func validateSecretBackendForUpsert(backend secrets.SecretBackend) error {
	if backend.ID == "" {
		return errors.NewNotValid(nil, "missing backend ID")
	}
	if backend.Name == "" {
		return errors.NewNotValid(nil, "missing backend name")
	}
	if backend.Name == juju.BackendName || backend.Name == provider.Auto {
		return errors.NotValidf("backend %q", backend.Name)
	}
	return nil
}

// CreateSecretBackend creates a new secret backend.
func (s *Service) CreateSecretBackend(ctx context.Context, backend secrets.SecretBackend) error {
	if err := validateSecretBackendForUpsert(backend); err != nil {
		return domain.CoerceError(err)
	}
	p, err := s.registry(backend.BackendType)
	if err != nil {
		return errors.Annotatef(
			domain.CoerceError(err), "creating backend provider type %q", backend.BackendType,
		)
	}
	configValidator, ok := p.(provider.ProviderConfig)
	if ok {
		defaults := configValidator.ConfigDefaults()
		if backend.Config == nil && len(defaults) > 0 {
			backend.Config = make(map[string]interface{})
		}
		for k, v := range defaults {
			if _, ok := backend.Config[k]; !ok {
				backend.Config[k] = v
			}
		}
		err = configValidator.ValidateConfig(nil, backend.Config)
		if err != nil {
			return errors.Annotatef(
				domain.CoerceError(err), "invalid config for provider %q", backend.BackendType,
			)
		}
	}
	if err := pingBackend(p, backend.Config); err != nil {
		return domain.CoerceError(err)
	}

	var nextRotateTime *time.Time
	if backend.TokenRotateInterval != nil && *backend.TokenRotateInterval > 0 {
		if !provider.HasAuthRefresh(p) {
			return errors.NotSupportedf("token refresh on secret backend of type %q", p.Type())
		}
		nextRotateTime, err = secrets.NextBackendRotateTime(s.clock.Now(), *backend.TokenRotateInterval)
		if err != nil {
			return domain.CoerceError(err)
		}
	}
	_, err = s.st.CreateSecretBackend(
		ctx, secretbackend.CreateSecretBackendParams{
			ID:                  backend.ID,
			Name:                backend.Name,
			BackendType:         backend.BackendType,
			TokenRotateInterval: backend.TokenRotateInterval,
			Config:              backend.Config,
			NextRotateTime:      nextRotateTime,
		},
	)
	return domain.CoerceError(err)
}

// UpdateSecretBackend updates an existing secret backend.
func (s *Service) UpdateSecretBackend(ctx context.Context, backend secrets.SecretBackend, force bool, reset ...string) error {
	if err := validateSecretBackendForUpsert(backend); err != nil {
		return domain.CoerceError(err)
	}
	existing, err := s.st.GetSecretBackendByName(ctx, backend.Name)
	if err != nil {
		return domain.CoerceError(err)
	}
	p, err := s.registry(existing.BackendType)
	if err != nil {
		return domain.CoerceError(err)
	}

	cfgToApply := make(map[string]interface{})
	for k, v := range existing.Config {
		cfgToApply[k] = v
	}
	for k, v := range backend.Config {
		cfgToApply[k] = v
	}
	for _, k := range reset {
		delete(cfgToApply, k)
	}
	configValidator, ok := p.(provider.ProviderConfig)
	if ok {
		defaults := configValidator.ConfigDefaults()
		for _, k := range reset {
			if defaultVal, ok := defaults[k]; ok {
				cfgToApply[k] = defaultVal
			}
		}
		err = configValidator.ValidateConfig(existing.Config, cfgToApply)
		if err != nil {
			return errors.Annotatef(
				domain.CoerceError(err), "invalid config for provider %q", existing.BackendType,
			)
		}
	}
	if !force {
		if err := pingBackend(p, cfgToApply); err != nil {
			return domain.CoerceError(err)
		}
	}
	var nextRotateTime *time.Time
	if backend.TokenRotateInterval != nil && *backend.TokenRotateInterval > 0 {
		if !provider.HasAuthRefresh(p) {
			return errors.NotSupportedf("token refresh on secret backend of type %q", p.Type())
		}
		nextRotateTime, err = secrets.NextBackendRotateTime(s.clock.Now(), *backend.TokenRotateInterval)
		if err != nil {
			return domain.CoerceError(err)
		}
	}
	err = s.st.UpdateSecretBackend(
		ctx, secretbackend.UpdateSecretBackendParams{
			ID:                  existing.ID,
			NameChange:          &backend.Name,
			TokenRotateInterval: backend.TokenRotateInterval,
			Config:              cfgToApply,
			NextRotateTime:      nextRotateTime,
		},
	)
	return domain.CoerceError(err)
}

// DeleteSecretBackend deletes a secret backend.
func (s *Service) DeleteSecretBackend(ctx context.Context, backendID string, force bool) error {
	err := s.st.DeleteSecretBackend(ctx, backendID, force)
	return domain.CoerceError(err)
}

// GetSecretBackendByName returns the secret backend for the given backend name.
func (s *Service) GetSecretBackendByName(ctx context.Context, name string) (*secrets.SecretBackend, error) {
	b, err := s.st.GetSecretBackendByName(ctx, name)
	if err != nil {
		return nil, domain.CoerceError(err)
	}
	return b, nil
}

// ListSecretBackends returns a list of all secret backends.
// TODO: for `commonsecrets.BackendSummaryInfo`!!!!
func (s *Service) ListSecretBackends(context.Context) ([]secrets.SecretBackend, error) {
	// TODO: implement once we have secret service in place.
	return nil, nil
}

// IncreCountForSecretBackend increments the secret count for the given secret backend.
func (s *Service) IncreCountForSecretBackend(ctx context.Context, backendID string) error {
	err := s.st.IncreCountForSecretBackend(ctx, backendID)
	return domain.CoerceError(err)
}

// DecreCountForSecretBackend decrements the secret count for the given secret backend.
func (s *Service) DecreCountForSecretBackend(ctx context.Context, backendID string) error {
	err := s.st.DecreCountForSecretBackend(ctx, backendID)
	return domain.CoerceError(err)
}

// RotateBackendToken rotates the token for the given secret backend.
func (s *Service) RotateBackendToken(ctx context.Context, backendID string) error {
	backendInfo, err := s.st.GetSecretBackend(ctx, backendID)
	if err != nil {
		return domain.CoerceError(err)
	}
	p, err := s.registry(backendInfo.BackendType)
	if err != nil {
		return domain.CoerceError(err)
	}
	if !provider.HasAuthRefresh(p) {
		return nil
	}

	if backendInfo.TokenRotateInterval == nil || *backendInfo.TokenRotateInterval == 0 {
		s.logger.Debugf("not rotating token for secret backend %q", backendInfo.Name)
		return nil
	}

	s.logger.Debugf("refresh token for backend %v", backendInfo.Name)
	cfg := provider.BackendConfig{
		BackendType: backendInfo.BackendType,
		Config:      backendInfo.Config,
	}
	var nextRotateTime time.Time
	auth, err := p.(provider.SupportAuthRefresh).RefreshAuth(cfg, *backendInfo.TokenRotateInterval)
	if err != nil {
		s.logger.Debugf("refreshing auth token for %q: %v", backendInfo.Name, err)
		// If there's a permission error, we can't recover from that.
		if errors.Is(err, internalsecrets.PermissionDenied) {
			return domain.CoerceError(err)
		}
	} else {
		err = s.st.UpdateSecretBackend(ctx, secretbackend.UpdateSecretBackendParams{
			ID:     backendID,
			Config: auth.Config,
		})
		if err == nil {
			next, _ := secrets.NextBackendRotateTime(s.clock.Now(), *backendInfo.TokenRotateInterval)
			nextRotateTime = *next
		}
	}

	if nextRotateTime.IsZero() {
		nextRotateTime = s.clock.Now().Add(2 * time.Minute)
	}
	s.logger.Debugf("updating token rotation for %q, next: %s", backendInfo.Name, nextRotateTime)
	err = s.st.SecretBackendRotated(ctx, backendID, nextRotateTime)
	return domain.CoerceError(err)
}

// WatchableService defines a service that can be watched for changes.
type WatchableService struct {
	Service
	watcherFactory secretbackend.WatcherFactory
}

// NewWatchableService creates a new WatchableService for interacting with the secret backend state and watching for changes.
func NewWatchableService(
	st State, logger Logger,
	wf secretbackend.WatcherFactory,
	controllerUUID string,
	clk clock.Clock,
	registry SecretProviderRegistry,
) *WatchableService {
	return &WatchableService{
		Service: Service{
			st:             st,
			logger:         logger,
			controllerUUID: controllerUUID,
			clock:          clk,
			registry:       registry,
		},
		watcherFactory: wf,
	}
}

// WatchSecretBackendRotationChanges returns a watcher for secret backend rotation changes.
func (s *WatchableService) WatchSecretBackendRotationChanges(ctx context.Context) (watcher.SecretBackendRotateWatcher, error) {
	w, err := s.st.WatchSecretBackendRotationChanges(ctx, s.watcherFactory)
	if err != nil {
		return nil, domain.CoerceError(err)
	}
	return w, nil
}
