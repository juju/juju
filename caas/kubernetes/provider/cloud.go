// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/cloud"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

// ClientConfigFuncGetter returns a function that will provide the client reader
type ClientConfigFuncGetter func(string) (clientconfig.ClientConfigFunc, error)

// GetClusterMetadataFunc returns the ClusterMetadata using the provided ClusterMetadataChecker
type GetClusterMetadataFunc func(caas.ClusterMetadataChecker) (*caas.ClusterMetadata, error)

// KubeCloudParams provides the needed information to extract a Cloud from available cluster information.
type KubeCloudParams struct {
	ClusterName        string
	ContextName        string
	CaasName           string
	HostCloudRegion    string
	CaasType           string
	ClientConfigGetter ClientConfigFuncGetter
}

// KubeCloudStorageParams allows storage details to be determined for a K8s cloud.
type KubeCloudStorageParams struct {
	WorkloadStorage        string
	HostCloudRegion        string
	MetadataChecker        caas.ClusterMetadataChecker
	GetClusterMetadataFunc GetClusterMetadataFunc
}

// CloudFromKubeConfig attempts to extract a cloud and credential details from the provided Kubeconfig.
func CloudFromKubeConfig(reader io.Reader, cloudParams KubeCloudParams) (cloud.Cloud, jujucloud.Credential, string, error) {
	return newCloudCredentialFromKubeConfig(reader, cloudParams)
}

func newCloudCredentialFromKubeConfig(reader io.Reader, cloudParams KubeCloudParams) (jujucloud.Cloud, jujucloud.Credential, string, error) {
	// Get Cloud (incl. endpoint) and credential details from the kubeconfig details.
	var credential jujucloud.Credential
	var context clientconfig.Context
	fail := func(e error) (jujucloud.Cloud, jujucloud.Credential, string, error) {
		return jujucloud.Cloud{}, credential, "", e
	}
	newCloud := jujucloud.Cloud{
		Name:            cloudParams.CaasName,
		Type:            cloudParams.CaasType,
		HostCloudRegion: cloudParams.HostCloudRegion,
	}
	clientConfigFunc, err := cloudParams.ClientConfigGetter(cloudParams.CaasType)
	if err != nil {
		return fail(errors.Trace(err))
	}
	caasConfig, err := clientConfigFunc(reader, cloudParams.ContextName, cloudParams.ClusterName, clientconfig.EnsureK8sCredential)
	if err != nil {
		return fail(errors.Trace(err))
	}
	logger.Debugf("caasConfig: %+v", caasConfig)

	if len(caasConfig.Contexts) == 0 {
		return fail(errors.Errorf("No k8s cluster definitions found in config"))
	}

	context = caasConfig.Contexts[reflect.ValueOf(caasConfig.Contexts).MapKeys()[0].Interface().(string)]

	credential = caasConfig.Credentials[context.CredentialName]
	newCloud.AuthTypes = []jujucloud.AuthType{credential.AuthType()}
	currentCloud := caasConfig.Clouds[context.CloudName]
	newCloud.Endpoint = currentCloud.Endpoint

	cloudCAData, ok := currentCloud.Attributes["CAData"].(string)
	if !ok {
		return fail(errors.Errorf("CAData attribute should be a string"))
	}
	newCloud.CACertificates = []string{cloudCAData}
	return newCloud, credential, context.CredentialName, nil
}

