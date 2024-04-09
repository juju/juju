// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/leadership"
	corelogger "github.com/juju/juju/core/logger"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLoggerWithTags("juju.apiserver.common.secrets", corelogger.SECRETS)

// For testing.
var (
	GetProvider            = provider.Provider
	GetSecretBackendsState = getSecretBackendsState
)

func getSecretBackendsState(m Model) state.SecretBackendsStorage {
	return state.NewSecretBackends(m.State())
}

// BackendConfigGetter is a func used to get secret backend config.
type BackendConfigGetter func(ctx context.Context, backendIDs []string, wantAll bool) (*provider.ModelBackendConfigInfo, error)

// BackendAdminConfigGetter is a func used to get admin level secret backend config.
type BackendAdminConfigGetter func(context.Context) (*provider.ModelBackendConfigInfo, error)

// BackendDrainConfigGetter is a func used to get secret backend config for draining.
type BackendDrainConfigGetter func(context.Context, string) (*provider.ModelBackendConfigInfo, error)

// AdminBackendConfigInfo returns the admin config for the secret backends is use by
// the specified model.
// If external backend is configured, it returns the external backend together with the "internal" backend and
// the k8s backend for k8s models.
func AdminBackendConfigInfo(
	ctx context.Context, model Model, cloudService common.CloudService, credentialService common.CredentialService,
) (*provider.ModelBackendConfigInfo, error) {
	cfg, err := model.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	backendName := cfg.SecretBackend()

	var info provider.ModelBackendConfigInfo
	info.Configs = make(map[string]provider.ModelBackendConfig)

	// We need to include builtin backends for secret draining and accessing those secrets while drain is in progress.
	// TODO(secrets) - only use those in use by model
	// For now, we'll return all backends on the controller.
	jujuBackendID := model.ControllerUUID()
	info.Configs[jujuBackendID] = provider.ModelBackendConfig{
		ControllerUUID: model.ControllerUUID(),
		ModelUUID:      model.UUID(),
		ModelName:      model.Name(),
		BackendConfig:  juju.BuiltInConfig(),
	}
	if backendName == provider.Auto || backendName == provider.Internal {
		info.ActiveID = jujuBackendID
	}

	if model.Type() == state.ModelTypeCAAS {
		spec, err := cloudSpecForModel(ctx, model, cloudService, credentialService)
		if err != nil {
			return nil, errors.Trace(err)
		}
		k8sConfig, err := kubernetes.BuiltInConfig(spec)
		if err != nil {
			return nil, errors.Trace(err)
		}
		k8sBackendID := model.UUID()
		info.Configs[k8sBackendID] = provider.ModelBackendConfig{
			ControllerUUID: model.ControllerUUID(),
			ModelUUID:      model.UUID(),
			ModelName:      model.Name(),
			BackendConfig:  *k8sConfig,
		}
		if backendName == provider.Auto {
			info.ActiveID = k8sBackendID
		}
	}

	backendState := GetSecretBackendsState(model)
	backends, err := backendState.ListSecretBackends()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, b := range backends {
		if b.Name == backendName {
			info.ActiveID = b.ID
		}
		info.Configs[b.ID] = provider.ModelBackendConfig{
			ControllerUUID: model.ControllerUUID(),
			ModelUUID:      model.UUID(),
			ModelName:      model.Name(),
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

// DrainBackendConfigInfo returns the secret backend config for the drain worker to use.
func DrainBackendConfigInfo(
	ctx context.Context, backendID string, model Model,
	secretService SecretService,
	cloudService common.CloudService, credentialService common.CredentialService,
	authTag names.Tag, leadershipChecker leadership.Checker,
) (*provider.ModelBackendConfigInfo, error) {
	adminModelCfg, err := AdminBackendConfigInfo(ctx, model, cloudService, credentialService)
	if err != nil {
		return nil, errors.Annotate(err, "getting configured secrets providers")
	}
	result := provider.ModelBackendConfigInfo{
		ActiveID: adminModelCfg.ActiveID,
		Configs:  make(map[string]provider.ModelBackendConfig),
	}
	if backendID == "" {
		backendID = adminModelCfg.ActiveID
	}

	cfg, ok := adminModelCfg.Configs[backendID]
	if !ok {
		return nil, errors.Errorf("missing secret backend %q", backendID)
	}
	backendCfg, err := backendConfigInfo(ctx, secretService, model, backendID, &cfg, authTag, leadershipChecker, true, true)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.Configs[backendID] = *backendCfg
	return &result, nil
}

// BackendConfigInfo returns the config to create a secret backend
// for the specified backend IDs.
// This is called to provide config to a client like a unit agent which
// needs to access secrets. The authTag is the agent which needs access.
// The client is expected to be restricted to write only those secrets
// owned by the agent, and read only those secrets shared with the agent.
// The result includes config for all relevant backends, including the id
// of the current active backend.
func BackendConfigInfo(
	ctx context.Context, model Model, sameController bool,
	secretService SecretService,
	cloudService common.CloudService, credentialService common.CredentialService,
	backendIDs []string, wantAll bool,
	authTag names.Tag, leadershipChecker leadership.Checker,
) (*provider.ModelBackendConfigInfo, error) {
	adminModelCfg, err := AdminBackendConfigInfo(ctx, model, cloudService, credentialService)
	if err != nil {
		return nil, errors.Annotate(err, "getting configured secrets providers")
	}
	result := provider.ModelBackendConfigInfo{
		ActiveID: adminModelCfg.ActiveID,
		Configs:  make(map[string]provider.ModelBackendConfig),
	}
	if len(backendIDs) == 0 {
		if wantAll {
			for id := range adminModelCfg.Configs {
				backendIDs = append(backendIDs, id)
			}
		} else {
			backendIDs = []string{adminModelCfg.ActiveID}
		}
	}
	for _, backendID := range backendIDs {
		cfg, ok := adminModelCfg.Configs[backendID]
		if !ok {
			return nil, errors.Errorf("missing secret backend %q", backendID)
		}
		backendCfg, err := backendConfigInfo(ctx, secretService, model, backendID, &cfg, authTag, leadershipChecker, sameController, false)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.Configs[backendID] = *backendCfg
	}
	return &result, nil
}

func backendConfigInfo(
	ctx context.Context,
	secretService SecretService,
	model Model, backendID string, adminCfg *provider.ModelBackendConfig,
	authTag names.Tag, leadershipChecker leadership.Checker, sameController, forDrain bool,
) (*provider.ModelBackendConfig, error) {
	p, err := GetProvider(adminCfg.BackendType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = p.Initialise(adminCfg)
	if err != nil {
		return nil, errors.Annotate(err, "initialising secrets provider")
	}

	ownedRevisions := map[string]provider.SecretRevisions{}
	readRevisions := map[string]provider.SecretRevisions{}
	switch t := authTag.(type) {
	case names.UnitTag:
		unitName := authTag.Id()
		// Find secrets owned by the agent
		// (or its app if the agent is a leader).
		owners := []secretservice.CharmSecretOwner{{
			Kind: secretservice.UnitOwner,
			ID:   unitName,
		}}
		appName := AuthTagApp(t)
		token := leadershipChecker.LeadershipCheck(appName, t.Id())
		err := token.Check()
		if err != nil && !leadership.IsNotLeaderError(err) {
			return nil, errors.Trace(err)
		}
		if err == nil {
			// Leader unit owns application level secrets.
			owners = append(owners, secretservice.CharmSecretOwner{
				Kind: secretservice.ApplicationOwner,
				ID:   appName,
			})
		} else {
			// Non leader units can read application level secrets.
			// Find secrets owned by the application.
			readOnlyOwners := []secretservice.CharmSecretOwner{{
				Kind: secretservice.ApplicationOwner,
				ID:   appName,
			}}
			secrets, revisionMetadata, err := secretService.ListCharmSecrets(ctx, readOnlyOwners...)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if err := composeExternalRevisions(backendID, secrets, revisionMetadata, readRevisions); err != nil {
				return nil, errors.Trace(err)
			}
		}
		secrets, revisionMetadata, err := secretService.ListCharmSecrets(ctx, owners...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if err := composeExternalRevisions(backendID, secrets, revisionMetadata, ownedRevisions); err != nil {
			return nil, errors.Trace(err)
		}

		// Granted secrets can be consumed in application level for all units.
		// We include secrets shared with the app or just the specified unit.
		consumers := []secretservice.SecretAccessor{{
			Kind: secretservice.UnitAccessor,
			ID:   unitName,
		}, {
			Kind: secretservice.ApplicationAccessor,
			ID:   appName,
		}}
		secrets, revisionMetadata, err = secretService.ListGrantedSecrets(ctx, consumers...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if err := composeExternalRevisions(backendID, secrets, revisionMetadata, readRevisions); err != nil {
			return nil, errors.Trace(err)
		}
	case names.ModelTag:
		// Model Tag is valid for user secrets.
		secrets, revisionMetadata, err := secretService.ListUserSecrets(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if err := composeExternalRevisions(backendID, secrets, revisionMetadata, ownedRevisions); err != nil {
			return nil, errors.Trace(err)
		}
	default:
		return nil, errors.NotSupportedf("login as %q", authTag)
	}

	logger.Debugf("secrets for %v:\nowned: %v\nconsumed:%v", authTag.String(), ownedRevisions, readRevisions)
	cfg, err := p.RestrictedConfig(ctx, adminCfg, sameController, forDrain, authTag, ownedRevisions[backendID], readRevisions[backendID])
	if err != nil {
		return nil, errors.Trace(err)
	}
	info := &provider.ModelBackendConfig{
		ControllerUUID: model.ControllerUUID(),
		ModelUUID:      model.UUID(),
		ModelName:      model.Name(),
		BackendConfig:  *cfg,
	}
	return info, nil
}

func composeExternalRevisions(
	backendID string,
	metadata []*coresecrets.SecretMetadata,
	revisionMetadata [][]*coresecrets.SecretRevisionMetadata,
	revisions map[string]provider.SecretRevisions,
) error {
	for i, md := range metadata {
		for _, rev := range revisionMetadata[i] {
			if rev.ValueRef == nil || rev.ValueRef.BackendID != backendID {
				continue
			}
			revs, ok := revisions[rev.ValueRef.BackendID]
			if !ok {
				revs = provider.SecretRevisions{}
			}
			revs.Add(md.URI, rev.ValueRef.RevisionID)
			revisions[rev.ValueRef.BackendID] = revs
		}
	}
	return nil
}

func cloudSpecForModel(
	ctx context.Context, m Model, cloudService common.CloudService, credentialService common.CredentialService,
) (cloudspec.CloudSpec, error) {
	c, err := cloudService.Cloud(ctx, m.CloudName())
	if err != nil {
		return cloudspec.CloudSpec{}, errors.Trace(err)
	}
	tag, ok := m.CloudCredentialTag()
	if !ok {
		return cloudspec.CloudSpec{}, errors.NotValidf("cloud credential for %s is empty", m.UUID())
	}
	cred, err := credentialService.CloudCredential(ctx, credential.KeyFromTag(tag))
	if err != nil {
		return cloudspec.CloudSpec{}, errors.Trace(err)
	}
	return cloudspec.MakeCloudSpec(*c, "", &cred)
}

// BackendFilter is used when listing secret backends.
type BackendFilter struct {
	Names []string
	All   bool
}

// BackendSummaryInfo returns a summary of the status of the secret backends relevant to the specified models.
// This method is used by the secretsbackend and modelmanager client facades; it is tested on the secretsbackend facade package.
func BackendSummaryInfo(
	statePool StatePool, backendState SecretsBackendState, secretState SecretsState, controllerUUID string, reveal bool, filter BackendFilter,
) ([]params.SecretBackendResult, error) {
	backendIDSecrets, err := secretState.ListModelSecrets(filter.All)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// If we want all backends, include those which are not in use.
	if filter.All {
		// The internal (controller) backend.
		if _, ok := backendIDSecrets[controllerUUID]; !ok {
			backendIDSecrets[controllerUUID] = set.NewStrings()
		}
		allBackends, err := backendState.ListSecretBackends()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, b := range allBackends {
			if _, ok := backendIDSecrets[b.ID]; ok {
				continue
			}
			backendIDSecrets[b.ID] = set.NewStrings()
		}
	}

	// Order for tests.
	var backendIDs []string
	for id := range backendIDSecrets {
		backendIDs = append(backendIDs, id)
	}
	sort.Strings(backendIDs)

	var results []params.SecretBackendResult
	wanted := set.NewStrings(filter.Names...)
	for _, id := range backendIDs {
		backendResult, err := getSecretBackendInfo(statePool, backendState, controllerUUID, id, wanted, reveal)
		if err != nil {
			// When we get not found, the backend has been deleted,even though it contained secrets.
			// We skip over such cases.
			if !errors.Is(err, errors.NotFound) {
				results = append(results, params.SecretBackendResult{
					ID:    id,
					Error: apiservererrors.ServerError(err)})
			}
			continue
		}
		// For local k8s secrets, corresponding to every hosted model,
		// do not include the result if there are no secrets.
		numSecrets := backendIDSecrets[id].Size()
		if numSecrets == 0 && filter.All && kubernetes.IsBuiltInName(backendResult.Result.Name) {
			continue
		}
		backendResult.NumSecrets = numSecrets
		results = append(results, *backendResult)
	}
	return results, nil
}

func getSecretBackendInfo(statePool StatePool, backendState SecretsBackendState, controllerUUID string, id string, wanted set.Strings, reveal bool) (*params.SecretBackendResult, error) {
	var (
		b   *coresecrets.SecretBackend
		err error
	)
	if !coresecrets.IsInternalSecretBackendID(id) {
		b, err = backendState.GetSecretBackendByID(id)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	pingRequired := true
	// Not an external backend, so might be internal/local.
	if b == nil {
		// No need to ping "internal" backends.
		pingRequired = false
		if id == controllerUUID {
			b = &coresecrets.SecretBackend{
				ID:          id,
				Name:        juju.BackendName,
				BackendType: juju.BackendType,
			}
		} else {
			model, releaser, err := statePool.GetModel(id)
			if err != nil {
				return nil, errors.Trace(err)
			}
			b = &coresecrets.SecretBackend{
				ID:          id,
				Name:        kubernetes.BuiltInName(model.Name()),
				BackendType: kubernetes.BackendType,
			}
			releaser()
		}
	}
	// Filter out unwanted backends - caller ignores not found errors.
	if !wanted.IsEmpty() && !wanted.Contains(b.Name) {
		return nil, errors.NotFoundf("backend %v", b.Name)
	}
	cfg := make(map[string]interface{})
	for k, v := range b.Config {
		cfg[k] = v
	}
	p, err := GetProvider(b.BackendType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	configValidator, ok := p.(provider.ProviderConfig)
	if ok {
		for n, f := range configValidator.ConfigSchema() {
			if f.Secret && !reveal {
				delete(cfg, n)
			}
		}
	}
	result := &params.SecretBackendResult{
		Result: params.SecretBackend{
			Name:                b.Name,
			BackendType:         b.BackendType,
			TokenRotateInterval: b.TokenRotateInterval,
			Config:              cfg,
		},
		ID:         id,
		NumSecrets: 0,
		Status:     status.Active.String(),
	}
	if pingRequired {
		err = PingBackend(p, b.Config)
		if err != nil {
			result.Status = status.Error.String()
			result.Message = err.Error()
		}
	}
	return result, nil
}

// PingBackend instantiates a backend and pings it.
func PingBackend(p provider.SecretBackendProvider, cfg provider.ConfigAttrs) error {
	b, err := p.NewBackend(&provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{BackendType: p.Type(), Config: cfg},
	})
	if err != nil {
		return errors.Annotate(err, "checking backend")
	}
	return b.Ping()
}
