// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8sapplication

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"
	"github.com/kr/pretty"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	cache "k8s.io/client-go/tools/cache"

	"github.com/juju/juju/caas"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	k8sresources "github.com/juju/juju/caas/kubernetes/provider/resources"
	k8sstorage "github.com/juju/juju/caas/kubernetes/provider/storage"
	k8sutils "github.com/juju/juju/caas/kubernetes/provider/utils"
	k8swatcher "github.com/juju/juju/caas/kubernetes/provider/watcher"
	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/storage"
)

var logger = loggo.GetLogger("juju.kubernetes.provider.application")

type app struct {
	name           string
	namespace      string
	model          string
	deploymentType caas.DeploymentType
	client         kubernetes.Interface
	newWatcher     k8swatcher.NewK8sWatcherFunc
	clock          clock.Clock
}

func NewApplication(name string,
	namespace string,
	model string,
	deploymentType caas.DeploymentType,
	client kubernetes.Interface,
	newWatcher k8swatcher.NewK8sWatcherFunc,
	clock clock.Clock) caas.Application {
	return &app{
		name:           name,
		namespace:      namespace,
		model:          model,
		deploymentType: deploymentType,
		client:         client,
		newWatcher:     newWatcher,
		clock:          clock,
	}
}

// Ensure creates or updates an application pod with the given application
// name, agent path, and application config.
func (a *app) Ensure(config *caas.ApplicationConfig) (err error) {
	defer func() {
		logger.Errorf("Ensure %s", err)
	}()
	logger.Debugf("creating/updating %s application", a.name)

	charmDeployment := config.Charm.Meta().Deployment
	if charmDeployment == nil {
		return errors.NotValidf("charm missing deployment config for %v application", a.name)
	}

	if string(a.deploymentType) != string(charmDeployment.DeploymentType) {
		return errors.NotValidf("charm deployment type mismatch with application")
	}

	applier := k8sresources.Applier{}
	secret := k8sresources.Secret{
		Secret: corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        a.secretName(),
				Namespace:   a.namespace,
				Labels:      a.labels(),
				Annotations: a.annotations(config),
			},
			Data: map[string][]byte{
				"JUJU_K8S_APPLICATION":          []byte(a.name),
				"JUJU_K8S_MODEL":                []byte(a.model),
				"JUJU_K8S_APPLICATION_PASSWORD": []byte(config.IntroductionSecret),
				"JUJU_K8S_CONTROLLER_ADDRESSES": []byte(config.ControllerAddresses),
				"JUJU_K8S_CONTROLLER_CA_CERT":   []byte(config.ControllerCertBundle),
			},
		},
	}
	applier.Apply(&secret)

	service := k8sresources.Service{
		Service: corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        a.name,
				Namespace:   a.namespace,
				Labels:      a.labels(),
				Annotations: a.annotations(config),
			},
			Spec: corev1.ServiceSpec{
				Selector: a.labels(),
				Type:     corev1.ServiceTypeClusterIP,
			},
		},
	}
	for _, v := range charmDeployment.ServicePorts {
		port := corev1.ServicePort{
			Name:       v.Name,
			Port:       int32(v.Port),
			TargetPort: intstr.FromInt(v.TargetPort),
			Protocol:   corev1.Protocol(v.Protocol),
			//AppProtocol:    core.Protocol(v.AppProtocol),
		}
		service.Spec.Ports = append(service.Spec.Ports, port)
	}
	applier.Apply(&service)

	// Set up the parameters for creating charm storage (if required).
	podSpec, err := a.applicationPodSpec(config)
	if err != nil {
		return errors.Annotate(err, "generating application podspec")
	}

	var handleVolume handleVolumeFunc = func(v corev1.Volume) error {
		podSpec.Volumes = append(podSpec.Volumes, v)
		return nil
	}
	var handleVolumeMount handleVolumeMountFunc = func(m corev1.VolumeMount) error {
		for i := range podSpec.Containers {
			podSpec.Containers[i].VolumeMounts = append(podSpec.Containers[i].VolumeMounts, m)
		}
		return nil
	}
	var createPVC handlePVCFunc = func(pvc corev1.PersistentVolumeClaim) error {
		r := k8sresources.PersistentVolumeClaim{
			PersistentVolumeClaim: pvc,
		}
		r.Namespace = a.namespace
		applier.Apply(&r)
		return nil
	}
	storageClasses, err := k8sresources.ListStorageClass(context.Background(), a.client, metav1.ListOptions{})
	if err != nil {
		return errors.Trace(err)
	}
	var handleStorageClass = func(sc storagev1.StorageClass) error {
		applier.Apply(&k8sresources.StorageClass{StorageClass: sc})
		return nil
	}
	var configureStorage = func(handlePVC handlePVCFunc) error {
		err = a.configureStorage(config.Filesystems, storageClasses, handleVolume, handleVolumeMount, handlePVC, handleStorageClass)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	switch a.deploymentType {
	case caas.DeploymentStateful:
		numPods := int32(1)
		statefulset := k8sresources.StatefulSet{
			StatefulSet: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:        a.name,
					Namespace:   a.namespace,
					Labels:      a.labels(),
					Annotations: a.annotations(config),
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &numPods,
					Selector: &metav1.LabelSelector{
						MatchLabels: a.labels(),
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels:      a.labels(),
							Annotations: a.annotations(config),
						},
						Spec: *podSpec,
					},
					PodManagementPolicy: appsv1.ParallelPodManagement,
				},
			},
		}
		err = configureStorage(func(pvc corev1.PersistentVolumeClaim) error {
			statefulset.Spec.VolumeClaimTemplates = append(statefulset.Spec.VolumeClaimTemplates, pvc)
			return nil
		})
		if err != nil {
			return errors.Trace(err)
		}
		applier.Apply(&statefulset)
	case caas.DeploymentStateless:
		numPods := int32(1)
		deployment := k8sresources.Deployment{
			Deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:        a.name,
					Namespace:   a.namespace,
					Labels:      a.labels(),
					Annotations: a.annotations(config),
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &numPods,
					Selector: &metav1.LabelSelector{
						MatchLabels: a.labels(),
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels:      a.labels(),
							Annotations: a.annotations(config),
						},
						Spec: *podSpec,
					},
				},
			},
		}
		err = configureStorage(createPVC)
		if err != nil {
			return errors.Trace(err)
		}
		applier.Apply(&deployment)
	case caas.DeploymentDaemon:
		daemonset := k8sresources.DaemonSet{
			DaemonSet: appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:        a.name,
					Namespace:   a.namespace,
					Labels:      a.labels(),
					Annotations: a.annotations(config),
				},
				Spec: appsv1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: a.labels(),
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels:      a.labels(),
							Annotations: a.annotations(config),
						},
						Spec: *podSpec,
					},
				},
			},
		}
		err = configureStorage(createPVC)
		if err != nil {
			return errors.Trace(err)
		}
		applier.Apply(&daemonset)
	default:
		return errors.NotSupportedf("unknown deployment type")
	}

	return applier.Run(context.Background(), a.client)
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

