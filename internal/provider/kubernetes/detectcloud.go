// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"

	"github.com/juju/errors"

	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/cloud"
)

// DetectClouds implements environs.CloudDetector.
func (p kubernetesEnvironProvider) DetectClouds() ([]cloud.Cloud, error) {
	clouds := []cloud.Cloud{}

	localKubeConfigClouds, err := localKubeConfigClouds()
	if err != nil {
		return clouds, errors.Annotate(err, "detecting local kube config clouds")
	}
	clouds = append(clouds, localKubeConfigClouds...)

	mk8sCloud, err := p.builtinCloudGetter(p.cmdRunner)
	if err == nil {
		clouds = append(clouds, mk8sCloud)
		return clouds, nil
	}
	if errors.Is(err, errors.NotFound) {
		err = errors.Annotatef(err, "microk8s is not installed")
	}
	logger.Debugf(context.TODO(), "failed to query local microk8s: %s", err.Error())
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
	if name == k8s.K8sCloudMicrok8s {
		// TODO: this whole thing is poorly written and we need to handle this better.
		// Also builtinCloudGetter should really be called, microk8sCloudGetter...
		microk8sCloud, err := p.builtinCloudGetter(p.cmdRunner)
		if err != nil {
			return cloud.Cloud{}, errors.Trace(err)
		}
		return microk8sCloud, nil
	}

	localKubeConfigClouds, err := localKubeConfigClouds()
	if err != nil {
		return cloud.Cloud{}, errors.Annotatef(err, "detecting local kube config clouds for %s", name)
	}

	for _, cloud := range localKubeConfigClouds {
		if cloud.Name == name {
			return cloud, nil
		}
	}

	return cloud.Cloud{}, errors.NotFoundf("cloud %s", name)
}
