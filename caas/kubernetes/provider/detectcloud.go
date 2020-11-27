// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/errors"

	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/cloud"
)

// DetectClouds implements environs.CloudDetector.
func (p kubernetesEnvironProvider) DetectClouds() ([]cloud.Cloud, error) {
	clouds := []cloud.Cloud{}
	mk8sCloud, _, _, err := p.builtinCloudGetter(p.cmdRunner)
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

// DetectCloud implements environs.CloudDetector.
func (p kubernetesEnvironProvider) DetectCloud(name string) (cloud.Cloud, error) {
	if name != k8s.K8sCloudMicrok8s {
		return cloud.Cloud{}, errors.NotFoundf("cloud %s", name)
	}

	mk8sCloud, _, _, err := p.builtinCloudGetter(p.cmdRunner)
	if err == nil {
		return mk8sCloud, nil
	}
	if errors.IsNotFound(err) {
		err = errors.Annotatef(err, "microk8s is not installed")
	}
	logger.Debugf("failed to query local microk8s: %s", err.Error())
	return cloud.Cloud{}, err
}
