// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"path"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	cache "k8s.io/client-go/tools/cache"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	k8swatcher "github.com/juju/juju/caas/kubernetes/provider/watcher"
	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/watcher"
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
	logger.Debugf("creating/updating %s application", a.name)

	charmDeployment := config.Charm.Meta().Deployment
	if charmDeployment == nil {
		return errors.NotValidf("charm missing deployment config for %v application", a.name)
	}

	if string(a.deploymentType) != string(charmDeployment.DeploymentType) {
		return errors.NotValidf("charm deployment type mismatch with application")
	}

	applier := resources.Applier{}
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
				"JUJU_K8S_MODEL":                []byte(a.model),
				"JUJU_K8S_APPLICATION_PASSWORD": []byte(config.IntroductionSecret),
				"JUJU_K8S_CONTROLLER_ADDRESSES": []byte(config.ControllerAddresses),
				"JUJU_K8S_CONTROLLER_CA_CERT":   []byte(config.ControllerCertBundle),
			},
		},
	}
	applier.Apply(&secret)

	service := resources.Service{
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
	pod, err := a.applicationPod(config)
	if err != nil {
		return errors.Annotate(err, "generating application podspec")
	}

	switch a.deploymentType {
	case caas.DeploymentStateful:
		numPods := int32(1)
		statefulset := resources.StatefulSet{
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
							Annotations: pod.Annotations,
						},
					},
					PodManagementPolicy: appsv1.ParallelPodManagement,
				},
			},
		}
		statefulset.Spec.Template.Spec = pod.Spec
		applier.Apply(&statefulset)
	case caas.DeploymentStateless:
		numPods := int32(1)
		deployment := resources.Deployment{
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
							Annotations: pod.Annotations,
						},
					},
				},
			},
		}
		deployment.Spec.Template.Spec = pod.Spec
		applier.Apply(&deployment)
	case caas.DeploymentDaemon:
		daemonset := resources.DaemonSet{
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
							Annotations: pod.Annotations,
						},
					},
				},
			},
		}
		daemonset.Spec.Template.Spec = pod.Spec
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
	ss := resources.NewStatefulSet(a.name, a.namespace)
	err = ss.Get(context.Background(), a.client)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, ss.DeletionTimestamp != nil, nil
}

func (a *app) deploymentExists() (exists bool, terminating bool, err error) {
	ss := resources.NewDeployment(a.name, a.namespace)
	err = ss.Get(context.Background(), a.client)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, ss.DeletionTimestamp != nil, nil
}

func (a *app) daemonSetExists() (exists bool, terminating bool, err error) {
	ss := resources.NewDaemonSet(a.name, a.namespace)
	err = ss.Get(context.Background(), a.client)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, ss.DeletionTimestamp != nil, nil
}

func (a *app) secretExists() (exists bool, terminating bool, err error) {
	ss := resources.NewSecret(a.secretName(), a.namespace)
	err = ss.Get(context.Background(), a.client)
	if errors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, ss.DeletionTimestamp != nil, nil
}

func (a *app) serviceExists() (exists bool, terminating bool, err error) {
	ss := resources.NewService(a.name, a.namespace)
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
	applier := &resources.Applier{}
	switch a.deploymentType {
	case caas.DeploymentStateful:
		applier.Delete(resources.NewStatefulSet(a.name, a.namespace))
	case caas.DeploymentStateless:
		applier.Delete(resources.NewDeployment(a.name, a.namespace))
	case caas.DeploymentDaemon:
		applier.Delete(resources.NewDaemonSet(a.name, a.namespace))
	default:
		return errors.NotSupportedf("unknown deployment type")
	}
	applier.Delete(resources.NewService(a.name, a.namespace))
	applier.Delete(resources.NewSecret(a.secretName(), a.namespace))
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
		ss := resources.NewStatefulSet(a.name, a.namespace)
		err := ss.Get(context.Background(), a.client)
		if err != nil {
			return caas.ApplicationState{}, errors.Trace(err)
		}
		if ss.Spec.Replicas == nil {
			return caas.ApplicationState{}, errors.Errorf("missing replicas")
		}
		state.DesiredReplicas = int(*ss.Spec.Replicas)
	case caas.DeploymentStateless:
		d := resources.NewDeployment(a.name, a.namespace)
		err := d.Get(context.Background(), a.client)
		if err != nil {
			return caas.ApplicationState{}, errors.Trace(err)
		}
		if d.Spec.Replicas == nil {
			return caas.ApplicationState{}, errors.Errorf("missing replicas")
		}
		state.DesiredReplicas = int(*d.Spec.Replicas)
	case caas.DeploymentDaemon:
		d := resources.NewDeployment(a.name, a.namespace)
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

// applicationPod returns a *core.Pod for the application pod
// of the specified application.
func (a *app) applicationPod(config *caas.ApplicationConfig) (*corev1.Pod, error) {
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
	readOnly := true
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        a.name,
			Annotations: a.annotations(config),
			Labels:      a.labels(),
		},
		Spec: corev1.PodSpec{
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
				}},
			}},
			Containers: []corev1.Container{{
				Name:            config.Charm.Meta().Name,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Image:           containerImageName,
				WorkingDir:      jujuDataDir,
				Command:         []string{"/juju/charm-base/opt/k8sagent"},
				Args:            []string{"unit", "--data-dir", jujuDataDir},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "juju-data-dir",
					MountPath: path.Join(jujuDataDir, constants.TemplateFileNameAgentConf),
					SubPath:   constants.TemplateFileNameAgentConf,
				}, {
					Name:      "juju-k8s-agent",
					MountPath: "/juju/charm-base",
				}},
				Ports: containerPorts,
			}},
			Volumes: []corev1.Volume{{
				Name: "juju-data-dir",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			}, {
				Name: "juju-k8s-agent",
				VolumeSource: corev1.VolumeSource{
					CSI: &corev1.CSIVolumeSource{
						Driver:   "image.csi.k8s.juju.is",
						ReadOnly: &readOnly,
						VolumeAttributes: map[string]string{
							"image": config.AgentImagePath,
						},
					},
				},
			}},
		},
	}, nil
}

func (a *app) annotations(config *caas.ApplicationConfig) annotations.Annotation {
	annotations := utils.ResourceTagsToAnnotations(config.ResourceTags).
		Add(constants.LabelVersion, config.AgentVersion.String())
	return annotations
}

func (a *app) labels() map[string]string {
	return map[string]string{constants.LabelApplication: a.name}
}

func (a *app) labelSelector() string {
	return utils.LabelSetToSelector(map[string]string{
		constants.LabelApplication: a.name,
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
