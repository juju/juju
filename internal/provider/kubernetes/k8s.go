// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/docker"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
	k8swatcher "github.com/juju/juju/internal/provider/kubernetes/watcher"
)

var logger = internallogger.GetLogger("juju.kubernetes.provider")

const (
	// labelResourceLifeCycleKey defines the label key for lifecycle of the global resources.
	labelResourceLifeCycleKey             = "juju-resource-lifecycle"
	labelResourceLifeCycleValueModel      = "model"
	labelResourceLifeCycleValuePersistent = "persistent"

	operatorInitContainerName = "juju-init"
	operatorContainerName     = "juju-operator"

	// InformerResyncPeriod is the default resync period set on IndexInformers
	InformerResyncPeriod = time.Minute * 5
)

// NewK8sRestClientFunc defines a function which returns a k8s rest client based on the supplied config.
type NewK8sRestClientFunc func(c *rest.Config) (rest.Interface, error)

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
	newRestClient NewK8sRestClientFunc

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

	environNetworking
}

// To regenerate the mocks for the kubernetes Client used by this broker,
// run "go generate" from the package directory.
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/k8sclient_mock.go k8s.io/client-go/kubernetes Interface
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/context_mock.go context Context
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/appv1_mock.go k8s.io/client-go/kubernetes/typed/apps/v1 AppsV1Interface,DeploymentInterface,StatefulSetInterface,DaemonSetInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/corev1_mock.go k8s.io/client-go/kubernetes/typed/core/v1 EventInterface,CoreV1Interface,NamespaceInterface,PodInterface,ServiceInterface,ConfigMapInterface,PersistentVolumeInterface,PersistentVolumeClaimInterface,SecretInterface,NodeInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/networkingv1beta1_mock.go -mock_names=IngressInterface=MockIngressV1Beta1Interface k8s.io/client-go/kubernetes/typed/networking/v1beta1 NetworkingV1beta1Interface,IngressInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/networkingv1_mock.go -mock_names=IngressInterface=MockIngressV1Interface k8s.io/client-go/kubernetes/typed/networking/v1 NetworkingV1Interface,IngressInterface,IngressClassInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/storagev1_mock.go k8s.io/client-go/kubernetes/typed/storage/v1 StorageV1Interface,StorageClassInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/rbacv1_mock.go k8s.io/client-go/kubernetes/typed/rbac/v1 RbacV1Interface,ClusterRoleBindingInterface,ClusterRoleInterface,RoleInterface,RoleBindingInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/apiextensionsv1beta1_mock.go -mock_names=CustomResourceDefinitionInterface=MockCustomResourceDefinitionV1Beta1Interface k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1 ApiextensionsV1beta1Interface,CustomResourceDefinitionInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/apiextensionsv1_mock.go -mock_names=CustomResourceDefinitionInterface=MockCustomResourceDefinitionV1Interface k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1 ApiextensionsV1Interface,CustomResourceDefinitionInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/apiextensionsclientset_mock.go -mock_names=Interface=MockApiExtensionsClientInterface k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset Interface
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/discovery_mock.go k8s.io/client-go/discovery DiscoveryInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/dynamic_mock.go -mock_names=Interface=MockDynamicInterface k8s.io/client-go/dynamic Interface,ResourceInterface,NamespaceableResourceInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/admissionregistrationv1beta1_mock.go -mock_names=MutatingWebhookConfigurationInterface=MockMutatingWebhookConfigurationV1Beta1Interface,ValidatingWebhookConfigurationInterface=MockValidatingWebhookConfigurationV1Beta1Interface k8s.io/client-go/kubernetes/typed/admissionregistration/v1beta1  AdmissionregistrationV1beta1Interface,MutatingWebhookConfigurationInterface,ValidatingWebhookConfigurationInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/admissionregistrationv1_mock.go -mock_names=MutatingWebhookConfigurationInterface=MockMutatingWebhookConfigurationV1Interface,ValidatingWebhookConfigurationInterface=MockValidatingWebhookConfigurationV1Interface k8s.io/client-go/kubernetes/typed/admissionregistration/v1  AdmissionregistrationV1Interface,MutatingWebhookConfigurationInterface,ValidatingWebhookConfigurationInterface
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/serviceaccountinformer_mock.go k8s.io/client-go/informers/core/v1 ServiceAccountInformer
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/serviceaccountlister_mock.go k8s.io/client-go/listers/core/v1 ServiceAccountLister,ServiceAccountNamespaceLister
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/sharedindexinformer_mock.go k8s.io/client-go/tools/cache SharedIndexInformer
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/restclient_mock.go -mock_names=Interface=MockRestClientInterface k8s.io/client-go/rest Interface
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/serviceaccount_mock.go k8s.io/client-go/kubernetes/typed/core/v1 ServiceAccountInterface

