// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	k8sstorage "k8s.io/api/storage/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"
	apimachineryversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloudconfig/podcfg"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/juju/paths"
	jujuversion "github.com/juju/juju/juju/version"
	"github.com/juju/juju/network"
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

	jujudToolDir = "/var/lib/juju/tools"

	annotationPrefix = "juju.io"
)

var (
	defaultPropagationPolicy = v1.DeletePropagationForeground

	annotationModelUUIDKey              = annotationPrefix + "/" + "model"
	annotationControllerUUIDKey         = annotationPrefix + "/" + "controller"
	annotationControllerIsControllerKey = annotationPrefix + "/" + "is-controller"
)

type kubernetesClient struct {
	clock jujuclock.Clock
	kubernetes.Interface
	apiextensionsClient apiextensionsclientset.Interface

	// namespace is the k8s namespace to use when
	// creating k8s resources.
	namespace string

	annotations k8sannotations.Annotation

	lock   sync.Mutex
	envCfg *config.Config

	// modelUUID is the UUID of the model this client acts on.
	modelUUID string

	// newWatcher is the k8s watcher generator.
	newWatcher NewK8sWatcherFunc
}

// To regenerate the mocks for the kubernetes Client used by this broker,
// run "go generate" from the package directory.
//go:generate mockgen -package mocks -destination mocks/k8sclient_mock.go k8s.io/client-go/kubernetes Interface
//go:generate mockgen -package mocks -destination mocks/appv1_mock.go k8s.io/client-go/kubernetes/typed/apps/v1 AppsV1Interface,DeploymentInterface,StatefulSetInterface
//go:generate mockgen -package mocks -destination mocks/corev1_mock.go k8s.io/client-go/kubernetes/typed/core/v1 CoreV1Interface,NamespaceInterface,PodInterface,ServiceInterface,ConfigMapInterface,PersistentVolumeInterface,PersistentVolumeClaimInterface,SecretInterface,NodeInterface
//go:generate mockgen -package mocks -destination mocks/extenstionsv1_mock.go k8s.io/client-go/kubernetes/typed/extensions/v1beta1 ExtensionsV1beta1Interface,IngressInterface
//go:generate mockgen -package mocks -destination mocks/storagev1_mock.go k8s.io/client-go/kubernetes/typed/storage/v1 StorageV1Interface,StorageClassInterface

// NewK8sClientFunc defines a function which returns a k8s client based on the supplied config.
type NewK8sClientFunc func(c *rest.Config) (kubernetes.Interface, apiextensionsclientset.Interface, error)

// NewK8sWatcherFunc defines a function which returns a k8s watcher based on the supplied config.
type NewK8sWatcherFunc func(wi watch.Interface, name string, clock jujuclock.Clock) (*kubernetesWatcher, error)

// NewK8sBroker returns a kubernetes client for the specified k8s cluster.
func NewK8sBroker(
	controllerUUID string,
	k8sRestConfig *rest.Config,
	cfg *config.Config,
	newClient NewK8sClientFunc,
	newWatcher NewK8sWatcherFunc,
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
		clock:               clock,
		Interface:           k8sClient,
		apiextensionsClient: apiextensionsClient,
		envCfg:              newCfg.Config,
		namespace:           newCfg.Name(),
		modelUUID:           modelUUID,
		newWatcher:          newWatcher,
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

// addAnnotations set an annotation to current namespace's annotations.
func (k *kubernetesClient) addAnnotations(key, value string) k8sannotations.Annotation {
	return k.annotations.Add(key, value)
}

// Config returns environ config.
func (k *kubernetesClient) Config() *config.Config {
	k.lock.Lock()
	defer k.lock.Unlock()
	cfg := k.envCfg
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
	k.envCfg = newCfg.Config
	return nil
}

func (k *kubernetesClient) validateOperatorStorage() (string, error) {
	storageClass, _ := k.envCfg.AllAttrs()[OperatorStorageKey].(string)
	if storageClass == "" {
		return "", errors.NewNotValid(nil, "config without operator-storage value not valid.\nRun juju add-k8s to reimport your k8s cluster.")
	}
	_, err := k.getStorageClass(storageClass)
	return storageClass, errors.Trace(err)
}

// PrepareForBootstrap prepares for bootstraping a controller.
func (k *kubernetesClient) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	alreadyExistErr := errors.NewAlreadyExists(nil,
		fmt.Sprintf(`a controller called %q already exists on this k8s cluster.
Please bootstrap again and choose a different controller name.`, k.namespace),
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
		Series:                 jujuversion.SupportedLTS(),
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
func (k *kubernetesClient) Destroy(callbacks context.ProviderCallContext) error {
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
	err = k.StorageV1().StorageClasses().DeleteCollection(&v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	}, v1.ListOptions{
		LabelSelector: modelSelector,
	})
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Annotate(err, "deleting model storage classes")
	}
	for {
		select {
		case <-callbacks.Dying():
			return nil
		case <-watcher.Changes():
			// ensure namespace has been deleted - notfound error expected.
			_, err := k.GetNamespace(k.namespace)
			if errors.IsNotFound(err) {
				// namespace ha been deleted.
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
	body, err := k.CoreV1().RESTClient().Get().AbsPath("/version").Do().Raw()
	if err != nil {
		return "", err
	}
	var info apimachineryversion.Info
	err = json.Unmarshal(body, &info)
	if err != nil {
		return "", errors.Annotatef(err, "got '%s' querying API version", string(body))
	}
	version := info.GitVersion
	// git version is "vX.Y.Z", strip the "v"
	version = strings.Trim(version, "v")
	return version, nil
}

// ensureOCIImageSecret ensures a secret exists for use with retrieving images from private registries
func (k *kubernetesClient) ensureOCIImageSecret(
	imageSecretName,
	appName string,
	imageDetails *caas.ImageDetails,
	annotations k8sannotations.Annotation,
) error {
	if imageDetails.Password == "" {
		return errors.New("attempting to create a secret with no password")
	}
	secretData, err := createDockerConfigJSON(imageDetails)
	if err != nil {
		return errors.Trace(err)
	}

	newSecret := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:        imageSecretName,
			Namespace:   k.namespace,
			Labels:      map[string]string{labelApplication: appName},
			Annotations: annotations.ToMap()},
		Type: core.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			core.DockerConfigJsonKey: secretData,
		},
	}
	return errors.Trace(k.ensureSecret(newSecret))
}

