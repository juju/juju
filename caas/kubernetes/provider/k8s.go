// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	k8sstorage "k8s.io/api/storage/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/cloudconfig/podcfg"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

var logger = loggo.GetLogger("juju.kubernetes.provider")

const (
	labelOperator        = "juju-operator"
	labelStorage         = "juju-storage"
	labelVersion         = "juju-version"
	labelApplication     = "juju-app"
	labelApplicationUUID = "juju-app-uuid"
	labelModel           = "juju-model"

	gpuAffinityNodeSelectorKey = "gpu"

	annotationPrefix = "juju.io"

	operatorContainerName = "juju-operator"

	// OperatorPodIPEnvName is the environment name for operator pod IP.
	OperatorPodIPEnvName = "JUJU_OPERATOR_POD_IP"

	// OperatorServiceIPEnvName is the environment name for operator service IP.
	OperatorServiceIPEnvName = "JUJU_OPERATOR_SERVICE_IP"

	// OperatorNamespaceEnvName is the environment name for k8s namespace the operator is in.
	OperatorNamespaceEnvName = "JUJU_OPERATOR_NAMESPACE"

	// JujuRunServerSocketPort is the port used by juju run callbacks.
	JujuRunServerSocketPort = 30666
)

var (
	defaultPropagationPolicy = v1.DeletePropagationForeground

	annotationModelUUIDKey              = annotationPrefix + "/" + "model"
	annotationControllerUUIDKey         = annotationPrefix + "/" + "controller"
	annotationControllerIsControllerKey = annotationPrefix + "/" + "is-controller"
)

type kubernetesClient struct {
	clock jujuclock.Clock

	// namespace is the k8s namespace to use when
	// creating k8s resources.
	namespace string

	annotations k8sannotations.Annotation

	lock                        sync.Mutex
	envCfgUnlocked              *config.Config
	clientUnlocked              kubernetes.Interface
	apiextensionsClientUnlocked apiextensionsclientset.Interface

	newClient NewK8sClientFunc

	// modelUUID is the UUID of the model this client acts on.
	modelUUID string

	// newWatcher is the k8s watcher generator.
	newWatcher NewK8sWatcherFunc

	// randomPrefix generates an annotation for stateful sets.
	randomPrefix RandomPrefixFunc
}

// To regenerate the mocks for the kubernetes Client used by this broker,
// run "go generate" from the package directory.
//go:generate mockgen -package mocks -destination mocks/k8sclient_mock.go k8s.io/client-go/kubernetes Interface
//go:generate mockgen -package mocks -destination mocks/appv1_mock.go k8s.io/client-go/kubernetes/typed/apps/v1 AppsV1Interface,DeploymentInterface,StatefulSetInterface
//go:generate mockgen -package mocks -destination mocks/corev1_mock.go k8s.io/client-go/kubernetes/typed/core/v1 EventInterface,CoreV1Interface,NamespaceInterface,PodInterface,ServiceInterface,ConfigMapInterface,PersistentVolumeInterface,PersistentVolumeClaimInterface,SecretInterface,NodeInterface
//go:generate mockgen -package mocks -destination mocks/extenstionsv1_mock.go k8s.io/client-go/kubernetes/typed/extensions/v1beta1 ExtensionsV1beta1Interface,IngressInterface
//go:generate mockgen -package mocks -destination mocks/storagev1_mock.go k8s.io/client-go/kubernetes/typed/storage/v1 StorageV1Interface,StorageClassInterface
//go:generate mockgen -package mocks -destination mocks/rbacv1_mock.go k8s.io/client-go/kubernetes/typed/rbac/v1 RbacV1Interface,ClusterRoleBindingInterface,ClusterRoleInterface,RoleInterface,RoleBindingInterface
//go:generate mockgen -package mocks -destination mocks/apiextensions_mock.go k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1 ApiextensionsV1beta1Interface,CustomResourceDefinitionInterface
//go:generate mockgen -package mocks -destination mocks/apiextensionsclientset_mock.go -mock_names=Interface=MockApiExtensionsClientInterface k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset  Interface
//go:generate mockgen -package mocks -destination mocks/discovery_mock.go k8s.io/client-go/discovery DiscoveryInterface

// NewK8sClientFunc defines a function which returns a k8s client based on the supplied config.
type NewK8sClientFunc func(c *rest.Config) (kubernetes.Interface, apiextensionsclientset.Interface, error)

// NewK8sWatcherFunc defines a function which returns a k8s watcher based on the supplied config.
type NewK8sWatcherFunc func(wi watch.Interface, name string, clock jujuclock.Clock) (*kubernetesWatcher, error)

// RandomPrefixFunc defines a function used to generate a random hex string.
type RandomPrefixFunc func() (string, error)

// newK8sBroker returns a kubernetes client for the specified k8s cluster.
func newK8sBroker(
	controllerUUID string,
	k8sRestConfig *rest.Config,
	cfg *config.Config,
	newClient NewK8sClientFunc,
	newWatcher NewK8sWatcherFunc,
	randomPrefix RandomPrefixFunc,
	clock jujuclock.Clock,
) (*kubernetesClient, error) {

	k8sClient, apiextensionsClient, err := newClient(k8sRestConfig)
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
	client := &kubernetesClient{
		clock:                       clock,
		clientUnlocked:              k8sClient,
		apiextensionsClientUnlocked: apiextensionsClient,
		envCfgUnlocked:              newCfg.Config,
		namespace:                   newCfg.Name(),
		modelUUID:                   modelUUID,
		newWatcher:                  newWatcher,
		newClient:                   newClient,
		randomPrefix:                randomPrefix,
		annotations: k8sannotations.New(nil).
			Add(annotationModelUUIDKey, modelUUID),
	}
	if controllerUUID != "" {
		// controllerUUID could be empty in add-k8s without -c because there might be no controller yet.
		client.annotations.Add(annotationControllerUUIDKey, controllerUUID)
	}
	return client, nil
}

// GetAnnotations returns current namespace's annotations.
func (k *kubernetesClient) GetAnnotations() k8sannotations.Annotation {
	return k.annotations
}

