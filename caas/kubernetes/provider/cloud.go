// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"io"
	"reflect"
	"strings"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/utils/v2"
	"github.com/juju/utils/v2/exec"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
)

// ClientConfigFuncGetter returns a function returning az reader that will read a k8s cluster config for a given cluster type
type ClientConfigFuncGetter func(string) (clientconfig.ClientConfigFunc, error)

// GetClusterMetadataFunc returns the ClusterMetadata using the provided ClusterMetadataChecker
type GetClusterMetadataFunc func(KubeCloudStorageParams) (*caas.ClusterMetadata, error)

// KubeCloudParams defines the parameters used to extract a k8s cluster definition from kubeconfig data.
type KubeCloudParams struct {
	ClusterName string
	ContextName string
	CloudName   string
	// CredentialUID ensures RBAC resources are unique.
	CredentialUID      string
	HostCloudRegion    string
	CaasType           string
	ClientConfigGetter ClientConfigFuncGetter
	Clock              jujuclock.Clock
}

// KubeCloudStorageParams defines the parameters used to determine storage details for a k8s cluster.
type KubeCloudStorageParams struct {
	WorkloadStorage        string
	HostCloudRegion        string
	MetadataChecker        caas.ClusterMetadataChecker
	GetClusterMetadataFunc GetClusterMetadataFunc
}

func newCloudCredentialFromKubeConfig(reader io.Reader, cloudParams KubeCloudParams) (cloud.Cloud, cloud.Credential, error) {
	// Get Cloud (incl. endpoint) and credential details from the kubeconfig details.
	var credential cloud.Credential
	var context clientconfig.Context
	fail := func(e error) (cloud.Cloud, cloud.Credential, error) {
		return cloud.Cloud{}, credential, e
	}
	newCloud := cloud.Cloud{
		Name:            cloudParams.CloudName,
		Type:            cloudParams.CaasType,
		HostCloudRegion: cloudParams.HostCloudRegion,
	}

	caasConfig, err := clientconfig.NewK8sClientConfigFromReader(
		cloudParams.CredentialUID, reader,
		cloudParams.ContextName, cloudParams.ClusterName,
		clientconfig.GetJujuAdminServiceAccountResolver(cloudParams.Clock),
	)
	if err != nil {
		return fail(errors.Trace(err))
	}
	logger.Debugf("caasConfig: %+v", caasConfig)

	if len(caasConfig.Contexts) == 0 {
		return fail(errors.Errorf("No k8s cluster definitions found in config"))
	}

	context = caasConfig.Contexts[reflect.ValueOf(caasConfig.Contexts).MapKeys()[0].Interface().(string)]

	credential = caasConfig.Credentials[context.CredentialName]
	newCloud.AuthTypes = []cloud.AuthType{credential.AuthType()}
	currentCloud := caasConfig.Clouds[context.CloudName]
	newCloud.Endpoint = currentCloud.Endpoint
	newCloud.SkipTLSVerify = currentCloud.SkipTLSVerify

	cloudCAData, ok := currentCloud.Attributes["CAData"].(string)
	if !ok {
		return fail(errors.Errorf("CAData attribute should be a string"))
	}
	newCloud.CACertificates = []string{cloudCAData}
	return newCloud, credential, nil
}

func updateK8sCloud(k8sCloud *cloud.Cloud, clusterMetadata *caas.ClusterMetadata, storageMsg string) string {
	var workloadSC, operatorSC string
	// Record the operator storage to use.
	if clusterMetadata.OperatorStorageClass != nil {
		operatorSC = clusterMetadata.OperatorStorageClass.Name
	} else {
		if storageMsg == "" {
			storageMsg += "\nwith "
		} else {
			storageMsg += "\nand "
		}
		storageMsg += fmt.Sprintf("operator storage provisioned by the workload storage class")
	}

	if clusterMetadata.NominatedStorageClass != nil {
		workloadSC = clusterMetadata.NominatedStorageClass.Name
	}

	if k8sCloud.Config == nil {
		k8sCloud.Config = make(map[string]interface{})
	}
	if _, ok := k8sCloud.Config[k8sconstants.WorkloadStorageKey]; !ok {
		k8sCloud.Config[k8sconstants.WorkloadStorageKey] = workloadSC
	}
	if _, ok := k8sCloud.Config[k8sconstants.OperatorStorageKey]; !ok {
		k8sCloud.Config[k8sconstants.OperatorStorageKey] = operatorSC
	}
	return storageMsg
}