func (k *kubernetesClient) ensureSecret(sec *core.Secret) error {
	secrets := k.CoreV1().Secrets(k.namespace)
	_, err := secrets.Update(sec)
	if k8serrors.IsNotFound(err) {
		_, err = secrets.Create(sec)
	}
	return errors.Trace(err)
}

// updateSecret updates a secret resource.
func (k *kubernetesClient) updateSecret(sec *core.Secret) error {
	_, err := k.CoreV1().Secrets(k.namespace).Update(sec)
	return errors.Trace(err)
}

// getSecret return a secret resource.
func (k *kubernetesClient) getSecret(secretName string) (*core.Secret, error) {
	secret, err := k.CoreV1().Secrets(k.namespace).Get(secretName, v1.GetOptions{IncludeUninitialized: true})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errors.NotFoundf("secret %q", secretName)
		}
		return nil, errors.Trace(err)
	}
	return secret, nil
}

// createSecret creates a secret resource.
func (k *kubernetesClient) createSecret(secret *core.Secret) error {
	_, err := k.CoreV1().Secrets(k.namespace).Create(secret)
	return errors.Trace(err)
}

// deleteSecret deletes a secret resource.
func (k *kubernetesClient) deleteSecret(secretName string) error {
	secrets := k.CoreV1().Secrets(k.namespace)
	err := secrets.Delete(secretName, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

// OperatorExists returns true if the operator for the specified
// application exists.
func (k *kubernetesClient) OperatorExists(appName string) (bool, error) {
	statefulsets := k.AppsV1().StatefulSets(k.namespace)
	_, err := statefulsets.Get(k.operatorName(appName), v1.GetOptions{IncludeUninitialized: true})
	if k8serrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	return true, nil
}

// EnsureOperator creates or updates an operator pod with the given application
// name, agent path, and operator config.
func (k *kubernetesClient) EnsureOperator(appName, agentPath string, config *caas.OperatorConfig) error {
	logger.Debugf("creating/updating %s operator", appName)

	operatorName := k.operatorName(appName)
	// TODO(caas) use secrets for storing agent password?
	if config.AgentConf == nil {
		// We expect that the config map already exists,
		// so make sure it does.
		configMaps := k.CoreV1().ConfigMaps(k.namespace)
		_, err := configMaps.Get(operatorConfigMapName(operatorName), v1.GetOptions{IncludeUninitialized: true})
		if err != nil {
			return errors.Annotatef(err, "config map for %q should already exist", appName)
		}
	} else {
		if err := k.ensureConfigMap(operatorConfigMap(appName, operatorName, config)); err != nil {
			return errors.Annotate(err, "creating or updating ConfigMap")
		}
	}

	annotations := resourceTagsToAnnotations(config.ResourceTags).
		Add(labelVersion, config.Version.String())

	// Set up the parameters for creating charm storage.
	operatorVolumeClaim := "charm"
	if isLegacyName(operatorName) {
		operatorVolumeClaim = fmt.Sprintf("%v-operator-volume", appName)
	}

	fsSize, err := resource.ParseQuantity(fmt.Sprintf("%dMi", config.CharmStorage.Size))
	if err != nil {
		return errors.Annotatef(err, "invalid volume size %v", config.CharmStorage.Size)
	}
	params := volumeParams{
		storageConfig:       &storageConfig{},
		pvcName:             operatorVolumeClaim,
		requestedVolumeSize: fsSize,
	}
	if config.CharmStorage.Provider != K8s_ProviderType {
		return errors.Errorf("expected charm storage provider %q, got %q", K8s_ProviderType, config.CharmStorage.Provider)
	}
	params.storageConfig, err = newStorageConfig(config.CharmStorage.Attributes)
	if err != nil {
		return errors.Annotatef(err, "invalid storage configuration for %v operator", appName)
	}
	// We want operator storage to be deleted when the operator goes away.
	params.storageConfig.reclaimPolicy = core.PersistentVolumeReclaimDelete
	logger.Debugf("operator storage config %#v", *params.storageConfig)

	// Attempt to get a persistent volume to store charm state etc.
	pvcSpec, err := k.maybeGetVolumeClaimSpec(params)
	if err != nil {
		return errors.Annotate(err, "finding operator volume claim")
	}

	pvc := &core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			Name:        params.pvcName,
			Annotations: resourceTagsToAnnotations(config.CharmStorage.ResourceTags).ToMap()},
		Spec: *pvcSpec,
	}
	pod := operatorPod(
		operatorName,
		appName,
		agentPath,
		config.OperatorImagePath,
		config.Version.String(),
		annotations.Copy(),
	)
	// Take a copy for use with statefulset.
	podWithoutStorage := pod

	numPods := int32(1)
	logger.Debugf("using persistent volume claim for operator %s: %+v", appName, pvc)
	statefulset := &apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:        operatorName,
			Labels:      map[string]string{labelOperator: appName},
			Annotations: annotations.ToMap()},
		Spec: apps.StatefulSetSpec{
			Replicas: &numPods,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{labelOperator: appName},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels:      map[string]string{labelOperator: appName},
					Annotations: pod.Annotations,
				},
			},
			PodManagementPolicy:  apps.ParallelPodManagement,
			VolumeClaimTemplates: []core.PersistentVolumeClaim{*pvc},
		},
	}
	pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, core.VolumeMount{
		Name:      pvc.Name,
		MountPath: agent.BaseDir(agentPath),
	})

	statefulset.Spec.Template.Spec = pod.Spec
	err = k.ensureStatefulSet(statefulset, podWithoutStorage.Spec)
	return errors.Annotatef(err, "creating or updating %v operator StatefulSet", appName)
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
	storageClasses := k.StorageV1().StorageClasses()
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
	storageClasses := k.StorageV1().StorageClasses()
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