// Version returns cluster version information.
func (k *kubernetesClient) Version() (ver *version.Number, err error) {
	k8sver, err := k.client().Discovery().ServerVersion()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ver = &version.Number{}
	if ver.Major, err = strconv.Atoi(k8sver.Major); err != nil {
		return nil, errors.Trace(err)
	}
	if ver.Minor, err = strconv.Atoi(k8sver.Minor); err != nil {
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

type resourcePurifier interface {
	SetResourceVersion(string)
}

// purifyResource purifies read only fields before creating/updating the resource.
func purifyResource(resource resourcePurifier) {
	resource.SetResourceVersion("")
}

func (k *kubernetesClient) extendedCient() apiextensionsclientset.Interface {
	k.lock.Lock()
	defer k.lock.Unlock()
	client := k.apiextensionsClientUnlocked
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
func (k *kubernetesClient) SetCloudSpec(spec environs.CloudSpec) error {
	k.lock.Lock()
	defer k.lock.Unlock()

	k8sRestConfig, err := cloudSpecToK8sRestConfig(spec)
	if err != nil {
		return errors.Annotate(err, "cannot set cloud spec")
	}

	k.clientUnlocked, k.apiextensionsClientUnlocked, err = k.newClient(k8sRestConfig)
	if err != nil {
		return errors.Annotate(err, "cannot set cloud spec")
	}
	return nil
}

// PrepareForBootstrap prepares for bootstraping a controller.
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

// Create implements environs.BootstrapEnviron.
func (k *kubernetesClient) Create(context.ProviderCallContext, environs.CreateParams) error {
	// must raise errors.AlreadyExistsf if it's already exist.
	return k.createNamespace(k.namespace)
}

// Bootstrap deploys controller with mongoDB together into k8s cluster.
func (k *kubernetesClient) Bootstrap(
	ctx environs.BootstrapContext,
	callCtx context.ProviderCallContext,
	args environs.BootstrapParams,
) (*environs.BootstrapResult, error) {

	if args.BootstrapSeries != "" {
		return nil, errors.NotSupportedf("set series for bootstrapping to kubernetes")
	}

	storageClass, err := k.validateOperatorStorage()
	if err != nil {
		return nil, errors.Trace(err)
	}

	finalizer := func(ctx environs.BootstrapContext, pcfg *podcfg.ControllerPodConfig, opts environs.BootstrapDialOpts) (err error) {
		if err = podcfg.FinishControllerPodConfig(pcfg, k.Config()); err != nil {
			return errors.Trace(err)
		}

		if err = pcfg.VerifyConfig(); err != nil {
			return errors.Trace(err)
		}

		logger.Debugf("controller pod config: \n%+v", pcfg)

		// validate hosted model name if we need to create it.
		if hostedModelName, has := pcfg.GetHostedModel(); has {
			_, err := k.getNamespaceByName(hostedModelName)
			if err == nil {
				return errors.NewAlreadyExists(nil,
					fmt.Sprintf(`
namespace %q already exists in the cluster,
please choose a different hosted model name then try again.`, hostedModelName),
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
				broker.SetNamespace(nsName)
				// ensure controller specific annotations.
				_ = broker.addAnnotations(annotationControllerIsControllerKey, "true")
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
		controllerStack, err := newcontrollerStack(ctx, JujuControllerStackName, storageClass, k, pcfg)
		if err != nil {
			return errors.Trace(err)
		}
		return errors.Annotate(
			controllerStack.Deploy(),
			"creating controller stack for controller",
		)
	}

	return &environs.BootstrapResult{
		// TODO(bootstrap): review this default arch and series(required for determining DataDir etc.) later.
		Arch:                   arch.AMD64,
		Series:                 "kubernetes",
		CaasBootstrapFinalizer: finalizer,
	}, nil
}

// DestroyController implements the Environ interface.
func (k *kubernetesClient) DestroyController(ctx context.ProviderCallContext, controllerUUID string) error {
	// ensures all annnotations are set correctly, then we will accurately find the controller namespace to destroy it.
	k.annotations.Merge(
		k8sannotations.New(nil).
			Add(annotationControllerUUIDKey, controllerUUID).
			Add(annotationControllerIsControllerKey, "true"),
	)
	return k.Destroy(ctx)
}

// Provider is part of the Broker interface.
func (*kubernetesClient) Provider() caas.ContainerEnvironProvider {
	return providerInstance
}

// Destroy is part of the Broker interface.
func (k *kubernetesClient) Destroy(callbacks context.ProviderCallContext) (err error) {
	defer func() {
		if err != nil && k8serrors.ReasonForError(err) == v1.StatusReasonUnknown {
			logger.Warningf("k8s cluster is not accessible: %v", err)
			err = nil
		}
	}()
	watcher, err := k.WatchNamespace()
	if err != nil {
		return errors.Trace(err)
	}
	defer watcher.Kill()

	if err := k.deleteNamespace(); err != nil {
		return errors.Annotate(err, "deleting model namespace")
	}

	// Delete any storage classes created as part of this model.
	// Storage classes live outside the namespace so need to be deleted separately.
	modelSelector := fmt.Sprintf("%s==%s", labelModel, k.namespace)
	err = k.client().StorageV1().StorageClasses().DeleteCollection(&v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	}, v1.ListOptions{
		LabelSelector: modelSelector,
	})
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Annotate(err, "deleting model storage classes")
	}

	// delete Juju admin RBAC resources.
	defer clientconfig.DeleteJujuAdminRBACResources(k.client())

	for {
		select {
		case <-callbacks.Dying():
			return nil
		case <-watcher.Changes():
			// ensure namespace deleted - notfound error expected.
			_, err := k.GetNamespace(k.namespace)
			if errors.IsNotFound(err) {
				// namespace has already been deleted.
				return nil
			}
			if err != nil {
				return errors.Trace(err)
			}
			logger.Debugf("namespace %q is still been terminating", k.namespace)
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

// ValidateStorageClass returns an error if the storage config is not valid.
func (k *kubernetesClient) ValidateStorageClass(config map[string]interface{}) error {
	cfg, err := newStorageConfig(config)
	if err != nil {
		return errors.Trace(err)
	}
	sc, err := k.getStorageClass(cfg.storageClass)
	if err != nil {
		return errors.NewNotValid(err, fmt.Sprintf("storage class %q", cfg.storageClass))
	}
	if cfg.storageProvisioner == "" {
		return nil
	}
	if sc.Provisioner != cfg.storageProvisioner {
		return errors.NewNotValid(
			nil,
			fmt.Sprintf("storage class %q has provisoner %q, not %q", cfg.storageClass, sc.Provisioner, cfg.storageProvisioner))
	}
	return nil
}

type volumeParams struct {
	storageConfig       *storageConfig
	pvcName             string
	requestedVolumeSize resource.Quantity
	accessMode          core.PersistentVolumeAccessMode
}

// maybeGetVolumeClaimSpec returns a persistent volume claim spec for the given
// parameters. If no suitable storage class is available, return a NotFound error.
func (k *kubernetesClient) maybeGetVolumeClaimSpec(params volumeParams) (*core.PersistentVolumeClaimSpec, error) {
	storageClassName := params.storageConfig.storageClass
	haveStorageClass := false
	if storageClassName == "" {
		return nil, errors.New("cannot create a volume claim spec without a storage class")
	}
	// See if the requested storage class exists already.
	sc, err := k.getStorageClass(storageClassName)
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, errors.Annotatef(err, "looking for storage class %q", storageClassName)
	}
	if err == nil {
		haveStorageClass = true
		storageClassName = sc.Name
	}
	if !haveStorageClass {
		params.storageConfig.storageClass = storageClassName
		sc, err := k.EnsureStorageProvisioner(caas.StorageProvisioner{
			Name:          params.storageConfig.storageClass,
			Namespace:     k.namespace,
			Provisioner:   params.storageConfig.storageProvisioner,
			Parameters:    params.storageConfig.parameters,
			ReclaimPolicy: string(params.storageConfig.reclaimPolicy),
		})
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		if err == nil {
			haveStorageClass = true
			storageClassName = sc.Name
		}
	}
	if !haveStorageClass {
		return nil, errors.NewNotFound(nil, fmt.Sprintf(
			"cannot create persistent volume as storage class %q cannot be found", storageClassName))
	}
	accessMode := params.accessMode
	if accessMode == "" {
		accessMode = core.ReadWriteOnce
	}
	return &core.PersistentVolumeClaimSpec{
		StorageClassName: &storageClassName,
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceStorage: params.requestedVolumeSize,
			},
		},
		AccessModes: []core.PersistentVolumeAccessMode{accessMode},
	}, nil
}

// getStorageClass returns a named storage class, first looking for
// one which is qualified by the current namespace if it's available.
func (k *kubernetesClient) getStorageClass(name string) (*k8sstorage.StorageClass, error) {
	storageClasses := k.client().StorageV1().StorageClasses()
	qualifiedName := qualifiedStorageClassName(k.namespace, name)
	sc, err := storageClasses.Get(qualifiedName, v1.GetOptions{})
	if err == nil {
		return sc, nil
	}
	if !k8serrors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	return storageClasses.Get(name, v1.GetOptions{})
}

// EnsureStorageProvisioner creates a storage class with the specified config, or returns an existing one.
func (k *kubernetesClient) EnsureStorageProvisioner(cfg caas.StorageProvisioner) (*caas.StorageProvisioner, error) {
	// First see if the named storage class exists.
	sc, err := k.getStorageClass(cfg.Name)
	if err == nil {
		return &caas.StorageProvisioner{
			Name:        sc.Name,
			Provisioner: sc.Provisioner,
			Parameters:  sc.Parameters,
		}, nil
	}
	if !k8serrors.IsNotFound(err) {
		return nil, errors.Annotatef(err, "getting storage class %q", cfg.Name)
	}
	// If it's not found but there's no provisioner specified, we can't
	// create it so just return not found.
	if cfg.Provisioner == "" {
		return nil, errors.NewNotFound(nil,
			fmt.Sprintf("storage class %q doesn't exist, but no storage provisioner has been specified",
				cfg.Name))
	}

	// Create the storage class with the specified provisioner.
	var reclaimPolicy *core.PersistentVolumeReclaimPolicy
	if cfg.ReclaimPolicy != "" {
		policy := core.PersistentVolumeReclaimPolicy(cfg.ReclaimPolicy)
		reclaimPolicy = &policy
	}
	storageClasses := k.client().StorageV1().StorageClasses()
	sc = &k8sstorage.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: qualifiedStorageClassName(cfg.Namespace, cfg.Name),
		},
		Provisioner:   cfg.Provisioner,
		ReclaimPolicy: reclaimPolicy,
		Parameters:    cfg.Parameters,
	}
	if cfg.Namespace != "" {
		sc.Labels = map[string]string{labelModel: k.namespace}
	}
	_, err = storageClasses.Create(sc)
	if err != nil {
		return nil, errors.Annotatef(err, "creating storage class %q", cfg.Name)
	}
	return &caas.StorageProvisioner{
		Name:        sc.Name,
		Provisioner: sc.Provisioner,
		Parameters:  sc.Parameters,
	}, nil
}

func getLoadBalancerAddress(svc *core.Service) string {
	// different cloud providers have a different way to report back the Load Balancer address.
	// This covers the cases we know about so far.
	lpAdd := svc.Spec.LoadBalancerIP
	if lpAdd != "" {
		return lpAdd
	}

	ing := svc.Status.LoadBalancer.Ingress
	if len(ing) == 0 {
		return ""
	}

	// It usually has only one record.
	firstOne := ing[0]
	if firstOne.IP != "" {
		return firstOne.IP
	}
	if firstOne.Hostname != "" {
		return firstOne.Hostname
	}
	return lpAdd
}

func getSvcAddresses(svc *core.Service, includeClusterIP bool) []network.ProviderAddress {
	var netAddrs []network.ProviderAddress

	addressExist := func(addr string) bool {
		for _, v := range netAddrs {
			if addr == v.Value {
				return true
			}
		}
		return false
	}
	appendUniqueAddrs := func(scope network.Scope, addrs ...string) {
		for _, v := range addrs {
			if v != "" && !addressExist(v) {
				netAddrs = append(netAddrs, network.NewScopedProviderAddress(v, scope))
			}
		}
	}

	t := svc.Spec.Type
	clusterIP := svc.Spec.ClusterIP
	switch t {
	case core.ServiceTypeClusterIP:
		appendUniqueAddrs(network.ScopeCloudLocal, clusterIP)
	case core.ServiceTypeExternalName:
		appendUniqueAddrs(network.ScopePublic, svc.Spec.ExternalName)
	case core.ServiceTypeNodePort:
		appendUniqueAddrs(network.ScopePublic, svc.Spec.ExternalIPs...)
	case core.ServiceTypeLoadBalancer:
		appendUniqueAddrs(network.ScopePublic, getLoadBalancerAddress(svc))
	}
	if includeClusterIP {
		// append clusterIP as a fixed internal address.
		appendUniqueAddrs(network.ScopeCloudLocal, clusterIP)
	}
	return netAddrs
}

// GetService returns the service for the specified application.
func (k *kubernetesClient) GetService(appName string, includeClusterIP bool) (*caas.Service, error) {
	services := k.client().CoreV1().Services(k.namespace)
	servicesList, err := services.List(v1.ListOptions{
		LabelSelector:        applicationSelector(appName),
		IncludeUninitialized: true,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result caas.Service
	// We may have the stateful set or deployment but service not done yet.
	if len(servicesList.Items) > 0 {
		service := servicesList.Items[0]
		result.Id = string(service.GetUID())
		result.Addresses = getSvcAddresses(&service, includeClusterIP)
	}

	deploymentName := k.deploymentName(appName)
	statefulsets := k.client().AppsV1().StatefulSets(k.namespace)
	ss, err := statefulsets.Get(deploymentName, v1.GetOptions{})
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
	if !k8serrors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	deployments := k.client().AppsV1().Deployments(k.namespace)
	deployment, err := deployments.Get(deploymentName, v1.GetOptions{})
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
		message, ssStatus, err := k.getDeploymentStatus(deployment)
		if err != nil {
			return nil, errors.Annotatef(err, "getting status for %s", ss.Name)
		}
		result.Status = status.StatusInfo{
			Status:  ssStatus,
			Message: message,
		}
	}
	return &result, nil
}

// DeleteService deletes the specified service with all related resources.
func (k *kubernetesClient) DeleteService(appName string) (err error) {
	logger.Debugf("deleting application %s", appName)

	deploymentName := k.deploymentName(appName)
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
	if err := k.deleteSecrets(appName); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteConfigMaps(appName); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteAllServiceAccountResources(appName); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteCustomResourceDefinitions(appName); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func resourceTagsToAnnotations(in map[string]string) k8sannotations.Annotation {
	tagsAnnotationsMap := map[string]string{
		tags.JujuController: "juju.io/controller",
		tags.JujuModel:      "juju.io/model",
	}

	out := k8sannotations.New(nil)
	for k, v := range in {
		if annotationKey, ok := tagsAnnotationsMap[k]; ok {
			k = annotationKey
		}
		out.Add(k, v)
	}
	return out
}

func processConstraints(pod *core.PodSpec, appName string, cons constraints.Value) error {
	// TODO(caas): Allow constraints to be set at the container level.
	if mem := cons.Mem; mem != nil {
		if err := configureConstraint(pod, "memory", fmt.Sprintf("%dMi", *mem)); err != nil {
			return errors.Annotatef(err, "configuring memory constraint for %s", appName)
		}
	}
	if cpu := cons.CpuPower; cpu != nil {
		if err := configureConstraint(pod, "cpu", fmt.Sprintf("%dm", *cpu)); err != nil {
			return errors.Annotatef(err, "configuring cpu constraint for %s", appName)
		}
	}

	// Translate tags to node affinity.
	if cons.Tags != nil {
		affinityLabels := *cons.Tags
		var (
			affinityTags     = make(map[string]string)
			antiAffinityTags = make(map[string]string)
		)
		for _, labelPair := range affinityLabels {
			parts := strings.Split(labelPair, "=")
			if len(parts) != 2 {
				return errors.Errorf("invalid node affinity constraints: %v", affinityLabels)
			}
			key := strings.Trim(parts[0], " ")
			value := strings.Trim(parts[1], " ")
			if strings.HasPrefix(key, "^") {
				if len(key) == 1 {
					return errors.Errorf("invalid node affinity constraints: %v", affinityLabels)
				}
				antiAffinityTags[key[1:]] = value
			} else {
				affinityTags[key] = value
			}
		}

		updateSelectorTerms := func(nodeSelectorTerm *core.NodeSelectorTerm, tags map[string]string, op core.NodeSelectorOperator) {
			// Sort for stable ordering.
			var keys []string
			for k := range tags {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, tag := range keys {
				allValues := strings.Split(tags[tag], "|")
				for i, v := range allValues {
					allValues[i] = strings.Trim(v, " ")
				}
				nodeSelectorTerm.MatchExpressions = append(nodeSelectorTerm.MatchExpressions, core.NodeSelectorRequirement{
					Key:      tag,
					Operator: op,
					Values:   allValues,
				})
			}
		}
		var nodeSelectorTerm core.NodeSelectorTerm
		updateSelectorTerms(&nodeSelectorTerm, affinityTags, core.NodeSelectorOpIn)
		updateSelectorTerms(&nodeSelectorTerm, antiAffinityTags, core.NodeSelectorOpNotIn)
		pod.Affinity = &core.Affinity{
			NodeAffinity: &core.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
					NodeSelectorTerms: []core.NodeSelectorTerm{nodeSelectorTerm},
				},
			},
		}
	}
	if cons.Zones != nil {
		zones := *cons.Zones
		affinity := pod.Affinity
		if affinity == nil {
			affinity = &core.Affinity{
				NodeAffinity: &core.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
						NodeSelectorTerms: []core.NodeSelectorTerm{{}},
					},
				},
			}
			pod.Affinity = affinity
		}
		nodeSelector := &affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0]
		nodeSelector.MatchExpressions = append(nodeSelector.MatchExpressions,
			core.NodeSelectorRequirement{
				Key:      "failure-domain.beta.kubernetes.io/zone",
				Operator: core.NodeSelectorOpIn,
				Values:   zones,
			})
	}
	return nil
}

