// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"sync"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/cloud"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/storage"
)

var (
	PrepareWorkloadSpec     = prepareWorkloadSpec
	OperatorPod             = operatorPod
	ExtractRegistryURL      = extractRegistryURL
	CreateDockerConfigJSON  = createDockerConfigJSON
	NewStorageConfig        = newStorageConfig
	CompileK8sCloudCheckers = compileK8sCloudCheckers
	ControllerCorelation    = controllerCorelation
	GetLocalMicroK8sConfig  = getLocalMicroK8sConfig
	AttemptMicroK8sCloud    = attemptMicroK8sCloudInternal
	EnsureMicroK8sSuitable  = ensureMicroK8sSuitable
	NewK8sBroker            = newK8sBroker
	ToYaml                  = toYaml
	Indent                  = indent
	ProcessSecretData       = processSecretData
	PushUniqVolume          = pushUniqVolume
)

type (
	KubernetesClient      = kubernetesClient
	ControllerServiceSpec = controllerServiceSpec
	CRDGetter             = crdGetter
)

type ControllerStackerForTest interface {
	controllerStacker
	GetAgentConfigContent(*gc.C) string
	GetSharedSecretAndSSLKey(*gc.C) (string, string)
	GetStorageSize() resource.Quantity
	GetControllerSvcSpec(string, *podcfg.BootstrapConfig) (*controllerServiceSpec, error)
}

func (cs *controllerStack) GetAgentConfigContent(c *gc.C) string {
	agentCfg, err := cs.agentConfig.Render()
	c.Assert(err, jc.ErrorIsNil)
	return string(agentCfg)
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

func PodSpec(u *workloadSpec) core.PodSpec {
	return u.Pod
}

func NewProvider() caas.ContainerEnvironProvider {
	return kubernetesEnvironProvider{}
}

func NewProviderWithFakes(
	runner CommandRunner,
	getter func(CommandRunner) (cloud.Cloud, jujucloud.Credential, string, error),
	brokerGetter func(environs.OpenParams) (caas.ClusterMetadataChecker, error)) caas.ContainerEnvironProvider {
	return kubernetesEnvironProvider{
		cmdRunner:          runner,
		builtinCloudGetter: getter,
		brokerGetter:       brokerGetter,
	}
}

func NewProviderCredentials(getter func(CommandRunner) (cloud.Cloud, jujucloud.Credential, string, error)) environProviderCredentials {
	return environProviderCredentials{
		builtinCloudGetter: getter,
	}
}

func (k *kubernetesClient) GetCRDsForCRs(crs map[string][]unstructured.Unstructured, getter CRDGetterInterface) (out map[string]*apiextensionsv1beta1.CustomResourceDefinition, err error) {
	return k.getCRDsForCRs(crs, getter)
}

func (k *kubernetesClient) FileSetToVolume(
	appName string,
	annotations map[string]string,
	workloadSpec *workloadSpec,
	fileSet specs.FileSet,
	cfgMapName configMapNameFunc,
) (core.Volume, error) {
	return k.fileSetToVolume(appName, annotations, workloadSpec, fileSet, cfgMapName)
}

func (k *kubernetesClient) ConfigurePodFiles(
	appName string,
	annotations map[string]string,
	workloadSpec *workloadSpec,
	containers []specs.ContainerSpec,
	cfgMapName configMapNameFunc,
) error {
	return k.configurePodFiles(appName, annotations, workloadSpec, containers, cfgMapName)
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

func GetStorageClass(cfg *storageConfig) string {
	return cfg.storageClass
}

func GetStorageProvisioner(cfg *storageConfig) string {
	return cfg.storageProvisioner
}

func GetStorageParameters(cfg *storageConfig) map[string]string {
	return cfg.parameters
}

func GetCloudProviderFromNodeMeta(node core.Node) (string, string) {
	return getCloudRegionFromNodeMeta(node)
}