// DeleteOperator deletes the specified operator.
func (k *kubernetesClient) DeleteOperator(appName string) (err error) {
	logger.Debugf("deleting %s operator", appName)

	operatorName := k.operatorName(appName)
	legacy := isLegacyName(operatorName)

	// First delete the config map(s).
	configMaps := k.CoreV1().ConfigMaps(k.namespace)
	configMapName := operatorConfigMapName(operatorName)
	err = configMaps.Delete(configMapName, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil
	}

	// Delete artefacts created by k8s itself.
	configMapName = appName + "-configurations-config"
	if legacy {
		configMapName = "juju-" + configMapName
	}
	err = configMaps.Delete(configMapName, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil
	}

	// Finally the operator itself.
	if err := k.deleteStatefulSet(operatorName); err != nil {
		return errors.Trace(err)
	}
	pods := k.CoreV1().Pods(k.namespace)
	podsList, err := pods.List(v1.ListOptions{
		LabelSelector: operatorSelector(appName),
	})
	if err != nil {
		return errors.Trace(err)
	}

	deploymentName := appName
	if legacy {
		deploymentName = "juju-" + appName
	}
	pvs := k.CoreV1().PersistentVolumes()
	for _, p := range podsList.Items {
		// Delete secrets.
		for _, c := range p.Spec.Containers {
			secretName := appSecretName(deploymentName, c.Name)
			if err := k.deleteSecret(secretName); err != nil {
				return errors.Annotatef(err, "deleting %s secret for container %s", appName, c.Name)
			}
		}
		// Delete operator storage volumes.
		volumeNames, err := k.deleteVolumeClaims(appName, &p)
		if err != nil {
			return errors.Trace(err)
		}
		// Just in case the volume reclaim policy is retain, we force deletion
		// for operators as the volume is an inseparable part of the operator.
		for _, volName := range volumeNames {
			err = pvs.Delete(volName, &v1.DeleteOptions{
				PropagationPolicy: &defaultPropagationPolicy,
			})
			if err != nil && !k8serrors.IsNotFound(err) {
				return errors.Annotatef(err, "deleting operator persistent volume %v for %v",
					volName, appName)
			}
		}
	}
	return errors.Trace(k.deleteDeployment(operatorName))
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

func getSvcAddresses(svc *core.Service, includeClusterIP bool) []network.Address {
	var netAddrs []network.Address

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
				netAddrs = append(netAddrs, network.Address{
					Value: v,
					Type:  network.DeriveAddressType(v),
					Scope: scope,
				})
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
	services := k.CoreV1().Services(k.namespace)
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
	statefulsets := k.AppsV1().StatefulSets(k.namespace)
	ss, err := statefulsets.Get(deploymentName, v1.GetOptions{})
	if err == nil {
		if ss.Spec.Replicas != nil {
			scale := int(*ss.Spec.Replicas)
			result.Scale = &scale
		}
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

	deployments := k.AppsV1().Deployments(k.namespace)
	deployment, err := deployments.Get(deploymentName, v1.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if err == nil {
		if deployment.Spec.Replicas != nil {
			scale := int(*deployment.Spec.Replicas)
			result.Scale = &scale
		}
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
	if err := k.deleteDeployment(deploymentName); err != nil {
		return errors.Trace(err)
	}
	secrets := k.CoreV1().Secrets(k.namespace)
	secretList, err := secrets.List(v1.ListOptions{
		LabelSelector: applicationSelector(appName),
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, s := range secretList.Items {
		if err := k.deleteSecret(s.Name); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// EnsureCustomResourceDefinition creates or updates a custom resource definition resource.
func (k *kubernetesClient) EnsureCustomResourceDefinition(appName string, podSpec *caas.PodSpec) error {
	for name, crd := range podSpec.CustomResourceDefinitions {
		crd, err := k.ensureCustomResourceDefinitionTemplate(name, crd)
		if err != nil {
			return errors.Annotate(err, fmt.Sprintf("ensure custom resource definition %q", name))
		}
		logger.Debugf("ensured custom resource definition %q", crd.ObjectMeta.Name)
	}
	return nil
}

func (k *kubernetesClient) ensureCustomResourceDefinitionTemplate(name string, spec apiextensionsv1beta1.CustomResourceDefinitionSpec) (
	crd *apiextensionsv1beta1.CustomResourceDefinition, err error) {
	crdIn := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: k.namespace,
		},
		Spec: spec,
	}
	apiextensionsV1beta1 := k.apiextensionsClient.ApiextensionsV1beta1()
	logger.Debugf("creating crd %#v", crdIn)
	crd, err = apiextensionsV1beta1.CustomResourceDefinitions().Create(crdIn)
	if k8serrors.IsAlreadyExists(err) {
		crd, err = apiextensionsV1beta1.CustomResourceDefinitions().Get(name, v1.GetOptions{})
		if err != nil {
			return nil, errors.Trace(err)
		}
		resourceVersion := crd.ObjectMeta.GetResourceVersion()
		crdIn.ObjectMeta.SetResourceVersion(resourceVersion)
		logger.Debugf("existing crd with resource version %q found, so update it %#v", resourceVersion, crdIn)
		crd, err = apiextensionsV1beta1.CustomResourceDefinitions().Update(crdIn)
	}
	return
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

// EnsureService creates or updates a service for pods with the given params.
func (k *kubernetesClient) EnsureService(
	appName string, statusCallback caas.StatusCallbackFunc, params *caas.ServiceParams, numUnits int, config application.ConfigAttributes,
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
	if params.PodSpec.OmitServiceFrontend && len(params.Filesystems) == 0 {
		return errors.Errorf("kubernetes service is required when using storage")
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

	unitSpec, err := makeUnitSpec(appName, deploymentName, params.PodSpec)
	if err != nil {
		return errors.Annotatef(err, "parsing unit spec for %s", appName)
	}
	if len(params.Devices) > 0 {
		if err = k.configureDevices(unitSpec, params.Devices); err != nil {
			return errors.Annotatef(err, "configuring devices for %s", appName)
		}
	}
	if mem := params.Constraints.Mem; mem != nil {
		if err = k.configureConstraint(unitSpec, "memory", fmt.Sprintf("%dMi", *mem)); err != nil {
			return errors.Annotatef(err, "configuring memory constraint for %s", appName)
		}
	}
	if cpu := params.Constraints.CpuPower; cpu != nil {
		if err = k.configureConstraint(unitSpec, "cpu", fmt.Sprintf("%dm", *cpu)); err != nil {
			return errors.Annotatef(err, "configuring cpu constraint for %s", appName)
		}
	}

	// Translate tags to node affinity.
	if params.Constraints.Tags != nil {
		affinityLabels := *params.Constraints.Tags
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
		unitSpec.Pod.Affinity = &core.Affinity{
			NodeAffinity: &core.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
					NodeSelectorTerms: []core.NodeSelectorTerm{nodeSelectorTerm},
				},
			},
		}
	}
	if params.Constraints.Zones != nil {
		zones := *params.Constraints.Zones
		affinity := unitSpec.Pod.Affinity
		if affinity == nil {
			affinity = &core.Affinity{
				NodeAffinity: &core.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
						NodeSelectorTerms: []core.NodeSelectorTerm{{}},
					},
				},
			}
			unitSpec.Pod.Affinity = affinity
		}
		nodeSelector := &affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0]
		nodeSelector.MatchExpressions = append(nodeSelector.MatchExpressions,
			core.NodeSelectorRequirement{
				Key:      "failure-domain.beta.kubernetes.io/zone",
				Operator: core.NodeSelectorOpIn,
				Values:   zones,
			})
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
		cleanups = append(cleanups, func() { k.deleteSecret(imageSecretName) })
	}
	// Add a deployment controller or stateful set configured to create the specified number of units/pods.
	// Defensively check to see if a stateful set is already used.
	useStatefulSet := len(params.Filesystems) > 0
	statefulsets := k.AppsV1().StatefulSets(k.namespace)
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
			var randPrefixBytes [4]byte
			if _, err := io.ReadFull(rand.Reader, randPrefixBytes[0:4]); err != nil {
				return errors.Trace(err)
			}
			randPrefix = fmt.Sprintf("%x", randPrefixBytes)
		}
	}

	numPods := int32(numUnits)
	if useStatefulSet {
		if err := k.configureStatefulSet(appName, deploymentName, randPrefix, annotations.Copy(), unitSpec, params.PodSpec.Containers, &numPods, params.Filesystems); err != nil {
			return errors.Annotate(err, "creating or updating StatefulSet")
		}
		cleanups = append(cleanups, func() { k.deleteDeployment(appName) })
	} else {
		if err := k.configureDeployment(appName, deploymentName, annotations.Copy(), unitSpec, params.PodSpec.Containers, &numPods); err != nil {
			return errors.Annotate(err, "creating or updating DeploymentController")
		}
		cleanups = append(cleanups, func() { k.deleteDeployment(appName) })
	}

	var ports []core.ContainerPort
	for _, c := range unitSpec.Pod.Containers {
		for _, p := range c.Ports {
			if p.ContainerPort == 0 {
				continue
			}
			ports = append(ports, p)
		}
	}
	if !params.PodSpec.OmitServiceFrontend {
		// Merge any service annotations from the charm.
		if unitSpec.Service != nil {
			annotations.Merge(k8sannotations.New(unitSpec.Service.Annotations))
		}
		// Merge any service annotations from the CLI.
		deployAnnotations, err := config.GetStringMap(serviceAnnotationsKey, nil)
		if err != nil {
			return errors.Annotatef(err, "unexpected annotations: %#v", config.Get(serviceAnnotationsKey, nil))
		}
		annotations.Merge(k8sannotations.New(deployAnnotations))

		config[serviceAnnotationsKey] = annotations.ToMap()
		if err := k.configureService(appName, deploymentName, ports, config); err != nil {
			return errors.Annotatef(err, "creating or updating service for %v", appName)
		}
	}
	return nil
}

// Upgrade sets the OCI image for the app to the specified version.
func (k *kubernetesClient) Upgrade(appName string, vers version.Number) error {
	deploymentName := k.deploymentName(appName)
	statefulsets := k.AppsV1().StatefulSets(k.namespace)
	existingStatefulSet, err := statefulsets.Get(deploymentName, v1.GetOptions{IncludeUninitialized: true})
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
		tagSep := strings.LastIndex(c.Image, ":")
		c.Image = fmt.Sprintf("%s:%s", c.Image[:tagSep], vers.String())
		existingStatefulSet.Spec.Template.Spec.Containers[i] = c
	}
	_, err = statefulsets.Update(existingStatefulSet)
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteAllPods(appName, deploymentName string) error {
	zero := int32(0)
	statefulsets := k.AppsV1().StatefulSets(k.namespace)
	statefulSet, err := statefulsets.Get(deploymentName, v1.GetOptions{IncludeUninitialized: true})
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if err == nil {
		statefulSet.Spec.Replicas = &zero
		_, err = statefulsets.Update(statefulSet)
		return errors.Trace(err)
	}

	deployments := k.AppsV1().Deployments(k.namespace)
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

func (k *kubernetesClient) configureDevices(unitSpec *unitSpec, devices []devices.KubernetesDeviceParams) error {
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

func (k *kubernetesClient) configureConstraint(unitSpec *unitSpec, constraint, value string) error {
	for i := range unitSpec.Pod.Containers {
		resources := unitSpec.Pod.Containers[i].Resources
		err := mergeConstraint(constraint, value, &resources)
		if err != nil {
			return errors.Annotatef(err, "merging constraint %q to %#v", constraint, resources)
		}
		unitSpec.Pod.Containers[i].Resources = resources
	}
	return nil
}

type configMapNameFunc func(fileSetName string) string

func (k *kubernetesClient) configurePodFiles(podSpec *core.PodSpec, containers []caas.ContainerSpec, cfgMapName configMapNameFunc) error {
	for i, container := range containers {
		for _, fileSet := range container.Files {
			cfgName := cfgMapName(fileSet.Name)
			vol := core.Volume{Name: cfgName}
			if err := k.ensureConfigMap(filesetConfigMap(cfgName, &fileSet)); err != nil {
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
	unitSpec *unitSpec,
	containers []caas.ContainerSpec,
	replicas *int32,
) error {
	logger.Debugf("creating/updating deployment for %s", appName)

	// Add the specified file to the pod spec.
	cfgName := func(fileSetName string) string {
		return applicationConfigMapName(deploymentName, fileSetName)
	}
	podSpec := unitSpec.Pod
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
	deployments := k.AppsV1().Deployments(k.namespace)
	_, err := deployments.Update(spec)
	if k8serrors.IsNotFound(err) {
		_, err = deployments.Create(spec)
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteDeployment(name string) error {
	deployments := k.AppsV1().Deployments(k.namespace)
	err := deployments.Delete(name, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) configureStatefulSet(
	appName, deploymentName, randPrefix string, annotations k8sannotations.Annotation, unitSpec *unitSpec,
	containers []caas.ContainerSpec, replicas *int32, filesystems []storage.KubernetesFilesystemParams,
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
			PodManagementPolicy: apps.ParallelPodManagement,
		},
	}
	podSpec := unitSpec.Pod
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
	statefulsets := k.AppsV1().StatefulSets(k.namespace)
	_, err := statefulsets.Update(spec)
	if k8serrors.IsNotFound(err) {
		_, err = statefulsets.Create(spec)
	}
	if !k8serrors.IsInvalid(err) {
		return errors.Trace(err)
	}

	// The statefulset already exists so all we are allowed to update is replicas,
	// template, update strategy. Juju may hand out info with a slightly different
	// requested volume size due to trying to adapt the unit model to the k8s world.
	existing, err := statefulsets.Get(spec.Name, v1.GetOptions{IncludeUninitialized: true})
	if err != nil {
		return errors.Trace(err)
	}
	// TODO(caas) - allow extra storage to be added
	existing.Spec.Selector = spec.Spec.Selector
	existing.Spec.Replicas = spec.Spec.Replicas
	existing.Spec.Template.Spec.Containers = existingPodSpec.Containers
	_, err = statefulsets.Update(existing)
	return errors.Trace(err)
}

// createStatefulSet deletes a statefulset resource.
func (k *kubernetesClient) createStatefulSet(spec *apps.StatefulSet) error {
	_, err := k.AppsV1().StatefulSets(k.namespace).Create(spec)
	return errors.Trace(err)
}

// deleteStatefulSet deletes a statefulset resource.
func (k *kubernetesClient) deleteStatefulSet(name string) error {
	deployments := k.AppsV1().StatefulSets(k.namespace)
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
		pvClaims := k.CoreV1().PersistentVolumeClaims(k.namespace)
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
	appName, deploymentName string, containerPorts []core.ContainerPort,
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

	serviceType := core.ServiceType(config.GetString(serviceTypeConfigKey, defaultServiceType))
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

// ensureK8sService ensures a k8s service resource.
func (k *kubernetesClient) ensureK8sService(spec *core.Service) error {
	services := k.CoreV1().Services(k.namespace)
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
func (k *kubernetesClient) deleteService(deploymentName string) error {
	services := k.CoreV1().Services(k.namespace)
	err := services.Delete(deploymentName, &v1.DeleteOptions{
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
	svc, err := k.CoreV1().Services(k.namespace).Get(deploymentName, v1.GetOptions{})
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
	ingress := k.ExtensionsV1beta1().Ingresses(k.namespace)
	_, err := ingress.Update(spec)
	if k8serrors.IsNotFound(err) {
		_, err = ingress.Create(spec)
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteIngress(appName string) error {
	deploymentName := k.deploymentName(appName)
	ingress := k.ExtensionsV1beta1().Ingresses(k.namespace)
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
	pods := k.CoreV1().Pods(k.namespace)
	w, err := pods.Watch(v1.ListOptions{
		LabelSelector: applicationSelector(appName),
		Watch:         true,
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
	statefulsets := k.AppsV1().StatefulSets(k.namespace)
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

	deployments := k.AppsV1().Deployments(k.namespace)
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

// WatchOperator returns a watcher which notifies when there
// are changes to the operator of the specified application.
func (k *kubernetesClient) WatchOperator(appName string) (watcher.NotifyWatcher, error) {
	pods := k.CoreV1().Pods(k.namespace)
	w, err := pods.Watch(v1.ListOptions{
		LabelSelector: operatorSelector(appName),
		Watch:         true,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return k.newWatcher(w, appName, k.clock)
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
	pods := k.CoreV1().Pods(k.namespace)
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
		providerId := string(p.UID)
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

func (k *kubernetesClient) volumeInfoForPVC(vol core.Volume, volMount core.VolumeMount, claimName string, now time.Time) (*caas.FilesystemInfo, error) {
	pvClaims := k.CoreV1().PersistentVolumeClaims(k.namespace)
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
		events := k.CoreV1().Events(k.namespace)
		eventList, err := events.List(v1.ListOptions{
			IncludeUninitialized: true,
			FieldSelector:        fields.OneTermEqualSelector("involvedObject.name", pvc.Name).String(),
		})
		if err != nil {
			return nil, errors.Annotate(err, "unable to get events for PVC")
		}
		// Take the most recent event.
		if count := len(eventList.Items); count > 0 {
			statusMessage = eventList.Items[count-1].Message
		}
	}

	pVolumes := k.CoreV1().PersistentVolumes()
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

// Operator returns an Operator with current status and life details.
func (k *kubernetesClient) Operator(appName string) (*caas.Operator, error) {
	pods := k.CoreV1().Pods(k.namespace)
	podsList, err := pods.List(v1.ListOptions{
		LabelSelector: operatorSelector(appName),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(podsList.Items) == 0 {
		return nil, errors.NotFoundf("operator pod for application %q", appName)
	}

	opPod := podsList.Items[0]
	terminated := opPod.DeletionTimestamp != nil
	now := time.Now()
	statusMessage, opStatus, since, err := k.getPODStatus(opPod, now)
	return &caas.Operator{
		Id:    string(opPod.UID),
		Dying: terminated,
		Status: status.StatusInfo{
			Status:  opStatus,
			Message: statusMessage,
			Since:   &since,
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
		events := k.CoreV1().Events(k.namespace)
		eventList, err := events.List(v1.ListOptions{
			IncludeUninitialized: true,
			FieldSelector:        fields.OneTermEqualSelector("involvedObject.name", pod.Name).String(),
		})
		if err != nil {
			return "", "", time.Time{}, errors.Trace(err)
		}
		// Take the most recent event.
		if count := len(eventList.Items); count > 0 {
			statusMessage = eventList.Items[count-1].Message
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
	return k.getStatusFromEvents(ss.Name, jujuStatus)
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
	return k.getStatusFromEvents(deployment.Name, jujuStatus)
}

func (k *kubernetesClient) getStatusFromEvents(parentName string, jujuStatus status.Status) (string, status.Status, error) {
	events := k.CoreV1().Events(k.namespace)
	eventList, err := events.List(v1.ListOptions{
		IncludeUninitialized: true,
		FieldSelector:        fields.OneTermEqualSelector("involvedObject.name", parentName).String(),
	})
	if err != nil {
		return "", "", errors.Trace(err)
	}
	var statusMessage string
	// Take the most recent event.
	if count := len(eventList.Items); count > 0 {
		evt := eventList.Items[count-1]
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
func filesetConfigMap(configMapName string, files *caas.FileSet) *core.ConfigMap {
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

// ensureConfigMap ensures a ConfigMap resource.
func (k *kubernetesClient) ensureConfigMap(configMap *core.ConfigMap) error {
	configMaps := k.CoreV1().ConfigMaps(k.namespace)
	_, err := configMaps.Update(configMap)
	if k8serrors.IsNotFound(err) {
		_, err = configMaps.Create(configMap)
	}
	return errors.Trace(err)
}

// deleteConfigMap deletes a ConfigMap resource.
func (k *kubernetesClient) deleteConfigMap(configMapName string) error {
	err := k.CoreV1().ConfigMaps(k.namespace).Delete(configMapName, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

// createConfigMap creates a ConfigMap resource.
func (k *kubernetesClient) createConfigMap(configMap *core.ConfigMap) error {
	_, err := k.CoreV1().ConfigMaps(k.namespace).Create(configMap)
	return errors.Trace(err)
}

// getConfigMap returns a ConfigMap resource.
func (k *kubernetesClient) getConfigMap(cmName string) (*core.ConfigMap, error) {
	cm, err := k.CoreV1().ConfigMaps(k.namespace).Get(cmName, v1.GetOptions{IncludeUninitialized: true})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errors.NotFoundf("configmap %q", cmName)
		}
		return nil, errors.Trace(err)
	}
	return cm, nil
}

// operatorPod returns a *core.Pod for the operator pod
// of the specified application.
func operatorPod(podName, appName, agentPath, operatorImagePath, version string, annotations k8sannotations.Annotation) *core.Pod {
	configMapName := operatorConfigMapName(podName)
	configVolName := configMapName

	if isLegacyName(podName) {
		configVolName += "-volume"
	}

	appTag := names.NewApplicationTag(appName)
	jujudCmd := fmt.Sprintf("./jujud caasoperator --application-name=%s --debug", appName)

	return &core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: podName,
			Annotations: podAnnotations(annotations.Copy()).
				Add(labelVersion, version).ToMap(),
			Labels: map[string]string{labelOperator: appName},
		},
		Spec: core.PodSpec{
			Containers: []core.Container{{
				Name:            "juju-operator",
				ImagePullPolicy: core.PullIfNotPresent,
				Image:           operatorImagePath,
				WorkingDir:      jujudToolDir,
				Command: []string{
					"/bin/sh",
				},
				Args: []string{
					"-c",
					fmt.Sprintf(caas.JujudStartUpSh, jujudCmd),
				},
				Env: []core.EnvVar{
					{Name: "JUJU_APPLICATION", Value: appName},
				},
				VolumeMounts: []core.VolumeMount{{
					Name:      configVolName,
					MountPath: filepath.Join(agent.Dir(agentPath, appTag), TemplateFileNameAgentConf),
					SubPath:   TemplateFileNameAgentConf,
				}},
			}},
			Volumes: []core.Volume{{
				Name: configVolName,
				VolumeSource: core.VolumeSource{
					ConfigMap: &core.ConfigMapVolumeSource{
						LocalObjectReference: core.LocalObjectReference{
							Name: configMapName,
						},
						Items: []core.KeyToPath{{
							Key:  appName + "-agent.conf",
							Path: TemplateFileNameAgentConf,
						}},
					},
				},
			}},
		},
	}
}

// operatorConfigMap returns a *core.ConfigMap for the operator pod
// of the specified application, with the specified configuration.
func operatorConfigMap(appName, operatorName string, config *caas.OperatorConfig) *core.ConfigMap {
	configMapName := operatorConfigMapName(operatorName)
	return &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name: configMapName,
		},
		Data: map[string]string{
			appName + "-agent.conf": string(config.AgentConf),
		},
	}
}

type unitSpec struct {
	Pod     core.PodSpec `json:"pod"`
	Service *K8sServiceSpec
}

var containerTemplate = `
  - name: {{.Name}}
    {{if .Ports}}
    ports:
    {{- range .Ports }}
        - containerPort: {{.ContainerPort}}
          {{if .Name}}name: {{.Name}}{{end}}
          {{if .Protocol}}protocol: {{.Protocol}}{{end}}
    {{- end}}
    {{end}}
    {{if .Command}}
    command: [{{- range $idx, $c := .Command -}}{{if ne $idx 0}},{{end}}"{{$c}}"{{- end -}}]
    {{end}}
    {{if .Args}}
    args: [{{- range $idx, $a := .Args -}}{{if ne $idx 0}},{{end}}"{{$a}}"{{- end -}}]
    {{end}}
    {{if .WorkingDir}}
    workingDir: {{.WorkingDir}}
    {{end}}
    {{if .Config}}
    env:
    {{- range $k, $v := .Config }}
        - name: {{$k}}
          value: {{$v}}
    {{- end}}
    {{end}}
`

var defaultPodTemplate = fmt.Sprintf(`
pod:
  containers:
  {{- range .Containers }}
%s
  {{- end}}
  {{if .InitContainers}}
  initContainers:
  {{- range .InitContainers }}
%s
  {{- end}}
  {{end}}
`[1:], containerTemplate, containerTemplate)

func makeUnitSpec(appName, deploymentName string, podSpec *caas.PodSpec) (*unitSpec, error) {
	// Fill out the easy bits using a template.
	tmpl := template.Must(template.New("").Parse(defaultPodTemplate))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, podSpec); err != nil {
		return nil, errors.Trace(err)
	}
	unitSpecString := buf.String()

	var unitSpec unitSpec
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(unitSpecString), len(unitSpecString))
	if err := decoder.Decode(&unitSpec); err != nil {
		logger.Errorf("unable to parse %q pod spec: %+v\n%v", appName, *podSpec, unitSpecString)
		return nil, errors.Trace(err)
	}

	// Now fill in the hard bits progamatically.
	if err := populateContainerDetails(deploymentName, &unitSpec.Pod, unitSpec.Pod.Containers, podSpec.Containers); err != nil {
		return nil, errors.Trace(err)
	}
	if err := populateContainerDetails(deploymentName, &unitSpec.Pod, unitSpec.Pod.InitContainers, podSpec.InitContainers); err != nil {
		return nil, errors.Trace(err)
	}

	if podSpec.ProviderPod != nil {
		spec, ok := podSpec.ProviderPod.(*K8sPodSpec)
		if !ok {
			return nil, errors.Errorf("unexpected kubernetes pod spec type %T", podSpec.ProviderPod)
		}
		unitSpec.Pod.ActiveDeadlineSeconds = spec.ActiveDeadlineSeconds
		unitSpec.Pod.ServiceAccountName = spec.ServiceAccountName
		unitSpec.Pod.TerminationGracePeriodSeconds = spec.TerminationGracePeriodSeconds
		unitSpec.Pod.Hostname = spec.Hostname
		unitSpec.Pod.Subdomain = spec.Subdomain
		unitSpec.Pod.DNSConfig = spec.DNSConfig
		unitSpec.Pod.DNSPolicy = spec.DNSPolicy
		unitSpec.Pod.Priority = spec.Priority
		unitSpec.Pod.PriorityClassName = spec.PriorityClassName
		unitSpec.Pod.SecurityContext = spec.SecurityContext
		unitSpec.Pod.RestartPolicy = spec.RestartPolicy
		unitSpec.Pod.AutomountServiceAccountToken = spec.AutomountServiceAccountToken
		unitSpec.Pod.ReadinessGates = spec.ReadinessGates
		unitSpec.Service = spec.Service
	}
	return &unitSpec, nil
}

func boolPtr(b bool) *bool {
	return &b
}

func defaultSecurityContext() *core.SecurityContext {
	// TODO - consider locking this down more but charms will break
	return &core.SecurityContext{
		AllowPrivilegeEscalation: boolPtr(false),
		ReadOnlyRootFilesystem:   boolPtr(false),
		RunAsNonRoot:             boolPtr(false),
	}
}

func populateContainerDetails(deploymentName string, pod *core.PodSpec, podContainers []core.Container, containers []caas.ContainerSpec) error {
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

		if c.ProviderContainer == nil {
			podContainers[i].SecurityContext = defaultSecurityContext()
			continue
		}
		spec, ok := c.ProviderContainer.(*K8sContainerSpec)
		if !ok {
			return errors.Errorf("unexpected kubernetes container spec type %T", c.ProviderContainer)
		}
		podContainers[i].ImagePullPolicy = spec.ImagePullPolicy
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
	statefulsets := k.AppsV1().StatefulSets(k.namespace)
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
