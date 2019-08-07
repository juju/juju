// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"net/url"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/utils/exec"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	csiapi "k8s.io/csi-api/pkg/client/clientset/versioned"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
)

type kubernetesEnvironProvider struct {
	environProviderCredentials
	cmdRunner          CommandRunner
	builtinCloudGetter func(CommandRunner) (cloud.Cloud, jujucloud.Credential, string, error)
	brokerGetter       func(environs.OpenParams) (caas.ClusterMetadataChecker, error)
}

var _ environs.EnvironProvider = (*kubernetesEnvironProvider)(nil)
var providerInstance = kubernetesEnvironProvider{
	environProviderCredentials: environProviderCredentials{
		cmdRunner:          defaultRunner{},
		builtinCloudGetter: attemptMicroK8sCloud,
	},
	cmdRunner:          defaultRunner{},
	builtinCloudGetter: attemptMicroK8sCloud,
	brokerGetter: func(args environs.OpenParams) (caas.ClusterMetadataChecker, error) {
		return caas.New(args)
	},
}

// Version is part of the EnvironProvider interface.
func (kubernetesEnvironProvider) Version() int {
	return 0
}

// CommandRunner allows to run commands on the underlying system
type CommandRunner interface {
	RunCommands(run exec.RunParams) (*exec.ExecResponse, error)
}

type defaultRunner struct{}

func (defaultRunner) RunCommands(run exec.RunParams) (*exec.ExecResponse, error) {
	return exec.RunCommands(run)
}

func newK8sClient(c *rest.Config) (kubernetes.Interface, apiextensionsclientset.Interface, csiapi.Interface, error) {
	k8sClient, err := kubernetes.NewForConfig(c)
	if err != nil {
		return nil, nil, nil, err
	}
	var apiextensionsclient *apiextensionsclientset.Clientset
	apiextensionsclient, err = apiextensionsclientset.NewForConfig(c)
	if err != nil {
		return nil, nil, nil, err
	}
	csiClient, err := csiapi.NewForConfig(c)
	if err != nil {
		return nil, nil, nil, err
	}
	return k8sClient, apiextensionsclient, csiClient, nil
}

func cloudSpecToK8sRestConfig(cloudSpec environs.CloudSpec) (*rest.Config, error) {
	if cloudSpec.Credential == nil {
		return nil, errors.Errorf("cloud %v has no credential", cloudSpec.Name)
	}

	var CAData []byte
	for _, cacert := range cloudSpec.CACertificates {
		CAData = append(CAData, cacert...)
	}

	credentialAttrs := cloudSpec.Credential.Attributes()
	return &rest.Config{
		Host:        cloudSpec.Endpoint,
		Username:    credentialAttrs[CredAttrUsername],
		Password:    credentialAttrs[CredAttrPassword],
		BearerToken: credentialAttrs[CredAttrToken],
		TLSClientConfig: rest.TLSClientConfig{
			CertData: []byte(credentialAttrs[CredAttrClientCertificateData]),
			KeyData:  []byte(credentialAttrs[CredAttrClientKeyData]),
			CAData:   CAData,
		},
	}, nil
}

// Open is part of the ContainerEnvironProvider interface.
func (p kubernetesEnvironProvider) Open(args environs.OpenParams) (caas.Broker, error) {
	logger.Debugf("opening model %q.", args.Config.Name())
	if err := p.validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	k8sRestConfig, err := cloudSpecToK8sRestConfig(args.Cloud)
	if err != nil {
		return nil, errors.Trace(err)
	}
	broker, err := newK8sBroker(
		args.ControllerUUID, k8sRestConfig, args.Config, newK8sClient, newKubernetesWatcher, jujuclock.WallClock,
	)
	if err != nil {
		return nil, err
	}
	return controllerCorelation(broker)
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
	if err := p.validateCloudSpec(args.Cloud); err != nil {
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

func (p kubernetesEnvironProvider) validateCloudSpec(spec environs.CloudSpec) error {

	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if _, err := url.Parse(spec.Endpoint); err != nil {
		return errors.NotValidf("endpoint %q", spec.Endpoint)
	}
	if spec.Credential == nil {
		return errors.NotValidf("missing credential")
	}
	if authType := spec.Credential.AuthType(); !p.supportedAuthTypes().Contains(authType) {
		return errors.NotSupportedf("%q auth-type", authType)
	}
	return nil
}