func (a *app) statefulSetExists() (exists bool, terminating bool, err error) {
	ss := k8sresources.NewStatefulSet(a.name, a.namespace)
	err = ss.Get(context.Background(), a.client)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, ss.DeletionTimestamp != nil, nil
}

func (a *app) deploymentExists() (exists bool, terminating bool, err error) {
	ss := k8sresources.NewDeployment(a.name, a.namespace)
	err = ss.Get(context.Background(), a.client)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, ss.DeletionTimestamp != nil, nil
}

func (a *app) daemonSetExists() (exists bool, terminating bool, err error) {
	ss := k8sresources.NewDaemonSet(a.name, a.namespace)
	err = ss.Get(context.Background(), a.client)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, ss.DeletionTimestamp != nil, nil
}

func (a *app) secretExists() (exists bool, terminating bool, err error) {
	ss := k8sresources.NewSecret(a.secretName(), a.namespace)
	err = ss.Get(context.Background(), a.client)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, ss.DeletionTimestamp != nil, nil
}

func (a *app) serviceExists() (exists bool, terminating bool, err error) {
	ss := k8sresources.NewService(a.name, a.namespace)
	err = ss.Get(context.Background(), a.client)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, ss.DeletionTimestamp != nil, nil
}

// Delete deletes the specified application.
func (a *app) Delete() error {
	logger.Debugf("deleting %s application", a.name)
	applier := &k8sresources.Applier{}
	switch a.deploymentType {
	case caas.DeploymentStateful:
		applier.Delete(k8sresources.NewStatefulSet(a.name, a.namespace))
	case caas.DeploymentStateless:
		applier.Delete(k8sresources.NewDeployment(a.name, a.namespace))
	case caas.DeploymentDaemon:
		applier.Delete(k8sresources.NewDaemonSet(a.name, a.namespace))
	default:
		return errors.NotSupportedf("unknown deployment type")
	}
	applier.Delete(k8sresources.NewService(a.name, a.namespace))
	applier.Delete(k8sresources.NewSecret(a.secretName(), a.namespace))
	return applier.Run(context.Background(), a.client)
}

