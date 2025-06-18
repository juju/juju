// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"
	"github.com/kr/pretty"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"

	"github.com/juju/juju/caas"
	k8sapplication "github.com/juju/juju/caas/kubernetes/provider/application"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	k8sstorage "github.com/juju/juju/caas/kubernetes/provider/storage"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	k8swatcher "github.com/juju/juju/caas/kubernetes/provider/watcher"
	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/cloudconfig/podcfg"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/assumes"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/paths"
	coreresources "github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	envcontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/storage"
	jujuversion "github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.kubernetes.provider")

const (
	// labelResourceLifeCycleKey defines the label key for lifecycle of the global resources.
	labelResourceLifeCycleKey             = "juju-resource-lifecycle"
	labelResourceLifeCycleValueModel      = "model"
	labelResourceLifeCycleValuePersistent = "persistent"

	gpuAffinityNodeSelectorKey = "gpu"

	operatorInitContainerName = "juju-init"
	operatorContainerName     = "juju-operator"

	dataDirVolumeName = "juju-data-dir"

	// InformerResyncPeriod is the default resync period set on IndexInformers
	InformerResyncPeriod = time.Minute * 5

	// A set of constants defining history limits for certain k8s deployment
	// types.
	// TODO We may want to make these configurable in the future.

	// daemonsetRevisionHistoryLimit is the number of old history states to
	// retain to allow rollbacks
	daemonsetRevisionHistoryLimit int32 = 0
	// deploymentRevisionHistoryLimit is the number of old ReplicaSets to retain
	// to allow rollback
	deploymentRevisionHistoryLimit int32 = 0
	// statefulSetRevisionHistoryLimit is the maximum number of revisions that
	// will be maintained in the StatefulSet's revision history
	statefulSetRevisionHistoryLimit int32 = 0
)

type kubernetesClient struct {
	clock jujuclock.Clock

	// namespace is the k8s namespace to use when
	// creating k8s resources.
	namespace string

	annotations k8sannotations.Annotation

	lock                        sync.Mutex
	envCfgUnlocked              *config.Config
	k8sCfgUnlocked              *rest.Config
	clientUnlocked              kubernetes.Interface
	apiextensionsClientUnlocked apiextensionsclientset.Interface
	dynamicClientUnlocked       dynamic.Interface

	newClient     NewK8sClientFunc
	newRestClient k8sspecs.NewK8sRestClientFunc

	// modelUUID is the UUID of the model this client acts on.
	modelUUID string
	// modelName is the name of the model.
	modelName string
	// controllerUUID is the UUID of the controller.
	controllerUUID string

	// newWatcher is the k8s watcher generator.
	newWatcher        k8swatcher.NewK8sWatcherFunc
	newStringsWatcher k8swatcher.NewK8sStringsWatcherFunc

	// informerFactoryUnlocked informer factory setup for tracking this model
	informerFactoryUnlocked informers.SharedInformerFactory

	// labelVersion describes if this client should use and implement legacy
	// labels or new ones
	labelVersion constants.LabelVersion

	// randomPrefix generates an annotation for stateful sets.
	randomPrefix utils.RandomPrefixFunc
}

// To regenerate the mocks for the kubernetes Client used by this broker,
// run "go generate" from the package directory.
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/k8sclient_mock.go k8s.io/client-go/kubernetes Interface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/context_mock.go context Context
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/appv1_mock.go k8s.io/client-go/kubernetes/typed/apps/v1 AppsV1Interface,DeploymentInterface,StatefulSetInterface,DaemonSetInterface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/corev1_mock.go k8s.io/client-go/kubernetes/typed/core/v1 EventInterface,CoreV1Interface,NamespaceInterface,PodInterface,ServiceInterface,ConfigMapInterface,PersistentVolumeInterface,PersistentVolumeClaimInterface,SecretInterface,NodeInterface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/networkingv1beta1_mock.go -mock_names=IngressInterface=MockIngressV1Beta1Interface k8s.io/client-go/kubernetes/typed/networking/v1beta1 NetworkingV1beta1Interface,IngressInterface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/networkingv1_mock.go -mock_names=IngressInterface=MockIngressV1Interface k8s.io/client-go/kubernetes/typed/networking/v1 NetworkingV1Interface,IngressInterface,IngressClassInterface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/storagev1_mock.go k8s.io/client-go/kubernetes/typed/storage/v1 StorageV1Interface,StorageClassInterface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/rbacv1_mock.go k8s.io/client-go/kubernetes/typed/rbac/v1 RbacV1Interface,ClusterRoleBindingInterface,ClusterRoleInterface,RoleInterface,RoleBindingInterface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/apiextensionsv1beta1_mock.go -mock_names=CustomResourceDefinitionInterface=MockCustomResourceDefinitionV1Beta1Interface k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1 ApiextensionsV1beta1Interface,CustomResourceDefinitionInterface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/apiextensionsv1_mock.go -mock_names=CustomResourceDefinitionInterface=MockCustomResourceDefinitionV1Interface k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1 ApiextensionsV1Interface,CustomResourceDefinitionInterface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/apiextensionsclientset_mock.go -mock_names=Interface=MockApiExtensionsClientInterface k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset Interface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/discovery_mock.go k8s.io/client-go/discovery DiscoveryInterface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/dynamic_mock.go -mock_names=Interface=MockDynamicInterface k8s.io/client-go/dynamic Interface,ResourceInterface,NamespaceableResourceInterface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/admissionregistrationv1beta1_mock.go -mock_names=MutatingWebhookConfigurationInterface=MockMutatingWebhookConfigurationV1Beta1Interface,ValidatingWebhookConfigurationInterface=MockValidatingWebhookConfigurationV1Beta1Interface k8s.io/client-go/kubernetes/typed/admissionregistration/v1beta1  AdmissionregistrationV1beta1Interface,MutatingWebhookConfigurationInterface,ValidatingWebhookConfigurationInterface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/admissionregistrationv1_mock.go -mock_names=MutatingWebhookConfigurationInterface=MockMutatingWebhookConfigurationV1Interface,ValidatingWebhookConfigurationInterface=MockValidatingWebhookConfigurationV1Interface k8s.io/client-go/kubernetes/typed/admissionregistration/v1  AdmissionregistrationV1Interface,MutatingWebhookConfigurationInterface,ValidatingWebhookConfigurationInterface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/serviceaccountinformer_mock.go k8s.io/client-go/informers/core/v1 ServiceAccountInformer
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/serviceaccountlister_mock.go k8s.io/client-go/listers/core/v1 ServiceAccountLister,ServiceAccountNamespaceLister
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/sharedindexinformer_mock.go k8s.io/client-go/tools/cache SharedIndexInformer
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/restclient_mock.go -mock_names=Interface=MockRestClientInterface k8s.io/client-go/rest Interface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/serviceaccount_mock.go k8s.io/client-go/kubernetes/typed/core/v1 ServiceAccountInterface

// NewK8sClientFunc defines a function which returns a k8s client based on the supplied config.
type NewK8sClientFunc func(c *rest.Config) (kubernetes.Interface, apiextensionsclientset.Interface, dynamic.Interface, error)

// newK8sBroker returns a kubernetes client for the specified k8s cluster.
func newK8sBroker(
	controllerUUID string,
	k8sRestConfig *rest.Config,
	cfg *config.Config,
	namespace string,
	newClient NewK8sClientFunc,
	newRestClient k8sspecs.NewK8sRestClientFunc,
	newWatcher k8swatcher.NewK8sWatcherFunc,
	newStringsWatcher k8swatcher.NewK8sStringsWatcherFunc,
	randomPrefix utils.RandomPrefixFunc,
	clock jujuclock.Clock,
) (*kubernetesClient, error) {
	k8sClient, apiextensionsClient, dynamicClient, err := newClient(k8sRestConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	newCfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelUUID := newCfg.UUID()
	if modelUUID == "" {
		return nil, errors.NotValidf("modelUUID is required")
	}
	modelName := newCfg.Name()
	if modelName == "" {
		return nil, errors.NotValidf("modelName is required")
	}

	labelVersion := constants.LastLabelVersion
	if namespace != "" {
		labelVersion, err = utils.DetectModelLabelVersion(
			namespace, modelName, modelUUID, controllerUUID, k8sClient.CoreV1().Namespaces())
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	client := &kubernetesClient{
		clock:                       clock,
		clientUnlocked:              k8sClient,
		apiextensionsClientUnlocked: apiextensionsClient,
		dynamicClientUnlocked:       dynamicClient,
		envCfgUnlocked:              newCfg.Config,
		k8sCfgUnlocked:              k8sRestConfig,
		informerFactoryUnlocked: informers.NewSharedInformerFactoryWithOptions(
			k8sClient,
			InformerResyncPeriod,
			informers.WithNamespace(namespace),
		),
		namespace:         namespace,
		modelUUID:         modelUUID,
		modelName:         modelName,
		controllerUUID:    controllerUUID,
		newWatcher:        newWatcher,
		newStringsWatcher: newStringsWatcher,
		newClient:         newClient,
		newRestClient:     newRestClient,
		randomPrefix:      randomPrefix,
		annotations: k8sannotations.New(nil).
			Add(utils.AnnotationModelUUIDKey(labelVersion), modelUUID),
		labelVersion: labelVersion,
	}
	if len(controllerUUID) > 0 {
		client.annotations.Add(utils.AnnotationControllerUUIDKey(labelVersion), controllerUUID)
	}
	if namespace == "" {
		return client, nil
	}

	ns, err := client.getNamespaceByName(namespace)
	if errors.Is(err, errors.NotFound) {
		return client, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	if !isK8sObjectOwnedByJuju(ns.ObjectMeta) {
		return client, nil
	}

	if err := client.ensureNamespaceAnnotationForControllerUUID(ns, controllerUUID, labelVersion); err != nil {
		return nil, errors.Trace(err)
	}
	return client, nil
}

func (k *kubernetesClient) ensureNamespaceAnnotationForControllerUUID(
	ns *core.Namespace,
	controllerUUID string,
	labelVersion constants.LabelVersion,
) error {
	if len(controllerUUID) == 0 {
		// controllerUUID could be empty in add-k8s without -c because there might be no controller yet.
		return nil
	}
	annotationControllerUUIDKey := utils.AnnotationControllerUUIDKey(labelVersion)
	if labelVersion > 0 {
		// Ignore the controller uuid since it is handled below for model migrations.
		expected := k.annotations.Copy()
		expected.Remove(annotationControllerUUIDKey)
		if ns != nil && !k8sannotations.New(ns.Annotations).HasAll(expected) {
			// This should never happen unless we changed annotations for a new juju version.
			// But in this case, we should have already managed to fix it in upgrade steps.
			return fmt.Errorf("annotations %v for namespace %q %w must include %v",
				ns.Annotations, k.namespace, errors.NotValid, k.annotations)
		}
	}
	if ns.Annotations[annotationControllerUUIDKey] == controllerUUID {
		// No change needs to be done.
		return nil
	}
	// The model was just migrated from a different controller.
	logger.Debugf("model %q was migrated from controller %q, updating the controller annotation to %q", k.namespace,
		ns.Annotations[annotationControllerUUIDKey], controllerUUID,
	)
	if err := k.ensureNamespaceAnnotations(ns); err != nil {
		return errors.Trace(err)
	}
	_, err := k.client().CoreV1().Namespaces().Update(context.TODO(), ns, v1.UpdateOptions{})
	return errors.Trace(err)
}

// GetAnnotations returns current namespace's annotations.
func (k *kubernetesClient) GetAnnotations() k8sannotations.Annotation {
	return k.annotations
}

var k8sversionNumberExtractor = regexp.MustCompile("[0-9]+")

// Version returns cluster version information.
func (k *kubernetesClient) Version() (ver *version.Number, err error) {
	k8sver, err := k.client().Discovery().ServerVersion()
	if err != nil {
		return nil, errors.Trace(err)
	}

	clean := func(s string) string {
		return k8sversionNumberExtractor.FindString(s)
	}

	ver = &version.Number{}
	if ver.Major, err = strconv.Atoi(clean(k8sver.Major)); err != nil {
		return nil, errors.Trace(err)
	}
	if ver.Minor, err = strconv.Atoi(clean(k8sver.Minor)); err != nil {
		return nil, errors.Trace(err)
	}
	return ver, nil
}

// addAnnotations set an annotation to current namespace's annotations.
func (k *kubernetesClient) addAnnotations(key, value string) k8sannotations.Annotation {
	return k.annotations.Add(key, value)
}

func (k *kubernetesClient) client() kubernetes.Interface {
	k.lock.Lock()
	defer k.lock.Unlock()
	client := k.clientUnlocked
	return client
}

func (k *kubernetesClient) extendedClient() apiextensionsclientset.Interface {
	k.lock.Lock()
	defer k.lock.Unlock()
	client := k.apiextensionsClientUnlocked
	return client
}

func (k *kubernetesClient) dynamicClient() dynamic.Interface {
	k.lock.Lock()
	defer k.lock.Unlock()
	client := k.dynamicClientUnlocked
	return client
}

// Config returns environ config.
func (k *kubernetesClient) Config() *config.Config {
	k.lock.Lock()
	defer k.lock.Unlock()
	cfg := k.envCfgUnlocked
	return cfg
}

func (k *kubernetesClient) k8sConfig() *rest.Config {
	k.lock.Lock()
	defer k.lock.Unlock()
	return rest.CopyConfig(k.k8sCfgUnlocked)
}

// SetConfig is specified in the Environ interface.
func (k *kubernetesClient) SetConfig(cfg *config.Config) error {
	k.lock.Lock()
	defer k.lock.Unlock()
	newCfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return errors.Trace(err)
	}
	k.envCfgUnlocked = newCfg.Config
	return nil
}

// SetCloudSpec is specified in the environs.Environ interface.
func (k *kubernetesClient) SetCloudSpec(_ context.Context, spec environscloudspec.CloudSpec) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	k.lock.Lock()
	defer k.lock.Unlock()

	k8sRestConfig, err := CloudSpecToK8sRestConfig(spec)
	if err != nil {
		return errors.Annotate(err, "cannot set cloud spec")
	}

	k.clientUnlocked, k.apiextensionsClientUnlocked, k.dynamicClientUnlocked, err = k.newClient(k8sRestConfig)
	if err != nil {
		return errors.Annotate(err, "cannot set cloud spec")
	}
	k.k8sCfgUnlocked = rest.CopyConfig(k8sRestConfig)

	k.informerFactoryUnlocked = informers.NewSharedInformerFactoryWithOptions(
		k.clientUnlocked,
		InformerResyncPeriod,
		informers.WithNamespace(k.namespace),
	)
	return nil
}

// PrepareForBootstrap prepares for bootstrapping a controller.
func (k *kubernetesClient) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	alreadyExistErr := errors.NewAlreadyExists(nil,
		fmt.Sprintf(`a controller called %q already exists on this k8s cluster.
Please bootstrap again and choose a different controller name.`, controllerName),
	)

	k.namespace = DecideControllerNamespace(controllerName)

	// ensure no existing namespace has the same name.
	_, err := k.getNamespaceByName(k.namespace)
	if err == nil {
		return alreadyExistErr
	}
	if !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	// Good, no existing namespace has the same name.
	// Now, try to find if there is any existing controller running in this cluster.
	// Note: we have to do this check before we are confident to support multi controllers running in same k8s cluster.

	_, err = k.listNamespacesByAnnotations(k.annotations)
	if err == nil {
		return alreadyExistErr
	}
	if !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	// All good, no existing controller found on the cluster.
	// The namespace will be set to controller-name in newcontrollerStack.

	// do validation on storage class.
	_, err = k.validateOperatorStorage()
	return errors.Trace(err)
}

