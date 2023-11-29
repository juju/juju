// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/leadership"
	corelogger "github.com/juju/juju/core/logger"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/secrets/provider/juju"
	"github.com/juju/juju/secrets/provider/kubernetes"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLoggerWithLabels("juju.apiserver.common.secrets", corelogger.SECRETS)

// For testing.
var (
	GetProvider            = provider.Provider
	GetSecretsState        = getSecretsState
	GetSecretBackendsState = getSecretBackendsState
)

func getSecretsState(m Model) state.SecretsStore {
	return state.NewSecrets(m.State())
}

func getSecretBackendsState(m Model) state.SecretBackendsStorage {
	return state.NewSecretBackends(m.State())
}

// BackendConfigGetter is a func used to get secret backend config.
type BackendConfigGetter func(backendIDs []string, wantAll bool) (*provider.ModelBackendConfigInfo, error)

// BackendAdminConfigGetter is a func used to get admin level secret backend config.
type BackendAdminConfigGetter func() (*provider.ModelBackendConfigInfo, error)

// BackendDrainConfigGetter is a func used to get secret backend config for draining.
type BackendDrainConfigGetter func(string) (*provider.ModelBackendConfigInfo, error)

// AdminBackendConfigInfo returns the admin config for the secret backends is use by
// the specified model.
// If external backend is configured, it returns the external backend together with the "internal" backend and
// the k8s backend for k8s models.
func AdminBackendConfigInfo(model Model) (*provider.ModelBackendConfigInfo, error) {
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
		spec, err := cloudSpecForModel(model)
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
func DrainBackendConfigInfo(backendID string, model Model, authTag names.Tag, leadershipChecker leadership.Checker) (*provider.ModelBackendConfigInfo, error) {
	adminModelCfg, err := AdminBackendConfigInfo(model)
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
	backendCfg, err := backendConfigInfo(model, backendID, &cfg, authTag, leadershipChecker, true)
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
func BackendConfigInfo(model Model, backendIDs []string, wantAll bool, authTag names.Tag, leadershipChecker leadership.Checker) (*provider.ModelBackendConfigInfo, error) {
	adminModelCfg, err := AdminBackendConfigInfo(model)
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
		backendCfg, err := backendConfigInfo(model, backendID, &cfg, authTag, leadershipChecker, false)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.Configs[backendID] = *backendCfg
	}
	return &result, nil
}

func backendConfigInfo(
	model Model, backendID string, adminCfg *provider.ModelBackendConfig,
	authTag names.Tag, leadershipChecker leadership.Checker, forDrain bool,
) (*provider.ModelBackendConfig, error) {
	p, err := GetProvider(adminCfg.BackendType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = p.Initialise(adminCfg)
	if err != nil {
		return nil, errors.Annotate(err, "initialising secrets provider")
	}
	secretsState := GetSecretsState(model)

	// Find secrets owned by the agent
	// (or its app if the agent is a leader).
	ownedFilter := state.SecretsFilter{
		OwnerTags: []names.Tag{authTag},
	}
	// Find secrets shared with the agent.
	// We include secrets shared with the app or just the specified unit.
	readFilter := state.SecretsFilter{
		ConsumerTags: []names.Tag{authTag},
	}
	// Find secrets owned by the application that should be readable for non leader units.
	readAppOwnedFilter := state.SecretsFilter{}
	switch t := authTag.(type) {
	case names.UnitTag:
		appName, _ := names.UnitApplication(t.Id())
		authApp := names.NewApplicationTag(appName)
		token := leadershipChecker.LeadershipCheck(appName, t.Id())
		err := token.Check()
		if err != nil && !leadership.IsNotLeaderError(err) {
			return nil, errors.Trace(err)
		}
		if err == nil {
			// Leader unit owns application level secrets.
			ownedFilter.OwnerTags = append(ownedFilter.OwnerTags, authApp)
		} else {
			// Non leader units can read application level secrets.
			// Find secrets owned by the application.
			readAppOwnedFilter.OwnerTags = append(readAppOwnedFilter.OwnerTags, authApp)
		}
		// Granted secrets can be consumed in application level for all units.
		readFilter.ConsumerTags = append(readFilter.ConsumerTags, authApp)
	case names.ApplicationTag:
	case names.ModelTag:
		// Model Tag is validate for user secrets.
	default:
		return nil, errors.NotSupportedf("login as %q", authTag)
	}

	ownedRevisions := map[string]provider.SecretRevisions{}
	if err := getExternalRevisions(secretsState, backendID, ownedFilter, ownedRevisions); err != nil {
		return nil, errors.Trace(err)
	}

	readRevisions := map[string]provider.SecretRevisions{}
	if err := getExternalRevisions(secretsState, backendID, readFilter, readRevisions); err != nil {
		return nil, errors.Trace(err)
	}

	if len(readAppOwnedFilter.OwnerTags) > 0 {
		if err := getExternalRevisions(secretsState, backendID, readAppOwnedFilter, readRevisions); err != nil {
			return nil, errors.Trace(err)
		}
	}

	logger.Debugf("secrets for %v:\nowned: %v\nconsumed:%v", authTag.String(), ownedRevisions, readRevisions)
	cfg, err := p.RestrictedConfig(adminCfg, forDrain, authTag, ownedRevisions[backendID], readRevisions[backendID])
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

func getExternalRevisions(backend state.SecretsStore, backendID string, filter state.SecretsFilter, revisions map[string]provider.SecretRevisions) error {
	secrets, err := backend.ListSecrets(filter)
	if err != nil {
		return errors.Trace(err)
	}
	for _, md := range secrets {
		revs, err := backend.ListSecretRevisions(md.URI)
		if err != nil {
			return errors.Annotatef(err, "cannot get revisions for secret %q", md.URI)
		}
		for _, rev := range revs {
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

func cloudSpecForModel(m Model) (cloudspec.CloudSpec, error) {
	c, err := m.Cloud()
	if err != nil {
		return cloudspec.CloudSpec{}, errors.Trace(err)
	}
	cred, err := m.CloudCredential()
	if err != nil {
		return cloudspec.CloudSpec{}, errors.Trace(err)
	}
	if cred == nil {
		return cloudspec.CloudSpec{}, errors.NotValidf("cloud credential for %s is empty", m.UUID())
	}
	cloudCredential := cloud.NewCredential(
		cloud.AuthType(cred.AuthType()),
		cred.Attributes(),
	)
	return cloudspec.MakeCloudSpec(c, "", &cloudCredential)
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
			if !errors.IsNotFound(err) {
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

// GetSecretMetadata returns the secrets metadata for the given filter.
func GetSecretMetadata(
	ownerTag names.Tag, secretsState SecretsMetaState, leadershipChecker leadership.Checker,
	filter func(*coresecrets.SecretMetadata, *coresecrets.SecretRevisionMetadata) bool,
) (params.ListSecretResults, error) {
	var result params.ListSecretResults
	listFilter := state.SecretsFilter{
		// TODO: there is a bug that operator agents can't get any unit owned secrets!
		// Because the ownerTag here is the application tag, but not unit tag.
		OwnerTags: []names.Tag{ownerTag},
	}
	if ownerTag.Kind() == names.UnitTagKind {
		// Unit leaders can also get metadata for secrets owned by the app.
		// TODO(wallyworld) - temp fix for old podspec charms
		isLeader, err := IsLeaderUnit(ownerTag, leadershipChecker)
		if err != nil {
			return result, errors.Trace(err)
		}
		if isLeader {
			appOwner := names.NewApplicationTag(AuthTagApp(ownerTag))
			listFilter.OwnerTags = append(listFilter.OwnerTags, appOwner)
		}
	}

	secrets, err := secretsState.ListSecrets(listFilter)
	if err != nil {
		return result, errors.Trace(err)
	}
	for _, md := range secrets {
		secretResult := params.ListSecretResult{
			URI:              md.URI.String(),
			Version:          md.Version,
			OwnerTag:         md.OwnerTag,
			RotatePolicy:     md.RotatePolicy.String(),
			NextRotateTime:   md.NextRotateTime,
			Description:      md.Description,
			Label:            md.Label,
			LatestRevision:   md.LatestRevision,
			LatestExpireTime: md.LatestExpireTime,
			CreateTime:       md.CreateTime,
			UpdateTime:       md.UpdateTime,
		}
		grants, err := secretsState.SecretGrants(md.URI, coresecrets.RoleView)
		if err != nil {
			return result, errors.Trace(err)
		}
		for _, g := range grants {
			secretResult.Access = append(secretResult.Access, params.AccessInfo{
				Target: g.Target, Scope: g.Scope, Role: g.Role,
			})
		}

		revs, err := secretsState.ListSecretRevisions(md.URI)
		if err != nil {
			return result, errors.Trace(err)
		}
		for _, r := range revs {
			if filter != nil && !filter(md, r) {
				continue
			}
			var valueRef *params.SecretValueRef
			if r.ValueRef != nil {
				valueRef = &params.SecretValueRef{
					BackendID:  r.ValueRef.BackendID,
					RevisionID: r.ValueRef.RevisionID,
				}
			}
			secretResult.Revisions = append(secretResult.Revisions, params.SecretRevision{
				Revision: r.Revision,
				ValueRef: valueRef,
			})
		}
		if len(secretResult.Revisions) == 0 {
			continue
		}
		result.Results = append(result.Results, secretResult)
	}
	return result, nil
}

// RemoveSecretsForAgent removes the specified secrets for agent.
// The secrets are only removed from the state and
// the caller must have permission to manage the secret(secret owners remove secrets from the backend on uniter side).
func RemoveSecretsForAgent(
	removeState SecretsRemoveState, adminConfigGetter BackendAdminConfigGetter,
	args params.DeleteSecretArgs,
	modelUUID string,
	canDelete func(*coresecrets.URI) error,
) (params.ErrorResults, error) {
	return removeSecrets(
		removeState, adminConfigGetter, args,
		modelUUID,
		canDelete,
		func(provider.SecretBackendProvider, provider.ModelBackendConfig, provider.SecretRevisions) error {
			return nil
		},
	)
}

// RemoveUserSecrets removes the specified user supplied secrets.
// The secrets are removed from the state and backend, and the caller must have model admin access.
func RemoveUserSecrets(
	removeState SecretsRemoveState, adminConfigGetter BackendAdminConfigGetter,
	authTag names.Tag, args params.DeleteSecretArgs,
	modelUUID string,
	canDelete func(*coresecrets.URI) error,
) (params.ErrorResults, error) {
	return removeSecrets(
		removeState, adminConfigGetter, args, modelUUID, canDelete,
		func(p provider.SecretBackendProvider, cfg provider.ModelBackendConfig, revs provider.SecretRevisions) error {
			backend, err := p.NewBackend(&cfg)
			if err != nil {
				return errors.Trace(err)
			}
			for _, revId := range revs.RevisionIDs() {
				if err = backend.DeleteContent(context.TODO(), revId); err != nil {
					return errors.Trace(err)
				}
			}
			if err := p.CleanupSecrets(&cfg, authTag, revs); err != nil {
				return errors.Trace(err)
			}
			return nil
		},
	)
}

func getSecretURIForLabel(secretsState ListSecretsState, modelUUID string, label string) (*coresecrets.URI, error) {
	results, err := secretsState.ListSecrets(state.SecretsFilter{
		Label:     &label,
		OwnerTags: []names.Tag{names.NewModelTag(modelUUID)},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results) == 0 {
		return nil, errors.NotFoundf("secret %q", label)
	}
	if len(results) > 1 {
		return nil, errors.NotFoundf("more than 1 secret with label %q", label)
	}
	return results[0].URI, nil
}

func removeSecrets(
	removeState SecretsRemoveState, adminConfigGetter BackendAdminConfigGetter,
	args params.DeleteSecretArgs,
	modelUUID string,
	canDelete func(*coresecrets.URI) error,
	removeFromBackend func(provider.SecretBackendProvider, provider.ModelBackendConfig, provider.SecretRevisions) error,
) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}

	if len(args.Args) == 0 {
		return result, nil
	}
	cfgInfo, err := adminConfigGetter()
	if err != nil {
		return result, errors.Trace(err)
	}

	removeFromExternal := func(uri *coresecrets.URI, revisions ...int) error {
		externalRevs := make(map[string]provider.SecretRevisions)
		gatherExternalRevs := func(valRef *coresecrets.ValueRef) {
			if valRef == nil {
				// Internal secret, nothing to do here.
				return
			}
			if _, ok := externalRevs[valRef.BackendID]; !ok {
				externalRevs[valRef.BackendID] = provider.SecretRevisions{}
			}
			externalRevs[valRef.BackendID].Add(uri, valRef.RevisionID)
		}
		if len(revisions) == 0 {
			// Remove all revisions.
			revs, err := removeState.ListSecretRevisions(uri)
			if err != nil {
				return errors.Trace(err)
			}
			for _, rev := range revs {
				gatherExternalRevs(rev.ValueRef)
			}
		} else {
			for _, rev := range revisions {
				revMeta, err := removeState.GetSecretRevision(uri, rev)
				if err != nil {
					return errors.Trace(err)
				}
				gatherExternalRevs(revMeta.ValueRef)
			}
		}

		for backendID, r := range externalRevs {
			backendCfg, ok := cfgInfo.Configs[backendID]
			if !ok {
				return errors.NotFoundf("secret backend %q", backendID)
			}
			provider, err := GetProvider(backendCfg.BackendType)
			if err != nil {
				return errors.Trace(err)
			}
			if err := removeFromBackend(provider, backendCfg, r); err != nil {
				return errors.Trace(err)
			}
		}
		return nil
	}

	for i, arg := range args.Args {
		if arg.URI == "" && arg.Label == "" {
			result.Results[i].Error = apiservererrors.ServerError(errors.New("must specify either URI or label"))
			continue
		}

		var (
			uri *coresecrets.URI
			err error
		)
		if arg.URI != "" {
			uri, err = coresecrets.ParseURI(arg.URI)
		} else {
			uri, err = getSecretURIForLabel(removeState, modelUUID, arg.Label)
		}
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if _, err := removeState.GetSecret(uri); err != nil {
			// Check if the uri exists or not.
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err := canDelete(uri); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err := removeFromExternal(uri, arg.Revisions...); err != nil {
			// We remove the secret from the backend first.
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if _, err = removeState.DeleteSecret(uri, arg.Revisions...); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return result, nil
}
