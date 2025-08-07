// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/loggo"
	"github.com/juju/version/v2"
	"github.com/kr/pretty"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/pointer"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/pebble"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	"github.com/juju/juju/caas/kubernetes/provider/storage"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	k8swatcher "github.com/juju/juju/caas/kubernetes/provider/watcher"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/juju/osenv"
	jujustorage "github.com/juju/juju/storage"
)

var logger = loggo.GetLogger("juju.kubernetes.provider.application")

const (
	containerAgentPebblePort = "38812"

	// containerProbeInitialDelay is the initial delay in seconds before the probe starts.
	containerProbeInitialDelay = 30
	// containerProbeTimeout is the timeout for the probe to complete in seconds.
	containerProbeTimeout = 1
	// containerProbePeriod is the number of seconds between each probe.
	containerProbePeriod = 5
	// containerProbeSuccess is the number of successful probes to mark the check as healthy.
	containerProbeSuccess = 1
	// containerLivenessProbeFailure is the number of failed liveness probes to mark the check as unhealthy.
	containerLivenessProbeFailure = 3
	// 	containerReadinessProbeFailure = 1 is the number of failed readiness probes to mark the check as unhealthy.
	containerReadinessProbeFailure = 1
	// containerStartupProbeFailure is the number of failed startup probes to mark the check as unhealthy.
	containerStartupProbeFailure = 1
	// containerStartupProbeFailureV36 is the number of failed startup probes to mark the check as unhealthy.
	// This is used for juju v3.6-beta3+.
	containerStartupProbeFailureV36 = 30
	// containerStartupProbePeriodV36 is the number of seconds between each startup probe.
	// This is used for juju v3.6-beta3+.
	containerStartupProbePeriodV36 = 1
)

var (
	containerAgentPebbleVersion   = version.MustParse("2.9.37")
	profileDirVersion             = version.MustParse("3.5-beta1")
	pebbleCopyOnceVersion         = version.MustParse("3.5-beta1")
	pebbleIdentitiesVersion       = version.MustParse("3.6-beta2")
	startupProbeVersion           = version.MustParse("3.6-beta3")
	charmContainerResourceVersion = version.MustParse("3.6.8")
)

type app struct {
	name           string
	namespace      string
	modelUUID      string
	modelName      string
	labelVersion   constants.LabelVersion
	deploymentType caas.DeploymentType
	coreClient     kubernetes.Interface
	extendedClient apiextensionsclientset.Interface
	newWatcher     k8swatcher.NewK8sWatcherFunc
	clock          clock.Clock

	// randomPrefix generates an annotation for stateful sets.
	randomPrefix utils.RandomPrefixFunc

	newApplier func() resources.Applier
}

// CharmContainerResourceRequirements defines the memory resource constraints
// for the workload pod's charm container.
type CharmContainerResourceRequirements struct {
	MemRequestMi string
	MemLimitMi   string
}

// NewApplication returns an application.
func NewApplication(
	name string,
	namespace string,
	modelUUID string,
	modelName string,
	labelVersion constants.LabelVersion,
	deploymentType caas.DeploymentType,
	coreClient kubernetes.Interface,
	extendedClient apiextensionsclientset.Interface,
	newWatcher k8swatcher.NewK8sWatcherFunc,
	clock clock.Clock,
	randomPrefix utils.RandomPrefixFunc,
) caas.Application {
	return newApplication(
		name,
		namespace,
		modelUUID,
		modelName,
		labelVersion,
		deploymentType,
		coreClient,
		extendedClient,
		newWatcher,
		clock,
		randomPrefix,
		resources.NewApplier,
	)
}

func newApplication(
	name string,
	namespace string,
	modelUUID string,
	modelName string,
	labelVersion constants.LabelVersion,
	deploymentType caas.DeploymentType,
	coreClient kubernetes.Interface,
	extendedClient apiextensionsclientset.Interface,
	newWatcher k8swatcher.NewK8sWatcherFunc,
	clock clock.Clock,
	randomPrefix utils.RandomPrefixFunc,
	newApplier func() resources.Applier,
) *app {
	return &app{
		name:           name,
		namespace:      namespace,
		modelUUID:      modelUUID,
		modelName:      modelName,
		labelVersion:   labelVersion,
		deploymentType: deploymentType,
		coreClient:     coreClient,
		extendedClient: extendedClient,
		newWatcher:     newWatcher,
		clock:          clock,
		randomPrefix:   randomPrefix,
		newApplier:     newApplier,
	}
}

// Ensure creates or updates an application pod with the given application
// name, agent path, and application config.
func (a *app) Ensure(config caas.ApplicationConfig) (err error) {
	// TODO: add support `numUnits`, `Constraints` and `Devices`.
	// TODO: storage handling for deployment/daemonset enhancement.
	defer func() {
		if err != nil {
			logger.Errorf("Ensure %s", err)
		}
	}()
	logger.Debugf("creating/updating %s application", a.name)

	applier := a.newApplier()

	err = a.applyServiceAccountAndSecrets(applier, config)
	if err != nil {
		return errors.Annotatef(err, "applying service account and secrets")
	}

	if err := a.configureDefaultService(a.annotations(config)); err != nil {
		return errors.Annotatef(err, "ensuring the default service %q", a.name)
	}

	// Set up the parameters for creating charm storage (if required).
	podSpec, err := a.ApplicationPodSpec(config)
	if err != nil {
		return errors.Annotate(err, "generating application podspec")
	}

	var handleVolume handleVolumeFunc = func(v corev1.Volume, mountPath string, readOnly bool) (*corev1.VolumeMount, error) {
		if err := storage.PushUniqueVolume(podSpec, v, false); err != nil {
			return nil, errors.Trace(err)
		}
		return &corev1.VolumeMount{
			Name:      v.Name,
			ReadOnly:  readOnly,
			MountPath: mountPath,
		}, nil
	}
	var handleVolumeMount handleVolumeMountFunc = func(storageName string, m corev1.VolumeMount) error {
		for i := range podSpec.Containers {
			name := podSpec.Containers[i].Name
			if name == constants.ApplicationCharmContainer {
				podSpec.Containers[i].VolumeMounts = append(podSpec.Containers[i].VolumeMounts, m)
				continue
			}
			for _, mount := range config.Containers[name].Mounts {
				if mount.StorageName == storageName {
					volumeMountCopy := m
					// TODO(sidecar): volumeMountCopy.MountPath was defined in `caas.ApplicationConfig.Filesystems[*].Attachment.Path`.
					// Consolidate `caas.ApplicationConfig.Filesystems[*].Attachment.Path` and `caas.ApplicationConfig.Containers[*].Mounts[*].Path`!!!
					volumeMountCopy.MountPath = mount.Path
					podSpec.Containers[i].VolumeMounts = append(podSpec.Containers[i].VolumeMounts, volumeMountCopy)
				}
			}
		}
		return nil
	}
	var handlePVCForStatelessResource handlePVCFunc = func(pvc corev1.PersistentVolumeClaim, mountPath string, readOnly bool) (*corev1.VolumeMount, error) {
		// Ensure PVC.
		r := resources.NewPersistentVolumeClaim(pvc.GetName(), a.namespace, &pvc)
		applier.Apply(r)

		// Push the volume to podspec.
		vol := corev1.Volume{
			Name: r.GetName(),
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: r.GetName(),
					ReadOnly:  readOnly,
				},
			},
		}
		return handleVolume(vol, mountPath, readOnly)
	}
	storageClasses, err := resources.ListStorageClass(context.Background(), a.coreClient, metav1.ListOptions{})
	if err != nil {
		return errors.Trace(err)
	}
	var handleStorageClass = func(sc storagev1.StorageClass) error {
		applier.Apply(&resources.StorageClass{StorageClass: sc})
		return nil
	}
	var configureStorage = func(storageUniqueID string, handlePVC handlePVCFunc) error {
		err := a.configureStorage(
			storageUniqueID,
			config.Filesystems,
			storageClasses,
			handleVolume, handleVolumeMount, handlePVC, handleStorageClass,
		)
		return errors.Trace(err)
	}

	switch a.deploymentType {
	case caas.DeploymentStateful:
		if err := a.configureHeadlessService(a.name, a.annotations(config)); err != nil {
			return errors.Annotatef(err, "creating or updating headless service for %q %q", a.deploymentType, a.name)
		}
		exists := true
		ss, getErr := a.getStatefulSet()
		if errors.IsNotFound(getErr) {
			exists = false
		} else if getErr != nil {
			return errors.Trace(getErr)
		}
		storageUniqueID, err := a.getStorageUniqPrefix(func() (annotationGetter, error) {
			return ss, getErr
		})
		if err != nil {
			return errors.Trace(err)
		}
		var numPods *int32
		if !exists {
			numPods = pointer.Int32(int32(config.InitialScale))
		}
		statefulset := resources.StatefulSet{
			StatefulSet: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      a.name,
					Namespace: a.namespace,
					Labels:    a.labels(),
					Annotations: a.annotations(config).
						Add(utils.AnnotationKeyApplicationUUID(a.labelVersion), storageUniqueID),
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: numPods,
					Selector: &metav1.LabelSelector{
						MatchLabels: a.selectorLabels(),
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels:      a.selectorLabels(),
							Annotations: a.annotations(config),
						},
						Spec: *podSpec,
					},
					PodManagementPolicy: appsv1.ParallelPodManagement,
					ServiceName:         HeadlessServiceName(a.name),
				},
			},
		}

		if err = configureStorage(
			storageUniqueID,
			func(pvc corev1.PersistentVolumeClaim, mountPath string, readOnly bool) (*corev1.VolumeMount, error) {
				if err := storage.PushUniqueVolumeClaimTemplate(&statefulset.Spec, pvc); err != nil {
					return nil, errors.Trace(err)
				}
				return &corev1.VolumeMount{
					Name:      pvc.GetName(),
					ReadOnly:  readOnly,
					MountPath: mountPath,
				}, nil
			},
		); err != nil {
			return errors.Trace(err)
		}

		applier.Apply(&statefulset)
	case caas.DeploymentStateless:
		exists := true
		d, getErr := a.getDeployment()
		if errors.IsNotFound(getErr) {
			exists = false
		} else if getErr != nil {
			return errors.Trace(getErr)
		}
		storageUniqueID, err := a.getStorageUniqPrefix(func() (annotationGetter, error) {
			return d, getErr
		})
		if err != nil {
			return errors.Trace(err)
		}
		var numPods *int32
		if !exists {
			numPods = pointer.Int32(int32(config.InitialScale))
		}
		// Config storage to update the podspec with storage info.
		if err = configureStorage(storageUniqueID, handlePVCForStatelessResource); err != nil {
			return errors.Trace(err)
		}
		deployment := resources.Deployment{
			Deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      a.name,
					Namespace: a.namespace,
					Labels:    a.labels(),
					Annotations: a.annotations(config).
						Add(utils.AnnotationKeyApplicationUUID(a.labelVersion), storageUniqueID),
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: numPods,
					Selector: &metav1.LabelSelector{
						MatchLabels: a.selectorLabels(),
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels:      a.selectorLabels(),
							Annotations: a.annotations(config),
						},
						Spec: *podSpec,
					},
				},
			},
		}

		applier.Apply(&deployment)
	case caas.DeploymentDaemon:
		storageUniqueID, err := a.getStorageUniqPrefix(func() (annotationGetter, error) {
			return a.getDaemonSet()
		})
		if err != nil {
			return errors.Trace(err)
		}
		// Config storage to update the podspec with storage info.
		if err = configureStorage(storageUniqueID, handlePVCForStatelessResource); err != nil {
			return errors.Trace(err)
		}
		daemonset := resources.DaemonSet{
			DaemonSet: appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      a.name,
					Namespace: a.namespace,
					Labels:    a.labels(),
					Annotations: a.annotations(config).
						Add(utils.AnnotationKeyApplicationUUID(a.labelVersion), storageUniqueID),
				},
				Spec: appsv1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: a.selectorLabels(),
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels:      a.selectorLabels(),
							Annotations: a.annotations(config),
						},
						Spec: *podSpec,
					},
				},
			},
		}
		applier.Apply(&daemonset)
	default:
		return errors.NotSupportedf("unknown deployment type")
	}

	return applier.Run(context.Background(), a.coreClient, a.extendedClient, false)
}

