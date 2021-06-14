// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"sync"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/caas"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/cloud"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/storage"
)

var (
	PrepareWorkloadSpec       = prepareWorkloadSpec
	OperatorPod               = operatorPod
	ExtractRegistryURL        = extractRegistryURL
	CreateDockerConfigJSON    = createDockerConfigJSON
	FindControllerNamespace   = findControllerNamespace
	GetLocalMicroK8sConfig    = getLocalMicroK8sConfig
	AttemptMicroK8sCloud      = attemptMicroK8sCloud
	AttemptMicroK8sCredential = attemptMicroK8sCredential
	EnsureMicroK8sSuitable    = ensureMicroK8sSuitable
	NewK8sBroker              = newK8sBroker
	ToYaml                    = toYaml
	Indent                    = indent
	ProcessSecretData         = processSecretData

	CompileK8sCloudCheckers                    = compileK8sCloudCheckers
	CompileLifecycleApplicationRemovalSelector = compileLifecycleApplicationRemovalSelector
	CompileLifecycleModelTeardownSelector      = compileLifecycleModelTeardownSelector

	LabelSetToRequirements = labelSetToRequirements
	MergeSelectors         = mergeSelectors

	UpdateStrategyForDeployment  = updateStrategyForDeployment
	UpdateStrategyForStatefulSet = updateStrategyForStatefulSet
	UpdateStrategyForDaemonSet   = updateStrategyForDaemonSet
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
	SetContext(ctx environs.BootstrapContext)
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

func Pod(u *workloadSpec) k8sspecs.PodSpecWithAnnotations {
	return u.Pod
}

func NewProvider() caas.ContainerEnvironProvider {
	return kubernetesEnvironProvider{}
}

func NewProviderWithFakes(
	runner CommandRunner,
	credentialGetter func(CommandRunner) (jujucloud.Credential, error),
	getter func(CommandRunner) (cloud.Cloud, error),
	brokerGetter func(environs.OpenParams) (caas.ClusterMetadataChecker, error)) caas.ContainerEnvironProvider {
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

func (k *kubernetesClient) GetCRDsForCRs(crs map[string][]unstructured.Unstructured, getter CRDGetterInterface) (out map[string]*apiextensionsv1.CustomResourceDefinition, err error) {
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

func GetCloudProviderFromNodeMeta(node core.Node) (string, string) {
	return getCloudRegionFromNodeMeta(node)
}
