// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	stdcontext "context"
	"net/url"
	osexec "os/exec"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/utils/v3/exec"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/caas"
	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	k8swatcher "github.com/juju/juju/caas/kubernetes/provider/watcher"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environsbootstrap "github.com/juju/juju/environs/bootstrap"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
)

type kubernetesEnvironProvider struct {
	environProviderCredentials
	cmdRunner          CommandRunner
	builtinCloudGetter func(CommandRunner) (cloud.Cloud, error)
	brokerGetter       func(environs.OpenParams) (caas.ClusterMetadataChecker, error)
}

var _ environs.EnvironProvider = (*kubernetesEnvironProvider)(nil)
var providerInstance = kubernetesEnvironProvider{
	environProviderCredentials: environProviderCredentials{
		cmdRunner:               defaultRunner{},
		builtinCredentialGetter: attemptMicroK8sCredential,
	},
	cmdRunner:          defaultRunner{},
	builtinCloudGetter: attemptMicroK8sCloud,
	brokerGetter: func(args environs.OpenParams) (caas.ClusterMetadataChecker, error) {
		return caas.New(stdcontext.TODO(), args)
	},
}

// Version is part of the EnvironProvider interface.
func (kubernetesEnvironProvider) Version() int {
	return 0
}

// CommandRunner allows to run commands on the underlying system
type CommandRunner interface {
	RunCommands(run exec.RunParams) (*exec.ExecResponse, error)
	LookPath(string) (string, error)
}

type defaultRunner struct{}

func (defaultRunner) RunCommands(run exec.RunParams) (*exec.ExecResponse, error) {
	return exec.RunCommands(run)
}

func (defaultRunner) LookPath(file string) (string, error) {
	return osexec.LookPath(file)
}

// NewK8sClients returns the k8s clients to access a cluster.
// Override for testing.
var NewK8sClients = func(c *rest.Config) (
	k8sClient kubernetes.Interface,
	apiextensionsclient apiextensionsclientset.Interface,
	dynamicClient dynamic.Interface,
	err error,
) {
	k8sClient, err = kubernetes.NewForConfig(c)
	if err != nil {
		return nil, nil, nil, err
	}
	apiextensionsclient, err = apiextensionsclientset.NewForConfig(c)
	if err != nil {
		return nil, nil, nil, err
	}
	dynamicClient, err = dynamic.NewForConfig(c)
	if err != nil {
		return nil, nil, nil, err
	}
	return k8sClient, apiextensionsclient, dynamicClient, nil
}

// CloudSpecToK8sRestConfig translates cloudspec to k8s rest config.
func CloudSpecToK8sRestConfig(cloudSpec environscloudspec.CloudSpec) (*rest.Config, error) {
	if cloudSpec.IsControllerCloud {
		rc, err := rest.InClusterConfig()
		if err != nil && err != rest.ErrNotInCluster {
			return nil, errors.Trace(err)
		}
		if rc != nil {
			logger.Tracef("using in-cluster config")
			return rc, nil
		}
	}

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
		Username:    credentialAttrs[k8scloud.CredAttrUsername],
		Password:    credentialAttrs[k8scloud.CredAttrPassword],
		BearerToken: credentialAttrs[k8scloud.CredAttrToken],
		TLSClientConfig: rest.TLSClientConfig{
			CertData: []byte(credentialAttrs[k8scloud.CredAttrClientCertificateData]),
			KeyData:  []byte(credentialAttrs[k8scloud.CredAttrClientKeyData]),
			CAData:   CAData,
			Insecure: cloudSpec.SkipTLSVerify,
		},
	}, nil
}

func newRestClient(cfg *rest.Config) (rest.Interface, error) {
	return rest.RESTClientFor(cfg)
}

// Open is part of the ContainerEnvironProvider interface.
func (p kubernetesEnvironProvider) Open(args environs.OpenParams) (caas.Broker, error) {
	logger.Debugf("opening model %q.", args.Config.Name())
	if err := p.validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	k8sRestConfig, err := CloudSpecToK8sRestConfig(args.Cloud)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if args.Config.Name() != environsbootstrap.ControllerModelName {
		broker, err := newK8sBroker(
			args.ControllerUUID, k8sRestConfig, args.Config, args.Config.Name(), NewK8sClients, newRestClient,
			k8swatcher.NewKubernetesNotifyWatcher, k8swatcher.NewKubernetesStringsWatcher, utils.RandomPrefix,
			jujuclock.WallClock)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return broker, nil
	}

	k8sClient, _, _, err := NewK8sClients(k8sRestConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ns, err := findControllerNamespace(
		k8sClient, args.ControllerUUID)
	if errors.IsNotFound(err) {
		// The controller is currently bootstrapping.
		return newK8sBroker(
			args.ControllerUUID, k8sRestConfig, args.Config, "",
			NewK8sClients, newRestClient, k8swatcher.NewKubernetesNotifyWatcher, k8swatcher.NewKubernetesStringsWatcher,
			utils.RandomPrefix, jujuclock.WallClock)
	} else if err != nil {
		return nil, err
	}

	return newK8sBroker(
		args.ControllerUUID, k8sRestConfig, args.Config, ns.Name,
		NewK8sClients, newRestClient, k8swatcher.NewKubernetesNotifyWatcher, k8swatcher.NewKubernetesStringsWatcher,
		utils.RandomPrefix, jujuclock.WallClock)
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
		attrs[config.StorageDefaultBlockSourceKey] = constants.StorageProviderType
	}
	if _, ok := args.Config.StorageDefaultFilesystemSource(); !ok {
		attrs[config.StorageDefaultFilesystemSourceKey] = constants.StorageProviderType
	}
	return args.Config.Apply(attrs)
}

// DetectRegions is specified in the environs.CloudRegionDetector interface.
func (p kubernetesEnvironProvider) DetectRegions() ([]cloud.Region, error) {
	return nil, errors.NotFoundf("regions")
}

func (p kubernetesEnvironProvider) validateCloudSpec(spec environscloudspec.CloudSpec) error {

	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if _, err := url.Parse(spec.Endpoint); err != nil {
		return errors.NotValidf("endpoint %q", spec.Endpoint)
	}
	if spec.Credential == nil {
		return errors.NotValidf("missing credential")
	}

	if authType := spec.Credential.AuthType(); !k8scloud.SupportedAuthTypes().Contains(authType) {
		return errors.NotSupportedf("%q auth-type", authType)
	}
	return nil
}