// EnsureService creates or updates a service for pods with the given params.
func (k *kubernetesClient) EnsureService(
	appName string,
	statusCallback caas.StatusCallbackFunc,
	params *caas.ServiceParams,
	numUnits int,
	config application.ConfigAttributes,
) (err error) {
	defer func() {
		if err != nil {
			_ = statusCallback(appName, status.Error, err.Error(), nil)
		}
	}()

	logger.Debugf("creating/updating application %s", appName)
	deploymentName := k.deploymentName(appName)

	if numUnits < 0 {
		return errors.Errorf("number of units must be >= 0")
	}
	if numUnits == 0 {
		return k.deleteAllPods(appName, deploymentName)
	}
	if params == nil || params.PodSpec == nil {
		return errors.Errorf("missing pod spec")
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

	workloadSpec, err := prepareWorkloadSpec(appName, deploymentName, params.PodSpec)
	if err != nil {
		return errors.Annotatef(err, "parsing unit spec for %s", appName)
	}

	// ensure configmap.
	if len(workloadSpec.ConfigMaps) > 0 {
		cmsCleanUps, err := k.ensureConfigMaps(appName, workloadSpec.ConfigMaps)
		cleanups = append(cleanups, cmsCleanUps...)
		if err != nil {
			return errors.Annotate(err, "creating or updating configmaps")
		}
	}

	// ensure secrets.
	if len(workloadSpec.Secrets) > 0 {
		secretsCleanUps, err := k.ensureSecrets(appName, workloadSpec.Secrets)
		cleanups = append(cleanups, secretsCleanUps...)
		if err != nil {
			return errors.Annotate(err, "creating or updating secrets")
		}
	}

	// ensure custom resource definitions first.
	crds := workloadSpec.CustomResourceDefinitions
	if len(crds) > 0 {
		crdCleanUps, err := k.ensureCustomResourceDefinitions(appName, crds)
		cleanups = append(cleanups, crdCleanUps...)
		if err != nil {
			return errors.Annotate(err, "creating or updating custom resource definitions")
		}
		logger.Debugf("created/updated custom resource definition for %q.", appName)
	}

	if workloadSpec.ServiceAccount != nil {
		saCleanups, err := k.ensureServiceAccountForApp(appName, workloadSpec.ServiceAccount)
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
	if err := processConstraints(&workloadSpec.Pod, appName, params.Constraints); err != nil {
		return errors.Trace(err)
	}

	annotations := resourceTagsToAnnotations(params.ResourceTags)

	for _, c := range params.PodSpec.Containers {
		if c.ImageDetails.Password == "" {
			continue
		}
		imageSecretName := appSecretName(deploymentName, c.Name)
		if err := k.ensureOCIImageSecret(imageSecretName, appName, &c.ImageDetails, annotations.Copy()); err != nil {
			return errors.Annotatef(err, "creating secrets for container: %s", c.Name)
		}
		cleanups = append(cleanups, func() { k.deleteSecret(imageSecretName, nil) })
	}

	// Add a deployment controller or stateful set configured to create the specified number of units/pods.
	// Defensively check to see if a stateful set is already used.
	var useStatefulSet bool
	if params.Deployment.DeploymentType != "" {
		useStatefulSet = params.Deployment.DeploymentType == caas.DeploymentStateful
	} else {
		useStatefulSet = len(params.Filesystems) > 0
	}
	statefulsets := k.client().AppsV1().StatefulSets(k.namespace)
	existingStatefulSet, err := statefulsets.Get(deploymentName, v1.GetOptions{IncludeUninitialized: true})
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if !useStatefulSet {
		useStatefulSet = err == nil
		if useStatefulSet {
			logger.Debugf("no updated filesystems but already using stateful set for %v", appName)
		}
	}
	var randPrefix string
	if useStatefulSet {
		// Include a random snippet in the pvc name so that if the same app
		// is deleted and redeployed again, the pvc retains a unique name.
		// Only generate it once, and record it on the stateful set.
		if existingStatefulSet != nil {
			randPrefix = existingStatefulSet.Annotations[labelApplicationUUID]
		}
		if randPrefix == "" {
			randPrefix, err = k.randomPrefix()
			if err != nil {
				return errors.Trace(err)
			}
		}
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
			return errors.Errorf("ports are required for kubernetes service")
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
	if useStatefulSet {
		if err := k.configureHeadlessService(appName, deploymentName, annotations.Copy()); err != nil {
			return errors.Annotate(err, "creating or updating headless service")
		}
		cleanups = append(cleanups, func() { k.deleteService(headlessServiceName(deploymentName)) })
		if err := k.configureStatefulSet(appName, deploymentName, randPrefix, annotations.Copy(), workloadSpec, params.PodSpec.Containers, &numPods, params.Filesystems); err != nil {
			return errors.Annotate(err, "creating or updating StatefulSet")
		}
		cleanups = append(cleanups, func() { k.deleteDeployment(appName) })
	} else {
		if err := k.configureDeployment(appName, deploymentName, annotations.Copy(), workloadSpec, params.PodSpec.Containers, &numPods); err != nil {
			return errors.Annotate(err, "creating or updating DeploymentController")
		}
		cleanups = append(cleanups, func() { k.deleteDeployment(appName) })
	}
	return nil
}

func randomPrefix() (string, error) {
	var randPrefixBytes [4]byte
	if _, err := io.ReadFull(rand.Reader, randPrefixBytes[0:4]); err != nil {
		return "", errors.Trace(err)
	}
	return fmt.Sprintf("%x", randPrefixBytes), nil
}

// Upgrade sets the OCI image for the app's operator to the specified version.
func (k *kubernetesClient) Upgrade(appName string, vers version.Number) error {
	var resourceName string
	if appName == JujuControllerStackName {
		// upgrading controller.
		resourceName = appName
	} else {
		// upgrading operator.
		resourceName = k.operatorName(appName)
	}
	logger.Debugf("Upgrading %q", resourceName)

	statefulsets := k.client().AppsV1().StatefulSets(k.namespace)
	existingStatefulSet, err := statefulsets.Get(resourceName, v1.GetOptions{IncludeUninitialized: true})
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Trace(err)
	}
	// TODO(wallyworld) - only support stateful set at the moment
	if err != nil {
		return errors.NotSupportedf("upgrading %v", appName)
	}
	for i, c := range existingStatefulSet.Spec.Template.Spec.Containers {
		if !podcfg.IsJujuOCIImage(c.Image) {
			continue
		}
		c.Image = podcfg.RebuildOldOperatorImagePath(c.Image, vers)
		existingStatefulSet.Spec.Template.Spec.Containers[i] = c
	}

	// update juju-version annotation.
	// TODO(caas): consider how to upgrade to current annotations format safely.
	// just ensure juju-version to current version for now.
	existingStatefulSet.SetAnnotations(
		k8sannotations.New(existingStatefulSet.GetAnnotations()).
			Add(labelVersion, vers.String()).ToMap(),
	)
	existingStatefulSet.Spec.Template.SetAnnotations(
		k8sannotations.New(existingStatefulSet.Spec.Template.GetAnnotations()).
			Add(labelVersion, vers.String()).ToMap(),
	)

	_, err = statefulsets.Update(existingStatefulSet)
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteAllPods(appName, deploymentName string) error {
	zero := int32(0)
	statefulsets := k.client().AppsV1().StatefulSets(k.namespace)
	statefulSet, err := statefulsets.Get(deploymentName, v1.GetOptions{IncludeUninitialized: true})
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if err == nil {
		statefulSet.Spec.Replicas = &zero
		_, err = statefulsets.Update(statefulSet)
		return errors.Trace(err)
	}

	deployments := k.client().AppsV1().Deployments(k.namespace)
	deployment, err := deployments.Get(deploymentName, v1.GetOptions{IncludeUninitialized: true})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	deployment.Spec.Replicas = &zero
	_, err = deployments.Update(deployment)
	return errors.Trace(err)
}

func (k *kubernetesClient) configureStorage(
	podSpec *core.PodSpec,
	statefulSet *apps.StatefulSetSpec,
	appName,
	randPrefix string,
	legacy bool,
	filesystems []storage.KubernetesFilesystemParams,
) error {
	baseDir, err := paths.StorageDir(CAASProviderType)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("configuring pod filesystems: %+v with rand %v", filesystems, randPrefix)
	for i, fs := range filesystems {
		var mountPath string
		if fs.Attachment != nil {
			mountPath = fs.Attachment.Path
		}
		if mountPath == "" {
			mountPath = fmt.Sprintf("%s/fs/%s/%s/%d", baseDir, appName, fs.StorageName, i)
		}
		fsSize, err := resource.ParseQuantity(fmt.Sprintf("%dMi", fs.Size))
		if err != nil {
			return errors.Annotatef(err, "invalid volume size %v", fs.Size)
		}

		var volumeSource *core.VolumeSource
		switch fs.Provider {
		case K8s_ProviderType:
		case provider.RootfsProviderType:
			volumeSource = &core.VolumeSource{
				EmptyDir: &core.EmptyDirVolumeSource{
					SizeLimit: &fsSize,
				},
			}
		case provider.TmpfsProviderType:
			medium, ok := fs.Attributes[storageMedium]
			if !ok {
				medium = core.StorageMediumMemory
			}
			volumeSource = &core.VolumeSource{
				EmptyDir: &core.EmptyDirVolumeSource{
					Medium:    core.StorageMedium(fmt.Sprintf("%v", medium)),
					SizeLimit: &fsSize,
				},
			}
		default:
			return errors.NotValidf("charm storage provider type %q for %v", fs.Provider, fs.StorageName)
		}
		if volumeSource != nil {
			logger.Debugf("using emptyDir for %s filesystem %s", appName, fs.StorageName)
			volName := fmt.Sprintf("%s-%d", fs.StorageName, i)
			podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, core.VolumeMount{
				Name:      volName,
				MountPath: mountPath,
			})
			podSpec.Volumes = append(podSpec.Volumes, core.Volume{
				Name:         volName,
				VolumeSource: *volumeSource,
			})
			continue
		}

		pvcNamePrefix := fmt.Sprintf("%s-%s", fs.StorageName, randPrefix)
		if legacy {
			pvcNamePrefix = fmt.Sprintf("juju-%s-%d", fs.StorageName, i)
		}
		params := volumeParams{
			pvcName:             pvcNamePrefix,
			requestedVolumeSize: fsSize,
		}
		params.storageConfig, err = newStorageConfig(fs.Attributes)
		if err != nil {
			return errors.Annotatef(err, "invalid storage configuration for %v", fs.StorageName)
		}
		pvcSpec, err := k.maybeGetVolumeClaimSpec(params)
		if err != nil {
			return errors.Annotatef(err, "finding volume for %s", fs.StorageName)
		}

		pvc := core.PersistentVolumeClaim{
			ObjectMeta: v1.ObjectMeta{
				Name: params.pvcName,
				Annotations: resourceTagsToAnnotations(fs.ResourceTags).
					Add(labelStorage, fs.StorageName).ToMap(),
			},
			Spec: *pvcSpec,
		}
		logger.Debugf("using persistent volume claim for %s filesystem %s: %+v", appName, fs.StorageName, pvc)
		statefulSet.VolumeClaimTemplates = append(statefulSet.VolumeClaimTemplates, pvc)
		podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, core.VolumeMount{
			Name:      pvc.Name,
			MountPath: mountPath,
		})
	}
	return nil
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
		unitSpec.Pod.NodeSelector = buildNodeSelector(nodeLabel)
	}
	return nil
}