func (a *app) applyServiceAccountAndSecrets(applier resources.Applier, config caas.ApplicationConfig) error {
	secret := resources.Secret{
		Secret: corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        a.secretName(),
				Namespace:   a.namespace,
				Labels:      a.labels(),
				Annotations: a.annotations(config),
			},
			Data: map[string][]byte{
				"JUJU_K8S_APPLICATION":          []byte(a.name),
				"JUJU_K8S_MODEL":                []byte(a.modelUUID),
				"JUJU_K8S_APPLICATION_PASSWORD": []byte(config.IntroductionSecret),
				"JUJU_K8S_CONTROLLER_ADDRESSES": []byte(config.ControllerAddresses),
				"JUJU_K8S_CONTROLLER_CA_CERT":   []byte(config.ControllerCertBundle),
			},
		},
	}
	applier.Apply(&secret)

	serviceAccount := resources.ServiceAccount{
		ServiceAccount: corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:        a.serviceAccountName(),
				Namespace:   a.namespace,
				Labels:      a.labels(),
				Annotations: a.annotations(config),
			},
			// Will be automounted by the pod.
			AutomountServiceAccountToken: pointer.Bool(false),
		},
	}
	applier.Apply(&serviceAccount)

	role := resources.Role{
		Role: rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:        a.serviceAccountName(),
				Namespace:   a.namespace,
				Labels:      a.labels(),
				Annotations: a.annotations(config),
			},
			Rules: a.roleRules(config.Trust),
		},
	}
	applier.Apply(&role)

	roleBinding := resources.RoleBinding{
		RoleBinding: rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:        a.serviceAccountName(),
				Namespace:   a.namespace,
				Labels:      a.labels(),
				Annotations: a.annotations(config),
			},
			RoleRef: rbacv1.RoleRef{
				Name: a.serviceAccountName(),
				Kind: "Role",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      rbacv1.ServiceAccountKind,
					Name:      a.serviceAccountName(),
					Namespace: a.namespace,
				},
			},
		},
	}
	applier.Apply(&roleBinding)

	clusterRole := resources.ClusterRole{
		ClusterRole: rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name:        a.qualifiedClusterName(),
				Labels:      a.labels(),
				Annotations: a.annotations(config),
			},
			Rules: a.clusterRoleRules(config.Trust),
		},
	}
	applier.Apply(&clusterRole)

	clusterRoleBinding := resources.ClusterRoleBinding{
		ClusterRoleBinding: rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:        a.qualifiedClusterName(),
				Labels:      a.labels(),
				Annotations: a.annotations(config),
			},
			RoleRef: rbacv1.RoleRef{
				Name: a.qualifiedClusterName(),
				Kind: "ClusterRole",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      rbacv1.ServiceAccountKind,
					Name:      a.serviceAccountName(),
					Namespace: a.namespace,
				},
			},
		},
	}
	applier.Apply(&clusterRoleBinding)

	return a.applyImagePullSecrets(applier, config)
}

// Upgrade upgrades the app to the specified version.
func (a *app) Upgrade(ver version.Number) error {
	// TODO(sidecar): Unify this with Ensure
	applier := a.newApplier()

	if err := a.upgradeMainResource(applier, ver); err != nil {
		return errors.Trace(err)
	}

	// TODO(sidecar):  we could query the cluster for all resources with the `app.juju.is/created-by` and `app.kubernetes.io/name` labels instead
	// (so longer as the resource also does have the juju version annotation already).
	// Then we don't have to worry about missing anything if something is added later and not also updated here.
	for _, r := range []annotationUpdater{
		resources.NewSecret(a.secretName(), a.namespace, nil),
		resources.NewServiceAccount(a.serviceAccountName(), a.namespace, nil),
		resources.NewRole(a.serviceAccountName(), a.namespace, nil),
		resources.NewRoleBinding(a.serviceAccountName(), a.namespace, nil),
		resources.NewClusterRole(a.qualifiedClusterName(), nil),
		resources.NewClusterRoleBinding(a.qualifiedClusterName(), nil),
		resources.NewService(a.name, a.namespace, nil),
	} {
		if err := r.Get(context.Background(), a.coreClient, a.extendedClient); err != nil {
			return errors.Trace(err)
		}
		existingAnnotations := annotations.New(r.GetAnnotations())
		r.SetAnnotations(a.upgradeAnnotations(existingAnnotations, ver))
		applier.Apply(r)
	}

	return applier.Run(context.Background(), a.coreClient, a.extendedClient, false)
}

type annotationUpdater interface {
	resources.Resource
	GetAnnotations() map[string]string
	SetAnnotations(annotations map[string]string)
}

func (a *app) upgradeHeadlessService(applier resources.Applier, ver version.Number) error {
	r := resources.NewService(HeadlessServiceName(a.name), a.namespace, nil)
	if err := r.Get(context.Background(), a.coreClient, a.extendedClient); err != nil {
		return errors.Trace(err)
	}
	r.SetAnnotations(a.upgradeAnnotations(annotations.New(r.GetAnnotations()), ver))
	applier.Apply(r)
	return nil
}

