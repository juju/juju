// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

// DetectClouds implements environs.CloudDetector.
func (p kubernetesEnvironProvider) DetectClouds() ([]cloud.Cloud, error) {
	clouds := []cloud.Cloud{}
	mk8sCloud, _, _, err := attemptMicroK8sCloud(p.cmdRunner)
	if err != nil {
		logger.Debugf("failed to query local microk8s: %s", err)
	} else {
		clouds = append(clouds, mk8sCloud)
	}
	return clouds, nil
}

// DetectCloud implements environs.CloudDetector.
func (p kubernetesEnvironProvider) DetectCloud(name string) (cloud.Cloud, error) {
	if name != builtinMicroK8sName {
		return cloud.Cloud{}, nil
	}

	mk8sCloud, _, _, err := attemptMicroK8sCloud(p.cmdRunner)
	if err == nil {
		return mk8sCloud, nil
	}
	logger.Debugf("failed to query local microk8s: %s", err)
	return cloud.Cloud{}, nil
}

// FinalizeCloud is part of the environs.CloudFinalizer interface.
func (p kubernetesEnvironProvider) FinalizeCloud(ctx environs.FinalizeCloudContext, cld cloud.Cloud) (cloud.Cloud, error) {
	cloudName := cld.Name
	if cloudName != builtinMicroK8sName {
		return cld, nil
	}
	// Need the credentials, need to query for those details
	mk8sCloud, credential, _, err := attemptMicroK8sCloud(p.cmdRunner)
	if err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}

	storageUpdateParams := KubeCloudStorageParams{
		ClusterMetadataCheckerGetter: func(cloud jujucloud.Cloud, credential jujucloud.Credential) (caas.ClusterMetadataChecker, error) {
			openParams, err := BaseKubeCloudOpenParams(cloud, credential)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return caas.New(openParams)
		},
		GetClusterMetadataFunc: func(broker caas.ClusterMetadataChecker) (*caas.ClusterMetadata, error) {
			clusterMetadata, err := broker.GetClusterMetadata("")
			if err != nil {
				return nil, errors.Trace(err)
			}
			return clusterMetadata, nil
		},
		Errors: KubeCloudParamErrors{
			ClusterQuery:         "Unable to query cluster. Ensure storage has been enabled with 'microk8s.enable storage'.",
			UnknownCluster:       "Unable to determine cluster details from microk8s.config",
			NoRecommendedStorage: "No recommended storage configuration is defined for microk8s.",
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