// Watch returns a watcher which notifies when there
// are changes to the application of the specified application.
func (a *app) Watch() (watcher.NotifyWatcher, error) {
	factory := informers.NewSharedInformerFactoryWithOptions(a.client, 0,
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
	return a.newWatcher(informer, a.name, a.clock)
}

func (a *app) WatchReplicas() (watcher.NotifyWatcher, error) {
	factory := informers.NewSharedInformerFactoryWithOptions(a.client, 0,
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
		ss := k8sresources.NewStatefulSet(a.name, a.namespace)
		err := ss.Get(context.Background(), a.client)
		if err != nil {
			return caas.ApplicationState{}, errors.Trace(err)
		}
		if ss.Spec.Replicas == nil {
			return caas.ApplicationState{}, errors.Errorf("missing replicas")
		}
		state.DesiredReplicas = int(*ss.Spec.Replicas)
	case caas.DeploymentStateless:
		d := k8sresources.NewDeployment(a.name, a.namespace)
		err := d.Get(context.Background(), a.client)
		if err != nil {
			return caas.ApplicationState{}, errors.Trace(err)
		}
		if d.Spec.Replicas == nil {
			return caas.ApplicationState{}, errors.Errorf("missing replicas")
		}
		state.DesiredReplicas = int(*d.Spec.Replicas)
	case caas.DeploymentDaemon:
		d := k8sresources.NewDeployment(a.name, a.namespace)
		err := d.Get(context.Background(), a.client)
		if err != nil {
			return caas.ApplicationState{}, errors.Trace(err)
		}
		state.DesiredReplicas = int(d.Status.Replicas)
	default:
		return caas.ApplicationState{}, errors.NotSupportedf("unknown deployment type")
	}
	next := ""
	for {
		res, err := a.client.CoreV1().Pods(a.namespace).List(context.Background(), metav1.ListOptions{
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
	return state, nil
}

// applicationPodSpec returns a PodSpec for the application pod
// of the specified application.
func (a *app) applicationPodSpec(config *caas.ApplicationConfig) (*corev1.PodSpec, error) {
	appSecret := a.secretName()

	if config.Charm.Meta().Deployment == nil {
		return nil, errors.NotValidf("charm missing deployment")
	}
	containerImageName := config.Charm.Meta().Deployment.ContainerImageName
	if containerImageName == "" {
		return nil, errors.NotValidf("charm missing container-image-name")
	}

	containerPorts := []corev1.ContainerPort(nil)
	for _, v := range config.Charm.Meta().Deployment.ServicePorts {
		containerPorts = append(containerPorts, corev1.ContainerPort{
			Name:          v.Name,
			Protocol:      corev1.Protocol(v.Protocol),
			ContainerPort: int32(v.TargetPort),
		})
	}

	jujuDataDir, err := paths.DataDir("kubernetes")
	if err != nil {
		return nil, errors.Trace(err)
	}

	automountToken := false
	return &corev1.PodSpec{
		AutomountServiceAccountToken: &automountToken,
		InitContainers: []corev1.Container{{
			Name:            "juju-unit-init",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Image:           config.AgentImagePath,
			WorkingDir:      jujuDataDir,
			Command:         []string{"/opt/k8sagent"},
			Args:            []string{"init"},
			Env: []corev1.EnvVar{
				{
					Name: "JUJU_K8S_POD_NAME",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "metadata.name",
						},
					},
				},
				{
					Name: "JUJU_K8S_POD_UUID",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "metadata.uid",
						},
					},
				},
			},
			EnvFrom: []corev1.EnvFromSource{{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: appSecret,
					},
				},
			}},
			VolumeMounts: []corev1.VolumeMount{{
				Name:      "juju-data-dir",
				MountPath: jujuDataDir,
				SubPath:   strings.TrimPrefix(jujuDataDir, "/"),
			}, {
				Name:      "juju-data-dir",
				MountPath: "/shared/usr/bin",
				SubPath:   "usr/bin",
			}},
		}},
		Containers: []corev1.Container{{
			Name:            "juju-unit-agent",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Image:           config.AgentImagePath,
			WorkingDir:      jujuDataDir,
			Command:         []string{"/opt/k8sagent"},
			Args:            []string{"unit", "--data-dir", jujuDataDir},
			VolumeMounts: []corev1.VolumeMount{{
				Name:      "juju-data-dir",
				MountPath: jujuDataDir,
				SubPath:   strings.TrimPrefix(jujuDataDir, "/"),
			}},
		}, {
			Name:            config.Charm.Meta().Name,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Image:           containerImageName,
			Command:         []string{"/usr/bin/pebble"},
			VolumeMounts: []corev1.VolumeMount{{
				Name:      "juju-data-dir",
				MountPath: "/usr/bin/pebble",
				SubPath:   "usr/bin/pebble",
				ReadOnly:  true,
			}},
			Ports: containerPorts,
		}},
		Volumes: []corev1.Volume{{
			Name: "juju-data-dir",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					SizeLimit: resource.NewScaledQuantity(1, resource.Giga),
				},
			},
		}},
	}, nil
}