func (a *app) upgradeMainResource(applier resources.Applier, ver version.Number) error {
	switch a.deploymentType {
	case caas.DeploymentStateful:
		if err := a.upgradeHeadlessService(applier, ver); err != nil {
			return errors.Trace(err)
		}

		ss := resources.NewStatefulSet(a.name, a.namespace, nil)
		if err := ss.Get(context.Background(), a.coreClient, a.extendedClient); err != nil {
			return errors.Trace(err)
		}
		initContainers := ss.Spec.Template.Spec.InitContainers
		if len(initContainers) != 1 {
			return errors.NotValidf("init container of %q", a.name)
		}
		initContainer := initContainers[0]
		var err error
		initContainer.Image, err = podcfg.RebuildOldOperatorImagePath(initContainer.Image, ver)
		if err != nil {
			return errors.Trace(err)
		}
		ss.Spec.Template.Spec.InitContainers = []corev1.Container{initContainer}
		ss.Spec.Template.SetAnnotations(a.upgradeAnnotations(annotations.New(ss.Spec.Template.GetAnnotations()), ver))
		ss.SetAnnotations(a.upgradeAnnotations(annotations.New(ss.GetAnnotations()), ver))
		applier.Apply(ss)
		return nil
	case caas.DeploymentStateless:
		return errors.NotSupportedf("upgrade for deployment type %q", a.deploymentType)
	case caas.DeploymentDaemon:
		return errors.NotSupportedf("upgrade for deployment type %q", a.deploymentType)
	default:
		return errors.NotSupportedf("unknown deployment type %q", a.deploymentType)
	}
}

// Exists indicates if the application for the specified
// application exists, and whether the application is terminating.
func (a *app) Exists() (caas.DeploymentState, error) {
	checks := []struct {
		label            string
		check            func() (bool, bool, error)
		forceTerminating bool
	}{
		{},
		{"secret", a.secretExists, false},
		{"service", a.serviceExists, false},
		{"roleBinding", a.roleBindingExists, false},
		{"role", a.roleExists, false},
		{"clusterRoleBinding", a.clusterRoleBindingExists, false},
		{"clusterRole", a.clusterRoleExists, false},
		{"serviceAccount", a.serviceAccountExists, false},
	}
	switch a.deploymentType {
	case caas.DeploymentStateful:
		checks[0].label = "statefulset"
		checks[0].check = a.statefulSetExists
	case caas.DeploymentStateless:
		checks[0].label = "deployment"
		checks[0].check = a.deploymentExists
	case caas.DeploymentDaemon:
		checks[0].label = "daemonset"
		checks[0].check = a.daemonSetExists
	default:
		return caas.DeploymentState{}, errors.NotSupportedf("unknown deployment type")
	}

	state := caas.DeploymentState{}
	for _, c := range checks {
		exists, terminating, err := c.check()
		if err != nil {
			return caas.DeploymentState{}, errors.Annotatef(err, "%s resource check", c.label)
		}
		if !exists {
			continue
		}
		state.Exists = true
		if terminating || c.forceTerminating {
			// Terminating is always set to true regardless of whether the resource is failed as terminating
			// since it's the overall state that is reported baca.
			logger.Debugf("application %q exists and is terminating due to dangling %s resource(s)", a.name, c.label)
			return caas.DeploymentState{Exists: true, Terminating: true}, nil
		}
	}
	return state, nil
}

// HeadlessServiceName is an idempotent function for returning the name of the
// endpoints service juju make for applications.
func HeadlessServiceName(appName string) string {
	return fmt.Sprintf("%s-endpoints", appName)
}

func (a *app) configureHeadlessService(name string, annotation annotations.Annotation) error {
	svc := resources.NewService(HeadlessServiceName(name), a.namespace, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels: a.labels(),
			Annotations: annotation.
				Add("service.alpha.kubernetes.io/tolerate-unready-endpoints", "true"),
		},
		Spec: corev1.ServiceSpec{
			Selector:                 a.selectorLabels(),
			Type:                     corev1.ServiceTypeClusterIP,
			ClusterIP:                "None",
			PublishNotReadyAddresses: true,
		},
	})
	return svc.Apply(context.Background(), a.coreClient, a.extendedClient)
}

const (
	placeholderPortName = "placeholder"
	placeholderPort     = 65535
)

// configureDefaultService configures the default service for the application.
// It's only configured once when the application was deployed in the first time.
func (a *app) configureDefaultService(annotation annotations.Annotation) (err error) {
	svc := resources.NewService(a.name, a.namespace, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      a.labels(),
			Annotations: annotation,
		},
		Spec: corev1.ServiceSpec{
			Selector: a.selectorLabels(),
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{{
				Name: placeholderPortName,
				Port: placeholderPort,
			}},
		},
	})
	return svc.Apply(context.Background(), a.coreClient, a.extendedClient)
}

// UpdateService updates the default service with specific service type and port mappings.
func (a *app) UpdateService(param caas.ServiceParam) error {
	// This method will be used for juju [un]expose.
	// TODO(sidecar): it might be changed later when we have proper modelling for the juju expose for the sidecar charms.
	svc, err := a.getService()
	if err != nil {
		return errors.Annotatef(err, "getting existing service %q", a.name)
	}
	svc.Service.Spec.Type = corev1.ServiceType(param.Type)
	svc.Service.Spec.Ports = make([]corev1.ServicePort, len(param.Ports))
	for i, p := range param.Ports {
		if svc.Service.Spec.Ports[i], err = convertServicePort(p); err != nil {
			return errors.Trace(err)
		}
	}

	applier := a.newApplier()
	applier.Apply(svc)
	if err := a.updateContainerPorts(applier, svc.Service.Spec.Ports); err != nil {
		return errors.Trace(err)
	}
	return applier.Run(context.Background(), a.coreClient, a.extendedClient, false)
}

func convertServicePort(port caas.ServicePort) (out corev1.ServicePort, err error) {
	var protocol corev1.Protocol

	switch port.Protocol {
	case "TCP", "tcp":
		protocol = corev1.ProtocolTCP
	case "UDP", "udp":
		protocol = corev1.ProtocolUDP
	case "SCTP", "sctp":
		protocol = corev1.ProtocolSCTP
	default:
		return out, errors.NotValidf("protocol %q for service %q", port.Protocol, port.Name)
	}

	return corev1.ServicePort{
		Name:       port.Name,
		Port:       int32(port.Port),
		TargetPort: intstr.FromInt(port.TargetPort),
		Protocol:   protocol,
	}, nil
}

func (a *app) getService() (*resources.Service, error) {
	svc := resources.NewService(a.name, a.namespace, nil)
	if err := svc.Get(context.Background(), a.coreClient, a.extendedClient); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errors.NotFoundf("service %q", a.name)
		}
		return nil, errors.Trace(err)
	}
	return svc, nil
}

const portNamePrefix = "juju-"

// UpdatePorts updates port mappings on the specified service.
func (a *app) UpdatePorts(ports []caas.ServicePort, updateContainerPorts bool) error {
	svc, err := a.getService()
	if err != nil {
		return errors.Annotatef(err, "getting existing service %q", a.name)
	}
	var replacePortsPatchType = types.MergePatchType
	// We want to replace rather than merge here.
	svc.PatchType = &replacePortsPatchType

	var expectedPorts []corev1.ServicePort
	for _, p := range svc.Service.Spec.Ports {
		if p.Name == placeholderPortName && len(ports) > 0 {
			// Ignore placeholder port if there are ports supplied by the charm.
			continue
		}
		if strings.HasPrefix(p.Name, portNamePrefix) {
			// Port managed by Juju - will be replaced,
			continue
		}
		// The ports that are not managed by Juju should be kept.
		expectedPorts = append(expectedPorts, p)
	}
	for _, port := range ports {
		sp, err := convertServicePort(port)
		if err != nil {
			return errors.Trace(err)
		}
		sp.Name = portNamePrefix + sp.Name
		expectedPorts = append(expectedPorts, sp)
	}
	svc.Service.Spec.Ports = expectedPorts

	applier := a.newApplier()
	applier.Apply(svc)
	if updateContainerPorts {
		if err := a.updateContainerPorts(applier, svc.Service.Spec.Ports); err != nil {
			return errors.Trace(err)
		}
	}
	err = applier.Run(context.Background(), a.coreClient, a.extendedClient, false)
	return errors.Trace(err)
}

func convertContainerPort(p corev1.ServicePort) corev1.ContainerPort {
	return corev1.ContainerPort{
		Name:          p.Name,
		ContainerPort: p.TargetPort.IntVal,
		Protocol:      p.Protocol,
	}
}

func (a *app) updateContainerPorts(applier resources.Applier, ports []corev1.ServicePort) error {
	updatePodSpec := func(spec *corev1.PodSpec, containerPorts []corev1.ContainerPort) {
		for i, c := range spec.Containers {
			ps := containerPorts
			if c.Name != constants.ApplicationCharmContainer {
				spec.Containers[i].Ports = ps
			}
		}
	}

	containerPorts := make([]corev1.ContainerPort, len(ports))
	for i, p := range ports {
		containerPorts[i] = convertContainerPort(p)
	}

	switch a.deploymentType {
	case caas.DeploymentStateful:
		ss := resources.NewStatefulSet(a.name, a.namespace, nil)
		if err := ss.Get(context.Background(), a.coreClient, a.extendedClient); err != nil {
			return errors.Trace(err)
		}

		updatePodSpec(&ss.StatefulSet.Spec.Template.Spec, containerPorts)
		applier.Apply(ss)
	case caas.DeploymentStateless:
		d := resources.NewDeployment(a.name, a.namespace, nil)
		if err := d.Get(context.Background(), a.coreClient, a.extendedClient); err != nil {
			return errors.Trace(err)
		}

		updatePodSpec(&d.Deployment.Spec.Template.Spec, containerPorts)
		applier.Apply(d)
	case caas.DeploymentDaemon:
		d := resources.NewDaemonSet(a.name, a.namespace, nil)
		if err := d.Get(context.Background(), a.coreClient, a.extendedClient); err != nil {
			return errors.Trace(err)
		}

		updatePodSpec(&d.DaemonSet.Spec.Template.Spec, containerPorts)
		applier.Apply(d)
	default:
		return errors.NotSupportedf("unknown deployment type")
	}
	return nil
}