// Create (environs.BootstrapEnviron) creates a new environ.
// It must raise an error satisfying IsAlreadyExists if the
// namespace is already used by another model.
func (k *kubernetesClient) Create(envcontext.ProviderCallContext, environs.CreateParams) error {
	return errors.Trace(k.createNamespace(k.namespace))
}

// EnsureImageRepoSecret ensures the image pull secret gets created.
func (k *kubernetesClient) EnsureImageRepoSecret(imageRepo docker.ImageRepoDetails) error {
	if !imageRepo.IsPrivate() {
		return nil
	}
	logger.Debugf("creating secret for image repo %q", imageRepo.Repository)
	secretData, err := imageRepo.SecretData()
	if err != nil {
		return errors.Trace(err)
	}
	_, err = k.ensureOCIImageSecret(
		constants.CAASImageRepoSecretName,
		utils.LabelsJuju, secretData,
		k.annotations,
	)
	return errors.Trace(err)
}

// Bootstrap deploys controller with mongoDB together into k8s cluster.
func (k *kubernetesClient) Bootstrap(
	ctx environs.BootstrapContext,
	callCtx envcontext.ProviderCallContext,
	args environs.BootstrapParams,
) (*environs.BootstrapResult, error) {

	if !args.BootstrapBase.Empty() {
		return nil, errors.NotSupportedf("set base for bootstrapping to kubernetes")
	}

	storageClass, err := k.validateOperatorStorage()
	if err != nil {
		return nil, errors.Trace(err)
	}

	finalizer := func(ctx environs.BootstrapContext, pcfg *podcfg.ControllerPodConfig, opts environs.BootstrapDialOpts) (err error) {
		podcfg.FinishControllerPodConfig(pcfg, k.Config(), args.ExtraAgentValuesForTesting)
		if err = pcfg.VerifyConfig(); err != nil {
			return errors.Trace(err)
		}

		logger.Debugf("controller pod config: \n%+v", pcfg)

		// validate initial model name if we need to create it.
		if initialModelName, has := pcfg.GetInitialModel(); has {
			_, err := k.getNamespaceByName(initialModelName)
			if err == nil {
				return errors.NewAlreadyExists(nil,
					fmt.Sprintf(`
namespace %q already exists in the cluster,
please choose a different initial model name then try again.`, initialModelName),
				)
			}
			if !errors.IsNotFound(err) {
				return errors.Trace(err)
			}
			// hosted model is all good.
		}

		// we use controller name to name controller namespace in bootstrap time.
		setControllerNamespace := func(controllerName string, broker *kubernetesClient) error {
			nsName := DecideControllerNamespace(controllerName)

			_, err := broker.GetNamespace(nsName)
			if errors.IsNotFound(err) {
				// all good.
				// ensure controller specific annotations.
				_ = broker.addAnnotations(utils.AnnotationControllerIsControllerKey(k.LabelVersion()), "true")
				return nil
			}
			if err == nil {
				// this should never happen because we avoid it in broker.PrepareForBootstrap before reaching here.
				return errors.NotValidf("existing namespace %q found", broker.namespace)
			}
			return errors.Trace(err)
		}

		if err := setControllerNamespace(pcfg.ControllerName, k); err != nil {
			return errors.Trace(err)
		}

		// create configmap, secret, volume, statefulset, etc resources for controller stack.
		controllerStack, err := newcontrollerStack(ctx, k8sconstants.JujuControllerStackName, storageClass, k, pcfg)
		if err != nil {
			return errors.Trace(err)
		}
		return errors.Annotate(
			controllerStack.Deploy(),
			"creating controller stack",
		)
	}

	podArch := arch.AMD64
	if args.BootstrapConstraints.HasArch() {
		podArch = *args.BootstrapConstraints.Arch
	}
	// TODO(wallyworld) - use actual series of controller pod image
	return &environs.BootstrapResult{
		Arch:                   podArch,
		Base:                   jujuversion.DefaultSupportedLTSBase(),
		CaasBootstrapFinalizer: finalizer,
	}, nil
}

// DestroyController implements the Environ interface.
func (k *kubernetesClient) DestroyController(ctx envcontext.ProviderCallContext, controllerUUID string) error {
	// ensures all annnotations are set correctly, then we will accurately find the controller namespace to destroy it.
	k.annotations.Merge(
		k8sannotations.New(nil).
			Add(utils.AnnotationControllerUUIDKey(k.LabelVersion()), controllerUUID).
			Add(utils.AnnotationControllerIsControllerKey(k.LabelVersion()), "true"),
	)
	return k.Destroy(ctx)
}

// SharedInformerFactory returns the default k8s SharedInformerFactory used by
// this broker.
func (k *kubernetesClient) SharedInformerFactory() informers.SharedInformerFactory {
	k.lock.Lock()
	defer k.lock.Unlock()
	return k.informerFactoryUnlocked
}

// ModelUUID returns the UUID of the model this broker was created for.
func (k *kubernetesClient) ModelUUID() string {
	return k.modelUUID
}

// ModelName returns the name of the model this broker was created for.
func (k *kubernetesClient) ModelName() string {
	return k.modelName
}

// ControllerUUID returns the UUID of the controller this broker was created for.
func (k *kubernetesClient) ControllerUUID() string {
	return k.controllerUUID
}

// Namespace returns the namespace of the model this broker was created for.
func (k *kubernetesClient) Namespace() string {
	return k.namespace
}

// Provider is part of the Broker interface.
func (*kubernetesClient) Provider() caas.ContainerEnvironProvider {
	return providerInstance
}

// Destroy is part of the Broker interface.
func (k *kubernetesClient) Destroy(ctx envcontext.ProviderCallContext) (err error) {
	defer func() {
		if errors.Cause(err) == context.DeadlineExceeded {
			logger.Warningf("destroy k8s model timeout")
			return
		}
		if err != nil && k8serrors.ReasonForError(err) == v1.StatusReasonUnknown {
			logger.Warningf("k8s cluster is not accessible: %v", err)
			err = nil
		}
	}()

	errChan := make(chan error, 1)
	done := make(chan struct{})

	destroyCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go k.deleteClusterScopeResourcesModelTeardown(destroyCtx, &wg, errChan)
	wg.Add(1)
	go k.deleteNamespaceModelTeardown(destroyCtx, &wg, errChan)

	go func() {
		wg.Wait()
		close(done)
	}()

	for {
		select {
		case err = <-errChan:
			if err != nil {
				return errors.Trace(err)
			}
		case <-destroyCtx.Done():
			return destroyCtx.Err()
		case <-done:
			return destroyCtx.Err()
		}
	}
}

// APIVersion returns the version info for the cluster.
func (k *kubernetesClient) APIVersion() (string, error) {
	ver, err := k.Version()
	if err != nil {
		return "", errors.Trace(err)
	}
	return ver.String(), nil
}

// getStorageClass returns a named storage class, first looking for
// one which is qualified by the current namespace if it's available.
func (k *kubernetesClient) getStorageClass(name string) (*storagev1.StorageClass, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	storageClasses := k.client().StorageV1().StorageClasses()
	qualifiedName := constants.QualifiedStorageClassName(k.namespace, name)
	sc, err := storageClasses.Get(context.TODO(), qualifiedName, v1.GetOptions{})
	if err == nil {
		return sc, nil
	}
	if !k8serrors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	return storageClasses.Get(context.TODO(), name, v1.GetOptions{})
}