// NewK8sClientFunc defines a function which returns a k8s client based on the supplied config.
type NewK8sClientFunc func(c *rest.Config) (kubernetes.Interface, apiextensionsclientset.Interface, dynamic.Interface, error)

// newK8sBroker returns a kubernetes client for the specified k8s cluster.
func newK8sBroker(
	ctx context.Context,
	controllerUUID string,
	k8sRestConfig *rest.Config,
	cfg *config.Config,
	namespace string,
	newClient NewK8sClientFunc,
	newRestClient NewK8sRestClientFunc,
	newWatcher k8swatcher.NewK8sWatcherFunc,
	newStringsWatcher k8swatcher.NewK8sStringsWatcherFunc,
	randomPrefix utils.RandomPrefixFunc,
	clock jujuclock.Clock,
) (*kubernetesClient, error) {
	k8sClient, apiextensionsClient, dynamicClient, err := newClient(k8sRestConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	newCfg, err := providerInstance.newConfig(ctx, cfg)
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
			ctx, namespace, modelName, modelUUID, controllerUUID, k8sClient.CoreV1().Namespaces())
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
		labelVersion:      labelVersion,
		environNetworking: environNetworking{},
	}
	if len(controllerUUID) > 0 {
		client.annotations.Add(utils.AnnotationControllerUUIDKey(labelVersion), controllerUUID)
	}
	if namespace == "" {
		return client, nil
	}

	ns, err := client.getNamespaceByName(ctx, namespace)
	if errors.Is(err, errors.NotFound) {
		return client, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	if !isK8sObjectOwnedByJuju(ns.ObjectMeta) {
		return client, nil
	}

	if err := client.ensureNamespaceAnnotationForControllerUUID(ctx, ns, controllerUUID, labelVersion); err != nil {
		return nil, errors.Trace(err)
	}
	return client, nil
}

func (k *kubernetesClient) ensureNamespaceAnnotationForControllerUUID(
	ctx context.Context,
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
	logger.Debugf(ctx, "model %q was migrated from controller %q, updating the controller annotation to %q", k.namespace,
		ns.Annotations[annotationControllerUUIDKey], controllerUUID,
	)
	if err := k.ensureNamespaceAnnotations(ns); err != nil {
		return errors.Trace(err)
	}
	_, err := k.client().CoreV1().Namespaces().Update(ctx, ns, v1.UpdateOptions{})
	return errors.Trace(err)
}

// GetAnnotations returns current namespace's annotations.
func (k *kubernetesClient) GetAnnotations() k8sannotations.Annotation {
	return k.annotations
}

var k8sversionNumberExtractor = regexp.MustCompile("[0-9]+")

// Version returns cluster version information.
func (k *kubernetesClient) Version() (ver *semversion.Number, err error) {
	k8sver, err := k.client().Discovery().ServerVersion()
	if err != nil {
		return nil, errors.Trace(err)
	}

	clean := func(s string) string {
		return k8sversionNumberExtractor.FindString(s)
	}

	ver = &semversion.Number{}
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

// SetConfig is specified in the Environ interface.
func (k *kubernetesClient) SetConfig(ctx context.Context, cfg *config.Config) error {
	k.lock.Lock()
	defer k.lock.Unlock()
	newCfg, err := providerInstance.newConfig(ctx, cfg)
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

	k8sRestConfig, err := k8s.CloudSpecToK8sRestConfig(spec)
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
	_, err := k.getNamespaceByName(ctx, k.namespace)
	if err == nil {
		return alreadyExistErr
	}
	if !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	}
	// Good, no existing namespace has the same name.
	// Now, try to find if there is any existing controller running in this cluster.
	// Note: we have to do this check before we are confident to support multi controllers running in same k8s cluster.

	_, err = k.listNamespacesByAnnotations(ctx, k.annotations)
	if err == nil {
		return alreadyExistErr
	}
	if !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	}
	// All good, no existing controller found on the cluster.
	// The namespace will be set to controller-name in newcontrollerStack.

	// do validation on storage class.
	_, err = k.validateControllerWorkloadStorage(ctx)
	return errors.Trace(err)
}