// UpdateKubeCloudWithStorage updates the passed Cloud with storage details retrieved from the clouds' cluster.
func UpdateKubeCloudWithStorage(k8sCloud *cloud.Cloud, credential jujucloud.Credential, storageParams KubeCloudStorageParams) (string, error) {
	fail := func(e error) (string, error) {
		return "", e
	}

	// Get the cluster metadata so we can see if there's suitable storage available.
	clusterMetadata, err := storageParams.GetClusterMetadataFunc(storageParams.MetadataChecker)
	if err != nil || clusterMetadata == nil {
		return fail(errors.Wrap(err, ClusterQueryError{}))
	}

	if storageParams.HostCloudRegion == "" && clusterMetadata.Regions != nil && clusterMetadata.Regions.Size() > 0 {
		storageParams.HostCloudRegion = clusterMetadata.Cloud + "/" + clusterMetadata.Regions.SortedValues()[0]
	}
	if storageParams.HostCloudRegion == "" {
		return fail(ClusterQueryError{})
	}
	_, region, err := ParseCloudRegion(storageParams.HostCloudRegion)
	if err != nil {
		return fail(errors.Annotatef(err, "validating cloud region %q", storageParams.HostCloudRegion))
	}
	k8sCloud.HostCloudRegion = storageParams.HostCloudRegion
	k8sCloud.Regions = []jujucloud.Region{{
		Name: region,
	}}

	// If the user has not specified storage, check that the cluster has Juju's opinionated defaults.
	cloudType := strings.Split(storageParams.HostCloudRegion, "/")[0]
	err = storageParams.MetadataChecker.CheckDefaultWorkloadStorage(cloudType, clusterMetadata.NominatedStorageClass)
	if errors.IsNotFound(err) {
		return fail(UnknownClusterError{cloudType})
	}
	if storageParams.WorkloadStorage == "" && caas.IsNonPreferredStorageError(err) {
		npse := err.(*caas.NonPreferredStorageError)
		return fail(NoRecommendedStorageError{Message: err.Error(), ProviderName: npse.Name})
	}
	if err != nil && !caas.IsNonPreferredStorageError(err) {
		return fail(errors.Trace(err))
	}

	// If no storage class exists, we need to create one with the opinionated defaults.
	var storageMsg string
	if storageParams.WorkloadStorage != "" && caas.IsNonPreferredStorageError(err) {
		preferredStorage := errors.Cause(err).(*caas.NonPreferredStorageError).PreferredStorage
		sp, err := storageParams.MetadataChecker.EnsureStorageProvisioner(caas.StorageProvisioner{
			Name:        storageParams.WorkloadStorage,
			Provisioner: preferredStorage.Provisioner,
			Parameters:  preferredStorage.Parameters,
		})
		if err != nil {
			return fail(errors.Annotatef(err, "creating storage class %q", storageParams.WorkloadStorage))
		}
		if sp.Provisioner == preferredStorage.Provisioner {
			storageMsg = fmt.Sprintf(" with %s default storage", preferredStorage.Name)
			if storageParams.WorkloadStorage != "" {
				storageMsg = fmt.Sprintf("%s provisioned\nby the existing %q storage class", storageMsg, storageParams.WorkloadStorage)
			}
		} else {
			storageMsg = fmt.Sprintf(" with storage provisioned\nby the existing %q storage class", storageParams.WorkloadStorage)
		}
	}
	if storageParams.WorkloadStorage == "" && clusterMetadata.NominatedStorageClass != nil {
		storageParams.WorkloadStorage = clusterMetadata.NominatedStorageClass.Name
	}

	// Record the operator storage to use.
	var operatorStorageName string
	if clusterMetadata.OperatorStorageClass != nil {
		operatorStorageName = clusterMetadata.OperatorStorageClass.Name
	} else {
		operatorStorageName = storageParams.WorkloadStorage
		if storageMsg == "" {
			storageMsg += "\nwith "
		} else {
			storageMsg += "\n"
		}
		storageMsg += fmt.Sprintf("operator storage provisioned by the workload storage class")
	}

	if k8sCloud.Config == nil {
		k8sCloud.Config = make(map[string]interface{})
	}
	if _, ok := k8sCloud.Config[WorkloadStorageKey]; !ok {
		k8sCloud.Config[WorkloadStorageKey] = storageParams.WorkloadStorage
	}
	if _, ok := k8sCloud.Config[OperatorStorageKey]; !ok {
		k8sCloud.Config[OperatorStorageKey] = operatorStorageName
	}

	return storageMsg, nil
}

// ParseCloudRegion breaks apart a clusters cloud region.
func ParseCloudRegion(cloudRegion string) (string, string, error) {
	fields := strings.SplitN(cloudRegion, "/", 2)
	if len(fields) != 2 || fields[0] == "" || fields[1] == "" {
		return "", "", errors.NotValidf("cloud region %q", cloudRegion)
	}
	return fields[0], fields[1], nil
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

// FinalizeCloud is part of the environs.CloudFinalizer interface.
func (p kubernetesEnvironProvider) FinalizeCloud(ctx environs.FinalizeCloudContext, cld cloud.Cloud) (cloud.Cloud, error) {
	cloudName := cld.Name
	if cloudName != caas.Microk8s {
		return cld, nil
	}
	// Need the credentials, need to query for those details
	mk8sCloud, credential, _, err := p.builtinCloudGetter(p.cmdRunner)
	if err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}

	openParams, err := BaseKubeCloudOpenParams(mk8sCloud, credential)
	if err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}
	broker, err := p.brokerGetter(openParams)
	if err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}
	storageUpdateParams := KubeCloudStorageParams{
		MetadataChecker: broker,
		GetClusterMetadataFunc: func(broker caas.ClusterMetadataChecker) (*caas.ClusterMetadata, error) {
			clusterMetadata, err := broker.GetClusterMetadata("")
			if err != nil {
				return nil, errors.Trace(err)
			}
			return clusterMetadata, nil
		},
	}
	_, err = UpdateKubeCloudWithStorage(&mk8sCloud, credential, storageUpdateParams)
	for i := range mk8sCloud.Regions {
		if mk8sCloud.Regions[i].Endpoint == "" {
			mk8sCloud.Regions[i].Endpoint = mk8sCloud.Endpoint
		}
	}
	if err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}
	return mk8sCloud, nil
}
