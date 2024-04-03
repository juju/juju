// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	coremodel "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/secretbackend"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
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
func NewService(st State, logger Logger, controllerUUID string, registry SecretProviderRegistry) *Service {
	return newService(st, logger, controllerUUID, clock.WallClock, registry)
}

func newService(
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
	ctx context.Context, modelUUID coremodel.UUID, cloud cloud.Cloud, cred cloud.Credential,
) (*provider.ModelBackendConfigInfo, error) {
	var info provider.ModelBackendConfigInfo
	m, err := s.st.GetModel(ctx, modelUUID)
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
		ModelUUID:      m.ID.String(),
		ModelName:      m.Name,
		BackendConfig:  juju.BuiltInConfig(),
	}
	currentBackend, err := s.st.GetSecretBackend(ctx, secretbackend.BackendIdentifier{ID: m.SecretBackendID})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if currentBackend.Name == provider.Auto || currentBackend.Name == provider.Internal {
		// TODO: The logic for determining the active backend ought to be relocated to an
		// independent function. This function should then be invoked either when the model
		// is initially created or when the model's secret backend configuration is reset
		//  due to the current backend being forcefully removed. Under these circumstances,
		// the model secret backend will never be set to `auto`; so here we can
		// simply return the value stored in the database.
		info.ActiveID = jujuBackendID
	}

	if m.Type == coremodel.CAAS {
		k8sConfig, err := getK8sBackendConfig(cloud, cred)
		if err != nil {
			return nil, errors.Trace(err)
		}
		k8sBackendID := m.ID.String()
		info.Configs[k8sBackendID] = provider.ModelBackendConfig{
			ControllerUUID: s.controllerUUID,
			ModelUUID:      m.ID.String(),
			ModelName:      m.Name,
			BackendConfig:  *k8sConfig,
		}
		if currentBackend.Name == provider.Auto {
			info.ActiveID = k8sBackendID
		}
	}

	backends, err := s.st.ListSecretBackends(ctx, true)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, b := range backends {
		if b.Name == currentBackend.Name {
			info.ActiveID = b.ID
		}
		info.Configs[b.ID] = provider.ModelBackendConfig{
			ControllerUUID: s.controllerUUID,
			ModelUUID:      m.ID.String(),
			ModelName:      m.Name,
			BackendConfig: provider.BackendConfig{
				BackendType: b.BackendType,
				Config:      convertConfigToAny(b.Config),
			},
		}
	}
	if info.ActiveID == "" {
		return nil, fmt.Errorf("%w: %q", secretbackenderrors.NotFound, currentBackend.Name)
	}
	return &info, nil
}

func convertConfigToAny(config map[string]string) map[string]interface{} {
	if len(config) == 0 {
		return nil
	}
	return transform.Map(config, func(k string, v string) (string, any) {
		return k, v
	})
}

func convertConfigToString(config map[string]interface{}) map[string]string {
	if len(config) == 0 {
		return nil
	}
	return transform.Map(config, func(k string, v interface{}) (string, string) {
		return k, fmt.Sprintf("%v", v)
	})
}

// GetSecretBackendConfigLegacy gets the config needed to create a client to secret backends.
// TODO - drop when we no longer support juju 3.1.x
func (s *Service) GetSecretBackendConfigLegacy(
	ctx context.Context, modelUUID coremodel.UUID, cloud cloud.Cloud, cred cloud.Credential,
) (*provider.ModelBackendConfigInfo, error) {
	// TODO: implement once we have secret service in place.
	return nil, nil
}

// GetSecretBackendConfig returns the secret backend configuration for the given backend ID.
func (s *Service) GetSecretBackendConfig(
	ctx context.Context, modelUUID coremodel.UUID, cloud cloud.Cloud, cred cloud.Credential,
) (*provider.ModelBackendConfigInfo, error) {
	// TODO: implement once we have secret service in place.
	return nil, nil
}

// GetSecretBackendConfigForDrain returns the secret backend configuration
// for the given backend ID for the drain worker.
func (s *Service) GetSecretBackendConfigForDrain(
	ctx context.Context, backendID string,
	modelUUID coremodel.UUID, cloud cloud.Cloud, cred cloud.Credential,
) (*provider.ModelBackendConfigInfo, error) {
	// TODO: implement once we have secret service in place.
	return nil, nil
}