func configureConstraint(pod *core.PodSpec, constraint, value string) error {
	for i := range pod.Containers {
		resources := pod.Containers[i].Resources
		err := mergeConstraint(constraint, value, &resources)
		if err != nil {
			return errors.Annotatef(err, "merging constraint %q to %#v", constraint, resources)
		}
		pod.Containers[i].Resources = resources
	}
	return nil
}

type configMapNameFunc func(fileSetName string) string

func (k *kubernetesClient) configurePodFiles(podSpec *core.PodSpec, containers []specs.ContainerSpec, cfgMapName configMapNameFunc) error {
	for i, container := range containers {
		for _, fileSet := range container.Files {
			cfgName := cfgMapName(fileSet.Name)
			vol := core.Volume{Name: cfgName}
			if _, err := k.ensureConfigMap(filesetConfigMap(cfgName, &fileSet)); err != nil {
				return errors.Annotatef(err, "creating or updating ConfigMap for file set %v", cfgName)
			}
			vol.ConfigMap = &core.ConfigMapVolumeSource{
				LocalObjectReference: core.LocalObjectReference{
					Name: cfgName,
				},
			}
			podSpec.Volumes = append(podSpec.Volumes, vol)
			podSpec.Containers[i].VolumeMounts = append(podSpec.Containers[i].VolumeMounts, core.VolumeMount{
				Name:      cfgName,
				MountPath: fileSet.MountPath,
			})
		}
	}
	return nil
}