func (a *app) getStatefulSet() (*resources.StatefulSet, error) {
	ss := resources.NewStatefulSet(a.name, a.namespace, nil)
	if err := ss.Get(context.Background(), a.coreClient, a.extendedClient); err != nil {
		return nil, err
	}
	return ss, nil
}

func (a *app) getDeployment() (*resources.Deployment, error) {
	ss := resources.NewDeployment(a.name, a.namespace, nil)
	if err := ss.Get(context.Background(), a.coreClient, a.extendedClient); err != nil {
		return nil, err
	}
	return ss, nil
}

func (a *app) getDaemonSet() (*resources.DaemonSet, error) {
	ss := resources.NewDaemonSet(a.name, a.namespace, nil)
	if err := ss.Get(context.Background(), a.coreClient, a.extendedClient); err != nil {
		return nil, err
	}
	return ss, nil
}

func (a *app) statefulSetExists() (exists bool, terminating bool, err error) {
	ss := resources.NewStatefulSet(a.name, a.namespace, nil)
	err = ss.Get(context.Background(), a.coreClient, a.extendedClient)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, ss.DeletionTimestamp != nil, nil
}

func (a *app) deploymentExists() (exists bool, terminating bool, err error) {
	ss := resources.NewDeployment(a.name, a.namespace, nil)
	err = ss.Get(context.Background(), a.coreClient, a.extendedClient)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, ss.DeletionTimestamp != nil, nil
}

func (a *app) daemonSetExists() (exists bool, terminating bool, err error) {
	ss := resources.NewDaemonSet(a.name, a.namespace, nil)
	err = ss.Get(context.Background(), a.coreClient, a.extendedClient)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, ss.DeletionTimestamp != nil, nil
}

func (a *app) secretExists() (exists bool, terminating bool, err error) {
	ss := resources.NewSecret(a.secretName(), a.namespace, nil)
	err = ss.Get(context.Background(), a.coreClient, a.extendedClient)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, ss.DeletionTimestamp != nil, nil
}

func (a *app) serviceExists() (exists bool, terminating bool, err error) {
	ss := resources.NewService(a.name, a.namespace, nil)
	err = ss.Get(context.Background(), a.coreClient, a.extendedClient)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, ss.DeletionTimestamp != nil, nil
}

func (a *app) roleExists() (exists bool, terminating bool, err error) {
	r := resources.NewRole(a.serviceAccountName(), a.namespace, nil)
	err = r.Get(context.Background(), a.coreClient, a.extendedClient)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, r.DeletionTimestamp != nil, nil
}

func (a *app) roleBindingExists() (exists bool, terminating bool, err error) {
	rb := resources.NewRoleBinding(a.serviceAccountName(), a.namespace, nil)
	err = rb.Get(context.Background(), a.coreClient, a.extendedClient)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, rb.DeletionTimestamp != nil, nil
}

func (a *app) clusterRoleExists() (exists bool, terminating bool, err error) {
	r := resources.NewClusterRole(a.qualifiedClusterName(), nil)
	err = r.Get(context.Background(), a.coreClient, a.extendedClient)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, r.DeletionTimestamp != nil, nil
}

func (a *app) clusterRoleBindingExists() (exists bool, terminating bool, err error) {
	rb := resources.NewClusterRoleBinding(a.qualifiedClusterName(), nil)
	err = rb.Get(context.Background(), a.coreClient, a.extendedClient)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, rb.DeletionTimestamp != nil, nil
}

func (a *app) serviceAccountExists() (exists bool, terminating bool, err error) {
	sa := resources.NewServiceAccount(a.serviceAccountName(), a.namespace, nil)
	err = sa.Get(context.Background(), a.coreClient, a.extendedClient)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, sa.DeletionTimestamp != nil, nil
}

