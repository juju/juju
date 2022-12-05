// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/caas"
	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.secrets.provider.kubernetes")

const (
	// BackendType is the type of the Kubernetes secrets backend.
	BackendType = "kubernetes"
)

// NewProvider returns a Kubernetes secrets provider.
func NewProvider() provider.SecretBackendProvider {
	return k8sProvider{}
}

type k8sProvider struct {
}

func (p k8sProvider) Type() string {
	return BackendType
}

// Initialise is not used.
func (p k8sProvider) Initialise(m provider.Model) error {
	return nil
}

// CleanupModel is not used.
func (p k8sProvider) CleanupModel(m provider.Model) error {
	return nil
}

func (p k8sProvider) getBroker(
	controllerUUID, modelUUID, modelName string, cloudSpec cloudspec.CloudSpec,
) (Broker, error) {
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		config.NameKey: modelName,
		config.UUIDKey: modelUUID,
		config.TypeKey: state.ModelTypeCAAS,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewCaas(context.TODO(), environs.OpenParams{
		ControllerUUID: controllerUUID,
		Cloud:          cloudSpec,
		Config:         cfg,
	})
}

// CleanupSecrets removes rules of the role associated with the removed secrets.
func (p k8sProvider) CleanupSecrets(m provider.Model, tag names.Tag, removed provider.SecretRevisions) error {
	if tag == nil {
		// This should never happen.
		// Because this method is used for uniter facade only.
		return errors.New("empty tag")
	}
	if len(removed) == 0 {
		return nil
	}
	cloudSpec, err := cloudSpecForModel(m)
	if err != nil {
		return errors.Trace(err)
	}
	broker, err := p.getBroker(m.ControllerUUID(), m.UUID(), m.Name(), cloudSpec)
	if err != nil {
		return errors.Trace(err)
	}
	_, err = broker.EnsureSecretAccessToken(tag, nil, nil, removed.Names())
	return errors.Trace(err)
}

func cloudSpecToBackendConfig(m provider.Model, spec cloudspec.CloudSpec) (*provider.BackendConfig, error) {
	cred, err := json.Marshal(spec.Credential)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &provider.BackendConfig{
		ControllerUUID: m.ControllerUUID(),
		ModelUUID:      m.UUID(),
		ModelName:      m.Name(),
		BackendType:    BackendType,
		Config: map[string]interface{}{
			"endpoint":            spec.Endpoint,
			"ca-certs":            spec.CACertificates,
			"is-controller-cloud": spec.IsControllerCloud,
			"credential":          string(cred),
		},
	}, nil
}

// BackendConfig returns the config needed to create a k8s secrets backend.
func (p k8sProvider) BackendConfig(m provider.Model, consumer names.Tag, owned provider.SecretRevisions, read provider.SecretRevisions) (*provider.BackendConfig, error) {
	logger.Tracef("getting k8s backend config for %q, owned %v, read %v", consumer, owned, read)

	cloudSpec, err := cloudSpecForModel(m)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if consumer == nil {
		return cloudSpecToBackendConfig(m, cloudSpec)
	}

	broker, err := p.getBroker(m.ControllerUUID(), m.UUID(), m.Name(), cloudSpec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	token, err := broker.EnsureSecretAccessToken(consumer, owned.Names(), read.Names(), nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cred, err := k8scloud.UpdateCredentialWithToken(*cloudSpec.Credential, token)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpec.Credential = &cred

	if cloudSpec.IsControllerCloud {
		// The cloudspec used for controller has a fake endpoint (address and port)
		// because we ignore the endpoint and load the in-cluster credential instead.
		// So we have to clean up the endpoint here for uniter to use.

		host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
		if len(host) != 0 && len(port) != 0 {
			cloudSpec.Endpoint = "https://" + net.JoinHostPort(host, port)
			logger.Tracef("patching endpoint to %q", cloudSpec.Endpoint)
			cloudSpec.IsControllerCloud = false
		}
	}
	return cloudSpecToBackendConfig(m, cloudSpec)
}

type Broker interface {
	caas.SecretsBackend
	caas.SecretsProvider
}

// NewCaas is patched for testing.
var NewCaas = newCaas

func newCaas(ctx context.Context, args environs.OpenParams) (Broker, error) {
	return caas.New(ctx, args)
}

// NewBackend returns a k8s backed secrets backend.
func (p k8sProvider) NewBackend(cfg *provider.BackendConfig) (provider.SecretsBackend, error) {
	cloudSpec := cloudspec.CloudSpec{
		Type:              "kubernetes",
		Name:              "secret-access",
		Endpoint:          cfg.Config["endpoint"].(string),
		IsControllerCloud: cfg.Config["is-controller-cloud"].(bool),
	}
	var ok bool
	cloudSpec.CACertificates, ok = cfg.Config["ca-certs"].([]string)
	if !ok {
		certs := cfg.Config["ca-certs"].([]interface{})
		cloudSpec.CACertificates = make([]string, len(certs))
		for i, cert := range certs {
			cloudSpec.CACertificates[i] = fmt.Sprintf("%s", cert)
		}
	}
	var cred cloud.Credential
	err := json.Unmarshal([]byte(cfg.Config["credential"].(string)), &cred)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpec.Credential = &cred

	broker, err := p.getBroker(cfg.ControllerUUID, cfg.ModelUUID, cfg.ModelName, cloudSpec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &k8sBackend{broker: broker}, nil
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
	if cloudCredential == nil {
		return cloudspec.CloudSpec{}, errors.NotValidf("cloud credential for %s is empty", m.UUID())
	}
	return cloudspec.MakeCloudSpec(c, "", cloudCredential)
}
