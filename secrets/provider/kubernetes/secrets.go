// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/secrets/provider"
)

const (
	// Store is the name of the Kubernetes secrets store.
	Store = "kubernetes"
)

// NewProvider returns a Kubernetes secrets provider.
func NewProvider() provider.SecretStoreProvider {
	return k8sProvider{}
}

type k8sProvider struct {
}

// StoreConfig returns the config needed to create a k8s secrets store.
func (p k8sProvider) StoreConfig(m provider.Model) (*provider.StoreConfig, error) {
	cloudSpec, err := cloudSpecForModel(m)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg, err := m.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cred, err := json.Marshal(cloudSpec.Credential)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpec.Credential = nil
	storeCfg := &provider.StoreConfig{
		StoreType: Store,
		Params: map[string]interface{}{
			"controller-uuid":     m.ControllerUUID(),
			"model-name":          cfg.Name(),
			"model-type":          cfg.Type(),
			"model-uuid":          cfg.UUID(),
			"endpoint":            cloudSpec.Endpoint,
			"ca-certs":            cloudSpec.CACertificates,
			"is-controller-cloud": cloudSpec.IsControllerCloud,
			"credential":          string(cred),
		},
	}
	return storeCfg, nil
}

// NewCaas is patched for testing.
var NewCaas = caas.New

// NewStore returns a k8s backed secrets store.
func (p k8sProvider) NewStore(cfg *provider.StoreConfig) (provider.SecretsStore, error) {
	modelName := cfg.Params["model-name"].(string)
	modelType := cfg.Params["model-type"].(string)
	modelUUID := cfg.Params["model-uuid"].(string)
	modelCfg, err := config.New(config.UseDefaults, map[string]interface{}{
		config.NameKey: modelName,
		config.TypeKey: modelType,
		config.UUIDKey: modelUUID,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpec := cloudspec.CloudSpec{
		Type:              "kubernetes",
		Name:              "secret-access",
		Endpoint:          cfg.Params["endpoint"].(string),
		IsControllerCloud: cfg.Params["is-controller-cloud"].(bool),
	}
	var ok bool
	cloudSpec.CACertificates, ok = cfg.Params["ca-certs"].([]string)
	if !ok {
		certs := cfg.Params["ca-certs"].([]interface{})
		cloudSpec.CACertificates = make([]string, len(certs))
		for i, cert := range certs {
			cloudSpec.CACertificates[i] = fmt.Sprintf("%s", cert)
		}
	}
	var cred cloud.Credential
	err = json.Unmarshal([]byte(cfg.Params["credential"].(string)), &cred)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpec.Credential = &cred

	broker, err := NewCaas(context.TODO(), environs.OpenParams{
		ControllerUUID: cfg.Params["controller-uuid"].(string),
		Cloud:          cloudSpec,
		Config:         modelCfg,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &k8sStore{broker: broker}, nil
}

func cloudSpecForModel(m provider.Model) (cloudspec.CloudSpec, error) {
	c, err := m.Cloud()
	if err != nil {
		return cloudspec.CloudSpec{}, errors.Trace(err)
	}
	cloudCredential, err := m.CloudCredential()
	if err != nil {
		return cloudspec.CloudSpec{}, errors.Trace(err)
	}
	return cloudspec.MakeCloudSpec(c, "", cloudCredential)
}

type k8sStore struct {
	broker caas.SecretsStore
}

// GetContent implements SecretsStore.
func (k k8sStore) GetContent(ctx context.Context, providerId string) (secrets.SecretValue, error) {
	return k.broker.GetJujuSecret(ctx, providerId)
}

// DeleteContent implements SecretsStore.
func (k k8sStore) DeleteContent(ctx context.Context, providerId string) error {
	return k.broker.DeleteJujuSecret(ctx, providerId)
}

// SaveContent implements SecretsStore.
func (k k8sStore) SaveContent(ctx context.Context, uri *secrets.URI, revision int, value secrets.SecretValue) (string, error) {
	return k.broker.SaveJujuSecret(ctx, uri, revision, value)
}