// Delete deletes the specified application.
func (a *app) Delete() error {
	logger.Infof("alvin Delete called for %s", a.name)
	logger.Debugf("deleting %s application", a.name)
	applier := a.newApplier()
	switch a.deploymentType {
	case caas.DeploymentStateful:
		applier.Delete(resources.NewStatefulSet(a.name, a.namespace, nil))
		applier.Delete(resources.NewService(HeadlessServiceName(a.name), a.namespace, nil))
	case caas.DeploymentStateless:
		applier.Delete(resources.NewDeployment(a.name, a.namespace, nil))
	case caas.DeploymentDaemon:
		applier.Delete(resources.NewDaemonSet(a.name, a.namespace, nil))
	default:
		return errors.NotSupportedf("unknown deployment type")
	}
	applier.Delete(resources.NewService(a.name, a.namespace, nil))
	applier.Delete(resources.NewSecret(a.secretName(), a.namespace, nil))
	applier.Delete(resources.NewRoleBinding(a.serviceAccountName(), a.namespace, nil))
	applier.Delete(resources.NewRole(a.serviceAccountName(), a.namespace, nil))
	applier.Delete(resources.NewClusterRoleBinding(a.qualifiedClusterName(), nil))
	applier.Delete(resources.NewClusterRole(a.qualifiedClusterName(), nil))
	applier.Delete(resources.NewServiceAccount(a.serviceAccountName(), a.namespace, nil))
	applier.Delete(resources.NewCustomResourceDefinition(a.name, nil))

	// Cleanup lists of resources.
	cleanup := []resources.Resource(nil)

	// List secrets to be deleted.
	secrets, err := resources.ListSecrets(context.Background(), a.coreClient, a.namespace, metav1.ListOptions{
		LabelSelector: a.labelSelector(),
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, s := range secrets {
		secret := s
		if a.matchImagePullSecret(secret.Name) {
			cleanup = append(cleanup, &secret)
		}
	}

	if len(cleanup) > 0 {
		applier.Delete(cleanup...)
	}

	return applier.Run(context.Background(), a.coreClient, a.extendedClient, false)
}

// Watch returns a watcher which notifies when there
// are changes to the application of the specified application.
func (a *app) Watch() (watcher.NotifyWatcher, error) {
	factory := informers.NewSharedInformerFactoryWithOptions(a.coreClient, 0,
		informers.WithNamespace(a.namespace),
		informers.WithTweakListOptions(func(o *metav1.ListOptions) {
			o.FieldSelector = a.fieldSelector()
		}),
	)
	var informer cache.SharedIndexInformer
	switch a.deploymentType {
	case caas.DeploymentStateful:
		informer = factory.Apps().V1().StatefulSets().Informer()
	case caas.DeploymentStateless:
		informer = factory.Apps().V1().Deployments().Informer()
	case caas.DeploymentDaemon:
		informer = factory.Apps().V1().DaemonSets().Informer()
	default:
		return nil, errors.NotSupportedf("unknown deployment type")
	}
	w1, err := a.newWatcher(informer, a.name, a.clock)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w2, err := a.newWatcher(factory.Core().V1().Services().Informer(), a.name, a.clock)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return watcher.NewMultiNotifyWatcher(w1, w2), nil
}

func (a *app) WatchReplicas() (watcher.NotifyWatcher, error) {
	factory := informers.NewSharedInformerFactoryWithOptions(a.coreClient, 0,
		informers.WithNamespace(a.namespace),
		informers.WithTweakListOptions(func(o *metav1.ListOptions) {
			o.LabelSelector = a.labelSelector()
		}),
	)
	return a.newWatcher(factory.Core().V1().Pods().Informer(), a.name, a.clock)
}

func (a *app) State() (caas.ApplicationState, error) {
	state := caas.ApplicationState{}
	switch a.deploymentType {
	case caas.DeploymentStateful:
		ss := resources.NewStatefulSet(a.name, a.namespace, nil)
		err := ss.Get(context.Background(), a.coreClient, a.extendedClient)
		if err != nil {
			return caas.ApplicationState{}, errors.Trace(err)
		}
		if ss.Spec.Replicas == nil {
			return caas.ApplicationState{}, errors.Errorf("missing replicas")
		}
		state.DesiredReplicas = int(*ss.Spec.Replicas)
	case caas.DeploymentStateless:
		d := resources.NewDeployment(a.name, a.namespace, nil)
		err := d.Get(context.Background(), a.coreClient, a.extendedClient)
		if err != nil {
			return caas.ApplicationState{}, errors.Trace(err)
		}
		if d.Spec.Replicas == nil {
			return caas.ApplicationState{}, errors.Errorf("missing replicas")
		}
		state.DesiredReplicas = int(*d.Spec.Replicas)
	case caas.DeploymentDaemon:
		d := resources.NewDaemonSet(a.name, a.namespace, nil)
		err := d.Get(context.Background(), a.coreClient, a.extendedClient)
		if err != nil {
			return caas.ApplicationState{}, errors.Trace(err)
		}
		state.DesiredReplicas = int(d.Status.DesiredNumberScheduled)
	default:
		return caas.ApplicationState{}, errors.NotSupportedf("unknown deployment type")
	}
	next := ""
	for {
		res, err := a.coreClient.CoreV1().Pods(a.namespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: a.labelSelector(),
			Continue:      next,
		})
		if err != nil {
			return caas.ApplicationState{}, errors.Trace(err)
		}
		for _, pod := range res.Items {
			state.Replicas = append(state.Replicas, pod.Name)
		}
		if res.RemainingItemCount == nil || *res.RemainingItemCount == 0 {
			break
		}
		next = res.Continue
	}
	sort.Strings(state.Replicas)
	return state, nil
}

// Service returns the service associated with the application.
func (a *app) Service() (*caas.Service, error) {
	svc, err := a.getService()
	if err != nil {
		return nil, errors.Trace(err)
	}
	ctx := context.Background()
	now := a.clock.Now()
	statusMessage, svcStatus, since, err := a.computeStatus(ctx, a.coreClient, now)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &caas.Service{
		Id:        string(svc.GetUID()),
		Addresses: utils.GetSvcAddresses(&svc.Service, false),
		Status: status.StatusInfo{
			Status:  svcStatus,
			Message: statusMessage,
			Since:   &since,
		},
		// Generate and Scale are not used here.
		Generation: nil,
		Scale:      nil,
	}, nil
}

func (a *app) computeStatus(ctx context.Context, client kubernetes.Interface, now time.Time) (string, status.Status, time.Time, error) {
	jujuStatus := status.Waiting
	switch a.deploymentType {
	case caas.DeploymentStateful:
		ss, err := a.getStatefulSet()
		if err != nil {
			return "", jujuStatus, now, errors.Trace(err)
		}
		if ss.GetDeletionTimestamp() != nil {
			return "", status.Terminated, now, nil
		} else if ss.Status.ReadyReplicas > 0 {
			return "", status.Active, now, nil
		}
		var statusMessage string
		events, err := ss.Events(ctx, client)
		if err != nil {
			return "", jujuStatus, now, errors.Trace(err)
		}
		// Take the most recent event.
		if count := len(events); count > 0 {
			evt := events[count-1]
			if evt.Type == corev1.EventTypeWarning && evt.Reason == "FailedCreate" {
				jujuStatus = status.Error
				statusMessage = evt.Message
			}
		}
		return statusMessage, jujuStatus, now, nil
	default:
		return "", jujuStatus, now, errors.NotSupportedf("deployment type %q", a.deploymentType)
	}
}

// Units of the application fetched from kubernetes by matching pod labels.
func (a *app) Units() ([]caas.Unit, error) {
	ctx := context.Background()
	now := a.clock.Now()
	var units []caas.Unit
	pods, err := resources.ListPods(ctx, a.coreClient, a.namespace, metav1.ListOptions{
		LabelSelector: a.labelSelector(),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, p := range pods {
		var ports []string
		for _, c := range p.Spec.Containers {
			for _, p := range c.Ports {
				ports = append(ports, fmt.Sprintf("%v/%v", p.ContainerPort, p.Protocol))
			}
		}
		terminated := p.DeletionTimestamp != nil
		statusMessage, unitStatus, since, err := p.ComputeStatus(ctx, a.coreClient, now)
		if err != nil {
			return nil, errors.Trace(err)
		}
		unitInfo := caas.Unit{
			Id:       p.Name,
			Address:  p.Status.PodIP,
			Ports:    ports,
			Dying:    terminated,
			Stateful: a.deploymentType == caas.DeploymentStateful,
			Status: status.StatusInfo{
				Status:  unitStatus,
				Message: statusMessage,
				Since:   &since,
			},
		}

		volumesByName := make(map[string]corev1.Volume)
		for _, pv := range p.Spec.Volumes {
			volumesByName[pv.Name] = pv
		}

		// Gather info about how filesystems are attached/mounted to the pod.
		// The mount name represents the filesystem tag name used by Juju.
		for _, volMount := range p.Spec.Containers[0].VolumeMounts {
			if volMount.Name == constants.CharmVolumeName {
				continue
			}
			vol, ok := volumesByName[volMount.Name]
			if !ok {
				logger.Warningf("volume for volume mount %q not found", volMount.Name)
				continue
			}

			// Ignore volume sources for volumes created for K8s pod-spec charms.
			// See: https://discourse.charmhub.io/t/k8s-spec-v3-changes/2698
			if vol.Secret != nil &&
				(strings.Contains(vol.Secret.SecretName, "-token") || strings.HasPrefix(vol.Secret.SecretName, "controller-")) {
				logger.Tracef("ignoring volume source for service account secret: %v", vol.Name)
				continue
			}
			if vol.Projected != nil {
				logger.Tracef("ignoring volume source for projected volume: %v", vol.Name)
				continue
			}
			if vol.ConfigMap != nil {
				logger.Tracef("ignoring volume source for configMap volume: %v", vol.Name)
				continue
			}
			if vol.HostPath != nil {
				logger.Tracef("ignoring volume source for hostPath volume: %v", vol.Name)
				continue
			}
			if vol.EmptyDir != nil {
				logger.Tracef("ignoring volume source for emptyDir volume: %v", vol.Name)
				continue
			}

			fsInfo, err := storage.FilesystemInfo(ctx, a.coreClient, a.extendedClient, a.namespace, vol, volMount, now)
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
			logger.Tracef("filesystem info for %v: %+v", volMount.Name, *fsInfo)
			unitInfo.FilesystemInfo = append(unitInfo.FilesystemInfo, *fsInfo)
		}
		units = append(units, unitInfo)
	}
	return units, nil
}

// ApplicationPodSpec returns a PodSpec for the application pod
// of the specified application.
func (a *app) ApplicationPodSpec(config caas.ApplicationConfig) (*corev1.PodSpec, error) {
	agentVersionNoBuild := config.AgentVersion
	agentVersionNoBuild.Build = 0

	jujuDataDir := paths.DataDir(paths.OSUnixLike)

	containerNames := config.ExistingContainers
	containers := []caas.ContainerConfig(nil)
	for _, v := range config.Containers {
		containerNames = append(containerNames, v.Name)
		containers = append(containers, v)
	}
	sort.Strings(containerNames)
	sort.Slice(containers, func(i, j int) bool {
		return containers[i].Name < containers[j].Name
	})

	requireSecurityContext := config.CharmUser != caas.RunAsDefault
	for _, v := range containers {
		requireSecurityContext = requireSecurityContext || v.Uid != nil || v.Gid != nil
	}

	env := []corev1.EnvVar{
		{
			Name:  constants.EnvJujuContainerNames,
			Value: strings.Join(containerNames, ","),
		},
		{
			Name:  constants.EnvAgentHTTPProbePort,
			Value: constants.AgentHTTPProbePort,
		},
	}
	if features := featureflag.AsEnvironmentValue(); features != "" {
		env = append(env, corev1.EnvVar{
			Name:  osenv.JujuFeatureFlagEnvKey,
			Value: features,
		})
	}
	charmContainerCommand := []string{"/charm/bin/pebble"}
	charmContainerArgs := []string{
		"run",
		"--http", fmt.Sprintf(":%s", containerAgentPebblePort),
		"--verbose",
	}
	charmContainerLivenessProbe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/v1/health?level=alive",
				Port: intstr.Parse(containerAgentPebblePort),
			},
		},
		InitialDelaySeconds: containerProbeInitialDelay,
		TimeoutSeconds:      containerProbeTimeout,
		PeriodSeconds:       containerProbePeriod,
		SuccessThreshold:    containerProbeSuccess,
		FailureThreshold:    containerLivenessProbeFailure,
	}
	charmContainerReadinessProbe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/v1/health?level=ready",
				Port: intstr.Parse(containerAgentPebblePort),
			},
		},
		InitialDelaySeconds: containerProbeInitialDelay,
		TimeoutSeconds:      containerProbeTimeout,
		PeriodSeconds:       containerProbePeriod,
		SuccessThreshold:    containerProbeSuccess,
		FailureThreshold:    containerReadinessProbeFailure,
	}
	charmContainerStartupProbe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/v1/health?level=alive",
				Port: intstr.Parse(containerAgentPebblePort),
			},
		},
		InitialDelaySeconds: containerProbeInitialDelay,
		TimeoutSeconds:      containerProbeTimeout,
		PeriodSeconds:       containerProbePeriod,
		SuccessThreshold:    containerProbeSuccess,
		FailureThreshold:    containerStartupProbeFailure,
	}
	// In 3.6-beta3 we moved to using startup probes with no startup delay.
	if agentVersionNoBuild.Compare(startupProbeVersion) >= 0 {
		charmContainerReadinessProbe.InitialDelaySeconds = 0
		charmContainerLivenessProbe.InitialDelaySeconds = 0
		charmContainerStartupProbe.InitialDelaySeconds = 0
		charmContainerStartupProbe.PeriodSeconds = containerStartupProbePeriodV36
		charmContainerStartupProbe.FailureThreshold = containerStartupProbeFailureV36
	}

	charmContainerExtraVolumeMounts := []corev1.VolumeMount{
		{
			Name:      constants.CharmVolumeName,
			MountPath: constants.DefaultPebbleDir,
			SubPath:   "containeragent/pebble",
		},
	}
	charmContainerExtraVolumeMounts = append(charmContainerExtraVolumeMounts, corev1.VolumeMount{
		Name:      constants.CharmVolumeName,
		MountPath: "/var/log/juju",
		SubPath:   "containeragent/var/log/juju",
	}, corev1.VolumeMount{
		Name:      constants.CharmVolumeName,
		MountPath: paths.JujuIntrospect(paths.OSUnixLike),
		SubPath:   "charm/bin/containeragent",
		ReadOnly:  true,
	}, corev1.VolumeMount{
		Name:      constants.CharmVolumeName,
		MountPath: paths.JujuExec(paths.OSUnixLike),
		SubPath:   "charm/bin/containeragent",
		ReadOnly:  true,
	})

	if agentVersionNoBuild.Compare(profileDirVersion) >= 0 {
		charmContainerExtraVolumeMounts = append(charmContainerExtraVolumeMounts, corev1.VolumeMount{
			Name:      constants.CharmVolumeName,
			MountPath: "/etc/profile.d/juju-introspection.sh",
			SubPath:   "containeragent/etc/profile.d/juju-introspection.sh",
			ReadOnly:  true,
		})
	}

	// (tlm) lp1997253. If the agent version is less than
	// containerAgentPebbleVersion we need to keep still using the old args
	// supported by the init command and the associated probes. By not doing
	// this we will have full container restarts.
	if config.AgentVersion.Compare(containerAgentPebbleVersion) < 0 {
		charmContainerExtraVolumeMounts = []corev1.VolumeMount{}
		charmContainerLivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: constants.AgentHTTPPathLiveness,
					Port: intstr.Parse(constants.AgentHTTPProbePort),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
			SuccessThreshold:    1,
			FailureThreshold:    2,
		}
		charmContainerReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: constants.AgentHTTPPathReadiness,
					Port: intstr.Parse(constants.AgentHTTPProbePort),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
			SuccessThreshold:    1,
			FailureThreshold:    2,
		}
		charmContainerStartupProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: constants.AgentHTTPPathStartup,
					Port: intstr.Parse(constants.AgentHTTPProbePort),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
			SuccessThreshold:    1,
			FailureThreshold:    2,
		}
		charmContainerCommand = []string{"/charm/bin/containeragent"}
		charmContainerArgs = []string{
			"unit",
			"--data-dir", jujuDataDir,
			"--charm-modified-version", strconv.Itoa(config.CharmModifiedVersion),
			"--append-env", "PATH=$PATH:/charm/bin",
			"--show-log",
		}
	}

	imagePullSecrets := []corev1.LocalObjectReference(nil)
	if config.IsPrivateImageRepo {
		imagePullSecrets = append(imagePullSecrets,
			corev1.LocalObjectReference{Name: constants.CAASImageRepoSecretName},
		)
	}

	charmContainer := corev1.Container{
		Name:            constants.ApplicationCharmContainer,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Image:           config.CharmBaseImagePath,
		WorkingDir:      jujuDataDir,
		Command:         charmContainerCommand,
		Args:            charmContainerArgs,
		Env:             env,
		LivenessProbe:   charmContainerLivenessProbe,
		ReadinessProbe:  charmContainerReadinessProbe,
		StartupProbe:    charmContainerStartupProbe,
		VolumeMounts: append([]corev1.VolumeMount{
			{
				Name:      constants.CharmVolumeName,
				MountPath: "/charm/bin",
				SubPath:   "charm/bin",
				ReadOnly:  true,
			},
			{
				Name:      constants.CharmVolumeName,
				MountPath: jujuDataDir,
				SubPath:   strings.TrimPrefix(jujuDataDir, "/"),
			},
			{
				Name:      constants.CharmVolumeName,
				MountPath: "/charm/containers",
				SubPath:   "charm/containers",
			},
		}, charmContainerExtraVolumeMounts...),
	}
	pebbleIdentitiesEnabled := false
	if requireSecurityContext {
		switch config.CharmUser {
		case caas.RunAsRoot:
			charmContainer.SecurityContext = &corev1.SecurityContext{
				RunAsUser:  pointer.Int64(0),
				RunAsGroup: pointer.Int64(0),
			}
		case caas.RunAsSudoer:
			charmContainer.SecurityContext = &corev1.SecurityContext{
				RunAsUser:  pointer.Int64(constants.JujuSudoUserID),
				RunAsGroup: pointer.Int64(constants.JujuSudoGroupID),
			}
			pebbleIdentitiesEnabled = true
		case caas.RunAsNonRoot:
			charmContainer.SecurityContext = &corev1.SecurityContext{
				RunAsUser:  pointer.Int64(constants.JujuUserID),
				RunAsGroup: pointer.Int64(constants.JujuGroupID),
			}
			pebbleIdentitiesEnabled = true
		}
		pebbleIdentitiesEnabled = pebbleIdentitiesEnabled &&
			agentVersionNoBuild.Compare(pebbleIdentitiesVersion) >= 0
	} else {
		// Pre-3.5 logic.
		charmContainer.SecurityContext = &corev1.SecurityContext{
			RunAsUser:  pointer.Int64(0),
			RunAsGroup: pointer.Int64(0),
		}
	}

	containerExtraEnv := []corev1.EnvVar{{
		Name:  "PEBBLE_SOCKET",
		Value: "/charm/container/pebble.socket",
	}}
	if agentVersionNoBuild.Compare(pebbleCopyOnceVersion) >= 0 {
		containerExtraEnv = append(containerExtraEnv, corev1.EnvVar{
			Name:  "PEBBLE",
			Value: "/charm/container/pebble",
		}, corev1.EnvVar{
			Name:  "PEBBLE_COPY_ONCE",
			Value: constants.DefaultPebbleDir,
		})
	}

	containerExtraArgs := []string{}
	if pebbleIdentitiesEnabled {
		containerExtraArgs = append(containerExtraArgs, "--identities", "/charm/etc/pebble/identities.yaml")
	}

	containerSpecs := []corev1.Container{charmContainer}
	for i, v := range containers {
		container := corev1.Container{
			Name:            v.Name,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Image:           v.Image.RegistryPath,
			Command:         []string{"/charm/bin/pebble"},
			Args: append([]string{
				"run",
				"--create-dirs",
				"--hold",
				"--http", fmt.Sprintf(":%s", pebble.WorkloadHealthCheckPort(i)),
				"--verbose",
			}, containerExtraArgs...),
			Env: append([]corev1.EnvVar{{
				Name:  "JUJU_CONTAINER_NAME",
				Value: v.Name,
			}}, containerExtraEnv...),
			LivenessProbe: &corev1.Probe{
				ProbeHandler:        pebble.LivenessHandler(pebble.WorkloadHealthCheckPort(i)),
				InitialDelaySeconds: containerProbeInitialDelay,
				TimeoutSeconds:      containerProbeTimeout,
				PeriodSeconds:       containerProbePeriod,
				SuccessThreshold:    containerProbeSuccess,
				FailureThreshold:    containerLivenessProbeFailure,
			},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler:        pebble.ReadinessHandler(pebble.WorkloadHealthCheckPort(i)),
				InitialDelaySeconds: containerProbeInitialDelay,
				TimeoutSeconds:      containerProbeTimeout,
				PeriodSeconds:       containerProbePeriod,
				SuccessThreshold:    containerProbeSuccess,
				FailureThreshold:    containerReadinessProbeFailure,
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      constants.CharmVolumeName,
					MountPath: "/charm/bin/pebble",
					SubPath:   "charm/bin/pebble",
					ReadOnly:  true,
				},
				{
					Name:      constants.CharmVolumeName,
					MountPath: "/charm/container",
					SubPath:   fmt.Sprintf("charm/containers/%s", v.Name),
				},
			},
		}
		if agentVersionNoBuild.Compare(startupProbeVersion) >= 0 {
			container.LivenessProbe.InitialDelaySeconds = 0
			container.ReadinessProbe.InitialDelaySeconds = 0
			container.StartupProbe = &corev1.Probe{
				ProbeHandler:     pebble.LivenessHandler(pebble.WorkloadHealthCheckPort(i)),
				TimeoutSeconds:   containerProbeTimeout,
				PeriodSeconds:    containerStartupProbePeriodV36,
				SuccessThreshold: containerProbeSuccess,
				FailureThreshold: containerStartupProbeFailureV36,
			}
		}
		if requireSecurityContext {
			container.SecurityContext = &corev1.SecurityContext{}
			if v.Uid != nil {
				container.SecurityContext.RunAsUser = pointer.Int64(int64(*v.Uid))
			}
			if v.Gid != nil {
				container.SecurityContext.RunAsGroup = pointer.Int64(int64(*v.Gid))
			}
		} else {
			// Pre-3.5 logic.
			container.SecurityContext = &corev1.SecurityContext{
				RunAsUser:  pointer.Int64(0),
				RunAsGroup: pointer.Int64(0),
			}
		}
		if pebbleIdentitiesEnabled {
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
				Name:      constants.CharmVolumeName,
				MountPath: "/charm/etc/pebble/identities.yaml",
				SubPath:   "charm/etc/pebble/identities.yaml",
				ReadOnly:  true,
			})
		}
		if v.Image.Password != "" {
			imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: a.imagePullSecretName(v.Name)})
		}
		containerSpecs = append(containerSpecs, container)
	}

	containerAgentArgs := []string{
		"init",
		"--containeragent-pebble-dir", "/containeragent/pebble",
		"--charm-modified-version", strconv.Itoa(config.CharmModifiedVersion),
		"--data-dir", jujuDataDir,
		"--bin-dir", "/charm/bin",
	}
	charmInitAdditionalMounts := []corev1.VolumeMount{
		{
			Name:      constants.CharmVolumeName,
			MountPath: "/containeragent/pebble",
			SubPath:   "containeragent/pebble",
		},
	}
	// (tlm) lp1997253. If the agent version is less then containerAgentPebbleVersion
	// we need to keep still using the old args supported by the init command
	if agentVersionNoBuild.Compare(containerAgentPebbleVersion) < 0 {
		charmInitAdditionalMounts = []corev1.VolumeMount{}
		containerAgentArgs = []string{
			"init",
			"--data-dir", jujuDataDir,
			"--bin-dir", "/charm/bin",
		}
	}

	if agentVersionNoBuild.Compare(profileDirVersion) >= 0 {
		containerAgentArgs = append(containerAgentArgs, "--profile-dir", "/containeragent/etc/profile.d")
		charmInitAdditionalMounts = append(charmInitAdditionalMounts, corev1.VolumeMount{
			Name:      constants.CharmVolumeName,
			MountPath: "/containeragent/etc/profile.d",
			SubPath:   "containeragent/etc/profile.d",
		})
	}

	if pebbleIdentitiesEnabled {
		containerAgentArgs = append(containerAgentArgs, "--pebble-identities-file", "/charm/etc/pebble/identities.yaml")
		charmInitAdditionalMounts = append(charmInitAdditionalMounts, corev1.VolumeMount{
			Name:      constants.CharmVolumeName,
			MountPath: "/charm/etc/pebble/",
			SubPath:   "charm/etc/pebble/",
		})
		uid := 0
		switch config.CharmUser {
		case caas.RunAsSudoer:
			uid = constants.JujuSudoUserID
		case caas.RunAsNonRoot:
			uid = constants.JujuUserID
		}
		containerAgentArgs = append(containerAgentArgs, "--pebble-charm-identity", strconv.Itoa(uid))
	}

	appSecret := a.secretName()
	charmInitContainer := corev1.Container{
		Name:            constants.ApplicationInitContainer,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Image:           config.AgentImagePath,
		WorkingDir:      jujuDataDir,
		Command:         []string{"/opt/containeragent"},
		Args:            containerAgentArgs,
		Env: []corev1.EnvVar{
			{
				Name:  constants.EnvJujuContainerNames,
				Value: strings.Join(containerNames, ","),
			},
			{
				Name: constants.EnvJujuK8sPodName,
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
			{
				Name: constants.EnvJujuK8sPodUUID,
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.uid",
					},
				},
			},
		},
		EnvFrom: []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: appSecret,
					},
				},
			},
		},
		VolumeMounts: append([]corev1.VolumeMount{
			{
				Name:      constants.CharmVolumeName,
				MountPath: jujuDataDir,
				SubPath:   strings.TrimPrefix(jujuDataDir, "/"),
			},
			{
				Name:      constants.CharmVolumeName,
				MountPath: "/charm/bin",
				SubPath:   "charm/bin",
			},
			// DO we need this in init container????
			{
				Name:      constants.CharmVolumeName,
				MountPath: "/charm/containers",
				SubPath:   "charm/containers",
			},
		}, charmInitAdditionalMounts...),
	}
	if requireSecurityContext {
		switch config.CharmUser {
		case caas.RunAsRoot:
			charmInitContainer.SecurityContext = &corev1.SecurityContext{
				RunAsUser:  pointer.Int64(0),
				RunAsGroup: pointer.Int64(0),
			}
		case caas.RunAsSudoer:
			charmInitContainer.SecurityContext = &corev1.SecurityContext{
				RunAsUser:  pointer.Int64(constants.JujuSudoUserID),
				RunAsGroup: pointer.Int64(constants.JujuSudoGroupID),
			}
		case caas.RunAsNonRoot:
			charmInitContainer.SecurityContext = &corev1.SecurityContext{
				RunAsUser:  pointer.Int64(constants.JujuUserID),
				RunAsGroup: pointer.Int64(constants.JujuGroupID),
			}
		}
	} else {
		// Pre-3.5 logic.
		charmInitContainer.SecurityContext = nil
	}

	automountToken := true
	spec := &corev1.PodSpec{
		AutomountServiceAccountToken:  &automountToken,
		ServiceAccountName:            a.serviceAccountName(),
		ImagePullSecrets:              imagePullSecrets,
		TerminationGracePeriodSeconds: pointer.Int64(30),
		InitContainers:                []corev1.Container{charmInitContainer},
		Containers:                    containerSpecs,
		Volumes: []corev1.Volume{
			{
				Name: constants.CharmVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		},
	}

	if agentVersionNoBuild.Compare(charmContainerResourceVersion) >= 0 {
		if err := ApplyWorkloadConstraints(spec, a.name, config.Constraints, configureWorkloadConstraintV2); err != nil {
			return nil, errors.Annotate(err, "processing workload container constraints")
		}
		charmConstraints := CharmContainerResourceRequirements{
			MemRequestMi: constants.CharmMemRequestMi,
			MemLimitMi:   constants.CharmMemLimitMi,
		}
		if config.Constraints.Mem != nil {
			charmConstraints.MemLimitMi = fmt.Sprintf("%dMi", *config.Constraints.Mem)
		}

		if err := ApplyCharmConstraints(spec, a.name, charmConstraints); err != nil {
			return nil, errors.Annotate(err, "processing charm container constraints")
		}
	} else {
		if err := ApplyWorkloadConstraints(spec, a.name, config.Constraints, configureWorkloadConstraint); err != nil {
			return nil, errors.Annotate(err, "processing workload container constraints")
		}
	}

	if requireSecurityContext {
		// Rootless charms are any charm after juju 3.5 that declare
		// either the charm as rootless or any workload.
		spec.SecurityContext = &corev1.PodSecurityContext{
			FSGroup:            pointer.Int64(constants.JujuFSGroupID),
			SupplementalGroups: []int64{constants.JujuFSGroupID},
		}
	}
	return spec, nil
}

