// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/kr/pretty"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/secrets/provider/juju"
	"github.com/juju/juju/secrets/provider/kubernetes"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.common.secrets")

// StoreConfigGetter is a func used to get secret store config.
type StoreConfigGetter func() (*provider.StoreConfig, error)

// ProviderInfoGetter is a func used to get a secret store provider.
type ProviderInfoGetter func() (provider.SecretStoreProvider, provider.Model, error)

// ProviderInfoForModel returns the secret store provider for the specified model.
func ProviderInfoForModel(model *state.Model) (provider.SecretStoreProvider, provider.Model, error) {
	logger.Criticalf("ProviderInfoForModel")
	p, err := providerForModel(model)
	if err != nil {
		return nil, nil, errors.Annotate(err, "getting configured secrets provider")
	}
	return p, &modelAdaptor{model}, nil
}

// providerForModel returns the secret store provider for the specified model.
// If no store is configured, the "juju" store is used for machine models and
// the k8s store is used for k8s models.
func providerForModel(model *state.Model) (provider.SecretStoreProvider, error) {
	cfg, err := model.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	storeType := cfg.SecretStore()
	if storeType == "" {
		storeType = juju.Store
		if model.Type() == state.ModelTypeCAAS && featureflag.Enabled(feature.SecretStores) {
			storeType = kubernetes.Store
		}
	}
	return provider.Provider(storeType)
}

// StoreConfig returns the config to create a secret store.
// This is called to provide config to a client like a unit agent which
// needs to access secrets. The authTag is the agent which needs access.
// The client is expected to be restricted to write only those secrets
// owned by the agent, and read only those secrets shared with the agent.
func StoreConfig(model *state.Model, authTag names.Tag, leadershipChecker leadership.Checker) (*provider.StoreConfig, error) {
	logger.Criticalf("StoreConfig %q", authTag)
	ma := &modelAdaptor{
		model,
	}
	p, err := providerForModel(model)
	if err != nil {
		return nil, errors.Annotate(err, "getting configured secrets provider")
	}
	err = p.Initialise(ma)
	if err != nil {
		return nil, errors.Annotate(err, "initialising secrets provider")
	}
	backend := state.NewSecrets(model.State())

	// Find secrets owned by the agent
	// (or its app if the agent is a leader).
	filter := state.SecretsFilter{
		OwnerTags: []names.Tag{authTag},
	}
	appName, _ := names.UnitApplication(authTag.Id())
	authApp := names.NewApplicationTag(appName)
	if authTag.Kind() == names.UnitTagKind {
		token := leadershipChecker.LeadershipCheck(appName, authTag.Id())
		err := token.Check()
		if err != nil && !leadership.IsNotLeaderError(err) {
			return nil, errors.Trace(err)
		}
		if err == nil {
			filter.OwnerTags = append(filter.OwnerTags, authApp)
		}
	}
	owned, err := backend.ListSecrets(filter)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ownedURIs := provider.NameMetaSlice{}
	for _, md := range owned {
		logger.Criticalf("owned => %s/%d", md.URI.ID, md.Version)
		ownedURIs.Add(md.URI, md.Version)
	}

	// Find secrets shared with the agent.
	// We include secrets shared with the app or just the specified unit.
	read, err := backend.ListSecrets(state.SecretsFilter{
		ConsumerTags: []names.Tag{authApp, authTag},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	readURIs := provider.NameMetaSlice{}
	for _, md := range read {
		logger.Criticalf("read => %s/%d", md.URI.ID, md.Version)
		readURIs.Add(md.URI, md.Version)
	}
	logger.Debugf("secrets for %v:\nowned: %v\nconsumed:%v", authTag.String(), ownedURIs, readURIs)
	cfg, err := p.StoreConfig(ma, authTag, ownedURIs, readURIs)
	logger.Criticalf("StoreConfig =====> cfg %s", pretty.Sprint(cfg))
	return cfg, errors.Trace(err)
}

// StoreForInspect returns the config to create a secret store client able
// to read any secrets for that model.
// This is called by the show-secret facade for admin users.
func StoreForInspect(model *state.Model) (provider.SecretsStore, error) {
	logger.Criticalf("StoreForInspect")
	p, err := providerForModel(model)
	if err != nil {
		return nil, errors.Annotate(err, "getting configured secrets provider")
	}
	ma := &modelAdaptor{
		model,
	}
	cfg, err := p.StoreConfig(ma, nil, nil, nil)
	if err != nil {
		return nil, errors.Annotate(err, "creating secrets store config")
	}
	return p.NewStore(cfg)
}

type modelAdaptor struct {
	*state.Model
}

// CloudCredential implements Model.
func (m *modelAdaptor) CloudCredential() (*cloud.Credential, error) {
	cred, ok, err := m.Model.CloudCredential()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !ok {
		return nil, nil
	}
	cloudCredentialValue := cloud.NewNamedCredential(cred.Name,
		cloud.AuthType(cred.AuthType),
		cred.Attributes,
		cred.Revoked,
	)
	return &cloudCredentialValue, nil
}