func podAnnotations(annotations k8sannotations.Annotation) k8sannotations.Annotation {
	// Add standard security annotations.
	return annotations.
		Add("apparmor.security.beta.kubernetes.io/pod", "runtime/default").
		Add("seccomp.security.beta.kubernetes.io/pod", "docker/default")
}

func (k *kubernetesClient) configureDeployment(
	appName, deploymentName string,
	annotations k8sannotations.Annotation,
	workloadSpec *workloadSpec,
	containers []specs.ContainerSpec,
	replicas *int32,
) error {
	logger.Debugf("creating/updating deployment for %s", appName)

	// Add the specified file to the pod spec.
	cfgName := func(fileSetName string) string {
		return applicationConfigMapName(deploymentName, fileSetName)
	}
	podSpec := workloadSpec.Pod
	if err := k.configurePodFiles(&podSpec, containers, cfgName); err != nil {
		return errors.Trace(err)
	}

	deployment := &apps.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:        deploymentName,
			Labels:      map[string]string{labelApplication: appName},
			Annotations: annotations.ToMap()},
		Spec: apps.DeploymentSpec{
			Replicas: replicas,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{labelApplication: appName},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: deploymentName + "-",
					Labels:       map[string]string{labelApplication: appName},
					Annotations:  podAnnotations(annotations.Copy()).ToMap(),
				},
				Spec: podSpec,
			},
		},
	}
	return k.ensureDeployment(deployment)
}

func (k *kubernetesClient) ensureDeployment(spec *apps.Deployment) error {
	deployments := k.client().AppsV1().Deployments(k.namespace)
	_, err := deployments.Update(spec)
	if k8serrors.IsNotFound(err) {
		_, err = deployments.Create(spec)
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteDeployment(name string) error {
	deployments := k.client().AppsV1().Deployments(k.namespace)
	err := deployments.Delete(name, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func getPodManagementPolicy(svc *specs.ServiceSpec) (out apps.PodManagementPolicyType) {
	// default to "Parallel".
	out = apps.ParallelPodManagement
	if svc == nil {
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

func (k *kubernetesClient) configureStatefulSet(
	appName, deploymentName, randPrefix string, annotations k8sannotations.Annotation, workloadSpec *workloadSpec,
	containers []specs.ContainerSpec, replicas *int32, filesystems []storage.KubernetesFilesystemParams,
) error {
	logger.Debugf("creating/updating stateful set for %s", appName)

	// Add the specified file to the pod spec.
	cfgName := func(fileSetName string) string {
		return applicationConfigMapName(deploymentName, fileSetName)
	}

	statefulset := &apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name: deploymentName,
			Annotations: k8sannotations.New(nil).
				Merge(annotations).
				Add(labelApplicationUUID, randPrefix).ToMap(),
		},
		Spec: apps.StatefulSetSpec{
			Replicas: replicas,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{labelApplication: appName},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels:      map[string]string{labelApplication: appName},
					Annotations: podAnnotations(annotations.Copy()).ToMap(),
				},
			},
			PodManagementPolicy: getPodManagementPolicy(workloadSpec.Service),
			ServiceName:         headlessServiceName(deploymentName),
		},
	}
	podSpec := workloadSpec.Pod
	if err := k.configurePodFiles(&podSpec, containers, cfgName); err != nil {
		return errors.Trace(err)
	}
	existingPodSpec := podSpec

	// Create a new stateful set with the necessary storage config.
	legacy := isLegacyName(deploymentName)
	if err := k.configureStorage(&podSpec, &statefulset.Spec, appName, randPrefix, legacy, filesystems); err != nil {
		return errors.Annotatef(err, "configuring storage for %s", appName)
	}
	statefulset.Spec.Template.Spec = podSpec
	return k.ensureStatefulSet(statefulset, existingPodSpec)
}

func (k *kubernetesClient) ensureStatefulSet(spec *apps.StatefulSet, existingPodSpec core.PodSpec) error {
	api := k.client().AppsV1().StatefulSets(k.namespace)

	_, err := api.Update(spec)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			_, err = api.Create(spec)
		}
		return errors.Trace(err)
	}

	// The statefulset already exists so all we are allowed to update is replicas,
	// template, update strategy. Juju may hand out info with a slightly different
	// requested volume size due to trying to adapt the unit model to the k8s world.
	existing, err := api.Get(spec.GetName(), v1.GetOptions{IncludeUninitialized: true})
	if err != nil {
		return errors.Trace(err)
	}
	// TODO(caas) - allow extra storage to be added
	existing.Spec.Replicas = spec.Spec.Replicas
	existing.Spec.Template.Spec.Containers = existingPodSpec.Containers
	existing.Spec.Template.Spec.ServiceAccountName = existingPodSpec.ServiceAccountName
	existing.Spec.Template.Spec.AutomountServiceAccountToken = existingPodSpec.AutomountServiceAccountToken
	// NB: we can't update the Spec.ServiceName as it is immutable.
	_, err = api.Update(existing)
	return errors.Trace(err)
}