// UpdateKubeCloudWithStorage updates the passed Cloud with storage details retrieved from the clouds' cluster.
func UpdateKubeCloudWithStorage(k8sCloud *cloud.Cloud, storageParams KubeCloudStorageParams) (storageMsg string, err error) {
	// Get the cluster metadata so we can see if there's suitable storage available.
	clusterMetadata, err := storageParams.GetClusterMetadataFunc(storageParams)
	defer func() {
		if err == nil {
			storageMsg = updateK8sCloud(k8sCloud, clusterMetadata, storageMsg)
		}
	}()

	if err != nil || clusterMetadata == nil {
		// err will be nil if user hit Ctrl+C.
		msg := "cannot get cluster metadata"
		if err != nil {
			msg = err.Error()
		}
		return "", ClusterQueryError{Message: msg}
	}

	if storageParams.HostCloudRegion == "" && clusterMetadata.Cloud != "" {
		var region string
		if clusterMetadata.Regions != nil && clusterMetadata.Regions.Size() > 0 {
			region = clusterMetadata.Regions.SortedValues()[0]
		}
		storageParams.HostCloudRegion = cloud.BuildHostCloudRegion(clusterMetadata.Cloud, region)
	}
	k8sCloud.HostCloudRegion = storageParams.HostCloudRegion

	var cloudType, region string
	if k8sCloud.HostCloudRegion != "" {
		cloudType, region, err = cloud.SplitHostCloudRegion(k8sCloud.HostCloudRegion)
		if err != nil {
			// Shouldn't happen as HostCloudRegion is validated earlier.
			return "", errors.Trace(err)
		}
		if region != "" {
			k8sCloud.Regions = []cloud.Region{{
				Name:     region,
				Endpoint: k8sCloud.Endpoint,
			}}
		}
	}

	// If the user has not specified storage and cloudType is usable, check Juju's opinionated defaults.
	err = storageParams.MetadataChecker.CheckDefaultWorkloadStorage(
		cloudType, clusterMetadata.NominatedStorageClass,
	)
	if storageParams.WorkloadStorage == "" {
		if err == nil {
			return
		}
		if caas.IsNonPreferredStorageError(err) {
			npse := err.(*caas.NonPreferredStorageError)
			return "", NoRecommendedStorageError{Message: err.Error(), ProviderName: npse.Name}
		}
		if errors.IsNotFound(err) {
			// No juju preferred storage config in jujuPreferredWorkloadStorage, for example, maas.
			if clusterMetadata.NominatedStorageClass == nil {
				// And no preferred storage classes with expected annotations found.
				//  - workloadStorageClassAnnotationKey
				//  - operatorStorageClassAnnotationKey
				return "", UnknownClusterError{CloudName: cloudType}
			}
			// Do further EnsureStorageProvisioner if preferred storage found via juju preferred/default annotations.
		} else if err != nil {
			return "", errors.Trace(err)
		}
	}

	// we need to create storage class with the opinionated defaults, or use an existing one if
	// --storage provided or nominated storage found but Juju does not have preferred storage config to
	// compare with for the cloudType(like maas for example);
	var (
		provisioner       string
		volumeBindingMode string
		params            map[string]string
	)
	scName := storageParams.WorkloadStorage
	nonPreferredStorageErr, ok := errors.Cause(err).(*caas.NonPreferredStorageError)
	if ok {
		provisioner = nonPreferredStorageErr.Provisioner
		volumeBindingMode = nonPreferredStorageErr.VolumeBindingMode
		params = nonPreferredStorageErr.Parameters
	} else if clusterMetadata.NominatedStorageClass != nil {
		if scName == "" {
			// no preferred storage class config but nominated storage found.
			scName = clusterMetadata.NominatedStorageClass.Name
		}
	}
	sp, existing, err := storageParams.MetadataChecker.EnsureStorageProvisioner(caas.StorageProvisioner{
		Name:              scName,
		Provisioner:       provisioner,
		Parameters:        params,
		VolumeBindingMode: volumeBindingMode,
	})
	if errors.IsNotFound(err) {
		return "", errors.Wrap(err, errors.NotFoundf("storage class %q", scName))
	}
	if err != nil {
		return "", errors.Annotatef(err, "creating storage class %q", scName)
	}
	if nonPreferredStorageErr != nil && sp.Provisioner == provisioner {
		storageMsg = fmt.Sprintf(" with %s default storage provisioned", nonPreferredStorageErr.Name)
	} else {
		storageMsg = " with storage provisioned"
	}
	scExisting := "existing"
	if !existing {
		scExisting = "new"
	}
	storageMsg += fmt.Sprintf("\nby the %s %q storage class", scExisting, scName)
	clusterMetadata.NominatedStorageClass = sp
	clusterMetadata.OperatorStorageClass = sp
	return storageMsg, nil
}

// BaseKubeCloudOpenParams provides a basic OpenParams for a cluster
func BaseKubeCloudOpenParams(cloud cloud.Cloud, credential cloud.Credential) (environs.OpenParams, error) {
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

	cloudSpec, err := environscloudspec.MakeCloudSpec(cloud, "", &credential)
	if err != nil {
		return environs.OpenParams{}, errors.Trace(err)
	}
	openParams := environs.OpenParams{
		Cloud: cloudSpec, Config: cfg,
	}
	return openParams, nil
}

