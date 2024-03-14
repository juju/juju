// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/environs/cloudspec"
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

func getK8sBackendConfig(cloud cloud.Cloud, cred cloud.Credential) (*provider.BackendConfig, error) {
	spec, err := cloudspec.MakeCloudSpec(cloud, "", &cred)
	if err != nil {
		return nil, errors.Trace(err)
	}
	k8sConfig, err := kubernetes.BuiltInConfig(spec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return k8sConfig, nil
}

// GetSecretBackendConfigForAdmin returns the secret backend configuration for the given backend ID for an admin user.
func (s *Service) GetSecretBackendConfigForAdmin(
	ctx context.Context, modelUUID coremodel.UUID, model secretbackend.ModelGetter, cloud cloud.Cloud, cred cloud.Credential,
) (*provider.ModelBackendConfigInfo, error) {
	var info provider.ModelBackendConfigInfo
	m, err := model.GetModel(ctx, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	info.Configs = make(map[string]provider.ModelBackendConfig)
	// We need to include builtin backends for secret draining and accessing those secrets while drain is in progress.
	// TODO(secrets) - only use those in use by model
	// For now, we'll return all backends on the controller.
	jujuBackendID := s.controllerUUID
	info.Configs[jujuBackendID] = provider.ModelBackendConfig{
		ControllerUUID: s.controllerUUID,
		ModelUUID:      m.UUID,
		ModelName:      m.Name,
		BackendConfig:  juju.BuiltInConfig(),
	}
	backend, err := model.GetSecretBackend(ctx, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if backend.Name == provider.Auto || backend.Name == provider.Internal {
		info.ActiveID = jujuBackendID
	}

	if m.ModelType == coremodel.CAAS {
		k8sConfig, err := getK8sBackendConfig(cloud, cred)
		if err != nil {
			return nil, errors.Trace(err)
		}
		k8sBackendID := m.UUID
		info.Configs[k8sBackendID] = provider.ModelBackendConfig{
			ControllerUUID: s.controllerUUID,
			ModelUUID:      m.UUID,
			ModelName:      m.Name,
			BackendConfig:  *k8sConfig,
		}
		if backend.Name == provider.Auto {
			info.ActiveID = k8sBackendID
		}
	}

	backends, err := s.st.ListSecretBackends(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, b := range backends {
		if b.Name == backend.Name {
			info.ActiveID = b.ID
		}
		info.Configs[b.ID] = provider.ModelBackendConfig{
			ControllerUUID: s.controllerUUID,
			ModelUUID:      m.UUID,
			ModelName:      m.Name,
			BackendConfig: provider.BackendConfig{
				BackendType: b.BackendType,
				Config:      b.Config,
			},
		}
	}
	if info.ActiveID == "" {
		return nil, errors.NotFoundf("secret backend %q", backend.Name)
	}
	return &info, nil
}

// GetSecretBackendConfigLegacy gets the config needed to create a client to secret backends.
// TODO - drop when we no longer support juju 3.1.x
func (s *Service) GetSecretBackendConfigLegacy(
	ctx context.Context, modelUUID coremodel.UUID, model secretbackend.ModelGetter,
	cloud cloud.Cloud, cred cloud.Credential,
) (*provider.ModelBackendConfigInfo, error) {
	// TODO: implement once we have secret service in place.
	return nil, nil
}

// GetSecretBackendConfig returns the secret backend configuration for the given backend ID.
func (s *Service) GetSecretBackendConfig(
	ctx context.Context, modelUUID coremodel.UUID, model secretbackend.ModelGetter,
	cloud cloud.Cloud, cred cloud.Credential,
) (*provider.ModelBackendConfigInfo, error) {
	// TODO: implement once we have secret service in place.
	return nil, nil
}

// GetSecretBackendConfigForDrain returns the secret backend configuration for the given backend ID for the drain worker.
func (s *Service) GetSecretBackendConfigForDrain(
	ctx context.Context, backendID string,
	modelUUID coremodel.UUID, model secretbackend.ModelGetter, cloud cloud.Cloud, cred cloud.Credential,
) (*provider.ModelBackendConfigInfo, error) {
	// TODO: implement once we have secret service in place.
	return nil, nil
}

// BackendSummaryInfo returns a summary of the secret backends.
// If we just want a model's in-use backends, it's the caller's
// resposibility to provide the backendIDs in the filter.
func (s *Service) BackendSummaryInfo(
	ctx context.Context,
	modelUUID coremodel.UUID, model secretbackend.ModelGetter, cloud cloud.Cloud, cred cloud.Credential,
	reveal bool, filter secretbackend.SecretBackendFilter,
) ([]*secretbackend.SecretBackendInfo, error) {
	backends, err := s.st.ListSecretBackends(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// If we want all backends, include those which are not in use.
	if filter.All {
		// The internal (controller) backend.
		backends = append(backends, &secretbackend.SecretBackendInfo{
			SecretBackend: secrets.SecretBackend{
				ID:          s.controllerUUID,
				Name:        juju.BackendName,
				BackendType: juju.BackendType,
			},
		})
		m, err := model.GetModel(ctx, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if m.ModelType == coremodel.CAAS {
			// The kubernetes backend.
			k8sConfig, err := getK8sBackendConfig(cloud, cred)
			if err != nil {
				return nil, errors.Trace(err)
			}
			k8sBackend := &secretbackend.SecretBackendInfo{
				SecretBackend: secrets.SecretBackend{
					ID:          m.UUID,
					Name:        kubernetes.BuiltInName(m.Name),
					BackendType: kubernetes.BackendType,
					Config:      k8sConfig.Config,
				},
				// TODO: implement secret count for secret backend.
				// For now, we just set it to 1 to indicate that the backend is in use.
				NumSecrets: 1,
			}
			// For local k8s secrets, corresponding to every hosted model,
			// do not include the result if there are no secrets.
			if k8sBackend.NumSecrets > 0 || !filter.All {
				backends = append(backends, k8sBackend)
			}
		}
	}
	wanted := set.NewStrings(filter.Names...)
	for i := 0; i < len(backends); {
		b := backends[i]
		if !wanted.IsEmpty() && !wanted.Contains(b.Name) {
			backends = append(backends[:i], backends[i+1:]...)
			continue
		} else {
			i++
		}
		p, err := s.registry(b.BackendType)
		if err != nil {
			return nil, errors.Trace(err)
		}
		b.Status = status.Active.String()
		if b.BackendType != juju.BackendType && b.BackendType != kubernetes.BackendType {
			if err := pingBackend(p, b.Config); err != nil {
				b.Status = status.Error.String()
				b.Message = err.Error()
			}
		}
		if len(b.Config) == 0 {
			continue
		}
		configValidator, ok := p.(provider.ProviderConfig)
		if !ok {
			continue
		}
		for n, f := range configValidator.ConfigSchema() {
			if f.Secret && !reveal {
				delete(b.Config, n)
			}
		}
	}
	return backends, nil
}

// CheckSecretBackend checks the secret backend for the given backend ID.
func (s *Service) CheckSecretBackend(ctx context.Context, backendID string) error {
	backend, err := s.st.GetSecretBackend(ctx, backendID)
	if err != nil {
		return errors.Trace(err)
	}
	p, err := s.registry(backend.BackendType)
	if err != nil {
		return errors.Trace(err)
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

// CreateSecretBackend creates a new secret backend.
func (s *Service) CreateSecretBackend(ctx context.Context, backend secrets.SecretBackend) error {
	if backend.ID == "" {
		return errors.NewNotValid(nil, "missing backend ID")
	}
	if backend.Name == "" {
		return errors.NewNotValid(nil, "missing backend name")
	}
	if backend.Name == juju.BackendName || backend.Name == provider.Auto {
		return errors.NotValidf("backend %q", backend.Name)
	}
	p, err := s.registry(backend.BackendType)
	if err != nil {
		return errors.Annotatef(
			err, "creating backend provider type %q", backend.BackendType,
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
				err, "invalid config for provider %q", backend.BackendType,
			)
		}
	}
	if err := pingBackend(p, backend.Config); err != nil {
		return errors.Trace(err)
	}

	var nextRotateTime *time.Time
	if backend.TokenRotateInterval != nil && *backend.TokenRotateInterval > 0 {
		if !provider.HasAuthRefresh(p) {
			return errors.NotSupportedf("token refresh on secret backend of type %q", p.Type())
		}
		nextRotateTime, err = secrets.NextBackendRotateTime(s.clock.Now(), *backend.TokenRotateInterval)
		if err != nil {
			return errors.Trace(err)
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
	if errors.Is(err, domain.ErrDuplicate) {
		return errors.AlreadyExistsf("secret backend with name %q", backend.Name)
	}
	return errors.Trace(err)
}

// UpdateSecretBackend updates an existing secret backend.
func (s *Service) UpdateSecretBackend(ctx context.Context, backend secrets.SecretBackend, force bool, reset ...string) error {
	if backend.ID == "" && backend.Name == "" {
		return errors.NewNotValid(nil, "missing backend ID or name")
	}
	if backend.Name == juju.BackendName || backend.Name == provider.Auto {
		return errors.NotValidf("backend %q", backend.Name)
	}
	var (
		existing *secrets.SecretBackend
		err      error
	)
	if backend.ID != "" {
		existing, err = s.st.GetSecretBackend(ctx, backend.ID)
	} else {
		existing, err = s.st.GetSecretBackendByName(ctx, backend.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}
	p, err := s.registry(existing.BackendType)
	if err != nil {
		return errors.Trace(err)
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
				errors.Trace(err), "invalid config for provider %q", existing.BackendType,
			)
		}
	}
	if !force {
		if err := pingBackend(p, cfgToApply); err != nil {
			return errors.Trace(err)
		}
	}
	var nextRotateTime *time.Time
	if backend.TokenRotateInterval != nil && *backend.TokenRotateInterval > 0 {
		if !provider.HasAuthRefresh(p) {
			return errors.NotSupportedf("token refresh on secret backend of type %q", p.Type())
		}
		nextRotateTime, err = secrets.NextBackendRotateTime(s.clock.Now(), *backend.TokenRotateInterval)
		if err != nil {
			return errors.Trace(err)
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
	return errors.Trace(err)
}

// DeleteSecretBackend deletes a secret backend.
func (s *Service) DeleteSecretBackend(ctx context.Context, backendID string, force bool) error {
	return s.st.DeleteSecretBackend(ctx, backendID, force)
}

// GetSecretBackendByName returns the secret backend for the given backend name.
func (s *Service) GetSecretBackendByName(ctx context.Context, name string) (*secrets.SecretBackend, error) {
	return s.st.GetSecretBackendByName(ctx, name)
}

// RotateBackendToken rotates the token for the given secret backend.
func (s *Service) RotateBackendToken(ctx context.Context, backendID string) error {
	backendInfo, err := s.st.GetSecretBackend(ctx, backendID)
	if err != nil {
		return errors.Trace(err)
	}
	p, err := s.registry(backendInfo.BackendType)
	if err != nil {
		return errors.Trace(err)
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
			return errors.Trace(err)
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
	return errors.Trace(err)
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
func (s *WatchableService) WatchSecretBackendRotationChanges() (watcher.SecretBackendRotateWatcher, error) {
	w, err := s.st.WatchSecretBackendRotationChanges(s.watcherFactory)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