// createStatefulSet deletes a statefulset resource.
func (k *kubernetesClient) createStatefulSet(spec *apps.StatefulSet) error {
	_, err := k.client().AppsV1().StatefulSets(k.namespace).Create(spec)
	return errors.Trace(err)
}

// deleteStatefulSet deletes a statefulset resource.
func (k *kubernetesClient) deleteStatefulSet(name string) error {
	deployments := k.client().AppsV1().StatefulSets(k.namespace)
	err := deployments.Delete(name, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}

	return errors.Trace(err)
}

func (k *kubernetesClient) deleteVolumeClaims(appName string, p *core.Pod) ([]string, error) {
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
		err := pvClaims.Delete(vol.PersistentVolumeClaim.ClaimName, &v1.DeleteOptions{
			PropagationPolicy: &defaultPropagationPolicy,
		})
		if err != nil && !k8serrors.IsNotFound(err) {
			return nil, errors.Annotatef(err, "deleting persistent volume claim %v for %v",
				vol.PersistentVolumeClaim.ClaimName, p.Name)
		}
		deletedClaimVolumes = append(deletedClaimVolumes, vol.Name)
	}
	return deletedClaimVolumes, nil
}

func (k *kubernetesClient) configureService(
	appName, deploymentName string,
	containerPorts []core.ContainerPort,
	params *caas.ServiceParams,
	config application.ConfigAttributes,
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

	serviceType := defaultServiceType
	if params.Deployment.ServiceType != "" {
		switch params.Deployment.ServiceType {
		case caas.ServiceCluster:
			serviceType = core.ServiceTypeClusterIP
		case caas.ServiceLoadBalancer:
			serviceType = core.ServiceTypeLoadBalancer
		case caas.ServiceExternal:
			serviceType = core.ServiceTypeExternalName
		case caas.ServiceOmit:
			logger.Debugf("no service to be created because service type is %q", params.Deployment.ServiceType)
			return nil
		default:
			return errors.NotSupportedf("service type %q", params.Deployment.ServiceType)
		}
	}
	serviceType = core.ServiceType(config.GetString(ServiceTypeConfigKey, string(serviceType)))
	annotations, err := config.GetStringMap(serviceAnnotationsKey, nil)
	if err != nil {
		return errors.Annotatef(err, "unexpected annotations: %#v", config.Get(serviceAnnotationsKey, nil))
	}
	service := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:        deploymentName,
			Labels:      map[string]string{labelApplication: appName},
			Annotations: annotations,
		},
		Spec: core.ServiceSpec{
			Selector:                 map[string]string{labelApplication: appName},
			Type:                     serviceType,
			Ports:                    ports,
			ExternalIPs:              config.Get(serviceExternalIPsConfigKey, []string(nil)).([]string),
			LoadBalancerIP:           config.GetString(serviceLoadBalancerIPKey, ""),
			LoadBalancerSourceRanges: config.Get(serviceLoadBalancerSourceRangesKey, []string(nil)).([]string),
			ExternalName:             config.GetString(serviceExternalNameKey, ""),
		},
	}
	return k.ensureK8sService(service)
}

func (k *kubernetesClient) configureHeadlessService(
	appName, deploymentName string, annotations k8sannotations.Annotation,
) error {
	logger.Debugf("creating/updating headless service for %s", appName)
	service := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   headlessServiceName(deploymentName),
			Labels: map[string]string{labelApplication: appName},
			Annotations: k8sannotations.New(nil).
				Merge(annotations).
				Add("service.alpha.kubernetes.io/tolerate-unready-endpoints", "true").ToMap(),
		},
		Spec: core.ServiceSpec{
			Selector:                 map[string]string{labelApplication: appName},
			Type:                     core.ServiceTypeClusterIP,
			ClusterIP:                "None",
			PublishNotReadyAddresses: true,
		},
	}
	return k.ensureK8sService(service)
}

// ensureK8sService ensures a k8s service resource.
func (k *kubernetesClient) ensureK8sService(spec *core.Service) error {
	services := k.client().CoreV1().Services(k.namespace)
	// Set any immutable fields if the service already exists.
	existing, err := services.Get(spec.Name, v1.GetOptions{IncludeUninitialized: true})
	if err == nil {
		spec.Spec.ClusterIP = existing.Spec.ClusterIP
		spec.ObjectMeta.ResourceVersion = existing.ObjectMeta.ResourceVersion
	}
	_, err = services.Update(spec)
	if k8serrors.IsNotFound(err) {
		_, err = services.Create(spec)
	}
	return errors.Trace(err)
}

// deleteService deletes a service resource.
func (k *kubernetesClient) deleteService(serviceName string) error {
	services := k.client().CoreV1().Services(k.namespace)
	err := services.Delete(serviceName, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

// ExposeService sets up external access to the specified application.
func (k *kubernetesClient) ExposeService(appName string, resourceTags map[string]string, config application.ConfigAttributes) error {
	logger.Debugf("creating/updating ingress resource for %s", appName)

	host := config.GetString(caas.JujuExternalHostNameKey, "")
	if host == "" {
		return errors.Errorf("external hostname required")
	}
	ingressClass := config.GetString(ingressClassKey, defaultIngressClass)
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

	deploymentName := k.deploymentName(appName)
	svc, err := k.client().CoreV1().Services(k.namespace).Get(deploymentName, v1.GetOptions{})
	if err != nil {
		return errors.Trace(err)
	}
	if len(svc.Spec.Ports) == 0 {
		return errors.Errorf("cannot create ingress rule for service %q without a port", svc.Name)
	}
	spec := &v1beta1.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Name:   deploymentName,
			Labels: resourceTags,
			Annotations: map[string]string{
				"ingress.kubernetes.io/rewrite-target":  "",
				"ingress.kubernetes.io/ssl-redirect":    strconv.FormatBool(ingressSSLRedirect),
				"kubernetes.io/ingress.class":           ingressClass,
				"kubernetes.io/ingress.allow-http":      strconv.FormatBool(ingressAllowHTTP),
				"ingress.kubernetes.io/ssl-passthrough": strconv.FormatBool(ingressSSLPassthrough),
			},
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{{
				Host: host,
				IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: &v1beta1.HTTPIngressRuleValue{
						Paths: []v1beta1.HTTPIngressPath{{
							Path: httpPath,
							Backend: v1beta1.IngressBackend{
								ServiceName: svc.Name, ServicePort: svc.Spec.Ports[0].TargetPort},
						}}},
				}}},
		},
	}
	return k.ensureIngress(spec)
}

// UnexposeService removes external access to the specified service.
func (k *kubernetesClient) UnexposeService(appName string) error {
	logger.Debugf("deleting ingress resource for %s", appName)
	return k.deleteIngress(appName)
}