// FinalizeCloud is part of the environs.CloudFinalizer interface.
func (p kubernetesEnvironProvider) FinalizeCloud(ctx environs.FinalizeCloudContext, cld cloud.Cloud) (cloud.Cloud, error) {
	// We set the clouds auth types to all kubernetes supported auth types here
	// so that finalize credentials is free to change the credentials of the
	// bootstrap. See lp-1918486
	cld.AuthTypes = k8scloud.SupportedAuthTypes()

	// if storage is already defined there is no need to query the cluster
	if opStorage, ok := cld.Config[k8sconstants.OperatorStorageKey]; ok && opStorage != "" {
		return cld, nil
	}

	var credentials cloud.Credential
	if cld.Name != caas.K8sCloudMicrok8s {
		creds, err := p.RegisterCredentials(cld)
		if err != nil {
			return cld, err
		}

		credentials = creds[cld.Name].AuthCredentials[creds[cld.Name].DefaultCredential]
	} else {
		if err := ensureMicroK8sSuitable(p.cmdRunner); err != nil {
			return cld, errors.Trace(err)
		}
		// Need the credentials, need to query for those details
		mk8sCloud, err := p.builtinCloudGetter(p.cmdRunner)
		if err != nil {
			return cloud.Cloud{}, errors.Trace(err)
		}
		cld = mk8sCloud

		creds, err := p.RegisterCredentials(cld)
		if err != nil {
			return cld, err
		}

		credentials = creds[cld.Name].AuthCredentials[creds[cld.Name].DefaultCredential]
	}

	if cld.SkipTLSVerify {
		logger.Warningf("k8s cloud %v is configured to skip server certificate validity checks", cld.Name)
	}

	openParams, err := BaseKubeCloudOpenParams(cld, credentials)
	if err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}
	broker, err := p.brokerGetter(openParams)
	if err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}
	storageUpdateParams := KubeCloudStorageParams{
		MetadataChecker: broker,
		GetClusterMetadataFunc: func(storageParams KubeCloudStorageParams) (*caas.ClusterMetadata, error) {
			clusterMetadata, err := storageParams.MetadataChecker.GetClusterMetadata("")
			if err != nil {
				return nil, errors.Trace(err)
			}
			return clusterMetadata, nil
		},
	}

	_, err = UpdateKubeCloudWithStorage(&cld, storageUpdateParams)
	if err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}

	if cld.HostCloudRegion == "" {
		cld.HostCloudRegion = caas.K8sCloudOther
	}

	return cld, nil
}

func ensureMicroK8sSuitable(cmdRunner CommandRunner) error {
	resp, err := cmdRunner.RunCommands(exec.RunParams{
		Commands: `id -nG "$(whoami)" | grep -qw "root\|microk8s"`,
	})
	if err != nil {
		return errors.Annotate(err, "checking microk8s setup")
	}
	if resp.Code != 0 {
		user, _ := utils.OSUsername()
		if user == "" {
			user = "<username>"
		}
		return errors.Errorf(`
The microk8s user group is created during the microk8s snap installation.
Users in that group are granted access to microk8s commands and this
is needed for Juju to be able to interact with microk8s.

Add yourself to that group before trying again:
  sudo usermod -a -G microk8s %s
`[1:], user)
	}

	status, err := microK8sStatus(cmdRunner)
	if err != nil {
		return errors.Trace(err)
	}
	var requiredAddons []string
	if storageStatus, ok := status.Addons["storage"]; ok {
		if storageStatus != "enabled" {
			requiredAddons = append(requiredAddons, "storage")
		}
	}
	if dns, ok := status.Addons["dns"]; ok {
		if dns != "enabled" {
			requiredAddons = append(requiredAddons, "dns")
		}
	}
	if len(requiredAddons) > 0 {
		return errors.Errorf("required addons not enabled for microk8s, run 'microk8s enable %s'", strings.Join(requiredAddons, " "))
	}
	return nil
}

func microK8sStatus(cmdRunner CommandRunner) (microk8sStatus, error) {
	var status microk8sStatus
	result, err := cmdRunner.RunCommands(exec.RunParams{
		Commands: "microk8s.status --wait-ready --timeout 15 --yaml",
	})
	if err != nil {
		return status, errors.Trace(err)
	}
	if result.Code != 0 {
		msg := string(result.Stderr)
		if msg == "" {
			msg = string(result.Stdout)
		}
		if msg == "" {
			msg = "unknown error running microk8s.status"
		}
		return status, errors.New(msg)
	}

	err = yaml.Unmarshal(result.Stdout, &status)
	if err != nil {
		return status, errors.Trace(err)
	}
	return status, nil
}

type microk8sStatus struct {
	Addons map[string]string `yaml:"addons"`
}