func (a *app) applyImagePullSecrets(applier resources.Applier, config caas.ApplicationConfig) error {
	desired := []resources.Resource(nil)
	for _, container := range config.Containers {
		if container.Image.Password == "" {
			continue
		}
		secretData, err := utils.CreateDockerConfigJSON(container.Image.Username, container.Image.Password, container.Image.RegistryPath)
		if err != nil {
			return errors.Trace(err)
		}
		secret := &resources.Secret{
			Secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        a.imagePullSecretName(container.Name),
					Namespace:   a.namespace,
					Labels:      a.labels(),
					Annotations: a.annotations(config),
				},
				Type: corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: secretData,
				},
			},
		}
		desired = append(desired, secret)
	}

	secrets, err := resources.ListSecrets(context.Background(), a.coreClient, a.namespace, metav1.ListOptions{
		LabelSelector: a.labelSelector(),
	})
	if err != nil {
		return errors.Trace(err)
	}

	existing := []resources.Resource(nil)
	for _, s := range secrets {
		secret := s
		if a.matchImagePullSecret(secret.Name) {
			existing = append(existing, &secret)
		}
	}

	applier.ApplySet(existing, desired)
	return nil
}

func (a *app) annotations(config caas.ApplicationConfig) annotations.Annotation {
	return utils.ResourceTagsToAnnotations(config.ResourceTags, a.labelVersion).
		Merge(utils.AnnotationsForVersion(config.AgentVersion.String(), a.labelVersion))
}

