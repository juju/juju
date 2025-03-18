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

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/domain/secretbackend"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	internalsecrets "github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
)

// SecretProviderRegistry is a function that returns a secret backend provider for the given backend type.
type SecretProviderRegistry func(backendType string) (provider.SecretBackendProvider, error)

type Service struct {
	st       State
	logger   logger.Logger
	clock    clock.Clock
	registry SecretProviderRegistry
}

// NewService creates a new Service for interacting with the secret backend state.
func NewService(
	st State, logger logger.Logger,
) *Service {
	return newService(
		st, logger, clock.WallClock, provider.Provider,
	)
}

func newService(
	st State, logger logger.Logger,
	clk clock.Clock,
	registry SecretProviderRegistry,
) *Service {
	return &Service{
		st:       st,
		logger:   logger,
		clock:    clk,
		registry: registry,
	}
}

// GetSecretBackendConfigForAdmin returns the secret backend configuration for the given backend ID for an admin user,
// returning an error satisfying [secretbackenderrors.NotFound] if the backend is not found.
func (s *Service) GetSecretBackendConfigForAdmin(ctx context.Context, modelUUID coremodel.UUID) (*provider.ModelBackendConfigInfo, error) {
	modelSecretBackend, err := s.st.GetModelSecretBackendDetails(ctx, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// We need to include builtin backends for secret draining and accessing those secrets while drain is in progress.
	// TODO(secrets) - only use those in use by model
	// For now, we'll return all backends on the controller.
	backends, err := s.st.ListSecretBackendsForModel(ctx, modelUUID, true)
	if err != nil {
		return nil, errors.Trace(err)
	}
	currentBackendName := modelSecretBackend.SecretBackendName

	var info provider.ModelBackendConfigInfo
	info.Configs = make(map[string]provider.ModelBackendConfig)
	for _, b := range backends {
		if b.Name == currentBackendName {
			info.ActiveID = b.ID
		}
		info.Configs[b.ID] = provider.ModelBackendConfig{
			ControllerUUID: modelSecretBackend.ControllerUUID,
			ModelUUID:      modelSecretBackend.ModelID.String(),
			ModelName:      modelSecretBackend.ModelName,
			BackendConfig: provider.BackendConfig{
				BackendType: b.BackendType,
				Config:      b.Config,
			},
		}
	}
	if info.ActiveID == "" {
		return nil, fmt.Errorf("%w: %q", secretbackenderrors.NotFound, currentBackendName)
	}
	return &info, nil
}

// DrainBackendConfigInfo returns the secret backend config for the drain worker to use.
func (s *Service) DrainBackendConfigInfo(
	ctx context.Context, p DrainBackendConfigParams,
) (*provider.ModelBackendConfigInfo, error) {
	if p.Accessor.Kind != secretservice.UnitAccessor && p.Accessor.Kind != secretservice.ModelAccessor {
		return nil, errors.NotSupportedf("secret accessor kind %q", p.Accessor.Kind)
	}

	adminModelCfg, err := s.GetSecretBackendConfigForAdmin(ctx, p.ModelUUID)
	if err != nil {
		return nil, errors.Annotatef(err, "getting admin config for secret backend %q", p.BackendID)
	}
	result := provider.ModelBackendConfigInfo{
		ActiveID: adminModelCfg.ActiveID,
		Configs:  make(map[string]provider.ModelBackendConfig),
	}
	if p.BackendID == "" {
		p.BackendID = adminModelCfg.ActiveID
	}

	cfg, ok := adminModelCfg.Configs[p.BackendID]
	if !ok {
		return nil, errors.Errorf("missing secret backend %q", p.BackendID)
	}
	backendCfg, err := s.backendConfigInfo(ctx,
		p.GrantedSecretsGetter, p.BackendID, &cfg, p.Accessor, p.LeaderToken, true, true)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.Configs[p.BackendID] = *backendCfg
	return &result, nil
}

// BackendConfigInfo returns the config to create a secret backend
// for the specified backend IDs.
// This is called to provide config to a client like a unit agent which
// needs to access secretService. The accessor is the agent which needs access.
// The client is expected to be restricted to write only those secretService
// owned by the agent, and read only those secretService shared with the agent.
// The result includes config for all relevant backends, including the id
// of the current active backend.
func (s *Service) BackendConfigInfo(
	ctx context.Context, p BackendConfigParams,
) (*provider.ModelBackendConfigInfo, error) {
	if p.Accessor.Kind != secretservice.UnitAccessor && p.Accessor.Kind != secretservice.ModelAccessor {
		return nil, errors.NotSupportedf("secret accessor kind %q", p.Accessor.Kind)
	}

	adminModelCfg, err := s.GetSecretBackendConfigForAdmin(ctx, p.ModelUUID)
	if err != nil {
		return nil, errors.Annotate(err, "getting configured secrets providers")
	}
	result := provider.ModelBackendConfigInfo{
		ActiveID: adminModelCfg.ActiveID,
		Configs:  make(map[string]provider.ModelBackendConfig),
	}
	if len(p.BackendIDs) == 0 {
		p.BackendIDs = []string{adminModelCfg.ActiveID}
	}
	for _, backendID := range p.BackendIDs {
		cfg, ok := adminModelCfg.Configs[backendID]
		if !ok {
			return nil, errors.Errorf("missing secret backend %q", backendID)
		}
		backendCfg, err := s.backendConfigInfo(ctx,
			p.GrantedSecretsGetter, backendID, &cfg, p.Accessor, p.LeaderToken, p.SameController, false)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.Configs[backendID] = *backendCfg
	}
	return &result, nil
}

func (s *Service) backendConfigInfo(
	ctx context.Context,
	grantedSecretsGetter secretservice.GrantedSecretsGetter,
	backendID string, cfg *provider.ModelBackendConfig,
	accessor secretservice.SecretAccessor, token leadership.Token, sameController, forDrain bool,
) (*provider.ModelBackendConfig, error) {
	if grantedSecretsGetter == nil {
		return nil, errors.Errorf("unexpected nil value for GrantedSecretsGetter")
	}

	p, err := s.registry(cfg.BackendType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = p.Initialise(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "initialising secrets provider")
	}

	ownedRevisions := provider.SecretRevisions{}
	readRevisions := provider.SecretRevisions{}

	var coreAccessor coresecrets.Accessor

	switch accessor.Kind {
	case secretservice.UnitAccessor:
		// Find secretService owned by the agent
		// (or its app if the agent is a leader).
		unitName, err := unit.NewName(accessor.ID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		coreAccessor = coresecrets.Accessor{
			Kind: coresecrets.UnitAccessor,
			ID:   unitName.String(),
		}
		owners := []secretservice.SecretAccessor{accessor}
		appName := unitName.Application()
		isLeader := false
		if token != nil {
			err := token.Check()
			if err != nil && !leadership.IsNotLeaderError(err) {
				return nil, errors.Trace(err)
			}
			isLeader = err == nil
		}
		if isLeader {
			// Leader unit owns application level secretService.
			owners = append(owners, secretservice.SecretAccessor{
				Kind: secretservice.ApplicationAccessor,
				ID:   appName,
			})
		} else {
			// Non leader units can read application level secretService.
			// Find secretService owned by the application.
			readOnlyOwner := secretservice.SecretAccessor{
				Kind: secretservice.ApplicationAccessor,
				ID:   appName,
			}
			revInfo, err := grantedSecretsGetter(ctx, backendID, coresecrets.RoleView, readOnlyOwner)
			if err != nil {
				return nil, errors.Trace(err)
			}
			for _, r := range revInfo {
				readRevisions.Add(r.URI, r.RevisionID)
			}
		}
		revInfo, err := grantedSecretsGetter(ctx, backendID, coresecrets.RoleManage, owners...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, r := range revInfo {
			ownedRevisions.Add(r.URI, r.RevisionID)
		}

		// Granted secretService can be consumed in application level for all units.
		// We include secretService shared with the app or just the specified unit.
		consumers := []secretservice.SecretAccessor{{
			Kind: secretservice.UnitAccessor,
			ID:   unitName.String(),
		}, {
			Kind: secretservice.ApplicationAccessor,
			ID:   appName,
		}}
		revInfo, err = grantedSecretsGetter(ctx, backendID, coresecrets.RoleView, consumers...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, r := range revInfo {
			readRevisions.Add(r.URI, r.RevisionID)
		}
	case secretservice.ModelAccessor:
		coreAccessor = coresecrets.Accessor{
			Kind: coresecrets.ModelAccessor,
			ID:   accessor.ID,
		}
		revInfo, err := grantedSecretsGetter(ctx, backendID, coresecrets.RoleManage, accessor)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, r := range revInfo {
			ownedRevisions.Add(r.URI, r.RevisionID)
		}
	default:
		return nil, errors.NotSupportedf("secret accessor kind %q", accessor.Kind)
	}

	s.logger.Debugf(ctx, "secrets for %s:\nowned: %v\nconsumed:%v", accessor, ownedRevisions, readRevisions)
	restrictedConfig, err := p.RestrictedConfig(ctx, cfg, sameController, forDrain, coreAccessor, ownedRevisions, readRevisions)
	if err != nil {
		return nil, errors.Trace(err)
	}
	info := &provider.ModelBackendConfig{
		ControllerUUID: cfg.ControllerUUID,
		ModelUUID:      cfg.ModelUUID,
		ModelName:      cfg.ModelName,
		BackendConfig:  *restrictedConfig,
	}
	return info, nil
}

func convertConfigToString(config map[string]interface{}) map[string]string {
	if len(config) == 0 {
		return nil
	}
	return transform.Map(config, func(k string, v interface{}) (string, string) {
		return k, fmt.Sprintf("%v", v)
	})
}

// BackendSummaryInfoForModel returns a summary of the secret backends
// which contain secrets from the specified model.
func (s *Service) BackendSummaryInfoForModel(ctx context.Context, modelUUID coremodel.UUID) ([]*SecretBackendInfo, error) {
	backends, err := s.st.ListSecretBackendsForModel(ctx, modelUUID, false)
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
				Config:              b.Config,
			},
			NumSecrets: b.NumSecrets,
		})
	}
	return s.composeBackendInfoResults(backendInfos, false)
}

