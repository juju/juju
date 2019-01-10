// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/keyvalues"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	k8sstorage "k8s.io/api/storage/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/network"
	"github.com/juju/juju/storage"
)

var logger = loggo.GetLogger("juju.kubernetes.provider")

const (
	labelOperator    = "juju-operator"
	labelStorage     = "juju-storage"
	labelVersion     = "juju-version"
	labelApplication = "juju-application"
	labelModel       = "juju-model"

	defaultOperatorStorageClassName = "juju-operator-storage"

	gpuAffinityNodeSelectorKey = "gpu"
)

var defaultPropagationPolicy = v1.DeletePropagationForeground

type kubernetesClient struct {
	clock jujuclock.Clock
	kubernetes.Interface
	apiextensionsClient apiextensionsclientset.Interface

	// namespace is the k8s namespace to use when
	// creating k8s resources.
	namespace string

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
	cloudSpec environs.CloudSpec,
	cfg *config.Config,
	newClient NewK8sClientFunc,
	newWatcher NewK8sWatcherFunc,
	clock jujuclock.Clock,
) (caas.Broker, error) {
	k8sConfig, err := newK8sConfig(cloudSpec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	k8sClient, apiextensionsClient, err := newClient(k8sConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	newCfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &kubernetesClient{
		clock:               clock,
		Interface:           k8sClient,
		apiextensionsClient: apiextensionsClient,
		namespace:           newCfg.Name(),
		envCfg:              newCfg,
		modelUUID:           newCfg.UUID(),
		newWatcher:          newWatcher,
	}, nil
}

func newK8sConfig(cloudSpec environs.CloudSpec) (*rest.Config, error) {
	if cloudSpec.Credential == nil {
		return nil, errors.Errorf("cloud %v has no credential", cloudSpec.Name)
	}

	var CAData []byte
	for _, cacert := range cloudSpec.CACertificates {
		CAData = append(CAData, cacert...)
	}

	credentialAttrs := cloudSpec.Credential.Attributes()
	return &rest.Config{
		Host:     cloudSpec.Endpoint,
		Username: credentialAttrs[CredAttrUsername],
		Password: credentialAttrs[CredAttrPassword],
		TLSClientConfig: rest.TLSClientConfig{
			CertData: []byte(credentialAttrs[CredAttrClientCertificateData]),
			KeyData:  []byte(credentialAttrs[CredAttrClientKeyData]),
			CAData:   CAData,
		},
	}, nil
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
	k.envCfg = newCfg
	return nil
}

// PrepareForBootstrap prepares for bootstraping a controller.
func (k *kubernetesClient) PrepareForBootstrap(ctx environs.BootstrapContext) error {
	return nil
}

const regionLabelName = "failure-domain.beta.kubernetes.io/region"

// ListHostCloudRegions lists all the cloud regions that this cluster has worker nodes/instances running in.
func (k *kubernetesClient) ListHostCloudRegions() (set.Strings, error) {
	// we only check 5 worker nodes as of now just run in the one region and
	// we are just looking for a running worker to sniff its region.
	nodes, err := k.CoreV1().Nodes().List(v1.ListOptions{Limit: 5})
	if err != nil {
		return nil, errors.Annotate(err, "listing nodes")
	}
	result := set.NewStrings()
	for _, n := range nodes.Items {
		var cloudRegion, v string
		var ok bool
		if v = getCloudProviderFromNodeMeta(n); v == "" {
			continue
		}
		cloudRegion += v
		if v, ok = n.Labels[regionLabelName]; !ok || v == "" {
			continue
		}
		cloudRegion += "/" + v
		result.Add(cloudRegion)
	}
	return result, nil
}

// Bootstrap deploys controller with mongoDB together into k8s cluster.
func (k *kubernetesClient) Bootstrap(ctx environs.BootstrapContext, callCtx context.ProviderCallContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	const (
		// TODO(caas): how to get these from oci path.
		Series = "bionic"
		Arch   = arch.AMD64
	)

	finalizer := func(ctx environs.BootstrapContext, pcfg *podcfg.ControllerPodConfig, opts environs.BootstrapDialOpts) error {
		envConfig := k.Config()
		if err := podcfg.FinishControllerPodConfig(pcfg, envConfig); err != nil {
			return errors.Trace(err)
		}

		if err := pcfg.VerifyConfig(); err != nil {
			return errors.Trace(err)
		}

		// prepare bootstrapParamsFile
		bootstrapParamsFileContent, err := pcfg.Bootstrap.StateInitializationParams.Marshal()
		if err != nil {
			return errors.Trace(err)
		}
		logger.Debugf("bootstrapParams file content: \n%s", string(bootstrapParamsFileContent))

		// TODO(caas): we'll need a different tag type other than machine tag.
		machineTag := names.NewMachineTag(pcfg.MachineId)
		acfg, err := pcfg.AgentConfig(machineTag)
		if err != nil {
			return errors.Trace(err)
		}
		agentConfigFileContent, err := acfg.Render()
		if err != nil {
			return errors.Trace(err)
		}
		logger.Debugf("agentConfig file content: \n%s", string(agentConfigFileContent))

		// TODO(caas): prepare
		// agent.conf,
		// bootstrap-params,
		// server.pem,
		// system-identity,
		// shared-secret, then generate configmap/secret.
		// Lastly, create StatefulSet for controller.
		return nil
	}
	return &environs.BootstrapResult{
		Arch:                   Arch,
		Series:                 Series,
		CaasBootstrapFinalizer: finalizer,
	}, nil
}

// DestroyController implements the Environ interface.
func (k *kubernetesClient) DestroyController(ctx context.ProviderCallContext, controllerUUID string) error {
	// TODO(caas): destroy controller and all models
	logger.Warningf("DestroyController is not supported yet on CAAS.")
	return nil
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
			_, err := k.GetNamespace("")
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

// Namespaces returns names of the namespaces on the cluster.
func (k *kubernetesClient) Namespaces() ([]string, error) {
	namespaces := k.CoreV1().Namespaces()
	ns, err := namespaces.List(v1.ListOptions{IncludeUninitialized: true})
	if err != nil {
		return nil, errors.Annotate(err, "listing namespaces")
	}
	result := make([]string, len(ns.Items))
	for i, n := range ns.Items {
		result[i] = n.Name
	}
	return result, nil
}

// GetNamespace returns the namespace for the specified name or current namespace.
func (k *kubernetesClient) GetNamespace(name string) (*core.Namespace, error) {
	if name == "" {
		name = k.namespace
	}
	ns, err := k.CoreV1().Namespaces().Get(name, v1.GetOptions{IncludeUninitialized: true})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("namespace %q", name)
	}
	if err != nil {
		return nil, errors.Annotate(err, "getting namespaces")
	}
	return ns, nil
}

// EnsureNamespace ensures this broker's namespace is created.
func (k *kubernetesClient) EnsureNamespace() error {
	ns := &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: k.namespace}}
	namespaces := k.CoreV1().Namespaces()
	_, err := namespaces.Update(ns)
	if k8serrors.IsNotFound(err) {
		_, err = namespaces.Create(ns)
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteNamespace() error {
	// deleteNamespace is used as a means to implement Destroy().
	// All model resources are provisioned in the namespace;
	// deleting the namespace will also delete those resources.
	err := k.CoreV1().Namespaces().Delete(k.namespace, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

// WatchNamespace returns a watcher which notifies when there
// are changes to current namespace.
func (k *kubernetesClient) WatchNamespace() (watcher.NotifyWatcher, error) {
	w, err := k.CoreV1().Namespaces().Watch(
		v1.ListOptions{
			FieldSelector:        fields.OneTermEqualSelector("metadata.name", k.namespace).String(),
			IncludeUninitialized: true,
		},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return k.newWatcher(w, k.namespace, k.clock)
}

// EnsureSecret ensures a secret exists for use with retrieving images from private registries
func (k *kubernetesClient) ensureSecret(imageSecretName, appName string, imageDetails *caas.ImageDetails, resourceTags map[string]string) error {
	if imageDetails.Password == "" {
		return errors.New("attempting to create a secret with no password")
	}
	secretData, err := createDockerConfigJSON(imageDetails)
	if err != nil {
		return errors.Trace(err)
	}
	secrets := k.CoreV1().Secrets(k.namespace)

	newSecret := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      imageSecretName,
			Namespace: k.namespace,
			Labels:    resourceTags},
		Type: core.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			core.DockerConfigJsonKey: secretData,
		},
	}

	_, err = secrets.Update(newSecret)
	if k8serrors.IsNotFound(err) {
		_, err = secrets.Create(newSecret)
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteSecret(imageSecretName string) error {
	secrets := k.CoreV1().Secrets(k.namespace)
	err := secrets.Delete(imageSecretName, &v1.DeleteOptions{
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
	_, err := statefulsets.Get(operatorName(appName), v1.GetOptions{IncludeUninitialized: true})
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

	// TODO(caas) - this is a stop gap until we implement a CAAS model manager worker
	// First up, ensure the namespace eis there if not already created.
	if err := k.EnsureNamespace(); err != nil {
		return errors.Annotatef(err, "ensuring operator namespace %v", k.namespace)
	}

	// TODO(caas) use secrets for storing agent password?
	if config.AgentConf == nil {
		// We expect that the config map already exists,
		// so make sure it does.
		configMaps := k.CoreV1().ConfigMaps(k.namespace)
		_, err := configMaps.Get(operatorConfigMapName(appName), v1.GetOptions{IncludeUninitialized: true})
		if err != nil {
			return errors.Annotatef(err, "config map for %q should already exist", appName)
		}
	} else {
		if err := k.ensureConfigMap(operatorConfigMap(appName, config)); err != nil {
			return errors.Annotate(err, "creating or updating ConfigMap")
		}
	}

	storageTags := make(map[string]string)
	for k, v := range config.CharmStorage.ResourceTags {
		storageTags[k] = v
	}
	storageTags[labelOperator] = appName

	tags := make(map[string]string)
	for k, v := range config.ResourceTags {
		tags[k] = v
	}
	tags[labelOperator] = appName

	// Set up the parameters for creating charm storage.
	volStorageLabel := fmt.Sprintf("%s-operator-storage", appName)
	params := volumeParams{
		storageConfig:       &storageConfig{existingStorageClass: defaultOperatorStorageClassName},
		storageLabels:       []string{volStorageLabel, k.namespace, "default"},
		pvcName:             operatorVolumeClaim(appName),
		requestedVolumeSize: fmt.Sprintf("%dMi", config.CharmStorage.Size),
	}
	if config.CharmStorage.Provider != K8s_ProviderType {
		return errors.Errorf("expected charm storage provider %q, got %q", K8s_ProviderType, config.CharmStorage.Provider)
	}
	if storageLabel, ok := config.CharmStorage.Attributes[storageLabel]; ok {
		params.storageLabels = append([]string{fmt.Sprintf("%v", storageLabel)}, params.storageLabels...)
	}
	var err error
	params.storageConfig, err = newStorageConfig(config.CharmStorage.Attributes, defaultOperatorStorageClassName)
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
			Name:   params.pvcName,
			Labels: storageTags},
		Spec: *pvcSpec,
	}
	pod := operatorPod(appName, agentPath, config.OperatorImagePath, config.Version.String(), tags)
	// Take a copy for use with statefulset.
	podWithoutStorage := pod

	numPods := int32(1)
	logger.Debugf("using persistent volume claim for operator %s: %+v", appName, pvc)
	statefulset := &apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   operatorName(appName),
			Labels: pod.Labels},
		Spec: apps.StatefulSetSpec{
			Replicas: &numPods,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{labelOperator: appName},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: pod.Labels,
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

// maybeGetStorageClass looks for a storage class to use when creating
// a persistent volume, using the specified name (if supplied), or a class
// matching the specified labels.
func (k *kubernetesClient) maybeGetStorageClass(labels ...string) (*k8sstorage.StorageClass, error) {
	// First try looking for a storage class with a Juju label.
	selector := fmt.Sprintf("%v in (%v)", labelStorage, strings.Join(labels, ", "))
	modelTerm := fmt.Sprintf("%s==%s", labelModel, k.namespace)
	modelSelector := selector + "," + modelTerm

	// Attempt to get a storage class tied to this model.
	storageClasses, err := k.StorageV1().StorageClasses().List(v1.ListOptions{
		LabelSelector: modelSelector,
	})
	if err != nil {
		return nil, errors.Annotatef(err, "looking for existing storage class with selector %q", modelSelector)
	}

	// If no storage classes tied to this model, look for a non-model specific
	// storage class with the relevant labels.
	if len(storageClasses.Items) == 0 {
		storageClasses, err = k.StorageV1().StorageClasses().List(v1.ListOptions{
			LabelSelector: selector,
		})
		if err != nil {
			return nil, errors.Annotatef(err, "looking for existing storage class with selector %q", modelSelector)
		}
	}
	logger.Debugf("available storage classes: %v", storageClasses.Items)
	// For now, pick the first matching storage class.
	if len(storageClasses.Items) > 0 {
		return &storageClasses.Items[0], nil
	}

	// Second look for the cluster default storage class, if defined.
	storageClasses, err = k.StorageV1().StorageClasses().List(v1.ListOptions{})
	if err != nil {
		return nil, errors.Annotate(err, "listing storage classes")
	}
	for _, sc := range storageClasses.Items {
		if v, ok := sc.Annotations["storageclass.kubernetes.io/is-default-class"]; ok && v != "false" {
			logger.Debugf("using default storage class: %v", sc.Name)
			return &sc, nil
		}
	}
	return nil, errors.NotFoundf("storage class for any %q", labels)
}

func operatorVolumeClaim(appName string) string {
	return fmt.Sprintf("%v-operator-volume", appName)
}

type volumeParams struct {
	storageLabels       []string
	storageConfig       *storageConfig
	pvcName             string
	requestedVolumeSize string
	accessMode          core.PersistentVolumeAccessMode
}

// maybeGetVolumeClaimSpec returns a persistent volume claim spec for the given
// parameters. If no suitable storage class is available, return a NotFound error.
func (k *kubernetesClient) maybeGetVolumeClaimSpec(params volumeParams) (*core.PersistentVolumeClaimSpec, error) {
	storageClassName := params.storageConfig.storageClass
	existingStorageClassName := params.storageConfig.existingStorageClass
	haveStorageClass := false
	// If no specific storage class has been specified but there's a default
	// fallback one, try and look for that first.
	if storageClassName == "" && existingStorageClassName != "" {
		sc, err := k.getStorageClass(existingStorageClassName)
		if err != nil && !k8serrors.IsNotFound(err) {
			return nil, errors.Annotatef(err, "looking for existing storage class %q", existingStorageClassName)
		}
		if err == nil {
			haveStorageClass = true
			storageClassName = sc.Name
		}
	}
	// If no storage class has been found or asked for,
	// look for one by matching labels.
	if storageClassName == "" && !haveStorageClass {
		sc, err := k.maybeGetStorageClass(params.storageLabels...)
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		if err == nil {
			haveStorageClass = true
			storageClassName = sc.Name
		}
	}
	// If a specific storage class has been requested, make sure it exists.
	if storageClassName != "" && !haveStorageClass {
		params.storageConfig.storageClass = storageClassName
		sc, err := k.ensureStorageClass(params.storageConfig)
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
			"cannot create persistent volume as no storage class matching %q exists and no default storage class is defined",
			params.storageLabels))
	}
	accessMode := params.accessMode
	if accessMode == "" {
		accessMode = core.ReadWriteOnce
	}
	fsSize, err := resource.ParseQuantity(params.requestedVolumeSize)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid volume size %v", params.requestedVolumeSize)
	}
	return &core.PersistentVolumeClaimSpec{
		StorageClassName: &storageClassName,
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceStorage: fsSize,
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

func (k *kubernetesClient) ensureStorageClass(cfg *storageConfig) (*k8sstorage.StorageClass, error) {
	// First see if the named storage class exists.
	sc, err := k.getStorageClass(cfg.storageClass)
	if err == nil {
		return sc, nil
	}
	if !k8serrors.IsNotFound(err) {
		return nil, errors.Annotatef(err, "getting storage class %q", cfg.storageClass)
	}
	// If it's not found but there's no provisioner specified, we can't
	// create it so just return not found.
	if err != nil && cfg.storageProvisioner == "" {
		return nil, errors.NewNotFound(nil,
			fmt.Sprintf("storage class %q doesn't exist, but no storage provisioner has been specified",
				cfg.storageClass))
	}

	// Create the storage class with the specified provisioner.
	storageClasses := k.StorageV1().StorageClasses()
	sc, err = storageClasses.Create(&k8sstorage.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name:   qualifiedStorageClassName(k.namespace, cfg.storageClass),
			Labels: map[string]string{labelModel: k.namespace},
		},
		Provisioner:   cfg.storageProvisioner,
		ReclaimPolicy: &cfg.reclaimPolicy,
		Parameters:    cfg.parameters,
	})
	return sc, errors.Annotatef(err, "creating storage class %q", cfg.storageClass)
}

// DeleteOperator deletes the specified operator.
func (k *kubernetesClient) DeleteOperator(appName string) (err error) {
	logger.Debugf("deleting %s operator", appName)

	// First delete the config map(s).
	configMaps := k.CoreV1().ConfigMaps(k.namespace)
	configMapName := operatorConfigMapName(appName)
	err = configMaps.Delete(configMapName, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil
	}
	configMapName = operatorConfigurationsConfigMapName(appName)
	err = configMaps.Delete(configMapName, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil
	}

	// Finally the operator itself.
	operatorName := operatorName(appName)
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

	pvs := k.CoreV1().PersistentVolumes()
	for _, p := range podsList.Items {
		// Delete secrets.
		for _, c := range p.Spec.Containers {
			secretName := appSecretName(appName, c.Name)
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

// Service returns the service for the specified application.
func (k *kubernetesClient) Service(appName string) (*caas.Service, error) {
	services := k.CoreV1().Services(k.namespace)
	servicesList, err := services.List(v1.ListOptions{
		LabelSelector: applicationSelector(appName),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(servicesList.Items) == 0 {
		return nil, errors.NotFoundf("service for %q", appName)
	}
	service := servicesList.Items[0]
	result := caas.Service{
		Id: string(service.UID),
	}
	if service.Spec.ClusterIP != "" {
		result.Addresses = append(result.Addresses, network.Address{
			Value: service.Spec.ClusterIP,
			Type:  network.DeriveAddressType(service.Spec.ClusterIP),
			Scope: network.ScopeCloudLocal,
		})
	}
	if service.Spec.LoadBalancerIP != "" {
		result.Addresses = append(result.Addresses, network.Address{
			Value: service.Spec.LoadBalancerIP,
			Type:  network.DeriveAddressType(service.Spec.LoadBalancerIP),
			Scope: network.ScopePublic,
		})
	}
	for _, addr := range service.Spec.ExternalIPs {
		result.Addresses = append(result.Addresses, network.Address{
			Value: addr,
			Type:  network.DeriveAddressType(addr),
			Scope: network.ScopePublic,
		})
	}
	return &result, nil
}

// DeleteService deletes the specified service.
func (k *kubernetesClient) DeleteService(appName string) (err error) {
	logger.Debugf("deleting application %s", appName)

	if err := k.deleteService(appName); err != nil {
		return errors.Trace(err)
	}
	deploymentName := deploymentName(appName)
	if err := k.deleteStatefulSet(deploymentName); err != nil {
		return errors.Trace(err)
	}
	if err := k.deleteDeployment(deploymentName); err != nil {
		return errors.Trace(err)
	}
	pods := k.CoreV1().Pods(k.namespace)
	podsList, err := pods.List(v1.ListOptions{
		LabelSelector: applicationSelector(appName),
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, p := range podsList.Items {
		if _, err := k.deleteVolumeClaims(appName, &p); err != nil {
			return errors.Trace(err)
		}
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
	for _, t := range podSpec.CustomResourceDefinitions {
		crd, err := k.ensureCustomResourceDefinitionTemplate(&t)
		if err != nil {
			return errors.Annotate(err, fmt.Sprintf("ensure custom resource definition %q", t.Kind))
		}
		logger.Debugf("ensured custom resource definition %q", crd.ObjectMeta.Name)
	}
	return nil
}

func (k *kubernetesClient) ensureCustomResourceDefinitionTemplate(t *caas.CustomResourceDefinition) (
	crd *apiextensionsv1beta1.CustomResourceDefinition, err error) {
	singularName := strings.ToLower(t.Kind)
	pluralName := fmt.Sprintf("%ss", singularName)
	crdFullName := fmt.Sprintf("%s.%s", pluralName, t.Group)
	crdIn := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:      crdFullName,
			Namespace: k.namespace,
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   t.Group,
			Version: t.Version,
			Scope:   apiextensionsv1beta1.ResourceScope(t.Scope),
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Plural:   pluralName,
				Kind:     t.Kind,
				Singular: singularName,
			},
			Validation: &apiextensionsv1beta1.CustomResourceValidation{
				OpenAPIV3Schema: &apiextensionsv1beta1.JSONSchemaProps{
					Properties: t.Validation.Properties,
				},
			},
		},
	}
	apiextensionsV1beta1 := k.apiextensionsClient.ApiextensionsV1beta1()
	logger.Debugf("creating crd %#v", crdIn)
	crd, err = apiextensionsV1beta1.CustomResourceDefinitions().Create(crdIn)
	if k8serrors.IsAlreadyExists(err) {
		crd, err = apiextensionsV1beta1.CustomResourceDefinitions().Get(crdFullName, v1.GetOptions{})
		resourceVersion := crd.ObjectMeta.GetResourceVersion()
		crdIn.ObjectMeta.SetResourceVersion(resourceVersion)
		logger.Debugf("existing crd with resource version %q found, so update it %#v", resourceVersion, crdIn)
		crd, err = apiextensionsV1beta1.CustomResourceDefinitions().Update(crdIn)
	}
	return
}

// EnsureService creates or updates a service for pods with the given params.
func (k *kubernetesClient) EnsureService(
	appName string, statusCallback caas.StatusCallbackFunc, params *caas.ServiceParams, numUnits int, config application.ConfigAttributes,
) (err error) {
	defer func() {
		if err != nil {
			statusCallback(appName, status.Error, err.Error(), nil)
		}
	}()

	logger.Debugf("creating/updating application %s", appName)

	if numUnits < 0 {
		return errors.Errorf("number of units must be >= 0")
	}
	if numUnits == 0 {
		return k.deleteAllPods(appName)
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

	unitSpec, err := makeUnitSpec(appName, params.PodSpec)
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
	if params.Placement != "" {
		affinityLabels, err := keyvalues.Parse(strings.Split(params.Placement, ","), false)
		if err != nil {
			return errors.Annotatef(err, "invalid placement directive %q", params.Placement)
		}
		unitSpec.Pod.NodeSelector = affinityLabels
	}

	resourceTags := make(map[string]string)
	for k, v := range params.ResourceTags {
		resourceTags[k] = v
	}
	resourceTags[labelApplication] = appName
	for _, c := range params.PodSpec.Containers {
		if c.ImageDetails.Password == "" {
			continue
		}
		imageSecretName := appSecretName(appName, c.Name)
		if err := k.ensureSecret(imageSecretName, appName, &c.ImageDetails, resourceTags); err != nil {
			return errors.Annotatef(err, "creating secrets for container: %s", c.Name)
		}
		cleanups = append(cleanups, func() { k.deleteSecret(imageSecretName) })
	}

	// Add a deployment controller or stateful set configured to create the specified number of units/pods.
	// Defensively check to see if a stateful set is already used.
	useStatefulSet := len(params.Filesystems) > 0
	if !useStatefulSet {
		statefulsets := k.AppsV1().StatefulSets(k.namespace)
		_, err := statefulsets.Get(deploymentName(appName), v1.GetOptions{IncludeUninitialized: true})
		if err != nil && !k8serrors.IsNotFound(err) {
			return errors.Trace(err)
		}
		useStatefulSet = err == nil
		if useStatefulSet {
			logger.Debugf("no updated filesystems but already using stateful set for %v", appName)
		}
	}

	numPods := int32(numUnits)
	if useStatefulSet {
		if err := k.configureStatefulSet(appName, resourceTags, unitSpec, params.PodSpec.Containers, &numPods, params.Filesystems); err != nil {
			return errors.Annotate(err, "creating or updating StatefulSet")
		}
		cleanups = append(cleanups, func() { k.deleteDeployment(appName) })
	} else {
		if err := k.configureDeployment(appName, deploymentName(appName), resourceTags, unitSpec, params.PodSpec.Containers, &numPods); err != nil {
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
		if err := k.configureService(appName, ports, resourceTags, config); err != nil {
			return errors.Annotatef(err, "creating or updating service for %v", appName)
		}
	}
	return nil
}

func (k *kubernetesClient) deleteAllPods(appName string) error {
	zero := int32(0)
	statefulsets := k.AppsV1().StatefulSets(k.namespace)
	statefulSet, err := statefulsets.Get(deploymentName(appName), v1.GetOptions{IncludeUninitialized: true})
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if err == nil {
		statefulSet.Spec.Replicas = &zero
		_, err = statefulsets.Update(statefulSet)
		return errors.Trace(err)
	}

	deployments := k.AppsV1().Deployments(k.namespace)
	deployment, err := deployments.Get(deploymentName(appName), v1.GetOptions{IncludeUninitialized: true})
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
	podSpec *core.PodSpec, statefulSet *apps.StatefulSetSpec, appName string, filesystems []storage.KubernetesFilesystemParams,
) error {
	baseDir, err := paths.StorageDir("kubernetes")
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("configuring pod filesystems: %+v", filesystems)
	for i, fs := range filesystems {
		if fs.Provider != K8s_ProviderType {
			return errors.Errorf("invalid storage provider type %q for %v", fs.Provider, fs.StorageName)
		}
		var mountPath string
		if fs.Attachment != nil {
			mountPath = fs.Attachment.Path
		}
		if mountPath == "" {
			mountPath = fmt.Sprintf("%s/fs/%s/%s/%d", baseDir, appName, fs.StorageName, i)
		}
		pvcNamePrefix := fmt.Sprintf("juju-%s-%d", fs.StorageName, i)
		volStorageLabel := fmt.Sprintf("%s-unit-storage", appName)
		params := volumeParams{
			storageLabels:       []string{volStorageLabel, k.namespace, "default"},
			pvcName:             pvcNamePrefix,
			requestedVolumeSize: fmt.Sprintf("%dMi", fs.Size),
		}
		if storageLabel, ok := fs.Attributes[storageLabel]; ok {
			params.storageLabels = append([]string{fmt.Sprintf("%v", storageLabel)}, params.storageLabels...)
		}
		params.storageConfig, err = newStorageConfig(fs.Attributes, defaultStorageClass)
		if err != nil {
			return errors.Annotatef(err, "invalid storage configuration for %v", fs.StorageName)
		}

		pvcSpec, err := k.maybeGetVolumeClaimSpec(params)
		if err != nil {
			return errors.Annotatef(err, "finding volume for %s", fs.StorageName)
		}
		tags := make(map[string]string)
		for k, v := range fs.ResourceTags {
			tags[k] = v
		}
		tags[labelApplication] = appName
		pvc := core.PersistentVolumeClaim{
			ObjectMeta: v1.ObjectMeta{
				Name:   params.pvcName,
				Labels: tags},
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

func (k *kubernetesClient) configureDeployment(
	appName, deploymentName string, labels map[string]string, unitSpec *unitSpec, containers []caas.ContainerSpec, replicas *int32,
) error {
	logger.Debugf("creating/updating deployment for %s", appName)

	// Add the specified file to the pod spec.
	cfgName := func(fileSetName string) string {
		return applicationConfigMapName(appName, fileSetName)
	}
	podSpec := unitSpec.Pod
	if err := k.configurePodFiles(&podSpec, containers, cfgName); err != nil {
		return errors.Trace(err)
	}

	deployment := &apps.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   deploymentName,
			Labels: labels},
		Spec: apps.DeploymentSpec{
			Replicas: replicas,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{labelApplication: appName},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: deploymentName + "-",
					Labels:       labels,
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
	appName string, labels map[string]string, unitSpec *unitSpec,
	containers []caas.ContainerSpec, replicas *int32, filesystems []storage.KubernetesFilesystemParams,
) error {
	logger.Debugf("creating/updating stateful set for %s", appName)

	// Add the specified file to the pod spec.
	cfgName := func(fileSetName string) string {
		return applicationConfigMapName(appName, fileSetName)
	}
	statefulset := &apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   deploymentName(appName),
			Labels: labels},
		Spec: apps.StatefulSetSpec{
			Replicas: replicas,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{labelApplication: appName},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: labels,
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
	if err := k.configureStorage(&podSpec, &statefulset.Spec, appName, filesystems); err != nil {
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
	existing.Spec.Replicas = spec.Spec.Replicas
	existing.Spec.Template.Spec.Containers = existingPodSpec.Containers
	_, err = statefulsets.Update(existing)
	return errors.Trace(err)
}

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
		valid := volMount.Name == operatorVolumeClaim(appName) || jujuPVNameRegexp.MatchString(volMount.Name)
		if !valid {
			logger.Debugf("ignoring non-Juju attachment %q", volMount.Name)
			continue
		}

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
	appName string, containerPorts []core.ContainerPort,
	tags map[string]string, config application.ConfigAttributes,
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
	service := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   deploymentName(appName),
			Labels: tags},
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
	return k.ensureService(service)
}

func (k *kubernetesClient) ensureService(spec *core.Service) error {
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

func (k *kubernetesClient) deleteService(appName string) error {
	services := k.CoreV1().Services(k.namespace)
	err := services.Delete(deploymentName(appName), &v1.DeleteOptions{
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

	svc, err := k.CoreV1().Services(k.namespace).Get(deploymentName(appName), v1.GetOptions{})
	if err != nil {
		return errors.Trace(err)
	}
	if len(svc.Spec.Ports) == 0 {
		return errors.Errorf("cannot create ingress rule for service %q without a port", svc.Name)
	}
	spec := &v1beta1.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Name:   deploymentName(appName),
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
	ingress := k.ExtensionsV1beta1().Ingresses(k.namespace)
	err := ingress.Delete(deploymentName(appName), &v1.DeleteOptions{
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

// jujuPVNameRegexp matches how Juju labels persistent volumes.
// The pattern is: juju-<storagename>-<digit>
var jujuPVNameRegexp = regexp.MustCompile(`^juju-(?P<storageName>\D+)-\d+$`)

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
		unitInfo := caas.Unit{
			Id:      string(p.UID),
			Address: p.Status.PodIP,
			Ports:   ports,
			Dying:   terminated,
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
		pVolumes := k.CoreV1().PersistentVolumes()

		// Gather info about how filesystems are attached/mounted to the pod.
		// The mount name represents the filesystem tag name used by Juju.
		for _, volMount := range p.Spec.Containers[0].VolumeMounts {
			valid := jujuPVNameRegexp.MatchString(volMount.Name)
			if !valid {
				logger.Debugf("ignoring non-Juju attachment %q", volMount.Name)
				continue
			}
			storageName := jujuPVNameRegexp.ReplaceAllString(volMount.Name, "$storageName")

			vol, ok := volumesByName[volMount.Name]
			if !ok {
				logger.Warningf("volume for volume mount %q not found", volMount.Name)
				continue
			}
			if vol.PersistentVolumeClaim == nil || vol.PersistentVolumeClaim.ClaimName == "" {
				// Ignore volumes which are not Juju managed filesystems.
				logger.Debugf("Ignoring blank PersistentVolumeClaim or ClaimName")
				continue
			}
			pvClaims := k.CoreV1().PersistentVolumeClaims(k.namespace)
			pvc, err := pvClaims.Get(vol.PersistentVolumeClaim.ClaimName, v1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				// Ignore claims which don't exist (yet).
				continue
			}
			if err != nil {
				return nil, errors.Annotate(err, "unable to get persistent volume claim")
			}

			if pvc.Status.Phase == core.ClaimPending {
				logger.Debugf(fmt.Sprintf("PersistentVolumeClaim for %v is pending", vol.PersistentVolumeClaim.ClaimName))
				continue
			}
			pv, err := pVolumes.Get(pvc.Spec.VolumeName, v1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				// Ignore volumes which don't exist (yet).
				continue
			}
			if err != nil {
				return nil, errors.Annotate(err, "unable to get persistent volume")
			}

			statusMessage := ""
			since = now
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

			unitInfo.FilesystemInfo = append(unitInfo.FilesystemInfo, caas.FilesystemInfo{
				StorageName:  storageName,
				Size:         uint64(vol.PersistentVolumeClaim.Size()),
				FilesystemId: pvc.Name,
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
			})
		}
		units = append(units, unitInfo)
	}
	return units, nil
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

func (k *kubernetesClient) ensureConfigMap(configMap *core.ConfigMap) error {
	configMaps := k.CoreV1().ConfigMaps(k.namespace)
	_, err := configMaps.Update(configMap)
	if k8serrors.IsNotFound(err) {
		_, err = configMaps.Create(configMap)
	}
	return errors.Trace(err)
}

// operatorPod returns a *core.Pod for the operator pod
// of the specified application.
func operatorPod(appName, agentPath, operatorImagePath, version string, tags map[string]string) *core.Pod {
	podName := operatorName(appName)
	configMapName := operatorConfigMapName(appName)
	configVolName := configMapName + "-volume"

	appTag := names.NewApplicationTag(appName)
	podLabels := make(map[string]string)
	for k, v := range tags {
		podLabels[k] = v
	}
	podLabels[labelVersion] = version
	return &core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:   podName,
			Labels: podLabels,
		},
		Spec: core.PodSpec{
			Containers: []core.Container{{
				Name:            "juju-operator",
				ImagePullPolicy: core.PullIfNotPresent,
				Image:           operatorImagePath,
				Env: []core.EnvVar{
					{Name: "JUJU_APPLICATION", Value: appName},
				},
				VolumeMounts: []core.VolumeMount{{
					Name:      configVolName,
					MountPath: filepath.Join(agent.Dir(agentPath, appTag), "template-agent.conf"),
					SubPath:   "template-agent.conf",
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
							Path: "template-agent.conf",
						}},
					},
				},
			}},
		},
	}
}

// operatorConfigMap returns a *core.ConfigMap for the operator pod
// of the specified application, with the specified configuration.
func operatorConfigMap(appName string, config *caas.OperatorConfig) *core.ConfigMap {
	configMapName := operatorConfigMapName(appName)
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
	Pod core.PodSpec `json:"pod"`
}

var defaultPodTemplate = `
pod:
  containers:
  {{- range .Containers }}
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
  {{- end}}
`[1:]

func makeUnitSpec(appName string, podSpec *caas.PodSpec) (*unitSpec, error) {
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

	var imageSecretNames []core.LocalObjectReference
	// Now fill in the hard bits progamatically.
	for i, c := range podSpec.Containers {
		if c.Image != "" {
			logger.Warningf("Image parameter deprecated, use ImageDetails")
			unitSpec.Pod.Containers[i].Image = c.Image
		} else {
			unitSpec.Pod.Containers[i].Image = c.ImageDetails.ImagePath
		}
		if c.ImageDetails.Password != "" {
			imageSecretNames = append(imageSecretNames, core.LocalObjectReference{Name: appSecretName(appName, c.Name)})
		}

		if c.ProviderContainer == nil {
			continue
		}
		spec, ok := c.ProviderContainer.(*K8sContainerSpec)
		if !ok {
			return nil, errors.Errorf("unexpected kubernetes container spec type %T", c.ProviderContainer)
		}
		unitSpec.Pod.Containers[i].ImagePullPolicy = spec.ImagePullPolicy
		if spec.LivenessProbe != nil {
			unitSpec.Pod.Containers[i].LivenessProbe = spec.LivenessProbe
		}
		if spec.ReadinessProbe != nil {
			unitSpec.Pod.Containers[i].ReadinessProbe = spec.ReadinessProbe
		}
	}
	unitSpec.Pod.ImagePullSecrets = imageSecretNames
	return &unitSpec, nil
}

func operatorName(appName string) string {
	return "juju-operator-" + appName
}

func operatorConfigMapName(appName string) string {
	return operatorName(appName) + "-config"
}

func operatorConfigurationsConfigMapName(appName string) string {
	return deploymentName(appName) + "-configurations-config"
}

func applicationConfigMapName(appName, fileSetName string) string {
	return fmt.Sprintf("%v-%v-config", deploymentName(appName), fileSetName)
}

func deploymentName(appName string) string {
	return "juju-" + appName
}

func appSecretName(appName, containerName string) string {
	// A pod may have multiple containers with different images and thus different secrets
	return "juju-" + appName + "-" + containerName + "-secret"
}

func qualifiedStorageClassName(namespace, storageClass string) string {
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