// GetService returns the service for the specified application.
func (k *kubernetesClient) GetService(appName string, mode caas.DeploymentMode, includeClusterIP bool) (*caas.Service, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	services := k.client().CoreV1().Services(k.namespace)
	labels := utils.LabelsForApp(appName, k.LabelVersion())
	if mode == caas.ModeOperator {
		labels = utils.LabelsForOperator(appName, OperatorAppTarget, k.LabelVersion())
	}
	if k.LabelVersion() != constants.LegacyLabelVersion {
		labels = utils.LabelsMerge(labels, utils.LabelsJuju)
	}

	servicesList, err := services.List(context.TODO(), v1.ListOptions{
		LabelSelector: utils.LabelsToSelector(labels).String(),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	var (
		result caas.Service
		svc    *core.Service
	)
	// We may have the stateful set or deployment but service not done yet.
	if len(servicesList.Items) > 0 {
		for _, v := range servicesList.Items {
			s := v
			// Ignore any headless service for this app.
			if !strings.HasSuffix(s.Name, "-endpoints") {
				svc = &s
				break
			}
		}
		if svc != nil {
			result.Id = string(svc.GetUID())
			result.Addresses = utils.GetSvcAddresses(svc, includeClusterIP)
		}
	}

	if mode == caas.ModeOperator {
		appName = k.operatorName(appName)
	}
	deploymentName := k.deploymentName(appName, true)
	statefulsets := k.client().AppsV1().StatefulSets(k.namespace)
	ss, err := statefulsets.Get(context.TODO(), deploymentName, v1.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if err == nil {
		if ss.Spec.Replicas != nil {
			scale := int(*ss.Spec.Replicas)
			result.Scale = &scale
		}
		gen := ss.GetGeneration()
		result.Generation = &gen
		message, ssStatus, err := k.getStatefulSetStatus(ss)
		if err != nil {
			return nil, errors.Annotatef(err, "getting status for %s", ss.Name)
		}
		result.Status = status.StatusInfo{
			Status:  ssStatus,
			Message: message,
		}
		return &result, nil
	}

	deployments := k.client().AppsV1().Deployments(k.namespace)
	deployment, err := deployments.Get(context.TODO(), deploymentName, v1.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if err == nil {
		if deployment.Spec.Replicas != nil {
			scale := int(*deployment.Spec.Replicas)
			result.Scale = &scale
		}
		gen := deployment.GetGeneration()
		result.Generation = &gen
		message, deployStatus, err := k.getDeploymentStatus(deployment)
		if err != nil {
			return nil, errors.Annotatef(err, "getting status for %s", ss.Name)
		}
		result.Status = status.StatusInfo{
			Status:  deployStatus,
			Message: message,
		}
		return &result, nil
	}

	daemonsets := k.client().AppsV1().DaemonSets(k.namespace)
	ds, err := daemonsets.Get(context.TODO(), deploymentName, v1.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if err == nil {
		// The total number of nodes that should be running the daemon pod (including nodes correctly running the daemon pod).
		scale := int(ds.Status.DesiredNumberScheduled)
		result.Scale = &scale

		gen := ds.GetGeneration()
		result.Generation = &gen
		message, dsStatus, err := k.getDaemonSetStatus(ds)
		if err != nil {
			return nil, errors.Annotatef(err, "getting status for %s", ss.Name)
		}
		result.Status = status.StatusInfo{
			Status:  dsStatus,
			Message: message,
		}
	}
	return &result, nil
}

// DeleteService deletes the specified service with all related resources.
func (k *kubernetesClient) DeleteService(appName string) (err error) {
	logger.Debugf("deleting application %s", appName)

	// We prefer deleting resources using labels to do bulk deletion.
	// Deleting resources using deployment name has been deprecated.
	// But we keep it for now because some old resources created by
	// very old Juju probably do not have proper labels set.
	deploymentName := k.deploymentName(appName, true)
	if err := k.deleteService(deploymentName); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteStatefulSet(deploymentName); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteService(headlessServiceName(deploymentName)); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteDeployment(deploymentName); err != nil {
		return errors.Trace(err)
	}

	if err := k.deleteStatefulSets(appName); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteDeployments(appName); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteServices(appName); err != nil {
		return errors.Trace(err)
	}

	if err := k.deleteSecrets(appName); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteConfigMaps(appName); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteAllServiceAccountResources(appName); err != nil {
		return errors.Trace(err)
	}
	// Order matters: delete custom resources first then custom resource definitions.
	if err := k.deleteCustomResourcesForApp(appName); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteCustomResourceDefinitionsForApp(appName); err != nil {
		return errors.Trace(err)
	}

	if err := k.deleteMutatingWebhookConfigurationsForApp(appName); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteValidatingWebhookConfigurationsForApp(appName); err != nil {
		return errors.Trace(err)
	}

	if err := k.deleteIngressResources(appName); err != nil {
		return errors.Trace(err)
	}

	if err := k.deleteDaemonSets(appName); err != nil {
		return errors.Trace(err)
	}
	return nil
}

const applyRawSpecTimeoutSeconds = 20

func (k *kubernetesClient) applyRawK8sSpec(
	appName, deploymentName string,
	statusCallback caas.StatusCallbackFunc,
	params *caas.ServiceParams,
	numUnits int,
	config coreconfig.ConfigAttributes,
) (err error) {
	if k.namespace == "" {
		return errNoNamespace
	}

	if params == nil || len(params.RawK8sSpec) == 0 {
		return errors.Errorf("missing raw k8s spec")
	}

	if params.Deployment.DeploymentType == "" {
		params.Deployment.DeploymentType = caas.DeploymentStateless
		if len(params.Filesystems) > 0 {
			params.Deployment.DeploymentType = caas.DeploymentStateful
		}
	}

	// TODO(caas): support Constraints, FileSystems, Devices, InitContainer for actions, etc.
	if err := params.Deployment.DeploymentType.Validate(); err != nil {
		return errors.Trace(err)
	}

	labelGetter := func(isNamespaced bool) map[string]string {
		labels := utils.SelectorLabelsForApp(appName, k.LabelVersion())
		if !isNamespaced {
			labels = utils.LabelsMerge(
				labels,
				utils.LabelsForModel(k.ModelName(), k.ModelUUID(), k.ControllerUUID(), k.LabelVersion()),
			)
		}
		return labels
	}
	annotations := utils.ResourceTagsToAnnotations(params.ResourceTags, k.LabelVersion())

	builder := k8sspecs.New(
		deploymentName, k.namespace, params.Deployment, k.k8sConfig(),
		labelGetter, annotations, k.newRestClient,
	)
	ctx, cancel := context.WithTimeout(context.Background(), applyRawSpecTimeoutSeconds*time.Second)
	defer cancel()
	return builder.Deploy(ctx, params.RawK8sSpec, true)
}

// EnsureService creates or updates a service for pods with the given params.
func (k *kubernetesClient) EnsureService(
	appName string,
	statusCallback caas.StatusCallbackFunc,
	params *caas.ServiceParams,
	numUnits int,
	config coreconfig.ConfigAttributes,
) (err error) {
	defer func() {
		if err != nil {
			_ = statusCallback(appName, status.Error, err.Error(), nil)
		}
	}()

	logger.Debugf("creating/updating application %s", appName)
	deploymentName := k.deploymentName(appName, true)

	if numUnits < 0 {
		return errors.Errorf("number of units must be >= 0")
	}
	if numUnits == 0 {
		return k.deleteAllPods(appName, deploymentName)
	}
	if params.PodSpec != nil && len(params.RawK8sSpec) > 0 {
		// This should never happen.
		return errors.NotValidf("both pod spec and raw k8s spec provided")
	}

	if params.PodSpec != nil {
		if config == nil {
			return errors.Errorf("config for k8s app %q cannot be nil", appName)
		}
		return k.ensureService(appName, deploymentName, statusCallback, params, numUnits, config)
	}
	if len(params.RawK8sSpec) > 0 {
		return k.applyRawK8sSpec(appName, deploymentName, statusCallback, params, numUnits, config)
	}
	return nil
}

func (k *kubernetesClient) ensureService(
	appName, deploymentName string,
	statusCallback caas.StatusCallbackFunc,
	params *caas.ServiceParams,
	numUnits int,
	config coreconfig.ConfigAttributes,
) (err error) {

	if params == nil || params.PodSpec == nil {
		return errors.Errorf("missing pod spec")
	}

	if err := params.Deployment.DeploymentType.Validate(); err != nil {
		return errors.Trace(err)
	}

	var cleanups []func()
	defer func() {
		if err == nil {
			return
		}
		for _, f := range cleanups {
			f()
		}
	}()

	workloadSpec, err := prepareWorkloadSpec(appName, deploymentName, params.PodSpec, params.ImageDetails)
	if err != nil {
		return errors.Annotatef(err, "parsing unit spec for %s", appName)
	}

	annotations := utils.ResourceTagsToAnnotations(params.ResourceTags, k.LabelVersion())

	// ensure services.
	if len(workloadSpec.Services) > 0 {
		servicesCleanUps, err := k.ensureServicesForApp(appName, deploymentName, annotations, workloadSpec.Services)
		cleanups = append(cleanups, servicesCleanUps...)
		if err != nil {
			return errors.Annotate(err, "creating or updating services")
		}
	}

	// ensure configmap.
	if len(workloadSpec.ConfigMaps) > 0 {
		cmsCleanUps, err := k.ensureConfigMaps(appName, annotations, workloadSpec.ConfigMaps)
		cleanups = append(cleanups, cmsCleanUps...)
		if err != nil {
			return errors.Annotate(err, "creating or updating configmaps")
		}
	}

	// ensure secrets.
	if len(workloadSpec.Secrets) > 0 {
		secretsCleanUps, err := k.ensureSecrets(appName, annotations, workloadSpec.Secrets)
		cleanups = append(cleanups, secretsCleanUps...)
		if err != nil {
			return errors.Annotate(err, "creating or updating secrets")
		}
	}

	// ensure custom resource definitions.
	crds := workloadSpec.CustomResourceDefinitions
	if len(crds) > 0 {
		crdCleanUps, err := k.ensureCustomResourceDefinitions(appName, annotations, crds)
		cleanups = append(cleanups, crdCleanUps...)
		if err != nil {
			return errors.Annotate(err, "creating or updating custom resource definitions")
		}
		logger.Debugf("created/updated custom resource definition for %q.", appName)

	}
	// ensure custom resources.
	crs := workloadSpec.CustomResources
	if len(crs) > 0 {
		crCleanUps, err := k.ensureCustomResources(appName, annotations, crs)
		cleanups = append(cleanups, crCleanUps...)
		if err != nil {
			return errors.Annotate(err, "creating or updating custom resources")
		}
		logger.Debugf("created/updated custom resources for %q.", appName)
	}

	// ensure mutating webhook configurations.
	mutatingWebhookConfigurations := workloadSpec.MutatingWebhookConfigurations
	if len(mutatingWebhookConfigurations) > 0 {
		cfgCleanUps, err := k.ensureMutatingWebhookConfigurations(appName, annotations, mutatingWebhookConfigurations)
		cleanups = append(cleanups, cfgCleanUps...)
		if err != nil {
			return errors.Annotate(err, "creating or updating mutating webhook configurations")
		}
		logger.Debugf("created/updated mutating webhook configurations for %q.", appName)
	}
	// ensure validating webhook configurations.
	validatingWebhookConfigurations := workloadSpec.ValidatingWebhookConfigurations
	if len(validatingWebhookConfigurations) > 0 {
		cfgCleanUps, err := k.ensureValidatingWebhookConfigurations(appName, annotations, validatingWebhookConfigurations)
		cleanups = append(cleanups, cfgCleanUps...)
		if err != nil {
			return errors.Annotate(err, "creating or updating validating webhook configurations")
		}
		logger.Debugf("created/updated validating webhook configurations for %q.", appName)
	}

	// ensure ingress resources.
	ings := workloadSpec.IngressResources
	if len(ings) > 0 {
		ingCleanUps, err := k.ensureIngressResources(appName, annotations, workloadSpec.IngressResources)
		cleanups = append(cleanups, ingCleanUps...)
		if err != nil {
			return errors.Annotate(err, "creating or updating ingress resources")
		}
		logger.Debugf("created/updated ingress resources for %q.", appName)
	}

	for _, sa := range workloadSpec.ServiceAccounts {
		saCleanups, err := k.ensureServiceAccountForApp(appName, annotations, sa)
		cleanups = append(cleanups, saCleanups...)
		if err != nil {
			return errors.Annotate(err, "creating or updating service account")
		}
	}

	if len(params.Devices) > 0 {
		if err = k.configureDevices(workloadSpec, params.Devices); err != nil {
			return errors.Annotatef(err, "configuring devices for %s", appName)
		}
	}
	if err := k8sapplication.ApplyConstraints(
		&workloadSpec.Pod.PodSpec, appName, params.Constraints, params.CharmsConstraints,
		func(pod *core.PodSpec, resourceName core.ResourceName, workloadConsVal string, charmConsVal string) error {
			if len(pod.Containers) == 0 {
				return nil
			}
			// Just the first container is enough for scheduling purposes.
			pod.Containers[0].Resources.Requests, err = k8sapplication.MergeConstraint(
				resourceName, workloadConsVal, pod.Containers[0].Resources.Requests,
			)
			if err != nil {
				return errors.Annotatef(err, "merging request constraint %s=%s", resourceName, workloadConsVal)
			}
			return nil
		},
	); err != nil {
		return errors.Trace(err)
	}

	for _, c := range params.PodSpec.Containers {
		if c.ImageDetails.Password == "" {
			continue
		}
		imageSecretName := appSecretName(deploymentName, c.Name)
		if err := k.ensureOCIImageSecretForApp(
			imageSecretName, appName, &c.ImageDetails, annotations.Copy(),
		); err != nil {
			return errors.Annotatef(err, "creating secrets for container: %s", c.Name)
		}
		cleanups = append(cleanups, func() { _ = k.deleteSecret(imageSecretName, "") })
	}
	// Add a deployment controller or stateful set configured to create the specified number of units/pods.
	// Defensively check to see if a stateful set is already used.
	if params.Deployment.DeploymentType == "" {
		// TODO(caas): we should really change `params.Deployment` to be required.
		params.Deployment.DeploymentType = caas.DeploymentStateless
		if len(params.Filesystems) > 0 {
			params.Deployment.DeploymentType = caas.DeploymentStateful
		}
	}
	if params.Deployment.DeploymentType != caas.DeploymentStateful {
		// TODO(caas): make sure deployment type is immutable.
		// Either not found or params.Deployment.DeploymentType == existing resource type.
		_, err := k.getStatefulSet(deploymentName)
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		if err == nil {
			params.Deployment.DeploymentType = caas.DeploymentStateful
			logger.Debugf("no updated filesystems but already using stateful set for %q", appName)
		}
	}

	if err = validateDeploymentType(params.Deployment.DeploymentType, params, workloadSpec.Service); err != nil {
		return errors.Trace(err)
	}

	hasService := !params.PodSpec.OmitServiceFrontend && !params.Deployment.ServiceType.IsOmit()
	if hasService {
		var ports []core.ContainerPort
		for _, c := range workloadSpec.Pod.Containers {
			for _, p := range c.Ports {
				if p.ContainerPort == 0 {
					continue
				}
				ports = append(ports, p)
			}
		}
		if len(ports) == 0 {
			return errors.Errorf("ports are required for kubernetes service %q", appName)
		}

		serviceAnnotations := annotations.Copy()
		// Merge any service annotations from the charm.
		if workloadSpec.Service != nil {
			serviceAnnotations.Merge(k8sannotations.New(workloadSpec.Service.Annotations))
		}
		// Merge any service annotations from the CLI.
		deployAnnotations, err := config.GetStringMap(serviceAnnotationsKey, nil)
		if err != nil {
			return errors.Annotatef(err, "unexpected annotations: %#v", config.Get(serviceAnnotationsKey, nil))
		}
		serviceAnnotations.Merge(k8sannotations.New(deployAnnotations))

		config[serviceAnnotationsKey] = serviceAnnotations.ToMap()
		if err := k.configureService(appName, deploymentName, ports, params, config); err != nil {
			return errors.Annotatef(err, "creating or updating service for %v", appName)
		}
	}

	numPods := int32(numUnits)
	workloadResourceAnnotations := annotations.Copy().
		// To solve https://bugs.launchpad.net/juju/+bug/1875481/comments/23 (`jujud caas-unit-init --upgrade`
		// does NOT work on containers are not using root as default USER),
		// CharmModifiedVersion is added for triggering rolling upgrade on workload pods to synchronise
		// charm files to workload pods via init container when charm was upgraded.
		// This approach was inspired from `kubectl rollout restart`.
		Add(utils.AnnotationCharmModifiedVersionKey(k.LabelVersion()), strconv.Itoa(params.CharmModifiedVersion))

	switch params.Deployment.DeploymentType {
	case caas.DeploymentStateful:
		if err := k.configureHeadlessService(appName, deploymentName, annotations.Copy()); err != nil {
			return errors.Annotate(err, "creating or updating headless service")
		}
		cleanups = append(cleanups, func() { _ = k.deleteService(headlessServiceName(deploymentName)) })
		if err := k.configureStatefulSet(appName, deploymentName, workloadResourceAnnotations.Copy(), workloadSpec, params.PodSpec.Containers, &numPods, params.Filesystems); err != nil {
			return errors.Annotate(err, "creating or updating StatefulSet")
		}
		cleanups = append(cleanups, func() { _ = k.deleteDeployment(appName) })
	case caas.DeploymentStateless:
		cleanUpDeployment, err := k.configureDeployment(appName, deploymentName, workloadResourceAnnotations.Copy(), workloadSpec, params.PodSpec.Containers, &numPods, params.Filesystems)
		cleanups = append(cleanups, cleanUpDeployment...)
		if err != nil {
			return errors.Annotate(err, "creating or updating Deployment")
		}
	case caas.DeploymentDaemon:
		cleanUpDaemonSet, err := k.configureDaemonSet(appName, deploymentName, workloadResourceAnnotations.Copy(), workloadSpec, params.PodSpec.Containers, params.Filesystems)
		cleanups = append(cleanups, cleanUpDaemonSet...)
		if err != nil {
			return errors.Annotate(err, "creating or updating DaemonSet")
		}
	default:
		// This should never happened because we have validated both in this method and in `charm.v6`.
		return errors.NotSupportedf("deployment type %q", params.Deployment.DeploymentType)
	}
	return nil
}

func validateDeploymentType(deploymentType caas.DeploymentType, params *caas.ServiceParams, svcSpec *specs.ServiceSpec) error {
	if svcSpec == nil {
		return nil
	}
	if deploymentType != caas.DeploymentStateful {
		if svcSpec.ScalePolicy != "" {
			return errors.NewNotValid(nil, fmt.Sprintf("ScalePolicy is only supported for %s applications", caas.DeploymentStateful))
		}
	}
	return nil
}

func (k *kubernetesClient) deleteAllPods(appName, deploymentName string) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	zero := int32(0)
	statefulsets := k.client().AppsV1().StatefulSets(k.namespace)
	statefulSet, err := statefulsets.Get(context.TODO(), deploymentName, v1.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if err == nil {
		statefulSet.Spec.Replicas = &zero
		_, err = statefulsets.Update(context.TODO(), statefulSet, v1.UpdateOptions{})
		return errors.Trace(err)
	}

	deployments := k.client().AppsV1().Deployments(k.namespace)
	deployment, err := deployments.Get(context.TODO(), deploymentName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	deployment.Spec.Replicas = &zero
	_, err = deployments.Update(context.TODO(), deployment, v1.UpdateOptions{})
	return errors.Trace(err)
}

type annotationGetter interface {
	GetAnnotations() map[string]string
}

// This random snippet will be included to the pvc name so that if the same app
// is deleted and redeployed again, the pvc retains a unique name.
// Only generate it once, and record it on the workload resource annotations .
func (k *kubernetesClient) getStorageUniqPrefix(getMeta func() (annotationGetter, error)) (string, error) {
	r, err := getMeta()
	if err == nil {
		if uniqID := r.GetAnnotations()[utils.AnnotationKeyApplicationUUID(k.LabelVersion())]; uniqID != "" {
			return uniqID, nil
		}
	} else if !errors.IsNotFound(err) {
		return "", errors.Trace(err)
	}
	return k.randomPrefix()
}

func (k *kubernetesClient) configureDevices(unitSpec *workloadSpec, devices []devices.KubernetesDeviceParams) error {
	for i := range unitSpec.Pod.Containers {
		resources := unitSpec.Pod.Containers[i].Resources
		for _, dev := range devices {
			err := mergeDeviceConstraints(dev, &resources)
			if err != nil {
				return errors.Annotatef(err, "merging device constraint %+v to %#v", dev, resources)
			}
		}
		unitSpec.Pod.Containers[i].Resources = resources
	}
	nodeLabel, err := getNodeSelectorFromDeviceConstraints(devices)
	if err != nil {
		return err
	}
	if nodeLabel != "" {
		nodeSelector := buildNodeSelector(nodeLabel)
		if unitSpec.Pod.NodeSelector != nil {
			for k, v := range nodeSelector {
				unitSpec.Pod.NodeSelector[k] = v
			}
		} else {
			unitSpec.Pod.NodeSelector = nodeSelector
		}
	}
	return nil
}

type configMapNameFunc func(fileSetName string) string

func (k *kubernetesClient) configurePodFiles(
	appName string,
	annotations map[string]string,
	workloadSpec *workloadSpec,
	containers []specs.ContainerSpec,
	cfgMapName configMapNameFunc,
) error {
	for _, container := range containers {
		for _, fileSet := range container.VolumeConfig {
			vol, err := k.fileSetToVolume(appName, annotations, workloadSpec, fileSet, cfgMapName)
			if err != nil {
				return errors.Trace(err)
			}
			if err = k8sstorage.PushUniqueVolume(&workloadSpec.Pod.PodSpec, vol, false); err != nil {
				return errors.Trace(err)
			}
			if err := configVolumeMount(
				container, workloadSpec,
				core.VolumeMount{
					// TODO(caas): add more config fields support(SubPath, ReadOnly, etc).
					Name:      vol.Name,
					MountPath: fileSet.MountPath,
				},
			); err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

func configVolumeMount(container specs.ContainerSpec, workloadSpec *workloadSpec, volMount core.VolumeMount) error {
	if container.Init {
		for i, c := range workloadSpec.Pod.InitContainers {
			if c.Name == container.Name {
				workloadSpec.Pod.InitContainers[i].VolumeMounts = append(workloadSpec.Pod.InitContainers[i].VolumeMounts, volMount)
				return nil
			}
		}
		return errors.Annotatef(errors.NotFoundf("init container %q", container.Name), "configuring volume mount %q", volMount.Name)
	}
	for i, c := range workloadSpec.Pod.Containers {
		if c.Name == container.Name {
			workloadSpec.Pod.Containers[i].VolumeMounts = append(workloadSpec.Pod.Containers[i].VolumeMounts, volMount)
			return nil
		}
	}
	return errors.Annotatef(errors.NotFoundf("container %q", container.Name), "configuring volume mount %q", volMount.Name)
}

func (k *kubernetesClient) configureStorage(
	appName string, legacy bool, uniquePrefix string,
	filesystems []storage.KubernetesFilesystemParams,
	podSpec *core.PodSpec,
	handlePVC func(core.PersistentVolumeClaim, string, bool) error,
) error {
	pvcNameGetter := func(i int, storageName string) string {
		s := fmt.Sprintf("%s-%s", storageName, uniquePrefix)
		if legacy {
			s = fmt.Sprintf("juju-%s-%d", storageName, i)
		}
		return s
	}
	fsNames := set.NewStrings()
	for i, fs := range filesystems {
		if fsNames.Contains(fs.StorageName) {
			return errors.NotValidf("duplicated storage name %q for %q", fs.StorageName, appName)
		}
		fsNames.Add(fs.StorageName)

		readOnly := false
		if fs.Attachment != nil {
			readOnly = fs.Attachment.ReadOnly
		}

		vol, pvc, err := k.filesystemToVolumeInfo(i, fs, pvcNameGetter)
		if err != nil {
			return errors.Trace(err)
		}
		mountPath := k8sstorage.GetMountPathForFilesystem(i, appName, fs)
		if vol != nil {
			logger.Debugf("using volume for %s filesystem %s: %s", appName, fs.StorageName, pretty.Sprint(*vol))
			if err = k8sstorage.PushUniqueVolume(podSpec, *vol, false); err != nil {
				return errors.Trace(err)
			}
			podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, core.VolumeMount{
				Name:      vol.Name,
				MountPath: mountPath,
			})
		}
		if pvc != nil && handlePVC != nil {
			logger.Debugf("using persistent volume claim for %s filesystem %s: %s", appName, fs.StorageName, pretty.Sprint(*pvc))
			if err = handlePVC(*pvc, mountPath, readOnly); err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

func ensureJujuInitContainer(podSpec *core.PodSpec, operatorImagePath string) error {
	initContainer, vol, volMounts, err := getJujuInitContainerAndStorageInfo(operatorImagePath)
	if err != nil {
		return errors.Trace(err)
	}

	replaceOrUpdateInitContainer := func() {
		for i, v := range podSpec.InitContainers {
			if v.Name == initContainer.Name {
				podSpec.InitContainers[i] = initContainer
				return
			}
		}
		podSpec.InitContainers = append(podSpec.InitContainers, initContainer)
	}
	replaceOrUpdateInitContainer()

	if err = k8sstorage.PushUniqueVolume(podSpec, vol, true); err != nil {
		return errors.Trace(err)
	}

	for i := range podSpec.Containers {
		container := &podSpec.Containers[i]
		for _, volMount := range volMounts {
			k8sstorage.PushUniqueVolumeMount(container, volMount)
		}
	}
	return nil
}

func getJujuInitContainerAndStorageInfo(operatorImagePath string) (container core.Container, vol core.Volume, volMounts []core.VolumeMount, err error) {
	dataDir := paths.DataDir(paths.OSUnixLike)
	jujuExec := paths.JujuExec(paths.OSUnixLike)
	jujudCmd := `
initCmd=$($JUJU_TOOLS_DIR/jujud help commands | grep caas-unit-init)
if test -n "$initCmd"; then
exec $JUJU_TOOLS_DIR/jujud caas-unit-init --debug --wait;
else
exit 0
fi`[1:]
	container = core.Container{
		Name:            caas.InitContainerName,
		Image:           operatorImagePath,
		ImagePullPolicy: core.PullIfNotPresent,
		VolumeMounts: []core.VolumeMount{{
			Name:      dataDirVolumeName,
			MountPath: dataDir,
		}},
		WorkingDir: dataDir,
		Command: []string{
			"/bin/sh",
		},
		Args: []string{
			"-c",
			fmt.Sprintf(
				caas.JujudStartUpSh,
				dataDir,
				"tools",
				jujudCmd,
			),
		},
	}
	vol = core.Volume{
		Name: dataDirVolumeName,
		VolumeSource: core.VolumeSource{
			EmptyDir: &core.EmptyDirVolumeSource{},
		},
	}
	volMounts = []core.VolumeMount{
		{Name: dataDirVolumeName, MountPath: dataDir},
		{Name: dataDirVolumeName, MountPath: jujuExec, SubPath: "tools/jujud"},
	}
	return container, vol, volMounts, nil
}

func podAnnotations(annotations k8sannotations.Annotation) k8sannotations.Annotation {
	// Add standard security annotations.
	return annotations.
		Add("apparmor.security.beta.kubernetes.io/pod", "runtime/default").
		Add("seccomp.security.beta.kubernetes.io/pod", "docker/default")
}

// https://kubernetes.io/docs/tasks/manage-daemon/update-daemon-set/#daemonset-update-strategy
func updateStrategyForDaemonSet(strategy specs.UpdateStrategy) (o apps.DaemonSetUpdateStrategy, err error) {
	strategyType := apps.DaemonSetUpdateStrategyType(strategy.Type)

	o = apps.DaemonSetUpdateStrategy{Type: strategyType}
	switch strategyType {
	case apps.OnDeleteDaemonSetStrategyType:
		if strategy.RollingUpdate != nil {
			return o, errors.NewNotValid(nil, fmt.Sprintf("rolling update spec is not supported for %q", strategyType))
		}
	case apps.RollingUpdateDaemonSetStrategyType:
		if strategy.RollingUpdate != nil {
			if strategy.RollingUpdate.Partition != nil || strategy.RollingUpdate.MaxSurge != nil {
				return o, errors.NotValidf("rolling update spec for daemonset")
			}
			if strategy.RollingUpdate.MaxUnavailable == nil {
				return o, errors.NewNotValid(nil, "rolling update spec maxUnavailable is missing")
			}
			o.RollingUpdate = &apps.RollingUpdateDaemonSet{
				MaxUnavailable: k8sspecs.IntOrStringToK8s(*strategy.RollingUpdate.MaxUnavailable),
			}
		}
	default:
		return o, errors.NotValidf("strategy type %q for daemonset", strategyType)
	}
	return o, nil
}

func (k *kubernetesClient) configureDaemonSet(
	appName, deploymentName string,
	annotations k8sannotations.Annotation,
	workloadSpec *workloadSpec,
	containers []specs.ContainerSpec,
	filesystems []storage.KubernetesFilesystemParams,
) (cleanUps []func(), err error) {
	logger.Debugf("creating/updating daemon set for %s", appName)

	// Add the specified file to the pod spec.
	cfgName := func(fileSetName string) string {
		return applicationConfigMapName(deploymentName, fileSetName)
	}
	if err := k.configurePodFiles(appName, annotations, workloadSpec, containers, cfgName); err != nil {
		return cleanUps, errors.Trace(err)
	}

	storageUniqueID, err := k.getStorageUniqPrefix(func() (annotationGetter, error) {
		return k.getDaemonSet(deploymentName)
	})
	if err != nil {
		return cleanUps, errors.Trace(err)
	}

	selectorLabels := utils.SelectorLabelsForApp(appName, k.LabelVersion())
	daemonSet := &apps.DaemonSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   deploymentName,
			Labels: utils.LabelsForApp(appName, k.LabelVersion()),
			Annotations: k8sannotations.New(nil).
				Merge(annotations).
				Add(utils.AnnotationKeyApplicationUUID(k.LabelVersion()), storageUniqueID).ToMap(),
		},
		Spec: apps.DaemonSetSpec{
			// TODO(caas): MinReadySeconds support.
			Selector: &v1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			RevisionHistoryLimit: pointer.Int32Ptr(daemonsetRevisionHistoryLimit),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: deploymentName + "-",
					Labels:       utils.LabelsMerge(workloadSpec.Pod.Labels, selectorLabels),
					Annotations:  podAnnotations(k8sannotations.New(workloadSpec.Pod.Annotations).Merge(annotations).Copy()).ToMap(),
				},
				Spec: workloadSpec.Pod.PodSpec,
			},
		},
	}
	if workloadSpec.Service != nil && workloadSpec.Service.UpdateStrategy != nil {
		if daemonSet.Spec.UpdateStrategy, err = updateStrategyForDaemonSet(*workloadSpec.Service.UpdateStrategy); err != nil {
			return cleanUps, errors.Trace(err)
		}
	}

	handlePVC := func(pvc core.PersistentVolumeClaim, mountPath string, readOnly bool) error {
		cs, err := k.configurePVCForStatelessResource(pvc, mountPath, readOnly, &daemonSet.Spec.Template.Spec)
		cleanUps = append(cleanUps, cs...)
		return errors.Trace(err)
	}
	// Storage support for daemonset is new.
	legacy := false
	if err := k.configureStorage(appName, legacy, storageUniqueID, filesystems, &daemonSet.Spec.Template.Spec, handlePVC); err != nil {
		return cleanUps, errors.Trace(err)
	}

	cU, err := k.ensureDaemonSet(daemonSet)
	cleanUps = append(cleanUps, cU)
	return cleanUps, errors.Trace(err)
}

// https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#strategy
func updateStrategyForDeployment(strategy specs.UpdateStrategy) (o apps.DeploymentStrategy, err error) {
	strategyType := apps.DeploymentStrategyType(strategy.Type)

	o = apps.DeploymentStrategy{Type: strategyType}
	switch strategyType {
	case apps.RecreateDeploymentStrategyType:
		if strategy.RollingUpdate != nil {
			return o, errors.NewNotValid(nil, fmt.Sprintf("rolling update spec is not supported for %q", strategyType))
		}
	case apps.RollingUpdateDeploymentStrategyType:
		if strategy.RollingUpdate != nil {
			if strategy.RollingUpdate.Partition != nil {
				return o, errors.NotValidf("rolling update spec for deployment")
			}
			if strategy.RollingUpdate.MaxSurge == nil && strategy.RollingUpdate.MaxUnavailable == nil {
				return o, errors.NewNotValid(nil, "empty rolling update spec")
			}
			o.RollingUpdate = &apps.RollingUpdateDeployment{}
			if strategy.RollingUpdate.MaxSurge != nil {
				o.RollingUpdate.MaxSurge = k8sspecs.IntOrStringToK8s(*strategy.RollingUpdate.MaxSurge)
			}
			if strategy.RollingUpdate.MaxUnavailable != nil {
				o.RollingUpdate.MaxUnavailable = k8sspecs.IntOrStringToK8s(*strategy.RollingUpdate.MaxUnavailable)
			}
		}
	default:
		return o, errors.NotValidf("strategy type %q for deployment", strategyType)
	}
	return o, nil
}

func (k *kubernetesClient) configureDeployment(
	appName, deploymentName string,
	annotations k8sannotations.Annotation,
	workloadSpec *workloadSpec,
	containers []specs.ContainerSpec,
	replicas *int32,
	filesystems []storage.KubernetesFilesystemParams,
) (cleanUps []func(), err error) {
	logger.Debugf("creating/updating deployment for %s", appName)

	// Add the specified file to the pod spec.
	cfgName := func(fileSetName string) string {
		return applicationConfigMapName(deploymentName, fileSetName)
	}
	if err := k.configurePodFiles(appName, annotations, workloadSpec, containers, cfgName); err != nil {
		return cleanUps, errors.Trace(err)
	}

	storageUniqueID, err := k.getStorageUniqPrefix(func() (annotationGetter, error) {
		return k.getDeployment(deploymentName)
	})
	if err != nil {
		return cleanUps, errors.Trace(err)
	}

	selectorLabels := utils.SelectorLabelsForApp(appName, k.LabelVersion())
	deployment := &apps.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   deploymentName,
			Labels: utils.LabelsForApp(appName, k.LabelVersion()),
			Annotations: k8sannotations.New(nil).
				Merge(annotations).
				Add(utils.AnnotationKeyApplicationUUID(k.LabelVersion()), storageUniqueID).ToMap(),
		},
		Spec: apps.DeploymentSpec{
			// TODO(caas): MinReadySeconds, ProgressDeadlineSeconds support.
			Replicas:             replicas,
			RevisionHistoryLimit: pointer.Int32Ptr(deploymentRevisionHistoryLimit),
			Selector: &v1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: deploymentName + "-",
					Labels:       utils.LabelsMerge(workloadSpec.Pod.Labels, selectorLabels),
					Annotations:  podAnnotations(k8sannotations.New(workloadSpec.Pod.Annotations).Merge(annotations).Copy()).ToMap(),
				},
				Spec: workloadSpec.Pod.PodSpec,
			},
		},
	}
	if workloadSpec.Service != nil && workloadSpec.Service.UpdateStrategy != nil {
		if deployment.Spec.Strategy, err = updateStrategyForDeployment(*workloadSpec.Service.UpdateStrategy); err != nil {
			return cleanUps, errors.Trace(err)
		}
	}
	handlePVC := func(pvc core.PersistentVolumeClaim, mountPath string, readOnly bool) error {
		cs, err := k.configurePVCForStatelessResource(pvc, mountPath, readOnly, &deployment.Spec.Template.Spec)
		cleanUps = append(cleanUps, cs...)
		return errors.Trace(err)
	}
	// Storage support for deployment is new.
	legacy := false
	if err := k.configureStorage(appName, legacy, storageUniqueID, filesystems, &deployment.Spec.Template.Spec, handlePVC); err != nil {
		return cleanUps, errors.Trace(err)
	}
	if err = k.ensureDeployment(deployment); err != nil {
		return cleanUps, errors.Trace(err)
	}
	cleanUps = append(cleanUps, func() { _ = k.deleteDeployment(appName) })
	return cleanUps, nil
}

func (k *kubernetesClient) configurePVCForStatelessResource(
	spec core.PersistentVolumeClaim, mountPath string, readOnly bool, podSpec *core.PodSpec,
) (cleanUps []func(), err error) {
	pvc, pvcCleanUp, err := k.ensurePVC(&spec)
	cleanUps = append(cleanUps, pvcCleanUp)
	if err != nil {
		return cleanUps, errors.Annotatef(err, "ensuring PVC %q", spec.GetName())
	}
	vol := core.Volume{
		Name: pvc.GetName(),
		VolumeSource: core.VolumeSource{
			PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
				ClaimName: pvc.GetName(),
				ReadOnly:  readOnly,
			},
		},
	}
	if err = k8sstorage.PushUniqueVolume(podSpec, vol, false); err != nil {
		return cleanUps, errors.Trace(err)
	}
	podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, core.VolumeMount{
		Name:      vol.Name,
		MountPath: mountPath,
	})
	return cleanUps, nil
}

func (k *kubernetesClient) ensureDeployment(spec *apps.Deployment) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	deployments := k.client().AppsV1().Deployments(k.namespace)
	_, err := k.createDeployment(spec)
	if err == nil || !errors.IsAlreadyExists(err) {
		return errors.Annotatef(err, "ensuring deployment %q", spec.GetName())
	}
	existing, err := k.getDeployment(spec.GetName())
	if err != nil {
		return errors.Trace(err)
	}
	existing.SetAnnotations(spec.GetAnnotations())
	existing.Spec = spec.Spec
	_, err = deployments.Update(context.TODO(), existing, v1.UpdateOptions{})
	if err != nil {
		return errors.Annotatef(err, "ensuring deployment %q", spec.GetName())
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) createDeployment(spec *apps.Deployment) (*apps.Deployment, error) {
	out, err := k.client().AppsV1().Deployments(k.namespace).Create(context.TODO(), spec, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("deployment %q", spec.GetName())
	}
	if k8serrors.IsInvalid(err) {
		return nil, errors.NotValidf("deployment %q", spec.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getDeployment(name string) (*apps.Deployment, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().AppsV1().Deployments(k.namespace).Get(context.TODO(), name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("deployment %q", name)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deleteDeployment(name string) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client().AppsV1().Deployments(k.namespace).Delete(context.TODO(), name, v1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteDeployments(appName string) error {
	err := k.client().AppsV1().Deployments(k.namespace).DeleteCollection(context.TODO(), v1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	}, v1.ListOptions{
		LabelSelector: utils.LabelsToSelector(
			utils.LabelsForApp(appName, k.LabelVersion())).String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func getPodManagementPolicy(svc *specs.ServiceSpec) (out apps.PodManagementPolicyType) {
	// Default to "Parallel".
	out = apps.ParallelPodManagement
	if svc == nil || svc.ScalePolicy == "" {
		return out
	}

	switch svc.ScalePolicy {
	case specs.SerialScale:
		return apps.OrderedReadyPodManagement
	case specs.ParallelScale:
		return apps.ParallelPodManagement
		// no need to consider other cases because we have done validation in podspec parsing stage.
	}
	return out
}

func (k *kubernetesClient) deleteVolumeClaims(appName string, p *core.Pod) ([]string, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	volumesByName := make(map[string]core.Volume)
	for _, pv := range p.Spec.Volumes {
		volumesByName[pv.Name] = pv
	}

	var deletedClaimVolumes []string
	for _, volMount := range p.Spec.Containers[0].VolumeMounts {
		vol, ok := volumesByName[volMount.Name]
		if !ok {
			logger.Warningf("volume for volume mount %q not found", volMount.Name)
			continue
		}
		if vol.PersistentVolumeClaim == nil {
			// Ignore volumes which are not Juju managed filesystems.
			continue
		}
		pvClaims := k.client().CoreV1().PersistentVolumeClaims(k.namespace)
		logger.Infof("deleting operator PVC %s for application %s due to call to kubernetesClient.deleteVolumeClaims", vol.PersistentVolumeClaim.ClaimName, appName)
		err := pvClaims.Delete(context.TODO(), vol.PersistentVolumeClaim.ClaimName, v1.DeleteOptions{
			PropagationPolicy: constants.DefaultPropagationPolicy(),
		})
		if err != nil && !k8serrors.IsNotFound(err) {
			return nil, errors.Annotatef(err, "deleting persistent volume claim %v for %v",
				vol.PersistentVolumeClaim.ClaimName, p.Name)
		}
		deletedClaimVolumes = append(deletedClaimVolumes, vol.Name)
	}
	return deletedClaimVolumes, nil
}

// CaasServiceToK8s translates a caas service type to a k8s one.
func CaasServiceToK8s(in caas.ServiceType) (core.ServiceType, error) {
	serviceType := defaultServiceType
	if in != "" {
		switch in {
		case caas.ServiceCluster:
			serviceType = core.ServiceTypeClusterIP
		case caas.ServiceLoadBalancer:
			serviceType = core.ServiceTypeLoadBalancer
		case caas.ServiceExternal:
			serviceType = core.ServiceTypeExternalName
		case caas.ServiceOmit:
			logger.Debugf("no service to be created because service type is %q", in)
			return "", nil
		default:
			return "", errors.NotSupportedf("service type %q", in)
		}
	}
	return serviceType, nil
}

func (k *kubernetesClient) configureService(
	appName, deploymentName string,
	containerPorts []core.ContainerPort,
	params *caas.ServiceParams,
	config coreconfig.ConfigAttributes,
) error {
	logger.Debugf("creating/updating service for %s", appName)

	var ports []core.ServicePort
	for i, cp := range containerPorts {
		// We normally expect a single container port for most use cases.
		// We allow the user to specify what first service port should be,
		// otherwise it just defaults to the container port.
		// TODO(caas) - consider allowing all service ports to be specified
		var targetPort intstr.IntOrString
		if i == 0 {
			targetPort = intstr.FromInt(config.GetInt(serviceTargetPortConfigKey, int(cp.ContainerPort)))
		}
		ports = append(ports, core.ServicePort{
			Name:       cp.Name,
			Protocol:   cp.Protocol,
			Port:       cp.ContainerPort,
			TargetPort: targetPort,
		})
	}

	serviceType := caas.ServiceType(config.GetString(ServiceTypeConfigKey, string(params.Deployment.ServiceType)))
	k8sServiceType, err := CaasServiceToK8s(serviceType)
	if err != nil {
		return errors.Trace(err)
	}
	annotations, err := config.GetStringMap(serviceAnnotationsKey, nil)
	if err != nil {
		return errors.Annotatef(err, "unexpected annotations: %#v", config.Get(serviceAnnotationsKey, nil))
	}
	service := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:        deploymentName,
			Labels:      utils.LabelsForApp(appName, k.LabelVersion()),
			Annotations: annotations,
		},
		Spec: core.ServiceSpec{
			Selector:                 utils.SelectorLabelsForApp(appName, k.LabelVersion()),
			Type:                     k8sServiceType,
			Ports:                    ports,
			ExternalIPs:              config.Get(serviceExternalIPsConfigKey, []string(nil)).([]string),
			LoadBalancerIP:           config.GetString(serviceLoadBalancerIPKey, ""),
			LoadBalancerSourceRanges: config.Get(serviceLoadBalancerSourceRangesKey, []string(nil)).([]string),
			ExternalName:             config.GetString(serviceExternalNameKey, ""),
		},
	}
	_, err = k.ensureK8sService(service)
	return err
}

func (k *kubernetesClient) configureHeadlessService(
	appName, deploymentName string, annotations k8sannotations.Annotation,
) error {
	logger.Debugf("creating/updating headless service for %s", appName)
	service := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   headlessServiceName(deploymentName),
			Labels: utils.LabelsForApp(appName, k.LabelVersion()),
			Annotations: k8sannotations.New(nil).
				Merge(annotations).
				Add("service.alpha.kubernetes.io/tolerate-unready-endpoints", "true").ToMap(),
		},
		Spec: core.ServiceSpec{
			Selector:                 utils.SelectorLabelsForApp(appName, k.LabelVersion()),
			Type:                     core.ServiceTypeClusterIP,
			ClusterIP:                "None",
			PublishNotReadyAddresses: true,
		},
	}
	_, err := k.ensureK8sService(service)
	return err
}

func (k *kubernetesClient) findDefaultIngressClassResource() (*string, error) {
	ics, err := k.listIngressClasses(nil)
	if err != nil {
		return nil, errors.Annotate(err, "finding the default ingress class")
	}
	for _, ic := range ics {
		if k8sannotations.New(ic.GetAnnotations()).Has("ingressclass.kubernetes.io/is-default-class", "true") {
			return &ic.Name, nil
		}
	}
	return nil, errors.NotFoundf("default ingress class")
}

// ExposeService sets up external access to the specified application.
func (k *kubernetesClient) ExposeService(appName string, resourceTags map[string]string, config coreconfig.ConfigAttributes) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	logger.Debugf("creating/updating ingress resource for %s", appName)

	host := config.GetString(caas.JujuExternalHostNameKey, "")
	if host == "" {
		return errors.Errorf("external hostname required")
	}

	ingressSSLRedirect := config.GetBool(ingressSSLRedirectKey, defaultIngressSSLRedirect)
	ingressSSLPassthrough := config.GetBool(ingressSSLPassthroughKey, defaultIngressSSLPassthrough)
	ingressAllowHTTP := config.GetBool(ingressAllowHTTPKey, defaultIngressAllowHTTPKey)
	httpPath := config.GetString(caas.JujuApplicationPath, caas.JujuDefaultApplicationPath)
	if httpPath == "$appname" {
		httpPath = appName
	}
	if !strings.HasPrefix(httpPath, "/") {
		httpPath = "/" + httpPath
	}

	deploymentName := k.deploymentName(appName, true)
	svc, err := k.client().CoreV1().Services(k.namespace).Get(context.TODO(), deploymentName, v1.GetOptions{})
	if err != nil {
		return errors.Trace(err)
	}
	if len(svc.Spec.Ports) == 0 {
		return errors.Errorf("cannot create ingress rule for service %q without a port", svc.Name)
	}
	spec := &networkingv1.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Name:   deploymentName,
			Labels: k8slabels.Merge(resourceTags, k.getIngressLabels(appName)),
			Annotations: map[string]string{
				"ingress.kubernetes.io/rewrite-target":  "",
				"ingress.kubernetes.io/ssl-redirect":    strconv.FormatBool(ingressSSLRedirect),
				"kubernetes.io/ingress.allow-http":      strconv.FormatBool(ingressAllowHTTP),
				"ingress.kubernetes.io/ssl-passthrough": strconv.FormatBool(ingressSSLPassthrough),
			},
		},
		Spec: networkingv1.IngressSpec{},
	}

	ingressClass := config.GetString(ingressClassKey, defaultIngressClass)
	if ingressClass == defaultIngressClass {
		if spec.Spec.IngressClassName, err = k.findDefaultIngressClassResource(); err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
	}
	pathType := networkingv1.PathTypeImplementationSpecific
	if spec.Spec.IngressClassName == nil {
		spec.Annotations["kubernetes.io/ingress.class"] = ingressClass
		pathType = networkingv1.PathTypePrefix
	}
	spec.Spec.Rules = append(spec.Spec.Rules,
		networkingv1.IngressRule{
			Host: host,
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{
						{
							Path:     httpPath,
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: svc.Name,
									Port: networkingv1.ServiceBackendPort{
										Number: int32(svc.Spec.Ports[0].TargetPort.IntValue()),
									},
								},
							},
						},
					},
				},
			},
		},
	)

	// TODO(caas): refactor juju expose to solve potential conflict with ingress definition in podspec.
	// https://bugs.launchpad.net/juju/+bug/1854123
	_, err = k.ensureIngressV1(appName, spec, true)
	return errors.Trace(err)
}