func (a *app) annotations(config *caas.ApplicationConfig) annotations.Annotation {
	annotations := k8sutils.ResourceTagsToAnnotations(config.ResourceTags).
		Add(k8sconstants.LabelVersion, config.AgentVersion.String())
	return annotations
}

func (a *app) labels() map[string]string {
	return map[string]string{k8sconstants.LabelApplication: a.name}
}

func (a *app) labelSelector() string {
	return k8sutils.LabelSetToSelector(map[string]string{
		k8sconstants.LabelApplication: a.name,
	}).String()
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

type handleVolumeFunc func(corev1.Volume) error
type handleVolumeMountFunc func(corev1.VolumeMount) error
type handlePVCFunc func(corev1.PersistentVolumeClaim) error
type handleStorageClassFunc func(storagev1.StorageClass) error

func (a *app) configureStorage(filesystems []storage.KubernetesFilesystemParams,
	storageClasses []k8sresources.StorageClass,
	handleVolume handleVolumeFunc,
	handleVolumeMount handleVolumeMountFunc,
	handlePVC handlePVCFunc,
	handleStorageClass handleStorageClassFunc,
) error {
	storageClassMap := make(map[string]k8sresources.StorageClass)
	for _, v := range storageClasses {
		storageClassMap[v.Name] = v
	}

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

		name := fmt.Sprintf("%s-%s", a.name, fs.StorageName)
		vol, pvc, sc, err := a.filesystemToVolumeInfo(name, fs, storageClassMap)
		if err != nil {
			return errors.Trace(err)
		}
		mountPath := k8sstorage.GetMountPathForFilesystem(index, a.name, fs)
		if vol != nil && handleVolume != nil {
			logger.Debugf("using volume for %s filesystem %s: %s", a.name, fs.StorageName, pretty.Sprint(*vol))
			err = handleVolume(*vol)
			if err != nil {
				return errors.Trace(err)
			}
		}
		if sc != nil && handleStorageClass != nil {
			logger.Debugf("creating storage class for %s filesystem %s: %s", a.name, fs.StorageName, pretty.Sprint(*sc))
			err = handleStorageClass(*sc)
			if err != nil {
				return errors.Trace(err)
			}
			storageClassMap[sc.Name] = k8sresources.StorageClass{StorageClass: *sc}
		}
		if pvc != nil && handlePVC != nil {
			logger.Debugf("using persistent volume claim for %s filesystem %s: %s", a.name, fs.StorageName, pretty.Sprint(*pvc))
			err = handlePVC(*pvc)
			if err != nil {
				return errors.Trace(err)
			}
		}
		err = handleVolumeMount(corev1.VolumeMount{
			Name:      name,
			MountPath: mountPath,
			ReadOnly:  readOnly,
		})
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (a *app) filesystemToVolumeInfo(name string,
	fs storage.KubernetesFilesystemParams,
	storageClasses map[string]k8sresources.StorageClass) (*corev1.Volume, *corev1.PersistentVolumeClaim, *storagev1.StorageClass, error) {
	fsSize, err := resource.ParseQuantity(fmt.Sprintf("%dMi", fs.Size))
	if err != nil {
		return nil, nil, nil, errors.Annotatef(err, "invalid volume size %v", fs.Size)
	}

	volumeSource, err := k8sstorage.VolumeSourceForFilesystem(fs)
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

	params, err := k8sstorage.ParseVolumeParams(name, fsSize, fs.Attributes)
	if err != nil {
		return nil, nil, nil, errors.Annotatef(err, "getting volume params for %s", fs.StorageName)
	}

	var newStorageClass *storagev1.StorageClass
	qualifiedStorageClassName := k8sconstants.QualifiedStorageClassName(a.namespace, params.StorageConfig.StorageClass)
	if _, ok := storageClasses[params.StorageConfig.StorageClass]; ok {
		// Do nothing
	} else if _, ok := storageClasses[qualifiedStorageClassName]; ok {
		params.StorageConfig.StorageClass = qualifiedStorageClassName
	} else {
		sp := k8sstorage.StorageProvisioner(a.namespace, *params)
		newStorageClass = k8sstorage.StorageClassSpec(sp)
		params.StorageConfig.StorageClass = newStorageClass.Name
	}

	pvcSpec := k8sstorage.PersistentVolumeClaimSpec(*params)
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: params.Name,
			Annotations: k8sutils.ResourceTagsToAnnotations(fs.ResourceTags).
				Add(k8sconstants.LabelStorage, fs.StorageName).ToMap(),
		},
		Spec: *pvcSpec,
	}
	return nil, pvc, newStorageClass, nil
}

func int64Ptr(v int64) *int64 {
	return &v
}

func boolPtr(b bool) *bool {
	return &b
}