// BackendSummaryInfo returns a summary of the secret backends.
// If names are specified, just those backends are included, else all.
func (s *Service) BackendSummaryInfo(ctx context.Context, reveal bool, names ...string) ([]*SecretBackendInfo, error) {
	backends, err := s.st.ListSecretBackends(ctx)
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
				Config:              b.Config,
			},
			NumSecrets: b.NumSecrets,
		})
	}
	return s.composeBackendInfoResults(backendInfos, reveal, names...)
}

func (s *Service) composeBackendInfoResults(backendInfos []*SecretBackendInfo, reveal bool, names ...string) ([]*SecretBackendInfo, error) {
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

// ListBackendIDs returns the IDs of all the secret backends.
func (s *Service) ListBackendIDs(ctx context.Context) ([]string, error) {
	result, err := s.st.ListSecretBackendIDs(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
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
		err = configValidator.ValidateConfig(nil, backend.Config, backend.TokenRotateInterval)
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
		err = configValidator.ValidateConfig(existing.Config, cfgToApply, params.TokenRotateInterval)
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
func (s *Service) DeleteSecretBackend(ctx context.Context, params DeleteSecretBackendParams) error {
	return s.st.DeleteSecretBackend(ctx, params.BackendIdentifier, params.DeleteInUse)
}

// RotateBackendToken rotates the token for the given secret backend.
func (s *Service) RotateBackendToken(ctx context.Context, backendID string) error {
	backendInfo, err := s.st.GetSecretBackend(ctx, secretbackend.BackendIdentifier{ID: backendID})
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
		s.logger.Debugf(ctx, "not rotating token for secret backend %q", backendInfo.Name)
		return nil
	}

	s.logger.Debugf(ctx, "refresh token for backend %v", backendInfo.Name)
	cfg := provider.BackendConfig{
		BackendType: backendInfo.BackendType,
		Config:      backendInfo.Config,
	}
	// Ideally, we should do this in a transaction, but it's not critical.
	// Because it's called by a single worker at a time.
	var nextRotateTime time.Time
	auth, err := p.(provider.SupportAuthRefresh).RefreshAuth(ctx, cfg, *backendInfo.TokenRotateInterval)
	if err != nil {
		s.logger.Debugf(ctx, "refreshing auth token for %q: %v", backendInfo.Name, err)
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
	s.logger.Debugf(ctx, "updating token rotation for %q, next: %s", backendInfo.Name, nextRotateTime)
	err = s.st.SecretBackendRotated(ctx, backendID, nextRotateTime)
	return errors.Trace(err)
}

// GetRevisionsToDrain looks at the supplied revisions and returns any which should be
// drained to a different backend for the specified model.
func (s *Service) GetRevisionsToDrain(ctx context.Context, modelUUID coremodel.UUID, revs []coresecrets.SecretExternalRevision) ([]RevisionInfo, error) {
	internalBackendUUID, activeBackendUUID, err := s.st.GetInternalAndActiveBackendUUIDs(ctx, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result []RevisionInfo
	for _, r := range revs {
		if r.ValueRef != nil {
			if r.ValueRef.BackendID == activeBackendUUID {
				// The secret is in the active backend, so we don't need to drain.
				continue
			}
		} else {
			// Only internal backend secrets have nil ValueRef.
			if internalBackendUUID == activeBackendUUID {
				// The nil valueRef means the secret is in the internal backend,
				// and if the internal backend is already the active backend, we don't need to drain.
				continue
			}
		}
		result = append(result, RevisionInfo{
			Revision: r.Revision,
			ValueRef: r.ValueRef,
		})
	}
	return result, nil
}

// WatchableService defines a service that can be watched for changes.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService creates a new WatchableService for interacting with the secret backend state and watching for changes.
func NewWatchableService(
	st State, logger logger.Logger,
	wf WatcherFactory,
) *WatchableService {
	return newWatchableService(
		st, logger, wf, clock.WallClock, provider.Provider,
	)
}

func newWatchableService(
	st State, logger logger.Logger,
	wf WatcherFactory,
	clk clock.Clock,
	registry SecretProviderRegistry,
) *WatchableService {
	return &WatchableService{
		Service: Service{
			st:       st,
			logger:   logger,
			clock:    clk,
			registry: registry,
		},
		watcherFactory: wf,
	}
}

// It's for testing.
var InitialNamespaceChanges = eventsource.InitialNamespaceChanges

// WatchSecretBackendRotationChanges returns a watcher for secret backend rotation changes.
func (s *WatchableService) WatchSecretBackendRotationChanges(context.Context) (watcher.SecretBackendRotateWatcher, error) {
	tableName, initialQ := s.st.InitialWatchStatementForSecretBackendRotationChanges()
	w, err := s.watcherFactory.NewNamespaceWatcher(tableName, changestream.All, InitialNamespaceChanges(initialQ))
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newSecretBackendRotateWatcher(w, s.logger, s.st.GetSecretBackendRotateChanges)
}

// WatchSecretBackendChanged notifies when the model secret backend has changed.
func (s *WatchableService) WatchModelSecretBackendChanged(_ context.Context, modelUUID coremodel.UUID) (watcher.NotifyWatcher, error) {
	w, err := s.watcherFactory.NewValueWatcher(s.st.NamespaceForWatchModelSecretBackend(), modelUUID.String(),
		changestream.Changed)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
