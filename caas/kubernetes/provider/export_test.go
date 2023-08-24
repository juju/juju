// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"sync"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/caas"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/internal/storage"
)

var (
	FindControllerNamespace = findControllerNamespace
	GetLocalMicroK8sConfig  = getLocalMicroK8sConfig
	AttemptMicroK8sCloud    = attemptMicroK8sCloud
	EnsureMicroK8sSuitable  = ensureMicroK8sSuitable
	NewK8sBroker            = newK8sBroker
	ProcessSecretData       = processSecretData

	CompileK8sCloudCheckers               = compileK8sCloudCheckers
	CompileLifecycleModelTeardownSelector = compileLifecycleModelTeardownSelector

	UpdateStrategyForStatefulSet = updateStrategyForStatefulSet
	DecideKubeConfigDir          = decideKubeConfigDir
	RulesForSecretAccess         = rulesForSecretAccess
)

type (
	KubernetesClient      = kubernetesClient
	ControllerServiceSpec = controllerServiceSpec
)

type ControllerStackerForTest interface {
	controllerStacker
	GetControllerAgentConfigContent(*gc.C) string
	GetControllerUnitAgentConfigContent(*gc.C) string
	GetControllerUnitAgentPassword() string
	GetSharedSecretAndSSLKey(*gc.C) (string, string)
	GetStorageSize() resource.Quantity
	GetControllerSvcSpec(string, *podcfg.BootstrapConfig) (*controllerServiceSpec, error)
	SetContext(ctx environs.BootstrapContext)
}

func (cs *controllerStack) GetControllerAgentConfigContent(c *gc.C) string {
	agentCfg, err := cs.agentConfig.Render()
	c.Assert(err, jc.ErrorIsNil)
	return string(agentCfg)
}

func (cs *controllerStack) GetControllerUnitAgentConfigContent(c *gc.C) string {
	agentCfg, err := cs.unitAgentConfig.Render()
	c.Assert(err, jc.ErrorIsNil)
	return string(agentCfg)
}

func (cs *controllerStack) GetControllerUnitAgentPassword() string {
	return cs.unitAgentConfig.OldPassword()
}

func (cs *controllerStack) GetSharedSecretAndSSLKey(c *gc.C) (string, string) {
	si, ok := cs.agentConfig.StateServingInfo()
	c.Assert(ok, jc.IsTrue)
	return si.SharedSecret, mongo.GenerateSSLKey(si.Cert, si.PrivateKey)
}

func (cs *controllerStack) GetStorageSize() resource.Quantity {
	return cs.storageSize
}

func (cs *controllerStack) GetControllerSvcSpec(cloudType string, cfg *podcfg.BootstrapConfig) (*controllerServiceSpec, error) {
	return cs.getControllerSvcSpec(cloudType, cfg)
}

func (cs *controllerStack) SetContext(ctx environs.BootstrapContext) {
	cs.ctx = ctx
}

func NewcontrollerStackForTest(
	ctx environs.BootstrapContext,
	stackName,
	storageClass string,
	broker *kubernetesClient,
	pcfg *podcfg.ControllerPodConfig,
) (ControllerStackerForTest, error) {
	cs, err := newcontrollerStack(ctx, stackName, storageClass, broker, pcfg)
	return cs.(*controllerStack), err
}

func NewProvider() caas.ContainerEnvironProvider {
	return kubernetesEnvironProvider{}
}

func NewProviderWithFakes(
	runner CommandRunner,
	credentialGetter func(CommandRunner) (jujucloud.Credential, error),
	getter func(CommandRunner) (jujucloud.Cloud, error),
	brokerGetter func(environs.OpenParams) (ClusterMetadataStorageChecker, error)) caas.ContainerEnvironProvider {
	return kubernetesEnvironProvider{
		environProviderCredentials: environProviderCredentials{
			cmdRunner:               runner,
			builtinCredentialGetter: credentialGetter,
		},
		cmdRunner:          runner,
		builtinCloudGetter: getter,
		brokerGetter:       brokerGetter,
	}
}

func NewProviderCredentials(
	getter func(CommandRunner) (jujucloud.Credential, error),
) environProviderCredentials {
	return environProviderCredentials{
		builtinCredentialGetter: getter,
	}
}

func (k *kubernetesClient) DeleteClusterScopeResourcesModelTeardown(ctx context.Context, wg *sync.WaitGroup, errChan chan<- error) {
	k.deleteClusterScopeResourcesModelTeardown(ctx, wg, errChan)
}

func (k *kubernetesClient) DeleteNamespaceModelTeardown(ctx context.Context, wg *sync.WaitGroup, errChan chan<- error) {
	k.deleteNamespaceModelTeardown(ctx, wg, errChan)
}

func StorageProvider(k8sClient kubernetes.Interface, namespace string) storage.Provider {
	return &storageProvider{&kubernetesClient{clientUnlocked: k8sClient, namespace: namespace}}
}

func GetCloudProviderFromNodeMeta(node core.Node) (string, string) {
	return getCloudRegionFromNodeMeta(node)
}

func (k *kubernetesClient) GetPod(podName string) (*core.Pod, error) {
	return k.getPod(podName)
}

func (k *kubernetesClient) GetStatefulSet(name string) (*apps.StatefulSet, error) {
	return k.getStatefulSet(name)
}
