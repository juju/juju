// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/cloud"
)

// DetectClouds implements environs.CloudDetector.
func (p kubernetesEnvironProvider) DetectClouds() ([]cloud.Cloud, error) {
	clouds := []cloud.Cloud{}

	localKubeConfigClouds, err := localKubeConfigClouds()
	if err != nil {
		return clouds, errors.Annotate(err, "detecing local kube config clouds")
	}
	clouds = append(clouds, localKubeConfigClouds...)

	mk8sCloud, err := p.builtinCloudGetter(p.cmdRunner)
	if err == nil {
		clouds = append(clouds, mk8sCloud)
		return clouds, nil
	}
	if errors.IsNotFound(err) {
		err = errors.Annotatef(err, "microk8s is not installed")
	}
	logger.Debugf("failed to query local microk8s: %s", err.Error())
	return clouds, nil
}

func localKubeConfigClouds() ([]cloud.Cloud, error) {
	k8sConfig, err := clientconfig.GetLocalKubeConfig()
	if err != nil {
		return []cloud.Cloud{}, errors.Annotate(err, "reading local kubeconf")
	}

	return k8scloud.CloudsFromKubeConfigContextsWithParams(
		k8scloud.CloudParamaters{
			Description: "A local Kubernetes context",
		},
		k8sConfig,
	)
}

// DetectCloud implements environs.CloudDetector.
func (p kubernetesEnvironProvider) DetectCloud(name string) (cloud.Cloud, error) {
	mk8sCloud, err := p.builtinCloudGetter(p.cmdRunner)
	if err == nil && name == caas.K8sCloudMicrok8s {
		return mk8sCloud, nil
	}
	if !errors.IsNotFound(err) && err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}

	localKubeConfigClouds, err := localKubeConfigClouds()
	if err != nil {
		return cloud.Cloud{}, errors.Annotatef(err, "detecing local kube config clouds for %s", name)
	}

	for _, cloud := range localKubeConfigClouds {
		if cloud.Name == name {
			return cloud, nil
		}
	}

	return cloud.Cloud{}, errors.NotFoundf("cloud %s", name)
}
