// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"strings"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/utils"
	"github.com/juju/utils/exec"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/cloud"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
)

var builtinMicroK8sClusterName = "microk8s-cluster"
var builtinMicroK8sName = "microk8s"

type kubernetesEnvironProvider struct {
	environProviderCredentials
	cmdRunner CommandRunner
}

var _ environs.EnvironProvider = (*kubernetesEnvironProvider)(nil)
var providerInstance = kubernetesEnvironProvider{
	cmdRunner: defaultRunner{},
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
	broker, err := NewK8sBroker(
		args.ControllerUUID, k8sRestConfig, args.Config, newK8sClient, newKubernetesWatcher, jujuclock.WallClock,
	)
	if err != nil {
		return nil, err
	}
	return controllerCorelation(broker)
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

// DetectClouds implements environs.CloudDetector.
func (p kubernetesEnvironProvider) DetectClouds() ([]cloud.Cloud, error) {
	clouds := []cloud.Cloud{}
	mk8sCloud, _, _, _, err := attemptMicroK8sCloud(p.cmdRunner)
	if err != nil {
		logger.Debugf("failed to query local microk8s: %s", err)
	} else {
		clouds = append(clouds, mk8sCloud)
	}
	return clouds, nil
}

// DetectCloud implements environs.CloudDetector.
func (p kubernetesEnvironProvider) DetectCloud(name string) (cloud.Cloud, error) {
	if name == builtinMicroK8sName {
		mk8sCloud, _, _, _, err := attemptMicroK8sCloud(p.cmdRunner)
		return mk8sCloud, err
	}
	return cloud.Cloud{}, nil
}

func attemptMicroK8sCloud(cmdRunner CommandRunner) (cloud.Cloud, jujucloud.Credential, string, string, error) {
	var newCloud cloud.Cloud
	fail := func(err error) (cloud.Cloud, jujucloud.Credential, string, string, error) {
		return newCloud, jujucloud.Credential{}, "", "", err
	}
	execParams := exec.RunParams{
		Commands: "microk8s.config",
	}
	result, err := cmdRunner.RunCommands(execParams)
	if err != nil {
		return fail(err)
	}
	if result.Code != 0 {
		return fail(errors.New(string(result.Stderr)))
	}

	rdr := bytes.NewReader(result.Stdout)

	cloudParams := KubeCloudParams{
		ClusterName: builtinMicroK8sClusterName,
		CaasName:    builtinMicroK8sName,
		CaasType:    CAASProviderType,
		Errors: KubeCloudParamErrors{
			ClusterQuery:         "Unable to query cluster. Ensure storage has been enabled with 'microk8s.enable storage'.",
			UnknownCluster:       "Unable to determine cluster details from microk8s.config",
			NoRecommendedStorage: "No recommended storage configuration is defined for microk8s.",
		},
		ClusterMetadataCheckerGetter: func(cloud jujucloud.Cloud, credential jujucloud.Credential) (caas.ClusterMetadataChecker, error) {
			openParams, err := BaseKubeCloudOpenParams(cloud, credential)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return caas.New(openParams)
		},
		ClientConfigGetter: func(caasType string) (clientconfig.ClientConfigFunc, error) {
			return clientconfig.NewClientConfigReader(caasType)
		},
		ClusterMetadataCheckerFunc: func(broker caas.ClusterMetadataChecker) (*caas.ClusterMetadata, error) {
			clusterMetadata, err := broker.GetClusterMetadata("")
			if err != nil {
				return nil, errors.Trace(err)
			}
			return clusterMetadata, nil
		},
	}
	return CloudFromKubeConfig(rdr, cloudParams)
}

type ClusterMetadataCheckerGetter func(cloud jujucloud.Cloud, credential jujucloud.Credential) (caas.ClusterMetadataChecker, error)
type ClientConfigFuncGetter func(string) (clientconfig.ClientConfigFunc, error)
type ClusterMetadataCheckerFunc func(caas.ClusterMetadataChecker) (*caas.ClusterMetadata, error)

// KubeCloudParams provides the needed information to extract a Cloud from available cluster information.
type KubeCloudParams struct {
	ClusterName                  string
	ContextName                  string
	CaasName                     string
	HostCloudRegion              string
	WorkloadStorage              string
	CaasType                     string
	Errors                       KubeCloudParamErrors
	ClusterMetadataCheckerGetter ClusterMetadataCheckerGetter
	ClientConfigGetter           ClientConfigFuncGetter
	ClusterMetadataCheckerFunc   ClusterMetadataCheckerFunc
}

//KubeCloudParamErrors allows errors to be customised based on need (e.g. interactive CLI command or behind the scenes query).
type KubeCloudParamErrors struct {
	ClusterQuery         string
	UnknownCluster       string
	NoRecommendedStorage string
}

// BaseKubeCloudOpenParams provides a basic OpenParams for a cluster
func BaseKubeCloudOpenParams(cloud jujucloud.Cloud, credential jujucloud.Credential) (environs.OpenParams, error) {
	// To get a k8s client, we need a config with minimal information.
	// It's not used unless operating on a real model but we need to supply it.
	uuid, err := utils.NewUUID()
	if err != nil {
		return environs.OpenParams{}, errors.Trace(err)
	}
	attrs := map[string]interface{}{
		config.NameKey: "add-cloud",
		config.TypeKey: "kubernetes",
		config.UUIDKey: uuid.String(),
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return environs.OpenParams{}, errors.Trace(err)
	}

	cloudSpec, err := environs.MakeCloudSpec(cloud, "", &credential)
	if err != nil {
		return environs.OpenParams{}, errors.Trace(err)
	}
	openParams := environs.OpenParams{
		Cloud: cloudSpec, Config: cfg,
	}
	return openParams, nil
}

// CloudFromKubeConfig attempts to extract a cloud and credential details from the provided Kubeconfig.
func CloudFromKubeConfig(reader io.Reader, betterName KubeCloudParams) (cloud.Cloud, jujucloud.Credential, string, string, error) {
	fail := func(e error) (cloud.Cloud, jujucloud.Credential, string, string, error) {
		return cloud.Cloud{}, jujucloud.Credential{}, "", "", e
	}
	newCloud, credential, context, err := newCloudCredentialFromKubeConfig(reader, betterName)
	if err != nil {
		return fail(errors.Trace(err))
	}

	broker, err := betterName.ClusterMetadataCheckerGetter(newCloud, credential)
	if err != nil {
		return fail(errors.Trace(err))
	}

	// Get the cluster metadata so we can see if there's suitable storage available.
	clusterMetadata, err := betterName.ClusterMetadataCheckerFunc(broker)
	if err != nil || clusterMetadata == nil {
		return fail(errors.Annotate(err, betterName.Errors.ClusterQuery))
	}

	if betterName.HostCloudRegion == "" && clusterMetadata.Regions != nil && clusterMetadata.Regions.Size() > 0 {
		betterName.HostCloudRegion = clusterMetadata.Cloud + "/" + clusterMetadata.Regions.SortedValues()[0]
	}
	if betterName.HostCloudRegion == "" {
		return fail(errors.New(betterName.Errors.ClusterQuery))
	}
	_, region, err := ParseCloudRegion(betterName.HostCloudRegion)
	if err != nil {
		return fail(errors.Annotatef(err, "validating cloud region %q", betterName.HostCloudRegion))
	}
	newCloud.HostCloudRegion = betterName.HostCloudRegion
	newCloud.Regions = []jujucloud.Region{{
		Name: region,
	}}

	// If the user has not specified storage, check that the cluster has Juju's opinionated defaults.
	cloudType := strings.Split(betterName.HostCloudRegion, "/")[0]
	err = broker.CheckDefaultWorkloadStorage(cloudType, clusterMetadata.NominatedStorageClass)
	if errors.IsNotFound(err) {
		return fail(errors.Errorf(betterName.Errors.UnknownCluster, cloudType))
	}
	if betterName.WorkloadStorage == "" && caas.IsNonPreferredStorageError(err) {
		npse := err.(*caas.NonPreferredStorageError)
		return fail(errors.Errorf(betterName.Errors.NoRecommendedStorage, npse.Name))
	}
	if err != nil && !caas.IsNonPreferredStorageError(err) {
		return fail(errors.Trace(err))
	}

	// If no storage class exists, we need to create one with the opinionated defaults.
	var storageMsg string
	if betterName.WorkloadStorage != "" && caas.IsNonPreferredStorageError(err) {
		preferredStorage := errors.Cause(err).(*caas.NonPreferredStorageError).PreferredStorage
		sp, err := broker.EnsureStorageProvisioner(caas.StorageProvisioner{
			Name:        betterName.WorkloadStorage,
			Provisioner: preferredStorage.Provisioner,
			Parameters:  preferredStorage.Parameters,
		})
		if err != nil {
			return fail(errors.Annotatef(err, "creating storage class %q", betterName.WorkloadStorage))
		}
		if sp.Provisioner == preferredStorage.Provisioner {
			storageMsg = fmt.Sprintf(" with %s default storage", preferredStorage.Name)
			if betterName.WorkloadStorage != "" {
				storageMsg = fmt.Sprintf("%s provisioned\nby the existing %q storage class", storageMsg, betterName.WorkloadStorage)
			}
		} else {
			storageMsg = fmt.Sprintf(" with storage provisioned\nby the existing %q storage class", betterName.WorkloadStorage)
		}
	}
	if betterName.WorkloadStorage == "" && clusterMetadata.NominatedStorageClass != nil {
		betterName.WorkloadStorage = clusterMetadata.NominatedStorageClass.Name
	}

	// Record the operator storage to use.
	var operatorStorageName string
	if clusterMetadata.OperatorStorageClass != nil {
		operatorStorageName = clusterMetadata.OperatorStorageClass.Name
	} else {
		operatorStorageName = betterName.WorkloadStorage
		if storageMsg == "" {
			storageMsg += "\nwith "
		} else {
			storageMsg += "\n"
		}
		storageMsg += fmt.Sprintf("operator storage provisioned by the workload storage class")
	}

	if newCloud.Config == nil {
		newCloud.Config = make(map[string]interface{})
	}
	if _, ok := newCloud.Config[WorkloadStorageKey]; !ok {
		newCloud.Config[WorkloadStorageKey] = betterName.WorkloadStorage
	}
	if _, ok := newCloud.Config[OperatorStorageKey]; !ok {
		newCloud.Config[OperatorStorageKey] = operatorStorageName
	}
	if _, ok := newCloud.Config[bootstrap.ControllerServiceTypeKey]; !ok {
		newCloud.Config[bootstrap.ControllerServiceTypeKey] = clusterMetadata.PreferredServiceType
	}

	return newCloud, credential, context.CredentialName, storageMsg, nil
}

func newCloudCredentialFromKubeConfig(reader io.Reader, betterName KubeCloudParams) (jujucloud.Cloud, jujucloud.Credential, clientconfig.Context, error) {
	var credential jujucloud.Credential
	var context clientconfig.Context
	newCloud := jujucloud.Cloud{
		Name:            betterName.CaasName,
		Type:            betterName.CaasType,
		HostCloudRegion: betterName.HostCloudRegion,
	}
	clientConfigFunc, err := betterName.ClientConfigGetter(betterName.CaasType)
	if err != nil {
		return newCloud, credential, context, errors.Trace(err)
	}
	caasConfig, err := clientConfigFunc(reader, betterName.ContextName, betterName.ClusterName, clientconfig.EnsureK8sCredential)
	if err != nil {
		return newCloud, credential, context, errors.Trace(err)
	}
	logger.Debugf("caasConfig: %+v", caasConfig)

	if len(caasConfig.Contexts) == 0 {
		return newCloud, credential, context, errors.Errorf("No k8s cluster definitions found in config")
	}

	context = caasConfig.Contexts[reflect.ValueOf(caasConfig.Contexts).MapKeys()[0].Interface().(string)]

	credential = caasConfig.Credentials[context.CredentialName]
	newCloud.AuthTypes = []jujucloud.AuthType{credential.AuthType()}
	currentCloud := caasConfig.Clouds[context.CloudName]
	newCloud.Endpoint = currentCloud.Endpoint

	cloudCAData, ok := currentCloud.Attributes["CAData"].(string)
	if !ok {
		return newCloud, credential, context, errors.Errorf("CAData attribute should be a string")
	}
	newCloud.CACertificates = []string{cloudCAData}
	return newCloud, credential, context, nil
}

// ParseCloudRegion breaks apart a clusters cloud region.
func ParseCloudRegion(cloudRegion string) (string, string, error) {
	fields := strings.SplitN(cloudRegion, "/", 2)
	if len(fields) != 2 || fields[0] == "" || fields[1] == "" {
		return "", "", errors.NotValidf("cloud region %q", cloudRegion)
	}
	return fields[0], fields[1], nil
}