func (a *app) upgradeAnnotations(anns annotations.Annotation, ver version.Number) annotations.Annotation {
	return anns.Merge(utils.AnnotationsForVersion(ver.String(), a.labelVersion))
}

func (a *app) labels() labels.Set {
	// TODO: add modelUUID for global resources?
	return utils.LabelsForApp(a.name, a.labelVersion)
}

func (a *app) selectorLabels() labels.Set {
	return utils.SelectorLabelsForApp(a.name, a.labelVersion)
}

func (a *app) labelSelector() string {
	return utils.LabelsToSelector(
		utils.SelectorLabelsForApp(a.name, a.labelVersion),
	).String()
}

func (a *app) fieldSelector() string {
	return fields.AndSelectors(
		fields.OneTermEqualSelector("metadata.name", a.name),
		fields.OneTermEqualSelector("metadata.namespace", a.namespace),
	).String()
}

func (a *app) secretName() string {
	return a.name + "-application-config"
}

func (a *app) serviceAccountName() string {
	return a.name
}

func (a *app) qualifiedClusterName() string {
	return fmt.Sprintf("%s-%s", a.modelName, a.name)
}

func (a *app) imagePullSecretName(containerName string) string {
	// A pod may have multiple containers with different images and thus different secrets
	return a.name + "-" + containerName + "-secret"
}

