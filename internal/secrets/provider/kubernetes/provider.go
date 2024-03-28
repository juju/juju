// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/juju/juju/caas"
	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.secrets.provider.kubernetes")

const (
	// BackendName is the name of the Kubernetes secrets backend.
	BackendName = "kubernetes"
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
func (p k8sProvider) Initialise(*provider.ModelBackendConfig) error {
	return nil
}

// CleanupModel is not used.
func (p k8sProvider) CleanupModel(*provider.ModelBackendConfig) error {
	return nil
}

func (p k8sProvider) getBroker(cfg *provider.ModelBackendConfig) (Broker, cloudspec.CloudSpec, error) {
	cloudSpec, err := p.configToCloudSpec(&cfg.BackendConfig)
	if err != nil {
		return nil, cloudspec.CloudSpec{}, errors.Trace(err)
	}
	envCfg, err := config.New(config.UseDefaults, map[string]interface{}{
		config.NameKey: cfg.ModelName,
		config.UUIDKey: cfg.ModelUUID,
		config.TypeKey: state.ModelTypeCAAS,
	})
	if err != nil {
		return nil, cloudspec.CloudSpec{}, errors.Trace(err)
	}
	broker, err := NewCaas(context.TODO(), environs.OpenParams{
		ControllerUUID: cfg.ControllerUUID,
		Cloud:          cloudSpec,
		Config:         envCfg,
	})
	return broker, cloudSpec, errors.Trace(err)
}

// CleanupSecrets removes rules of the role associated with the removed secrets.
func (p k8sProvider) CleanupSecrets(ctx context.Context, cfg *provider.ModelBackendConfig, tag names.Tag, removed provider.SecretRevisions) error {
	if tag == nil {
		// This should never happen.
		// Because this method is used for uniter facade only.
		return errors.New("empty tag")
	}
	if len(removed) == 0 {
		return nil
	}
	broker, _, err := p.getBroker(cfg)
	if err != nil {
		return errors.Trace(err)
	}
	_, err = broker.EnsureSecretAccessToken(ctx, tag, nil, nil, removed.RevisionIDs())
	return errors.Trace(err)
}

func cloudSpecToBackendConfig(spec cloudspec.CloudSpec) (*provider.BackendConfig, error) {
	cred, err := json.Marshal(spec.Credential)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &provider.BackendConfig{
		BackendType: BackendType,
		Config: map[string]interface{}{
			"endpoint":            spec.Endpoint,
			"ca-certs":            spec.CACertificates,
			"is-controller-cloud": spec.IsControllerCloud,
			"credential":          string(cred),
		},
	}, nil
}

// BuiltInConfig returns the config needed to create a k8s secrets backend
// using the same namespace as the specified model.
func BuiltInConfig(cloudSpec cloudspec.CloudSpec) (*provider.BackendConfig, error) {
	return cloudSpecToBackendConfig(cloudSpec)
}

// BuiltInName returns the backend name for the k8s in-model backend.
func BuiltInName(modelName string) string {
	return modelName + "-local"
}

// IsBuiltInName returns true of the backend name is the built-in one.
func IsBuiltInName(backendName string) bool {
	return strings.HasSuffix(backendName, "-local")
}

// RestrictedConfig returns the config needed to create a
// secrets backend client restricted to manage the specified
// owned secrets and read shared secrets for the given entity tag.
func (p k8sProvider) RestrictedConfig(
	ctx context.Context,
	adminCfg *provider.ModelBackendConfig, sameController, forDrain bool, consumer names.Tag, owned provider.SecretRevisions, read provider.SecretRevisions,
) (*provider.BackendConfig, error) {
	logger.Tracef("getting k8s backend config for %q, owned %v, read %v", consumer, owned, read)

	if consumer == nil {
		return &adminCfg.BackendConfig, nil
	}

	broker, cloudSpec, err := p.getBroker(adminCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	token, err := broker.EnsureSecretAccessToken(ctx, consumer, owned.RevisionIDs(), read.RevisionIDs(), nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cred, err := k8scloud.UpdateCredentialWithToken(*cloudSpec.Credential, token)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpec.Credential = &cred

	if sameController && cloudSpec.IsControllerCloud {
		// The cloudspec used for controller has a fake endpoint (address and port)
		// because we ignore the endpoint and load the in-cluster credential instead.
		// So we have to clean up the endpoint here for uniter to use.

		host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
		if len(host) != 0 && len(port) != 0 {
			cloudSpec.Endpoint = "https://" + net.JoinHostPort(host, port)
			logger.Tracef("patching endpoint to %q", cloudSpec.Endpoint)
			cloudSpec.IsControllerCloud = false
		}
	} else {
		cloudSpec.IsControllerCloud = false
	}
	return cloudSpecToBackendConfig(cloudSpec)
}

type Broker interface {
	caas.SecretsBackend
	caas.SecretsProvider
	Version() (ver *version.Number, err error)
}

// NewCaas is patched for testing.
var NewCaas = newCaas

func newCaas(ctx context.Context, args environs.OpenParams) (Broker, error) {
	return caas.New(ctx, args)
}

// NewBackend returns a k8s backed secrets backend.
func (p k8sProvider) NewBackend(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
	broker, _, err := p.getBroker(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "getting cluster client")
	}
	return &k8sBackend{broker: broker, pinger: func() error {
		_, err := broker.Version()
		if err == nil {
			return err
		}
		return errors.Annotate(err, "backend not reachable")
	}}, nil
}

func (p k8sProvider) configToCloudSpec(cfg *provider.BackendConfig) (cloudspec.CloudSpec, error) {
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
		return cloudspec.CloudSpec{}, errors.Trace(err)
	}
	cloudSpec.Credential = &cred
	return cloudSpec, nil
}
