// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/secrets/provider/juju"
	"github.com/juju/juju/secrets/provider/kubernetes"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.common.secrets")

// For testing.
var (
	GetProvider     = provider.Provider
	GetStateBackEnd = getStateBackEnd
)

func getStateBackEnd(m Model) state.SecretsStore {
	return state.NewSecrets(m.State())
}

// Model defines a subset of state model methods.
type Model interface {
	ControllerUUID() string
	Cloud() (cloud.Cloud, error)
	CloudCredential() (state.Credential, bool, error)
	Config() (*config.Config, error)
	UUID() string
	Type() state.ModelType
	State() *state.State
}

// BackendConfigGetter is a func used to get secret backend config.
type BackendConfigGetter func() (*provider.BackendConfig, error)

// ProviderInfoGetter is a func used to get a secret backend provider.
type ProviderInfoGetter func() (provider.SecretBackendProvider, provider.Model, error)

// ProviderInfoForModel returns the secret backend provider for the specified model.
func ProviderInfoForModel(model Model) (provider.SecretBackendProvider, provider.Model, error) {
	p, err := providerForModel(model)
	if err != nil {
		return nil, nil, errors.Annotate(err, "getting configured secrets provider")
	}
	return p, &modelAdaptor{model}, nil
}

// providerForModel returns the secret backend provider for the specified model.
// If no backend is configured, the "juju" backend is used for machine models and
// the k8s backend is used for k8s models.
func providerForModel(model Model) (provider.SecretBackendProvider, error) {
	cfg, err := model.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	backendType := cfg.SecretStore()
	if backendType == "" {
		backendType = juju.Backend
		if featureflag.Enabled(feature.DeveloperMode) && model.Type() == state.ModelTypeCAAS {
			backendType = kubernetes.Backend
		}
	}
	return GetProvider(backendType)
}

// BackendConfig returns the config to create a secret backend.
// This is called to provide config to a client like a unit agent which
// needs to access secrets. The authTag is the agent which needs access.
// The client is expected to be restricted to write only those secrets
// owned by the agent, and read only those secrets shared with the agent.
func BackendConfig(model Model, authTag names.Tag, leadershipChecker leadership.Checker) (*provider.BackendConfig, error) {
	ma := &modelAdaptor{model}
	p, err := providerForModel(model)
	if err != nil {
		return nil, errors.Annotate(err, "getting configured secrets provider")
	}
	err = p.Initialise(ma)
	if err != nil {
		return nil, errors.Annotate(err, "initialising secrets provider")
	}
	backend := GetStateBackEnd(model)

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
	default:
		return nil, errors.NotSupportedf("login as %q", authTag)
	}

	ownedRevisions := provider.SecretRevisions{}
	if err := getRevisions(backend, ownedFilter, ownedRevisions); err != nil {
		return nil, errors.Trace(err)
	}

	readRevisions := provider.SecretRevisions{}
	if err := getRevisions(backend, readFilter, readRevisions); err != nil {
		return nil, errors.Trace(err)
	}

	if len(readAppOwnedFilter.OwnerTags) > 0 {
		if err := getRevisions(backend, readAppOwnedFilter, readRevisions); err != nil {
			return nil, errors.Trace(err)
		}
	}

	logger.Debugf("secrets for %v:\nowned: %v\nconsumed:%v", authTag.String(), ownedRevisions, readRevisions)
	cfg, err := p.BackendConfig(ma, authTag, ownedRevisions, readRevisions)
	return cfg, errors.Trace(err)
}

func getRevisions(backend state.SecretsStore, filter state.SecretsFilter, revisions provider.SecretRevisions) error {
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
			revisions.Add(md.URI, rev.Revision)
		}
	}
	return nil
}

// StoreForInspect returns the config to create a secret backend client able
// to read any secrets for that model.
// This is called by the show-secret facade for admin users.
func StoreForInspect(model Model) (provider.SecretsBackend, error) {
	p, err := providerForModel(model)
	if err != nil {
		return nil, errors.Annotate(err, "getting configured secrets provider")
	}
	ma := &modelAdaptor{model}

	err = p.Initialise(ma)
	if err != nil {
		return nil, errors.Annotate(err, "initialising secrets provider")
	}

	cfg, err := p.BackendConfig(ma, nil, nil, nil)
	if err != nil {
		return nil, errors.Annotate(err, "creating secrets backend config")
	}
	return p.NewBackend(cfg)
}

type modelAdaptor struct {
	Model
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