func (a *app) matchImagePullSecret(name string) bool {
	// Don't match secrets without a container name.
	if name == a.name+"-secret" {
		return false
	}
	return strings.HasPrefix(name, a.name+"-") && strings.HasSuffix(name, "-secret")
}

type annotationGetter interface {
	GetAnnotations() map[string]string
}

func (a *app) getStorageUniqPrefix(getMeta func() (annotationGetter, error)) (string, error) {
	if r, err := getMeta(); err == nil {
		// TODO: remove this function with existing one once we consolidated the annotation keys.
		if uniqID := r.GetAnnotations()[utils.AnnotationKeyApplicationUUID(a.labelVersion)]; len(uniqID) > 0 {
			return uniqID, nil
		}
	} else if !errors.IsNotFound(err) {
		return "", errors.Trace(err)
	}
	return a.randomPrefix()
}

type handleVolumeFunc func(vol corev1.Volume, mountPath string, readOnly bool) (*corev1.VolumeMount, error)
type handlePVCFunc func(pvc corev1.PersistentVolumeClaim, mountPath string, readOnly bool) (*corev1.VolumeMount, error)
type handleVolumeMountFunc func(string, corev1.VolumeMount) error
type handleStorageClassFunc func(storagev1.StorageClass) error

func (a *app) volumeName(storageName string) string {
	return fmt.Sprintf("%s-%s", a.name, storageName)
}

// pvcNames returns a mapping of volume name to PVC name for this app's PVCs.
func (a *app) pvcNames(storagePrefix string) (map[string]string, error) {
	// Fetch all Juju PVCs associated with this app
	labelSelectors := map[string]string{
		"app.kubernetes.io/managed-by": "juju",
		"app.kubernetes.io/name":       a.name,
	}
	opts := metav1.ListOptions{
		LabelSelector: utils.LabelsToSelector(labelSelectors).String(),
	}
	pvcs, err := resources.ListPersistentVolumeClaims(context.Background(), a.coreClient, a.namespace, opts)
	if err != nil {
		return nil, errors.Annotate(err, "fetching persistent volume claims")
	}

	names := make(map[string]string)
	for _, pvc := range pvcs {
		// Look up Juju storage name
		s, ok := pvc.Labels["storage.juju.is/name"]
		if !ok {
			continue
		}

		// Try to match different PVC name formats that have evolved over time
		storagePart := s + "-" + storagePrefix
		regexes := []string{
			// Sidecar "{appName}-{storageName}-{uniqueId}", e.g., "dex-auth-test-0837847d-dex-auth-0"
			"^" + regexp.QuoteMeta(a.name+"-"+storagePart),
			// Pod-spec "{storageName}-{uniqueId}", e.g., "test-0837847d-dex-auth-0"
			"^" + regexp.QuoteMeta(storagePart),
			// Legacy "juju-{storageName}-{n}", e.g., "juju-test-1-dex-auth-0"
			"^juju-" + regexp.QuoteMeta(s) + `-[0-9]+`,
		}
		for _, regex := range regexes {
			r, err := regexp.Compile(regex)
			if err != nil {
				return nil, errors.Trace(err)
			}
			match := r.FindString(pvc.Name)
			if match != "" {
				names[a.volumeName(s)] = match
				break
			}
		}
	}
	return names, nil
}

func (a *app) configureStorage(
	storageUniqueID string,
	filesystems []jujustorage.KubernetesFilesystemParams,
	storageClasses []resources.StorageClass,
	handleVolume handleVolumeFunc,
	handleVolumeMount handleVolumeMountFunc,
	handlePVC handlePVCFunc,
	handleStorageClass handleStorageClassFunc,
) error {
	storageClassMap := make(map[string]resources.StorageClass)
	for _, v := range storageClasses {
		storageClassMap[v.Name] = v
	}

	pvcNames, err := a.pvcNames(storageUniqueID)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Tracef("persistent volume claim name mapping = %v", pvcNames)

	fsNames := set.NewStrings()
	for index, fs := range filesystems {
		if fsNames.Contains(fs.StorageName) {
			return errors.NotValidf("duplicated storage name %q for %q", fs.StorageName, a.name)
		}
		fsNames.Add(fs.StorageName)

		logger.Debugf("%s has filesystem %s: %s", a.name, fs.StorageName, pretty.Sprint(fs))

		readOnly := false
		if fs.Attachment != nil {
			readOnly = fs.Attachment.ReadOnly
		}

		name := a.volumeName(fs.StorageName)
		pvcNameGetter := func(volName string) string {
			if n, ok := pvcNames[volName]; ok {
				logger.Debugf("using existing persistent volume claim %q (volume %q)", n, volName)
				return n
			}
			return fmt.Sprintf("%s-%s", volName, storageUniqueID)
		}

		vol, pvc, sc, err := a.filesystemToVolumeInfo(name, fs, storageClassMap, pvcNameGetter)
		if err != nil {
			return errors.Trace(err)
		}

		var volumeMount *corev1.VolumeMount
		mountPath := storage.GetMountPathForFilesystem(index, a.name, fs)
		if vol != nil && handleVolume != nil {
			logger.Debugf("using volume for %s filesystem %s: %s", a.name, fs.StorageName, pretty.Sprint(*vol))
			volumeMount, err = handleVolume(*vol, mountPath, readOnly)
			if err != nil {
				return errors.Trace(err)
			}
		}
		if sc != nil && handleStorageClass != nil {
			logger.Debugf("creating storage class for %s filesystem %s: %s", a.name, fs.StorageName, pretty.Sprint(*sc))
			if err = handleStorageClass(*sc); err != nil {
				return errors.Trace(err)
			}
			storageClassMap[sc.Name] = resources.StorageClass{StorageClass: *sc}
		}
		if pvc != nil && handlePVC != nil {
			logger.Debugf("using persistent volume claim for %s filesystem %s: %s", a.name, fs.StorageName, pretty.Sprint(*pvc))
			volumeMount, err = handlePVC(*pvc, mountPath, readOnly)
			if err != nil {
				return errors.Trace(err)
			}
		}

		if volumeMount != nil {
			if err = handleVolumeMount(fs.StorageName, *volumeMount); err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

func (a *app) filesystemToVolumeInfo(
	name string,
	fs jujustorage.KubernetesFilesystemParams,
	storageClasses map[string]resources.StorageClass,
	pvcNameGetter func(volName string) string,
) (*corev1.Volume, *corev1.PersistentVolumeClaim, *storagev1.StorageClass, error) {
	fsSize, err := resource.ParseQuantity(fmt.Sprintf("%dMi", fs.Size))
	if err != nil {
		return nil, nil, nil, errors.Annotatef(err, "invalid volume size %v", fs.Size)
	}

	volumeSource, err := storage.VolumeSourceForFilesystem(fs)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}
	if volumeSource != nil {
		vol := &corev1.Volume{
			Name:         name,
			VolumeSource: *volumeSource,
		}
		return vol, nil, nil, nil
	}

	params, err := storage.ParseVolumeParams(pvcNameGetter(name), fsSize, fs.Attributes)
	if err != nil {
		return nil, nil, nil, errors.Annotatef(err, "getting volume params for %s", fs.StorageName)
	}

	var newStorageClass *storagev1.StorageClass
	qualifiedStorageClassName := constants.QualifiedStorageClassName(a.namespace, params.StorageConfig.StorageClass)
	if _, ok := storageClasses[params.StorageConfig.StorageClass]; ok {
		// Do nothing
	} else if _, ok := storageClasses[qualifiedStorageClassName]; ok {
		params.StorageConfig.StorageClass = qualifiedStorageClassName
	} else {
		sp := storage.StorageProvisioner(a.namespace, a.modelName, a.modelUUID, *params)
		newStorageClass = storage.StorageClassSpec(sp, a.labelVersion)
		params.StorageConfig.StorageClass = newStorageClass.Name
	}

	labels := utils.LabelsMerge(
		utils.LabelsForStorage(fs.StorageName, a.labelVersion),
		utils.LabelsJuju)

	pvcSpec := storage.PersistentVolumeClaimSpec(*params)
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:   params.Name,
			Labels: labels,
			Annotations: utils.ResourceTagsToAnnotations(fs.ResourceTags, a.labelVersion).
				Merge(utils.AnnotationsForStorage(fs.StorageName, a.labelVersion)).
				ToMap(),
		},
		Spec: *pvcSpec,
	}
	return nil, pvc, newStorageClass, nil
}