// UnexposeService removes external access to the specified service.
func (k *kubernetesClient) UnexposeService(appName string) error {
	logger.Debugf("deleting ingress resource for %s", appName)
	deploymentName := k.deploymentName(appName, true)
	return errors.Trace(k.deleteIngress(deploymentName, ""))
}

func (k *kubernetesClient) applicationSelector(appName string, mode caas.DeploymentMode) string {
	if mode == caas.ModeOperator {
		return operatorSelector(appName, k.LabelVersion())
	}
	return utils.LabelsToSelector(
		utils.SelectorLabelsForApp(appName, k.LabelVersion())).String()
}

// AnnotateUnit annotates the specified pod (name or uid) with a unit tag.
func (k *kubernetesClient) AnnotateUnit(appName string, mode caas.DeploymentMode, podName string, unit names.UnitTag) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	pods := k.client().CoreV1().Pods(k.namespace)

	pod, err := pods.Get(context.TODO(), podName, v1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return errors.Trace(err)
		}
		pods, err := pods.List(context.TODO(), v1.ListOptions{
			LabelSelector: k.applicationSelector(appName, mode),
		})
		// TODO(caas): remove getting pod by Id (a bit expensive) once we started to store podName in cloudContainer doc.
		if err != nil {
			return errors.Trace(err)
		}
		for _, v := range pods.Items {
			if string(v.GetUID()) == podName {
				p := v
				pod = &p
				break
			}
		}
	}
	if pod == nil {
		return errors.NotFoundf("pod %q", podName)
	}

	unitID := unit.Id()
	if pod.Annotations != nil && pod.Annotations[utils.AnnotationUnitKey(k.LabelVersion())] == unitID {
		return nil
	}

	patch := struct {
		ObjectMeta struct {
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
	}{}
	patch.ObjectMeta.Annotations = map[string]string{
		utils.AnnotationUnitKey(k.LabelVersion()): unitID,
	}
	jsonPatch, err := json.Marshal(patch)
	if err != nil {
		return errors.Trace(err)
	}

	_, err = pods.Patch(context.TODO(), pod.Name, types.MergePatchType, jsonPatch, v1.PatchOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("pod %q", podName)
	}
	return errors.Trace(err)
}