// ValidateProviderForNewModel is part of the [environs.ModelResources] interface.
func (k *kubernetesClient) ValidateProviderForNewModel(ctx context.Context) error {
	_, err := k.client().CoreV1().Namespaces().Get(ctx, k.namespace, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err == nil {
		return errors.NewAlreadyExists(
			nil, fmt.Sprintf("validating model: namespace %q already exists", k.namespace))
	}
	return errors.Trace(err)
}

// CreateModelResources is part of the [environs.ModelResources] interface.
func (k *kubernetesClient) CreateModelResources(ctx context.Context, args environs.CreateParams) error {
	return errors.Trace(k.createNamespace(ctx, k.namespace))
}

// EnsureImageRepoSecret ensures the image pull secret gets created.
func (k *kubernetesClient) EnsureImageRepoSecret(ctx context.Context, imageRepo docker.ImageRepoDetails) error {
	if !imageRepo.IsPrivate() {
		return nil
	}
	logger.Debugf(ctx, "creating secret for image repo %q", imageRepo.Repository)
	secretData, err := imageRepo.SecretData()
	if err != nil {
		return errors.Trace(err)
	}
	_, err = k.ensureOCIImageSecret(
		ctx,
		constants.CAASImageRepoSecretName,
		utils.LabelsJuju, secretData,
		k.annotations,
	)
	return errors.Trace(err)
}

func (k *kubernetesClient) validateControllerWorkloadStorage(ctx context.Context) (string, error) {
	storageClass, _ := k.Config().AllAttrs()[constants.WorkloadStorageKey].(string)
	if storageClass == "" {
		return "", errors.NewNotValid(nil, "config without workload-storage value not valid.\nRun juju add-k8s to reimport your k8s cluster.")
	}
	_, err := k.getStorageClass(ctx, storageClass)
	return storageClass, errors.Trace(err)
}

// Bootstrap deploys a controller into k8s cluster.
func (k *kubernetesClient) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {

	if !args.BootstrapBase.Empty() {
		return nil, errors.NotSupportedf("set base for bootstrapping to kubernetes")
	}

	// Validate workload storage is available to the controller.
	storageClass, err := k.validateControllerWorkloadStorage(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	finalizer := func(ctx environs.BootstrapContext, pcfg *podcfg.ControllerPodConfig, opts environs.BootstrapDialOpts) (err error) {
		podcfg.FinishControllerPodConfig(pcfg, k.Config(), args.ExtraAgentValuesForTesting)
		if err = pcfg.VerifyConfig(); err != nil {
			return errors.Trace(err)
		}

		logger.Debugf(ctx, "controller pod config: \n%+v", pcfg)

		// we use controller name to name controller namespace in bootstrap time.
		setControllerNamespace := func(controllerName string, broker *kubernetesClient) error {
			nsName := DecideControllerNamespace(controllerName)

			_, err := broker.GetNamespace(ctx, nsName)
			if errors.Is(err, errors.NotFound) {
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
		controllerStack, err := newControllerStack(ctx, constants.JujuControllerStackName, storageClass, k, pcfg)
		if err != nil {
			return errors.Trace(err)
		}
		return errors.Annotate(
			controllerStack.Deploy(ctx),
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
func (k *kubernetesClient) DestroyController(ctx context.Context, controllerUUID string) error {
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
func (k *kubernetesClient) Destroy(ctx context.Context) (err error) {
	defer func() {
		if errors.Cause(err) == context.DeadlineExceeded {
			logger.Warningf(ctx, "destroy k8s model timeout")
			return
		}
		if err != nil && k8serrors.ReasonForError(err) == v1.StatusReasonUnknown {
			logger.Warningf(ctx, "k8s cluster is not accessible: %v", err)
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
func (k *kubernetesClient) getStorageClass(ctx context.Context, name string) (*storagev1.StorageClass, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	storageClasses := k.client().StorageV1().StorageClasses()
	qualifiedName := constants.QualifiedStorageClassName(k.namespace, name)
	sc, err := storageClasses.Get(ctx, qualifiedName, v1.GetOptions{})
	if err == nil {
		return sc, nil
	}
	if !k8serrors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	return storageClasses.Get(ctx, name, v1.GetOptions{})
}

// GetService returns the service for the specified application.
func (k *kubernetesClient) GetService(ctx context.Context, appName string, includeClusterIP bool) (*caas.Service, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	services := k.client().CoreV1().Services(k.namespace)
	labels := utils.LabelsForApp(appName, k.LabelVersion())
	if k.LabelVersion() != constants.LegacyLabelVersion {
		labels = utils.LabelsMerge(labels, utils.LabelsJuju)
	}

	servicesList, err := services.List(ctx, v1.ListOptions{
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

	deploymentName := k.deploymentName(ctx, appName, true)
	statefulsets := k.client().AppsV1().StatefulSets(k.namespace)
	ss, err := statefulsets.Get(ctx, deploymentName, v1.GetOptions{})
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
		message, ssStatus, err := k.getStatefulSetStatus(ctx, ss)
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
	deployment, err := deployments.Get(ctx, deploymentName, v1.GetOptions{})
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
		message, deployStatus, err := k.getDeploymentStatus(ctx, deployment)
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
	ds, err := daemonsets.Get(ctx, deploymentName, v1.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if err == nil {
		// The total number of nodes that should be running the daemon pod (including nodes correctly running the daemon pod).
		scale := int(ds.Status.DesiredNumberScheduled)
		result.Scale = &scale

		gen := ds.GetGeneration()
		result.Generation = &gen
		message, dsStatus, err := k.getDaemonSetStatus(ctx, ds)
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

func (k *kubernetesClient) ensureDeployment(ctx context.Context, spec *apps.Deployment) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	deployments := k.client().AppsV1().Deployments(k.namespace)
	_, err := k.createDeployment(ctx, spec)
	if err == nil || !errors.Is(err, errors.AlreadyExists) {
		return errors.Annotatef(err, "ensuring deployment %q", spec.GetName())
	}
	existing, err := k.getDeployment(ctx, spec.GetName())
	if err != nil {
		return errors.Trace(err)
	}
	existing.SetAnnotations(spec.GetAnnotations())
	existing.Spec = spec.Spec
	_, err = deployments.Update(ctx, existing, v1.UpdateOptions{})
	if err != nil {
		return errors.Annotatef(err, "ensuring deployment %q", spec.GetName())
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) createDeployment(ctx context.Context, spec *apps.Deployment) (*apps.Deployment, error) {
	out, err := k.client().AppsV1().Deployments(k.namespace).Create(ctx, spec, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("deployment %q", spec.GetName())
	}
	if k8serrors.IsInvalid(err) {
		return nil, errors.NotValidf("deployment %q", spec.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getDeployment(ctx context.Context, name string) (*apps.Deployment, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().AppsV1().Deployments(k.namespace).Get(ctx, name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("deployment %q", name)
	}
	return out, errors.Trace(err)
}

// CaasServiceToK8s translates a caas service type to a k8s one.
func CaasServiceToK8s(in caas.ServiceType) (core.ServiceType, error) {
	serviceType := core.ServiceTypeClusterIP
	if in != "" {
		switch in {
		case caas.ServiceCluster:
			serviceType = core.ServiceTypeClusterIP
		case caas.ServiceLoadBalancer:
			serviceType = core.ServiceTypeLoadBalancer
		case caas.ServiceExternal:
			serviceType = core.ServiceTypeExternalName
		case caas.ServiceOmit:
			logger.Debugf(context.TODO(), "no service to be created because service type is %q", in)
			return "", nil
		default:
			return "", errors.NotSupportedf("service type %q", in)
		}
	}
	return serviceType, nil
}

func (k *kubernetesClient) applicationSelector(appName string) string {
	return utils.LabelsToSelector(
		utils.SelectorLabelsForApp(appName, k.LabelVersion())).String()
}

// AnnotateUnit annotates the specified pod (name or uid) with a unit tag.
func (k *kubernetesClient) AnnotateUnit(ctx context.Context, appName string, podName string, unit names.UnitTag) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	pods := k.client().CoreV1().Pods(k.namespace)

	pod, err := pods.Get(ctx, podName, v1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return errors.Trace(err)
		}
		pods, err := pods.List(ctx, v1.ListOptions{
			LabelSelector: k.applicationSelector(appName),
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

	_, err = pods.Patch(ctx, pod.Name, types.MergePatchType, jsonPatch, v1.PatchOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("pod %q", podName)
	}
	return errors.Trace(err)
}

// WatchUnits returns a watcher which notifies when there
// are changes to units of the specified application.
func (k *kubernetesClient) WatchUnits(appName string) (watcher.NotifyWatcher, error) {
	selector := k.applicationSelector(appName)
	logger.Debugf(context.TODO(), "selecting units %q to watch", selector)
	factory := informers.NewSharedInformerFactoryWithOptions(k.client(), 0,
		informers.WithNamespace(k.namespace),
		informers.WithTweakListOptions(func(o *v1.ListOptions) {
			o.LabelSelector = selector
		}),
	)
	return k.newWatcher(factory.Core().V1().Pods().Informer(), appName, k.clock)
}

// CheckCloudCredentials verifies the the cloud credentials provided to the
// broker are functioning.
func (k *kubernetesClient) CheckCloudCredentials(ctx context.Context) error {
	if _, err := k.Namespaces(ctx); err != nil {
		// If this call could not be made with provided credential, we
		// know that the credential is invalid.
		return errors.Trace(err)
	}
	return nil
}

// Units returns all units and any associated filesystems of the specified application.
// Filesystems are mounted via volumes bound to the unit.
func (k *kubernetesClient) Units(ctx context.Context, appName string) ([]caas.Unit, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	pods := k.client().CoreV1().Pods(k.namespace)
	podsList, err := pods.List(ctx, v1.ListOptions{
		LabelSelector: k.applicationSelector(appName),
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
			return k.getEvents(ctx, p.Name, "Pod")
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
				logger.Warningf(ctx, "volume for volume mount %q not found", volMount.Name)
				continue
			}
			var fsInfo *caas.FilesystemInfo
			if vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.ClaimName != "" {
				fsInfo, err = k.volumeInfoForPVC(ctx, vol, volMount, vol.PersistentVolumeClaim.ClaimName, now)
			} else if vol.EmptyDir != nil {
				fsInfo, err = k.volumeInfoForEmptyDir(vol, volMount, now)
			} else {
				// Ignore volumes which are not Juju managed filesystems.
				logger.Debugf(ctx, "ignoring blank EmptyDir, PersistentVolumeClaim or ClaimName")
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
			logger.Debugf(ctx, "filesystem info for %v: %+v", volMount.Name, *fsInfo)
			unitInfo.FilesystemInfo = append(unitInfo.FilesystemInfo, *fsInfo)
		}
		units = append(units, unitInfo)
	}
	return units, nil
}

// ListPods filters a list of pods for the provided namespace and labels.
func (k *kubernetesClient) ListPods(ctx context.Context, namespace string, selector k8slabels.Selector) ([]core.Pod, error) {
	listOps := v1.ListOptions{
		LabelSelector: selector.String(),
	}
	list, err := k.client().CoreV1().Pods(namespace).List(ctx, listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(list.Items) == 0 {
		return nil, errors.NotFoundf("pods with selector %q", selector)
	}
	return list.Items, nil
}

func (k *kubernetesClient) getPod(ctx context.Context, podName string) (*core.Pod, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	pods := k.client().CoreV1().Pods(k.namespace)
	pod, err := pods.Get(ctx, podName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("pod %q", podName)
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return pod, nil
}

func (k *kubernetesClient) getStatefulSetStatus(ctx context.Context, ss *apps.StatefulSet) (string, status.Status, error) {
	terminated := ss.DeletionTimestamp != nil
	jujuStatus := status.Waiting
	if terminated {
		jujuStatus = status.Terminated
	}
	if ss.Status.ReadyReplicas == ss.Status.Replicas {
		jujuStatus = status.Active
	}
	return k.getStatusFromEvents(ctx, ss.Name, "StatefulSet", jujuStatus)
}

func (k *kubernetesClient) getDeploymentStatus(ctx context.Context, deployment *apps.Deployment) (string, status.Status, error) {
	terminated := deployment.DeletionTimestamp != nil
	jujuStatus := status.Waiting
	if terminated {
		jujuStatus = status.Terminated
	}
	if deployment.Status.ReadyReplicas == deployment.Status.Replicas {
		jujuStatus = status.Active
	}
	return k.getStatusFromEvents(ctx, deployment.Name, "Deployment", jujuStatus)
}

func (k *kubernetesClient) getDaemonSetStatus(ctx context.Context, ds *apps.DaemonSet) (string, status.Status, error) {
	terminated := ds.DeletionTimestamp != nil
	jujuStatus := status.Waiting
	if terminated {
		jujuStatus = status.Terminated
	}
	if ds.Status.NumberReady == ds.Status.DesiredNumberScheduled {
		jujuStatus = status.Active
	}
	return k.getStatusFromEvents(ctx, ds.Name, "DaemonSet", jujuStatus)
}

func (k *kubernetesClient) getStatusFromEvents(ctx context.Context, name, kind string, jujuStatus status.Status) (string, status.Status, error) {
	events, err := k.getEvents(ctx, name, kind)
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

func boolPtr(b bool) *bool {
	return &b
}

// legacyAppName returns true if there are any artifacts for
// appName which indicate that this deployment was for Juju 2.5.0.
func (k *kubernetesClient) legacyAppName(ctx context.Context, appName string) bool {
	legacyName := "juju-operator-" + appName
	_, err := k.getStatefulSet(ctx, legacyName)
	return err == nil
}

func (k *kubernetesClient) deploymentName(ctx context.Context, appName string, legacySupport bool) string {
	if !legacySupport {
		// No need to check old operator statefulset for brand new features like raw k8s spec.
		return appName
	}
	if k.legacyAppName(ctx, appName) {
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
