// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
)

type kubernetesEnvironProvider struct {
	environProviderCredentials
}

var _ environs.EnvironProvider = (*kubernetesEnvironProvider)(nil)
var providerInstance = kubernetesEnvironProvider{}

// Version is part of the EnvironProvider interface.
func (kubernetesEnvironProvider) Version() int {
	return 0
}

func newK8sClient(c *rest.Config) (kubernetes.Interface, apiextensionsclientset.Interface, error) {
	k8sClient, err := kubernetes.NewForConfig(c)
	if err != nil {
		return nil, nil, err
	}
	var apiextensionsclient *apiextensionsclientset.Clientset
	apiextensionsclient, err = apiextensionsclientset.NewForConfig(c)
	if err != nil {
		return nil, nil, err
	}
	return k8sClient, apiextensionsclient, nil
}

// Open is part of the ContainerEnvironProvider interface.
func (kubernetesEnvironProvider) Open(args environs.OpenParams) (caas.Broker, error) {
	logger.Debugf("opening model %q.", args.Config.Name())
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	broker, err := NewK8sBroker(args.Cloud, args.Config, newK8sClient)
	if err != nil {
		return nil, err
	}
	return broker, nil
}

// ParsePodSpec is part of the ContainerEnvironProvider interface.
func (kubernetesEnvironProvider) ParsePodSpec(in string) (*caas.PodSpec, error) {
	spec, err := parseK8sPodSpec(in)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return spec, spec.Validate()
}

// CloudSchema returns the schema for adding new clouds of this type.
func (p kubernetesEnvironProvider) CloudSchema() *jsonschema.Schema {
	return nil
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p kubernetesEnvironProvider) Ping(ctx context.ProviderCallContext, endpoint string) error {
	return errors.NotImplementedf("Ping")
}

// PrepareConfig is specified in the EnvironProvider interface.
func (p kubernetesEnvironProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	// Set the default storage sources.
	attrs := make(map[string]interface{})
	if _, ok := args.Config.StorageDefaultBlockSource(); !ok {
		attrs[config.StorageDefaultBlockSourceKey] = K8s_ProviderType
	}
	if _, ok := args.Config.StorageDefaultFilesystemSource(); !ok {
		attrs[config.StorageDefaultFilesystemSourceKey] = K8s_ProviderType
	}
	return args.Config.Apply(attrs)
}

// DetectRegions is specified in the environs.CloudRegionDetector interface.
func (p kubernetesEnvironProvider) DetectRegions() ([]cloud.Region, error) {
	return nil, errors.NotFoundf("regions")
}

func (p kubernetesEnvironProvider) Validate(cfg, old *config.Config) (*config.Config, error) {
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (p kubernetesEnvironProvider) newConfig(cfg *config.Config) (*config.Config, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return valid, nil
}

func validateCloudSpec(spec environs.CloudSpec) error {
	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if _, err := url.Parse(spec.Endpoint); err != nil {
		return errors.NotValidf("endpoint %q", spec.Endpoint)
	}
	if spec.Credential == nil {
		return errors.NotValidf("missing credential")
	}
	if authType := spec.Credential.AuthType(); authType != cloud.UserPassAuthType && authType != cloud.CertificateAuthType {
		return errors.NotSupportedf("%q auth-type", authType)
	}
	return nil
}