// WatchUnits returns a watcher which notifies when there
// are changes to units of the specified application.
func (k *kubernetesClient) WatchUnits(appName string, mode caas.DeploymentMode) (watcher.NotifyWatcher, error) {
	selector := k.applicationSelector(appName, mode)
	logger.Debugf("selecting units %q to watch", selector)
	factory := informers.NewSharedInformerFactoryWithOptions(k.client(), 0,
		informers.WithNamespace(k.namespace),
		informers.WithTweakListOptions(func(o *v1.ListOptions) {
			o.LabelSelector = selector
		}),
	)
	return k.newWatcher(factory.Core().V1().Pods().Informer(), appName, k.clock)
}

// WatchContainerStart returns a watcher which is notified when a container matching containerName regexp
// is starting/restarting. Each string represents the provider id for the unit the container belongs to.
// If containerName regexp matches empty string, then the first workload container
// is used.
func (k *kubernetesClient) WatchContainerStart(appName string, containerName string) (watcher.StringsWatcher, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	pods := k.client().CoreV1().Pods(k.namespace)
	selector := k.applicationSelector(appName, caas.ModeWorkload)
	logger.Debugf("selecting units %q to watch", selector)
	factory := informers.NewSharedInformerFactoryWithOptions(k.client(), 0,
		informers.WithNamespace(k.namespace),
		informers.WithTweakListOptions(func(o *v1.ListOptions) {
			o.LabelSelector = selector
		}),
	)

	podsList, err := pods.List(context.TODO(), v1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	containerNameRegex, err := regexp.Compile("^" + containerName + "$")
	if err != nil {
		return nil, errors.Trace(err)
	}

	running := func(pod *core.Pod) set.Strings {
		if _, ok := pod.Annotations[utils.AnnotationUnitKey(k.LabelVersion())]; !ok {
			// Ignore pods that aren't annotated as a unit yet.
			return set.Strings{}
		}
		running := set.Strings{}
		for _, cs := range pod.Status.InitContainerStatuses {
			if containerNameRegex.MatchString(cs.Name) {
				if cs.State.Running != nil {
					running.Add(cs.Name)
				}
			}
		}
		for i, cs := range pod.Status.ContainerStatuses {
			useDefault := i == 0 && containerNameRegex.MatchString("")
			if containerNameRegex.MatchString(cs.Name) || useDefault {
				if cs.State.Running != nil {
					running.Add(cs.Name)
				}
			}
		}
		return running
	}

	podInitState := map[string]set.Strings{}
	var initialEvents []string
	for _, pod := range podsList.Items {
		if containers := running(&pod); !containers.IsEmpty() {
			podInitState[string(pod.GetUID())] = containers
			initialEvents = append(initialEvents, providerID(&pod))
		}
	}

	filterEvent := func(evt k8swatcher.WatchEvent, obj interface{}) (string, bool) {
		pod, ok := obj.(*core.Pod)
		if !ok {
			return "", false
		}
		key := string(pod.GetUID())
		if evt == k8swatcher.WatchEventDelete {
			delete(podInitState, key)
			return "", false
		}
		if containers := running(pod); !containers.IsEmpty() {
			if last, ok := podInitState[key]; ok {
				podInitState[key] = containers
				if !containers.Difference(last).IsEmpty() {
					return providerID(pod), true
				}
			} else {
				podInitState[key] = containers
				return providerID(pod), true
			}
		} else {
			delete(podInitState, key)
		}
		return "", false
	}

	return k.newStringsWatcher(factory.Core().V1().Pods().Informer(),
		appName, k.clock, initialEvents, filterEvent)
}

// WatchService returns a watcher which notifies when there
// are changes to the deployment of the specified application.
func (k *kubernetesClient) WatchService(appName string, mode caas.DeploymentMode) (watcher.NotifyWatcher, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	// Application may be a statefulset or deployment. It may not have
	// been set up when the watcher is started so we don't know which it
	// is ahead of time. So use a multi-watcher to cover both cases.
	factory := informers.NewSharedInformerFactoryWithOptions(k.client(), 0,
		informers.WithNamespace(k.namespace),
		informers.WithTweakListOptions(func(o *v1.ListOptions) {
			o.LabelSelector = k.applicationSelector(appName, mode)
		}),
	)

	w1, err := k.newWatcher(factory.Apps().V1().StatefulSets().Informer(), appName, k.clock)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w2, err := k.newWatcher(factory.Apps().V1().Deployments().Informer(), appName, k.clock)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w3, err := k.newWatcher(factory.Core().V1().Services().Informer(), appName, k.clock)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return watcher.NewMultiNotifyWatcher(w1, w2, w3), nil
}

// CheckCloudCredentials verifies the the cloud credentials provided to the
// broker are functioning.
func (k *kubernetesClient) CheckCloudCredentials() error {
	if _, err := k.Namespaces(); err != nil {
		// If this call could not be made with provided credential, we
		// know that the credential is invalid.
		return errors.Trace(err)
	}
	return nil
}

// Units returns all units and any associated filesystems of the specified application.
// Filesystems are mounted via volumes bound to the unit.
func (k *kubernetesClient) Units(appName string, mode caas.DeploymentMode) ([]caas.Unit, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	pods := k.client().CoreV1().Pods(k.namespace)
	podsList, err := pods.List(context.TODO(), v1.ListOptions{
		LabelSelector: k.applicationSelector(appName, mode),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var units []caas.Unit
	now := k.clock.Now()
	for _, p := range podsList.Items {
		var ports []string
		for _, c := range p.Spec.Containers {
			for _, p := range c.Ports {
				ports = append(ports, fmt.Sprintf("%v/%v", p.ContainerPort, p.Protocol))
			}
		}

		eventGetter := func() ([]core.Event, error) {
			return k.getEvents(p.Name, "Pod")
		}

		terminated := p.DeletionTimestamp != nil
		statusMessage, unitStatus, since, err := resources.PodToJujuStatus(p, now, eventGetter)

		if err != nil {
			return nil, errors.Trace(err)
		}
		unitInfo := caas.Unit{
			Id:       providerID(&p),
			Address:  p.Status.PodIP,
			Ports:    ports,
			Dying:    terminated,
			Stateful: isStateful(&p),
			Status: status.StatusInfo{
				Status:  unitStatus,
				Message: statusMessage,
				Since:   &since,
			},
		}

		volumesByName := make(map[string]core.Volume)
		for _, pv := range p.Spec.Volumes {
			volumesByName[pv.Name] = pv
		}

		// Gather info about how filesystems are attached/mounted to the pod.
		// The mount name represents the filesystem tag name used by Juju.
		for _, volMount := range p.Spec.Containers[0].VolumeMounts {
			vol, ok := volumesByName[volMount.Name]
			if !ok {
				logger.Warningf("volume for volume mount %q not found", volMount.Name)
				continue
			}
			var fsInfo *caas.FilesystemInfo
			if vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.ClaimName != "" {
				fsInfo, err = k.volumeInfoForPVC(vol, volMount, vol.PersistentVolumeClaim.ClaimName, now)
			} else if vol.EmptyDir != nil {
				fsInfo, err = k.volumeInfoForEmptyDir(vol, volMount, now)
			} else {
				// Ignore volumes which are not Juju managed filesystems.
				logger.Debugf("ignoring blank EmptyDir, PersistentVolumeClaim or ClaimName")
				continue
			}
			if err != nil {
				return nil, errors.Annotatef(err, "finding filesystem info for %v", volMount.Name)
			}
			if fsInfo == nil {
				continue
			}
			if fsInfo.StorageName == "" {
				if valid := constants.LegacyPVNameRegexp.MatchString(volMount.Name); valid {
					fsInfo.StorageName = constants.LegacyPVNameRegexp.ReplaceAllString(volMount.Name, "$storageName")
				} else if valid := constants.PVNameRegexp.MatchString(volMount.Name); valid {
					fsInfo.StorageName = constants.PVNameRegexp.ReplaceAllString(volMount.Name, "$storageName")
				}
			}
			logger.Debugf("filesystem info for %v: %+v", volMount.Name, *fsInfo)
			unitInfo.FilesystemInfo = append(unitInfo.FilesystemInfo, *fsInfo)
		}
		units = append(units, unitInfo)
	}
	return units, nil
}

// ListPods filters a list of pods for the provided namespace and labels.
func (k *kubernetesClient) ListPods(namespace string, selector k8slabels.Selector) ([]core.Pod, error) {
	listOps := v1.ListOptions{
		LabelSelector: selector.String(),
	}
	list, err := k.client().CoreV1().Pods(namespace).List(context.TODO(), listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(list.Items) == 0 {
		return nil, errors.NotFoundf("pods with selector %q", selector)
	}
	return list.Items, nil
}

func (k *kubernetesClient) getPod(podName string) (*core.Pod, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	pods := k.client().CoreV1().Pods(k.namespace)
	pod, err := pods.Get(context.TODO(), podName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("pod %q", podName)
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return pod, nil
}

func (k *kubernetesClient) getStatefulSetStatus(ss *apps.StatefulSet) (string, status.Status, error) {
	terminated := ss.DeletionTimestamp != nil
	jujuStatus := status.Waiting
	if terminated {
		jujuStatus = status.Terminated
	}
	if ss.Status.ReadyReplicas == ss.Status.Replicas {
		jujuStatus = status.Active
	}
	return k.getStatusFromEvents(ss.Name, "StatefulSet", jujuStatus)
}

func (k *kubernetesClient) getDeploymentStatus(deployment *apps.Deployment) (string, status.Status, error) {
	terminated := deployment.DeletionTimestamp != nil
	jujuStatus := status.Waiting
	if terminated {
		jujuStatus = status.Terminated
	}
	if deployment.Status.ReadyReplicas == deployment.Status.Replicas {
		jujuStatus = status.Active
	}
	return k.getStatusFromEvents(deployment.Name, "Deployment", jujuStatus)
}

func (k *kubernetesClient) getDaemonSetStatus(ds *apps.DaemonSet) (string, status.Status, error) {
	terminated := ds.DeletionTimestamp != nil
	jujuStatus := status.Waiting
	if terminated {
		jujuStatus = status.Terminated
	}
	if ds.Status.NumberReady == ds.Status.DesiredNumberScheduled {
		jujuStatus = status.Active
	}
	return k.getStatusFromEvents(ds.Name, "DaemonSet", jujuStatus)
}

func (k *kubernetesClient) getStatusFromEvents(name, kind string, jujuStatus status.Status) (string, status.Status, error) {
	events, err := k.getEvents(name, kind)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	var statusMessage string
	// Take the most recent event.
	if count := len(events); count > 0 {
		evt := events[count-1]
		if jujuStatus == "" {
			if evt.Type == core.EventTypeWarning && evt.Reason == "FailedCreate" {
				jujuStatus = status.Error
				statusMessage = evt.Message
			}
		}
	}
	return statusMessage, jujuStatus, nil
}

// filesetConfigMap returns a *core.ConfigMap for a pod
// of the specified unit, with the specified files.
func filesetConfigMap(configMapName string, labels, annotations map[string]string, files *specs.FileSet) *core.ConfigMap {
	result := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:        configMapName,
			Labels:      labels,
			Annotations: annotations,
		},
		Data: map[string]string{},
	}
	for _, f := range files.Files {
		result.Data[f.Path] = f.Content
	}
	return result
}

// workloadSpec represents the k8s resources need to be created for the workload.
type workloadSpec struct {
	Pod     k8sspecs.PodSpecWithAnnotations
	Service *specs.ServiceSpec

	Secrets                         []k8sspecs.K8sSecret
	Services                        []k8sspecs.K8sService
	ConfigMaps                      map[string]specs.ConfigMap
	ServiceAccounts                 []k8sspecs.K8sRBACSpecConverter
	CustomResourceDefinitions       []k8sspecs.K8sCustomResourceDefinition
	CustomResources                 map[string][]unstructured.Unstructured
	MutatingWebhookConfigurations   []k8sspecs.K8sMutatingWebhook
	ValidatingWebhookConfigurations []k8sspecs.K8sValidatingWebhook
	IngressResources                []k8sspecs.K8sIngress
}

func processContainers(deploymentName string, podSpec *specs.PodSpec, spec *core.PodSpec) error {

	type containers struct {
		Containers     []specs.ContainerSpec
		InitContainers []specs.ContainerSpec
	}

	var cs containers
	for _, c := range podSpec.Containers {
		if c.Init {
			cs.InitContainers = append(cs.InitContainers, c)
		} else {
			cs.Containers = append(cs.Containers, c)
		}
	}

	// Fill out the easy bits using a template.
	var buf bytes.Buffer
	if err := defaultPodTemplate.Execute(&buf, cs); err != nil {
		logger.Debugf("unable to execute template for containers: %+v, err: %+v", cs, err)
		return errors.Trace(err)
	}

	workloadSpecString := buf.String()
	decoder := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(workloadSpecString), len(workloadSpecString))
	if err := decoder.Decode(&spec); err != nil {
		logger.Debugf("unable to parse pod spec, unit spec: \n%v", workloadSpecString)
		return errors.Trace(err)
	}

	// Now fill in the hard bits progamatically.
	if err := populateContainerDetails(deploymentName, spec, spec.Containers, cs.Containers); err != nil {
		return errors.Trace(err)
	}
	if err := populateContainerDetails(deploymentName, spec, spec.InitContainers, cs.InitContainers); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func prepareWorkloadSpec(
	appName, deploymentName string, podSpec *specs.PodSpec, imageDetails coreresources.DockerImageDetails,
) (*workloadSpec, error) {
	var spec workloadSpec
	if err := processContainers(deploymentName, podSpec, &spec.Pod.PodSpec); err != nil {
		logger.Errorf("unable to parse %q pod spec: \n%+v", appName, *podSpec)
		return nil, errors.Annotatef(err, "processing container specs for app %q", appName)
	}
	if err := ensureJujuInitContainer(&spec.Pod.PodSpec, imageDetails.RegistryPath); err != nil {
		return nil, errors.Annotatef(err, "adding init container for app %q", appName)
	}
	if imageDetails.IsPrivate() {
		spec.Pod.PodSpec.ImagePullSecrets = append(
			spec.Pod.PodSpec.ImagePullSecrets,
			core.LocalObjectReference{Name: constants.CAASImageRepoSecretName},
		)
	}

	spec.Service = podSpec.Service
	spec.ConfigMaps = podSpec.ConfigMaps
	if podSpec.ServiceAccount != nil {
		// Use application name for the prime service account name.
		podSpec.ServiceAccount.SetName(appName)
		primeSA, err := k8sspecs.PrimeServiceAccountToK8sRBACResources(*podSpec.ServiceAccount)
		if err != nil {
			return nil, errors.Annotatef(err, "converting prime service account for app %q", appName)
		}
		spec.ServiceAccounts = append(spec.ServiceAccounts, primeSA)

		spec.Pod.ServiceAccountName = podSpec.ServiceAccount.GetName()
		spec.Pod.AutomountServiceAccountToken = podSpec.ServiceAccount.AutomountServiceAccountToken
	}
	if podSpec.ProviderPod != nil {
		pSpec, ok := podSpec.ProviderPod.(*k8sspecs.K8sPodSpec)
		if !ok {
			return nil, errors.Errorf("unexpected kubernetes pod spec type %T", podSpec.ProviderPod)
		}

		k8sResources := pSpec.KubernetesResources
		if k8sResources != nil {
			spec.Secrets = k8sResources.Secrets
			spec.Services = k8sResources.Services
			spec.CustomResourceDefinitions = k8sResources.CustomResourceDefinitions
			spec.CustomResources = k8sResources.CustomResources
			spec.MutatingWebhookConfigurations = k8sResources.MutatingWebhookConfigurations
			spec.ValidatingWebhookConfigurations = k8sResources.ValidatingWebhookConfigurations
			spec.IngressResources = k8sResources.IngressResources
			if k8sResources.Pod != nil {
				spec.Pod.Labels = utils.LabelsMerge(nil, k8sResources.Pod.Labels)
				spec.Pod.Annotations = k8sResources.Pod.Annotations.Copy()
				spec.Pod.RestartPolicy = k8sResources.Pod.RestartPolicy
				spec.Pod.ActiveDeadlineSeconds = k8sResources.Pod.ActiveDeadlineSeconds
				spec.Pod.TerminationGracePeriodSeconds = k8sResources.Pod.TerminationGracePeriodSeconds
				spec.Pod.SecurityContext = k8sResources.Pod.SecurityContext
				spec.Pod.ReadinessGates = k8sResources.Pod.ReadinessGates
				spec.Pod.DNSPolicy = k8sResources.Pod.DNSPolicy
				spec.Pod.HostNetwork = k8sResources.Pod.HostNetwork
				spec.Pod.HostPID = k8sResources.Pod.HostPID
				spec.Pod.PriorityClassName = k8sResources.Pod.PriorityClassName
				spec.Pod.Priority = k8sResources.Pod.Priority
			}
			spec.ServiceAccounts = append(spec.ServiceAccounts, &k8sResources.K8sRBACResources)
		}
	}
	return &spec, nil
}

func boolPtr(b bool) *bool {
	return &b
}

func defaultSecurityContext() *core.SecurityContext {
	// TODO(caas): consider locking this down more but charms will break
	return &core.SecurityContext{
		AllowPrivilegeEscalation: boolPtr(true), // allow privilege for juju run and actions.
		ReadOnlyRootFilesystem:   boolPtr(false),
		RunAsNonRoot:             boolPtr(false),
	}
}

func populateContainerDetails(deploymentName string, pod *core.PodSpec, podContainers []core.Container, containers []specs.ContainerSpec) (err error) {
	for i, c := range containers {
		pc := &podContainers[i]
		if c.Image != "" {
			logger.Warningf("Image parameter deprecated, use ImageDetails")
			pc.Image = c.Image
		} else {
			pc.Image = c.ImageDetails.ImagePath
		}
		if c.ImageDetails.Password != "" {
			pod.ImagePullSecrets = append(pod.ImagePullSecrets, core.LocalObjectReference{Name: appSecretName(deploymentName, c.Name)})
		}
		if c.ImagePullPolicy != "" {
			pc.ImagePullPolicy = core.PullPolicy(c.ImagePullPolicy)
		}

		if pc.Env, pc.EnvFrom, err = k8sspecs.ContainerConfigToK8sEnvConfig(c.EnvConfig); err != nil {
			return errors.Trace(err)
		}

		pc.SecurityContext = defaultSecurityContext()
		if c.ProviderContainer == nil {
			continue
		}
		spec, ok := c.ProviderContainer.(*k8sspecs.K8sContainerSpec)
		if !ok {
			return errors.Errorf("unexpected kubernetes container spec type %T", c.ProviderContainer)
		}
		if spec.LivenessProbe != nil {
			pc.LivenessProbe = spec.LivenessProbe
		}
		if spec.ReadinessProbe != nil {
			pc.ReadinessProbe = spec.ReadinessProbe
		}
		if spec.StartupProbe != nil {
			pc.StartupProbe = spec.StartupProbe
		}
		if spec.SecurityContext != nil {
			pc.SecurityContext = spec.SecurityContext
		}
	}
	return nil
}

// legacyAppName returns true if there are any artifacts for
// appName which indicate that this deployment was for Juju 2.5.0.
func (k *kubernetesClient) legacyAppName(appName string) bool {
	legacyName := "juju-operator-" + appName
	_, err := k.getStatefulSet(legacyName)
	return err == nil
}

func (k *kubernetesClient) operatorName(appName string) string {
	if k.legacyAppName(appName) {
		return "juju-operator-" + appName
	}
	return appName + "-operator"
}

func (k *kubernetesClient) deploymentName(appName string, legacySupport bool) string {
	if !legacySupport {
		// No need to check old operator statefulset for brand new features like raw k8s spec.
		return appName
	}
	if k.legacyAppName(appName) {
		return "juju-" + appName
	}
	return appName
}

// SupportedFeatures implements environs.SupportedFeatureEnumerator.
func (k *kubernetesClient) SupportedFeatures() (assumes.FeatureSet, error) {
	var fs assumes.FeatureSet

	k8sAPIVersion, err := k.Version()
	if err != nil {
		return fs, errors.Annotatef(err, "querying kubernetes API version")
	}

	fs.Add(assumes.K8sAPIFeature(*k8sAPIVersion))
	return fs, nil
}

func isLegacyName(resourceName string) bool {
	return strings.HasPrefix(resourceName, "juju-")
}

func operatorConfigMapName(operatorName string) string {
	return operatorName + "-config"
}

func applicationConfigMapName(deploymentName, fileSetName string) string {
	return fmt.Sprintf("%v-%v-config", deploymentName, fileSetName)
}

func appSecretName(deploymentName, containerName string) string {
	// A pod may have multiple containers with different images and thus different secrets
	return deploymentName + "-" + containerName + "-secret"
}

func mergeDeviceConstraints(device devices.KubernetesDeviceParams, resources *core.ResourceRequirements) error {
	if resources.Limits == nil {
		resources.Limits = core.ResourceList{}
	}
	if resources.Requests == nil {
		resources.Requests = core.ResourceList{}
	}

	resourceName := core.ResourceName(device.Type)
	if v, ok := resources.Limits[resourceName]; ok {
		return errors.NotValidf("resource limit for %q has already been set to %v! resource limit %q", resourceName, v, resourceName)
	}
	if v, ok := resources.Requests[resourceName]; ok {
		return errors.NotValidf("resource request for %q has already been set to %v! resource limit %q", resourceName, v, resourceName)
	}
	// GPU request/limit have to be set to same value equals to the Count.
	// - https://kubernetes.io/docs/tasks/manage-gpus/scheduling-gpus/#clusters-containing-different-types-of-nvidia-gpus
	resources.Limits[resourceName] = *resource.NewQuantity(device.Count, resource.DecimalSI)
	resources.Requests[resourceName] = *resource.NewQuantity(device.Count, resource.DecimalSI)
	return nil
}

func buildNodeSelector(nodeLabel string) map[string]string {
	// TODO(caas): to support GKE, set it to `cloud.google.com/gke-accelerator`,
	// current only set to generic `accelerator` because we do not have k8s provider concept yet.
	key := "accelerator"
	return map[string]string{key: nodeLabel}
}

func getNodeSelectorFromDeviceConstraints(devices []devices.KubernetesDeviceParams) (string, error) {
	var nodeSelector string
	for _, device := range devices {
		if device.Attributes == nil {
			continue
		}
		if label, ok := device.Attributes[gpuAffinityNodeSelectorKey]; ok {
			if nodeSelector != "" && nodeSelector != label {
				return "", errors.NotValidf(
					"node affinity labels have to be same for all device constraints in same pod - containers in same pod are scheduled in same node.")
			}
			nodeSelector = label
		}
	}
	return nodeSelector, nil
}

func headlessServiceName(deploymentName string) string {
	return fmt.Sprintf("%s-endpoints", deploymentName)
}

func providerID(pod *core.Pod) string {
	// Pods managed by a stateful set use the pod name
	// as the provider id as this is stable across pod restarts.
	if isStateful(pod) {
		return pod.Name
	}
	return string(pod.GetUID())
}

func isStateful(pod *core.Pod) bool {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "StatefulSet" {
			return true
		}
	}
	return false
}
