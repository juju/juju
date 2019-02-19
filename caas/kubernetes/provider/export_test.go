// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/storage"
)

var (
	MakeUnitSpec             = makeUnitSpec
	ParseK8sPodSpec          = parseK8sPodSpec
	OperatorPod              = operatorPod
	ExtractRegistryURL       = extractRegistryURL
	CreateDockerConfigJSON   = createDockerConfigJSON
	NewStorageConfig         = newStorageConfig
	NewKubernetesWatcher     = newKubernetesWatcher
	CompileK8sCloudCheckers  = compileK8sCloudCheckers
	CloudSpecToK8sRestConfig = cloudSpecToK8sRestConfig
)

type KubernetesWatcher = kubernetesWatcher

type ControllerStackerForTest interface {
	controllerStacker
	GetAgentConfigContent(c *gc.C) string
	GetStorageSize() resource.Quantity
}

func (cs controllerStack) GetAgentConfigContent(c *gc.C) string {
	agentCfg, err := cs.agentConfig.Render()
	c.Assert(err, jc.ErrorIsNil)
	return string(agentCfg)
}

func (cs controllerStack) GetStorageSize() resource.Quantity {
	return cs.storageSize
}

func NewcontrollerStackForTest(stackName string, broker caas.Broker, pcfg *podcfg.ControllerPodConfig) (ControllerStackerForTest, error) {
	cs, err := newcontrollerStack(stackName, broker.(*kubernetesClient), pcfg)
	return cs.(controllerStack), err
}

func PodSpec(u *unitSpec) core.PodSpec {
	return u.Pod
}

func NewProvider() caas.ContainerEnvironProvider {
	return kubernetesEnvironProvider{}
}

func StorageProvider(k8sClient kubernetes.Interface, namespace string) storage.Provider {
	return &storageProvider{&kubernetesClient{Interface: k8sClient, namespace: namespace}}
}

func StorageClass(cfg *storageConfig) string {
	return cfg.storageClass
}

func ExistingStorageClass(cfg *storageConfig) string {
	return cfg.existingStorageClass
}

func StorageProvisioner(cfg *storageConfig) string {
	return cfg.storageProvisioner
}

func StorageParameters(cfg *storageConfig) map[string]string {
	return cfg.parameters
}

func GetCloudProviderFromNodeMeta(node core.Node) string {
	return getCloudProviderFromNodeMeta(node)
}
