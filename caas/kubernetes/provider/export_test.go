// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	core "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/storage"
)

var (
	MakeUnitSpec           = makeUnitSpec
	ParseK8sPodSpec        = parseK8sPodSpec
	OperatorPod            = operatorPod
	ExtractRegistryURL     = extractRegistryURL
	CreateDockerConfigJSON = createDockerConfigJSON
)

func PodSpec(u *unitSpec) core.PodSpec {
	return u.Pod
}

func NewProvider() caas.ContainerEnvironProvider {
	return kubernetesEnvironProvider{}
}

func StorageProvider(k8sClient kubernetes.Interface, namespace string) storage.Provider {
	return &storageProvider{&kubernetesClient{Interface: k8sClient, namespace: namespace}}
}