func (k *kubernetesClient) ensureIngress(spec *v1beta1.Ingress) error {
	ingress := k.client().ExtensionsV1beta1().Ingresses(k.namespace)
	_, err := ingress.Update(spec)
	if k8serrors.IsNotFound(err) {
		_, err = ingress.Create(spec)
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteIngress(appName string) error {
	deploymentName := k.deploymentName(appName)
	ingress := k.client().ExtensionsV1beta1().Ingresses(k.namespace)
	err := ingress.Delete(deploymentName, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func operatorSelector(appName string) string {
	return fmt.Sprintf("%v==%v", labelOperator, appName)
}

func applicationSelector(appName string) string {
	return fmt.Sprintf("%v==%v", labelApplication, appName)
}

// WatchUnits returns a watcher which notifies when there
// are changes to units of the specified application.
func (k *kubernetesClient) WatchUnits(appName string) (watcher.NotifyWatcher, error) {
	selector := applicationSelector(appName)
	logger.Debugf("selecting units %q to watch", selector)
	w, err := k.client().CoreV1().Pods(k.namespace).Watch(v1.ListOptions{
		LabelSelector:        selector,
		Watch:                true,
		IncludeUninitialized: true,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return k.newWatcher(w, appName, k.clock)
}

// WatchService returns a watcher which notifies when there
// are changes to the deployment of the specified application.
func (k *kubernetesClient) WatchService(appName string) (watcher.NotifyWatcher, error) {
	// Application may be a statefulset or deployment. It may not have
	// been set up when the watcher is started so we don't know which it
	// is ahead of time. So use a multi-watcher to cover both cases.
	statefulsets := k.client().AppsV1().StatefulSets(k.namespace)
	sswatcher, err := statefulsets.Watch(v1.ListOptions{
		LabelSelector: applicationSelector(appName),
		Watch:         true,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	w1, err := k.newWatcher(sswatcher, appName, k.clock)
	if err != nil {
		return nil, errors.Trace(err)
	}

	deployments := k.client().AppsV1().Deployments(k.namespace)
	dwatcher, err := deployments.Watch(v1.ListOptions{
		LabelSelector: applicationSelector(appName),
		Watch:         true,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	w2, err := k.newWatcher(dwatcher, appName, k.clock)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return watcher.NewMultiNotifyWatcher(w1, w2), nil
}

// legacyJujuPVNameRegexp matches how Juju labels persistent volumes.
// The pattern is: juju-<storagename>-<digit>
var legacyJujuPVNameRegexp = regexp.MustCompile(`^juju-(?P<storageName>\D+)-\d+$`)

// jujuPVNameRegexp matches how Juju labels persistent volumes.
// The pattern is: <storagename>-<digit>
var jujuPVNameRegexp = regexp.MustCompile(`^(?P<storageName>\D+)-\w+$`)

// Units returns all units and any associated filesystems of the specified application.
// Filesystems are mounted via volumes bound to the unit.
func (k *kubernetesClient) Units(appName string) ([]caas.Unit, error) {
	pods := k.client().CoreV1().Pods(k.namespace)
	podsList, err := pods.List(v1.ListOptions{
		LabelSelector: applicationSelector(appName),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var units []caas.Unit
	now := time.Now()
	for _, p := range podsList.Items {
		var ports []string
		for _, c := range p.Spec.Containers {
			for _, p := range c.Ports {
				ports = append(ports, fmt.Sprintf("%v/%v", p.ContainerPort, p.Protocol))
			}
		}
		terminated := p.DeletionTimestamp != nil
		statusMessage, unitStatus, since, err := k.getPODStatus(p, now)
		if err != nil {
			return nil, errors.Trace(err)
		}
		providerId := string(p.GetUID())
		stateful := false

		// Pods managed by a stateful set use the pod name
		// as the provider id as this is stable across pod restarts.
		for _, ref := range p.OwnerReferences {
			if stateful = ref.Kind == "StatefulSet"; stateful {
				providerId = p.Name
				break
			}
		}
		unitInfo := caas.Unit{
			Id:       providerId,
			Address:  p.Status.PodIP,
			Ports:    ports,
			Dying:    terminated,
			Stateful: stateful,
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
				logger.Debugf("Ignoring blank EmptyDir, PersistentVolumeClaim or ClaimName")
				continue
			}
			if err != nil {
				return nil, errors.Annotatef(err, "finding filesystem info for %v", volMount.Name)
			}
			if fsInfo == nil {
				continue
			}
			if fsInfo.StorageName == "" {
				if valid := legacyJujuPVNameRegexp.MatchString(volMount.Name); valid {
					fsInfo.StorageName = legacyJujuPVNameRegexp.ReplaceAllString(volMount.Name, "$storageName")
				} else if valid := jujuPVNameRegexp.MatchString(volMount.Name); valid {
					fsInfo.StorageName = jujuPVNameRegexp.ReplaceAllString(volMount.Name, "$storageName")
				}
			}
			logger.Debugf("filesystem info for %v: %+v", volMount.Name, *fsInfo)
			unitInfo.FilesystemInfo = append(unitInfo.FilesystemInfo, *fsInfo)
		}
		units = append(units, unitInfo)
	}
	return units, nil
}

func (k *kubernetesClient) getPod(podName string) (*core.Pod, error) {
	pods := k.client().CoreV1().Pods(k.namespace)
	pod, err := pods.Get(podName, v1.GetOptions{
		IncludeUninitialized: true,
	})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("pod not found")
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return pod, nil
}

func (k *kubernetesClient) volumeInfoForEmptyDir(vol core.Volume, volMount core.VolumeMount, now time.Time) (*caas.FilesystemInfo, error) {
	size := uint64(vol.EmptyDir.SizeLimit.Size())
	return &caas.FilesystemInfo{
		Size:         size,
		FilesystemId: vol.Name,
		MountPoint:   volMount.MountPath,
		ReadOnly:     volMount.ReadOnly,
		Status: status.StatusInfo{
			Status: status.Attached,
			Since:  &now,
		},
		Volume: caas.VolumeInfo{
			VolumeId:   vol.Name,
			Size:       size,
			Persistent: false,
			Status: status.StatusInfo{
				Status: status.Attached,
				Since:  &now,
			},
		},
	}, nil
}

func (k *kubernetesClient) getPVC(claimName string) (*core.PersistentVolumeClaim, error) {
	pvcs := k.client().CoreV1().PersistentVolumeClaims(k.namespace)
	pvc, err := pvcs.Get(claimName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("pvc not found")
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return pvc, nil
}

func (k *kubernetesClient) volumeInfoForPVC(vol core.Volume, volMount core.VolumeMount, claimName string, now time.Time) (*caas.FilesystemInfo, error) {
	pvClaims := k.client().CoreV1().PersistentVolumeClaims(k.namespace)
	pvc, err := pvClaims.Get(claimName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		// Ignore claims which don't exist (yet).
		return nil, nil
	}
	if err != nil {
		return nil, errors.Annotate(err, "unable to get persistent volume claim")
	}

	if pvc.Status.Phase == core.ClaimPending {
		logger.Debugf(fmt.Sprintf("PersistentVolumeClaim for %v is pending", claimName))
		return nil, nil
	}

	storageName := pvc.Labels[labelStorage]
	if storageName == "" {
		if valid := legacyJujuPVNameRegexp.MatchString(volMount.Name); valid {
			storageName = legacyJujuPVNameRegexp.ReplaceAllString(volMount.Name, "$storageName")
		} else if valid := jujuPVNameRegexp.MatchString(volMount.Name); valid {
			storageName = jujuPVNameRegexp.ReplaceAllString(volMount.Name, "$storageName")
		}
	}

	statusMessage := ""
	since := now
	if len(pvc.Status.Conditions) > 0 {
		statusMessage = pvc.Status.Conditions[0].Message
		since = pvc.Status.Conditions[0].LastProbeTime.Time
	}
	if statusMessage == "" {
		// If there are any events for this pvc we can use the
		// most recent to set the status.
		eventList, err := k.getEvents(pvc.Name, "PersistentVolumeClaim")
		if err != nil {
			return nil, errors.Annotate(err, "unable to get events for PVC")
		}
		// Take the most recent event.
		if count := len(eventList); count > 0 {
			statusMessage = eventList[count-1].Message
		}
	}

	pVolumes := k.client().CoreV1().PersistentVolumes()
	pv, err := pVolumes.Get(pvc.Spec.VolumeName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		// Ignore volumes which don't exist (yet).
		return nil, nil
	}
	if err != nil {
		return nil, errors.Annotate(err, "unable to get persistent volume")
	}

	return &caas.FilesystemInfo{
		StorageName:  storageName,
		Size:         uint64(vol.PersistentVolumeClaim.Size()),
		FilesystemId: string(pvc.UID),
		MountPoint:   volMount.MountPath,
		ReadOnly:     volMount.ReadOnly,
		Status: status.StatusInfo{
			Status:  k.jujuFilesystemStatus(pvc.Status.Phase),
			Message: statusMessage,
			Since:   &since,
		},
		Volume: caas.VolumeInfo{
			VolumeId:   pv.Name,
			Size:       uint64(pv.Size()),
			Persistent: pv.Spec.PersistentVolumeReclaimPolicy == core.PersistentVolumeReclaimRetain,
			Status: status.StatusInfo{
				Status:  k.jujuVolumeStatus(pv.Status.Phase),
				Message: pv.Status.Message,
				Since:   &since,
			},
		},
	}, nil
}

func (k *kubernetesClient) getPODStatus(pod core.Pod, now time.Time) (string, status.Status, time.Time, error) {
	terminated := pod.DeletionTimestamp != nil
	jujuStatus := k.jujuStatus(pod.Status.Phase, terminated)
	statusMessage := pod.Status.Message
	since := now
	if statusMessage == "" {
		for _, cond := range pod.Status.Conditions {
			statusMessage = cond.Message
			since = cond.LastProbeTime.Time
			if cond.Type == core.PodScheduled && cond.Reason == core.PodReasonUnschedulable {
				jujuStatus = status.Blocked
				break
			}
		}
	}

	if statusMessage == "" {
		// If there are any events for this pod we can use the
		// most recent to set the status.
		eventList, err := k.getEvents(pod.Name, "Pod")
		if err != nil {
			return "", "", time.Time{}, errors.Trace(err)
		}
		// Take the most recent event.
		if count := len(eventList); count > 0 {
			statusMessage = eventList[count-1].Message
		}
	}

	return statusMessage, jujuStatus, since, nil
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
				jujuStatus = status.Blocked
				statusMessage = evt.Message
			}
		}
	}
	return statusMessage, jujuStatus, nil
}

func (k *kubernetesClient) jujuStatus(podPhase core.PodPhase, terminated bool) status.Status {
	if terminated {
		return status.Terminated
	}
	switch podPhase {
	case core.PodRunning:
		return status.Running
	case core.PodFailed:
		return status.Error
	case core.PodPending:
		return status.Allocating
	default:
		return status.Unknown
	}
}

func (k *kubernetesClient) jujuFilesystemStatus(pvcPhase core.PersistentVolumeClaimPhase) status.Status {
	switch pvcPhase {
	case core.ClaimPending:
		return status.Pending
	case core.ClaimBound:
		return status.Attached
	case core.ClaimLost:
		return status.Detached
	default:
		return status.Unknown
	}
}

func (k *kubernetesClient) jujuVolumeStatus(pvPhase core.PersistentVolumePhase) status.Status {
	switch pvPhase {
	case core.VolumePending:
		return status.Pending
	case core.VolumeBound:
		return status.Attached
	case core.VolumeAvailable, core.VolumeReleased:
		return status.Detached
	case core.VolumeFailed:
		return status.Error
	default:
		return status.Unknown
	}
}

// filesetConfigMap returns a *core.ConfigMap for a pod
// of the specified unit, with the specified files.
func filesetConfigMap(configMapName string, files *specs.FileSet) *core.ConfigMap {
	result := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name: configMapName,
		},
		Data: map[string]string{},
	}
	for name, data := range files.Files {
		result.Data[name] = data
	}
	return result
}

// workloadSpec represents the k8s resources need to be created for the workload.
type workloadSpec struct {
	Pod     core.PodSpec `json:"pod"`
	Service *specs.ServiceSpec

	Secrets                   []k8sspecs.Secret
	ConfigMaps                map[string]specs.ConfigMap
	ServiceAccount            *specs.ServiceAccountSpec
	CustomResourceDefinitions map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec
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

func prepareWorkloadSpec(appName, deploymentName string, podSpec *specs.PodSpec) (*workloadSpec, error) {
	var spec workloadSpec
	if err := processContainers(deploymentName, podSpec, &spec.Pod); err != nil {
		logger.Errorf("unable to parse %q pod spec: \n%+v", appName, *podSpec)
		return nil, errors.Annotatef(err, "processing container specs for app %q", appName)
	}

	spec.Service = podSpec.Service
	spec.ConfigMaps = podSpec.ConfigMaps
	if podSpec.ServiceAccount != nil {
		spec.ServiceAccount = podSpec.ServiceAccount
		// use application name for the service account if RBAC was requested.
		spec.ServiceAccount.SetName(appName)
	}
	if podSpec.ProviderPod != nil {
		pSpec, ok := podSpec.ProviderPod.(*k8sspecs.K8sPodSpec)
		if !ok {
			return nil, errors.Errorf("unexpected kubernetes pod spec type %T", podSpec.ProviderPod)
		}

		k8sResources := pSpec.KubernetesResources
		if k8sResources != nil {
			spec.Secrets = k8sResources.Secrets
			spec.CustomResourceDefinitions = k8sResources.CustomResourceDefinitions
			if k8sResources.Pod != nil {
				spec.Pod.ActiveDeadlineSeconds = k8sResources.Pod.ActiveDeadlineSeconds
				spec.Pod.TerminationGracePeriodSeconds = k8sResources.Pod.TerminationGracePeriodSeconds
				spec.Pod.DNSPolicy = k8sResources.Pod.DNSPolicy
				spec.Pod.SecurityContext = k8sResources.Pod.SecurityContext
				spec.Pod.RestartPolicy = k8sResources.Pod.RestartPolicy
				spec.Pod.ReadinessGates = k8sResources.Pod.ReadinessGates
			}
		}
		if spec.ServiceAccount != nil {
			spec.Pod.ServiceAccountName = spec.ServiceAccount.GetName()
			spec.Pod.AutomountServiceAccountToken = spec.ServiceAccount.AutomountServiceAccountToken
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

func populateContainerDetails(deploymentName string, pod *core.PodSpec, podContainers []core.Container, containers []specs.ContainerSpec) error {
	for i, c := range containers {
		if c.Image != "" {
			logger.Warningf("Image parameter deprecated, use ImageDetails")
			podContainers[i].Image = c.Image
		} else {
			podContainers[i].Image = c.ImageDetails.ImagePath
		}
		if c.ImageDetails.Password != "" {
			pod.ImagePullSecrets = append(pod.ImagePullSecrets, core.LocalObjectReference{Name: appSecretName(deploymentName, c.Name)})
		}
		if c.ImagePullPolicy != "" {
			podContainers[i].ImagePullPolicy = core.PullPolicy(c.ImagePullPolicy)
		}

		if c.ProviderContainer == nil {
			podContainers[i].SecurityContext = defaultSecurityContext()
			continue
		}
		spec, ok := c.ProviderContainer.(*k8sspecs.K8sContainerSpec)
		if !ok {
			return errors.Errorf("unexpected kubernetes container spec type %T", c.ProviderContainer)
		}
		if spec.LivenessProbe != nil {
			podContainers[i].LivenessProbe = spec.LivenessProbe
		}
		if spec.ReadinessProbe != nil {
			podContainers[i].ReadinessProbe = spec.ReadinessProbe
		}
		if spec.SecurityContext != nil {
			podContainers[i].SecurityContext = spec.SecurityContext
		} else {
			podContainers[i].SecurityContext = defaultSecurityContext()
		}
	}
	return nil
}

// legacyAppName returns true if there are any artifacts for
// appName which indicate that this deployment was for Juju 2.5.0.
func (k *kubernetesClient) legacyAppName(appName string) bool {
	statefulsets := k.client().AppsV1().StatefulSets(k.namespace)
	legacyName := "juju-operator-" + appName
	_, err := statefulsets.Get(legacyName, v1.GetOptions{IncludeUninitialized: true})
	return err == nil
}

func (k *kubernetesClient) operatorName(appName string) string {
	if k.legacyAppName(appName) {
		return "juju-operator-" + appName
	}
	return appName + "-operator"
}

func (k *kubernetesClient) deploymentName(appName string) string {
	if k.legacyAppName(appName) {
		return "juju-" + appName
	}
	return appName
}

func labelsToSelector(labels map[string]string) string {
	var selectors []string
	for k, v := range labels {
		selectors = append(selectors, fmt.Sprintf("%v==%v", k, v))
	}
	sort.Strings(selectors) // for testing.
	return strings.Join(selectors, ",")
}

func newUIDPreconditions(uid k8stypes.UID) *v1.Preconditions {
	return &v1.Preconditions{UID: &uid}
}

func newPreconditionDeleteOptions(uid k8stypes.UID) *v1.DeleteOptions {
	// TODO(caas): refactor all deleting single resource operation has this UID ensured precondition.
	return &v1.DeleteOptions{
		Preconditions:     newUIDPreconditions(uid),
		PropagationPolicy: &defaultPropagationPolicy,
	}
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

func qualifiedStorageClassName(namespace, storageClass string) string {
	if namespace == "" {
		return storageClass
	}
	return namespace + "-" + storageClass
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

func mergeConstraint(constraint string, value string, resources *core.ResourceRequirements) error {
	if resources.Limits == nil {
		resources.Limits = core.ResourceList{}
	}
	resourceName := core.ResourceName(constraint)
	if v, ok := resources.Limits[resourceName]; ok {
		return errors.NotValidf("resource limit for %q has already been set to %v!", resourceName, v)
	}
	parsedValue, err := resource.ParseQuantity(value)
	if err != nil {
		return errors.Annotatef(err, "invalid constraint value %q for %v", value, constraint)
	}
	resources.Limits[resourceName] = parsedValue
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