// BackendSummaryInfo returns a summary of the secret backends.
// If we just want a model's in-use backends, it's the caller's
// resposibility to provide the backendIDs in the filter.
func (s *Service) BackendSummaryInfo(
	ctx context.Context,
	modelUUID coremodel.UUID, cloud cloud.Cloud, cred cloud.Credential,
	reveal, all bool, names ...string,
) ([]*SecretBackendInfo, error) {
	backends, err := s.st.ListSecretBackends(ctx, all)
	if err != nil {
		return nil, errors.Trace(err)
	}
	backendInfos := make([]*SecretBackendInfo, 0, len(backends))
	for _, b := range backends {
		backendInfos = append(backendInfos, &SecretBackendInfo{
			SecretBackend: coresecrets.SecretBackend{
				ID:                  b.ID,
				Name:                b.Name,
				BackendType:         b.BackendType,
				TokenRotateInterval: b.TokenRotateInterval,
				Config:              convertConfigToAny(b.Config),
			},
		})
	}
	// If we want all backends, include those which are not in use.
	// TODO: once the in-use backends feature is implemented, below logic should be updated.
	// The ListSecretBackends will include the internal and k8s backends. We just need to fill
	// in the backend configuration for the k8s backend.
	if all {
		// The internal (controller) backend.
		backendInfos = append(backendInfos, &SecretBackendInfo{
			SecretBackend: coresecrets.SecretBackend{
				ID:          s.controllerUUID,
				Name:        juju.BackendName,
				BackendType: juju.BackendType,
			},
		})
		m, err := s.st.GetModel(ctx, modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if m.Type == coremodel.CAAS {
			// The kubernetes backend.
			k8sConfig, err := getK8sBackendConfig(cloud, cred)
			if err != nil {
				return nil, errors.Trace(err)
			}
			k8sBackend := &SecretBackendInfo{
				SecretBackend: coresecrets.SecretBackend{
					ID:          m.ID.String(),
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
			if k8sBackend.NumSecrets > 0 || !all {
				backendInfos = append(backendInfos, k8sBackend)
			}
		}
	}
	wanted := set.NewStrings(names...)
	for i := 0; i < len(backendInfos); {
		b := backendInfos[i]
		if !wanted.IsEmpty() && !wanted.Contains(b.Name) {
			backendInfos = append(backendInfos[:i], backendInfos[i+1:]...)
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
			if !f.Secret || reveal {
				continue
			}
			delete(b.Config, n)
		}
	}
	return backendInfos, nil
}

// PingSecretBackend checks the secret backend for the given backend ID.
func (s *Service) PingSecretBackend(ctx context.Context, backendID string) error {
	backend, err := s.st.GetSecretBackend(ctx, secretbackend.BackendIdentifier{ID: backendID})
	if err != nil {
		return errors.Trace(err)
	}
	p, err := s.registry(backend.BackendType)
	if err != nil {
		return errors.Trace(err)
	}
	return pingBackend(p, convertConfigToAny(backend.Config))
}

// pingBackend instantiates a backend and pings it.
func pingBackend(p provider.SecretBackendProvider, cfg provider.ConfigAttrs) error {
	b, err := p.NewBackend(&provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{BackendType: p.Type(), Config: cfg},
	})
	if err != nil {
		return errors.Trace(err)
	}
	return b.Ping()
}

func validateExternalBackendName(name string) error {
	if name == juju.BackendName ||
		name == kubernetes.BackendName ||
		name == provider.Auto ||
		name == provider.Internal {
		return fmt.Errorf("%w: reserved name %q", secretbackenderrors.NotValid, name)
	}
	return nil
}

// CreateSecretBackend creates a new secret backend.
func (s *Service) CreateSecretBackend(ctx context.Context, backend coresecrets.SecretBackend) error {
	if backend.ID == "" {
		return fmt.Errorf("%w: missing ID", secretbackenderrors.NotValid)
	}
	if backend.Name == "" {
		return fmt.Errorf("%w: missing name", secretbackenderrors.NotValid)
	}
	if err := validateExternalBackendName(backend.Name); err != nil {
		return errors.Trace(err)
	}
	p, err := s.registry(backend.BackendType)
	if err != nil {
		return fmt.Errorf("getting backend provider type %q: %w", backend.BackendType, err)
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
			return fmt.Errorf("%w: config for provider %q: %w", secretbackenderrors.NotValid, backend.BackendType, err)
		}
	}
	if err := pingBackend(p, backend.Config); err != nil {
		return errors.Trace(err)
	}

	var nextRotateTime *time.Time
	if backend.TokenRotateInterval != nil && *backend.TokenRotateInterval > 0 {
		if !provider.HasAuthRefresh(p) {
			return fmt.Errorf("%w: token refresh on secret backend of type %q", secretbackenderrors.NotSupported, p.Type())
		}
		nextRotateTime, err = coresecrets.NextBackendRotateTime(s.clock.Now(), *backend.TokenRotateInterval)
		if err != nil {
			return errors.Trace(err)
		}
	}
	_, err = s.st.CreateSecretBackend(
		ctx, secretbackend.CreateSecretBackendParams{
			BackendIdentifier: secretbackend.BackendIdentifier{
				ID:   backend.ID,
				Name: backend.Name,
			},
			BackendType:         backend.BackendType,
			TokenRotateInterval: backend.TokenRotateInterval,
			Config:              convertConfigToString(backend.Config),
			NextRotateTime:      nextRotateTime,
		},
	)
	if errors.Is(err, domain.ErrDuplicate) {
		return fmt.Errorf("%w: secret backend with name %q", secretbackenderrors.AlreadyExists, backend.Name)
	}
	return errors.Trace(err)
}

// UpdateSecretBackend updates an existing secret backend.
func (s *Service) UpdateSecretBackend(ctx context.Context, params UpdateSecretBackendParams) error {
	if err := params.Validate(); err != nil {
		return errors.Trace(err)
	}

	if params.NewName != nil {
		if err := validateExternalBackendName(*params.NewName); err != nil {
			return errors.Trace(err)
		}
	}

	// TODO: we should get the latest existing backend, merge the config then validate inside
	// the update operation transaction.
	existing, err := s.st.GetSecretBackend(ctx, params.BackendIdentifier)
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
	for k, v := range params.Config {
		cfgToApply[k] = v
	}
	for _, k := range params.Reset {
		delete(cfgToApply, k)
	}
	configValidator, ok := p.(provider.ProviderConfig)
	if ok {
		defaults := configValidator.ConfigDefaults()
		for _, k := range params.Reset {
			if defaultVal, ok := defaults[k]; ok {
				cfgToApply[k] = defaultVal
			}
		}
		err = configValidator.ValidateConfig(convertConfigToAny(existing.Config), cfgToApply)
		if err != nil {
			return fmt.Errorf("%w: config for provider %q: %w", secretbackenderrors.NotValid, existing.BackendType, err)
		}
	}
	if !params.SkipPing {
		if err := pingBackend(p, cfgToApply); err != nil {
			return errors.Trace(err)
		}
	}
	params.Config = convertConfigToString(cfgToApply)

	if params.TokenRotateInterval != nil && *params.TokenRotateInterval > 0 {
		if !provider.HasAuthRefresh(p) {
			return errors.NotSupportedf("token refresh on secret backend of type %q", p.Type())
		}
		params.NextRotateTime, err = coresecrets.NextBackendRotateTime(s.clock.Now(), *params.TokenRotateInterval)
		if err != nil {
			return errors.Trace(err)
		}
	}
	_, err = s.st.UpdateSecretBackend(ctx, params.UpdateSecretBackendParams)
	return errors.Trace(err)
}

// DeleteSecretBackend deletes a secret backend.
func (s *Service) DeleteSecretBackend(ctx context.Context, backendID string, force bool) error {
	return s.st.DeleteSecretBackend(ctx, backendID, force)
}

// GetSecretBackendByName returns the secret backend for the given backend name.
func (s *Service) GetSecretBackendByName(ctx context.Context, name string) (*coresecrets.SecretBackend, error) {
	sb, err := s.st.GetSecretBackend(ctx, secretbackend.BackendIdentifier{Name: name})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &coresecrets.SecretBackend{
		ID:                  sb.ID,
		Name:                sb.Name,
		BackendType:         sb.BackendType,
		TokenRotateInterval: sb.TokenRotateInterval,
		Config:              convertConfigToAny(sb.Config),
	}, nil
}

// RotateBackendToken rotates the token for the given secret backend.
func (s *Service) RotateBackendToken(ctx context.Context, backendID string) error {
	backendInfo, err := s.st.GetSecretBackend(ctx,
		secretbackend.BackendIdentifier{ID: backendID},
	)
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
		Config:      convertConfigToAny(backendInfo.Config),
	}
	// Ideally, we should do this in a transaction, but it's not critical.
	// Because it's called by a single worker at a time.
	var nextRotateTime time.Time
	auth, err := p.(provider.SupportAuthRefresh).RefreshAuth(cfg, *backendInfo.TokenRotateInterval)
	if err != nil {
		s.logger.Debugf("refreshing auth token for %q: %v", backendInfo.Name, err)
		// If there's a permission error, we can't recover from that.
		if errors.Is(err, internalsecrets.PermissionDenied) {
			return errors.Trace(err)
		}
	} else {
		_, err = s.st.UpdateSecretBackend(ctx, secretbackend.UpdateSecretBackendParams{
			BackendIdentifier: secretbackend.BackendIdentifier{ID: backendID},
			Config:            convertConfigToString(auth.Config),
		})
		if err == nil {
			next, _ := coresecrets.NextBackendRotateTime(s.clock.Now(), *backendInfo.TokenRotateInterval)
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

// GetModelSecretBackend returns the secret backend for the given model UUID.
func (s *Service) GetModelSecretBackend(ctx context.Context, modelUUID coremodel.UUID) (string, error) {
	return s.st.GetModelSecretBackend(ctx, modelUUID)
}

// SetModelSecretBackend sets the secret backend for the given model UUID.
func (s *Service) SetModelSecretBackend(ctx context.Context, modelUUID coremodel.UUID, backendName string) error {
	return s.st.SetModelSecretBackend(ctx, modelUUID, backendName)
}

// WatchableService defines a service that can be watched for changes.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService creates a new WatchableService for interacting with the secret backend state and watching for changes.
func NewWatchableService(
	st State, logger Logger,
	wf WatcherFactory,
	controllerUUID string,
	registry SecretProviderRegistry,
) *WatchableService {
	return newWatchableService(
		st, logger, wf, controllerUUID, clock.WallClock, registry,
	)
}

func newWatchableService(
	st State, logger Logger,
	wf WatcherFactory,
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
	tableName, initialQ := s.st.InitialWatchStatement()
	w, err := s.watcherFactory.NewNamespaceWatcher(tableName, changestream.All, initialQ)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newSecretBackendRotateWatcher(w, s.logger, s.st.GetSecretBackendRotateChanges), nil
}
