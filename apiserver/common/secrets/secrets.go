// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/leadership"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/internal/secrets/provider"
)

var logger = loggo.GetLoggerWithTags("juju.apiserver.common.secrets", corelogger.SECRETS)

// For testing.
var (
	GetProvider = provider.Provider
)

// BackendConfigGetter is a func used to get secret backend config.
type BackendConfigGetter func(ctx context.Context, backendIDs []string, wantAll bool) (*provider.ModelBackendConfigInfo, error)

// BackendDrainConfigGetter is a func used to get secret backend config for draining.
type BackendDrainConfigGetter func(context.Context, string) (*provider.ModelBackendConfigInfo, error)

// DrainBackendConfigInfo returns the secret backend config for the drain worker to use.
func DrainBackendConfigInfo(
	ctx context.Context, backendID string, model Model,
	secretService SecretService,
	secretBackendService SecretBackendService,
	authTag names.Tag, leadershipChecker leadership.Checker,
) (*provider.ModelBackendConfigInfo, error) {
	adminModelCfg, err := secretBackendService.GetSecretBackendConfigForAdmin(ctx, coremodel.UUID(model.UUID()))
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
	secretBackendService SecretBackendService,
	backendIDs []string, wantAll bool,
	authTag names.Tag, leadershipChecker leadership.Checker,
) (*provider.ModelBackendConfigInfo, error) {
	adminModelCfg, err := secretBackendService.GetSecretBackendConfigForAdmin(ctx, coremodel.UUID(model.UUID()))
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
		appName, _ := names.UnitApplication(t.Id())
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
