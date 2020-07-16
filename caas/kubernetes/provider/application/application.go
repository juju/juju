// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"path"

	"github.com/juju/charm/v7"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	k8swatcher "github.com/juju/juju/caas/kubernetes/provider/watcher"
	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/watcher"
)

var logger = loggo.GetLogger("juju.kubernetes.provider.application")

type app struct {
	name       string
	namespace  string
	model      string
	client     kubernetes.Interface
	newWatcher k8swatcher.NewK8sWatcherFunc
	clock      clock.Clock
}

func NewApplication(name string,
	namespace string,
	model string,
	client kubernetes.Interface,
	newWatcher k8swatcher.NewK8sWatcherFunc,
	clock clock.Clock) caas.Application {
	return &app{
		name:       name,
		namespace:  namespace,
		model:      model,
		client:     client,
		newWatcher: newWatcher,
		clock:      clock,
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

	var cleanups []func()
	defer func() {
		if err == nil {
			return
		}
		for _, f := range cleanups {
			f()
		}
	}()

	secret := a.secret(config)
	if cleanUp, err := a.ensureSecret(secret); err != nil {
		return errors.Annotatef(err, "creating or updating secret for %v application", a.name)
	} else {
		cleanups = append(cleanups, cleanUp)
	}

	service := &corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:        a.name,
			Labels:      a.labels(),
			Annotations: a.annotations(config),
		},
		Spec: corev1.ServiceSpec{
			Selector: a.labels(),
			Type:     corev1.ServiceTypeClusterIP,
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
	if err := a.ensureService(service); err != nil {
		return errors.Annotatef(err, "creating or updating service for %v application", a.name)
	}
	cleanups = append(cleanups, func() { _ = a.deleteService(a.name) })

	// Set up the parameters for creating charm storage (if required).
	pod, err := a.applicationPod(config)
	if err != nil {
		return errors.Annotate(err, "generating application podspec")
	}

	switch charmDeployment.DeploymentType {
	case charm.DeploymentStateful:
		numPods := int32(1)
		statefulset := &appsv1.StatefulSet{
			ObjectMeta: v1.ObjectMeta{
				Name:        a.name,
				Labels:      a.labels(),
				Annotations: a.annotations(config),
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: &numPods,
				Selector: &v1.LabelSelector{
					MatchLabels: a.labels(),
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels:      a.labels(),
						Annotations: pod.Annotations,
					},
				},
				PodManagementPolicy: appsv1.ParallelPodManagement,
			},
		}
		statefulset.Spec.Template.Spec = pod.Spec
		err = a.ensureStatefulSet(statefulset, pod.Spec)
		return errors.Annotatef(err, "creating or updating %v application StatefulSet", a.name)
	case charm.DeploymentStateless:
		return errors.NotSupportedf("deployment type stateless")
	case charm.DeploymentDaemon:
		return errors.NotSupportedf("deployment type daemon")
	default:
		return errors.NotSupportedf("unknown deployment type")
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
		{"statefulset", a.statefulSetExists, false},
		//{"deployment", a.deploymentExists, false},
		{"secret", a.secretExists, false},
		{"service", a.serviceExists, false},
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
	statefulSets := a.client.AppsV1().StatefulSets(a.namespace)
	application, err := statefulSets.Get(context.TODO(), a.name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, application.DeletionTimestamp != nil, nil
}

func (a *app) secretExists() (exists bool, terminating bool, err error) {
	secrets := a.client.CoreV1().Secrets(a.namespace)
	s, err := secrets.Get(context.TODO(), a.secretName(), v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, s.DeletionTimestamp != nil, nil
}

func (a *app) serviceExists() (exists bool, terminating bool, err error) {
	services := a.client.CoreV1().Services(a.namespace)
	s, err := services.Get(context.TODO(), a.name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, s.DeletionTimestamp != nil, nil
}

func (a *app) deploymentExists() (exists bool, terminating bool, err error) {
	deployments := a.client.AppsV1().Deployments(a.namespace)
	application, err := deployments.Get(context.TODO(), a.name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Trace(err)
	}
	return true, application.DeletionTimestamp != nil, nil
}

// Delete deletes the specified application.
func (a *app) Delete() error {
	logger.Debugf("deleting %s application", a.name)
	if err := a.deleteStatefulSet(a.name); err != nil {
		return errors.Trace(err)
	}
	if err := a.deleteService(a.name); err != nil {
		return errors.Trace(err)
	}
	secrets := a.client.CoreV1().Secrets(a.namespace)
	err := secrets.Delete(context.TODO(), a.secretName(), v1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Trace(err)
	}
	return nil
}

// Watch returns a watcher which notifies when there
// are changes to the application of the specified application.
func (a *app) Watch() (watcher.NotifyWatcher, error) {
	factory := informers.NewSharedInformerFactoryWithOptions(a.client, 0,
		informers.WithNamespace(a.namespace),
		informers.WithTweakListOptions(func(o *v1.ListOptions) {
			o.LabelSelector = a.labelSelector()
		}),
	)
	return a.newWatcher(factory.Core().V1().Pods().Informer(), a.name, a.clock)
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
		ObjectMeta: v1.ObjectMeta{
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

func (a *app) secret(config *caas.ApplicationConfig) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:        a.secretName(),
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
	}
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

func (a *app) secretName() string {
	return a.name + "-application-config"
}

// ensureService ensures a k8s service resource.
func (a *app) ensureService(spec *corev1.Service) error {
	services := a.client.CoreV1().Services(a.namespace)
	// Set any immutable fields if the service already exists.
	existing, err := services.Get(context.TODO(), spec.Name, v1.GetOptions{})
	if err == nil {
		spec.Spec.ClusterIP = existing.Spec.ClusterIP
		spec.ObjectMeta.ResourceVersion = existing.ObjectMeta.ResourceVersion
	}
	_, err = services.Update(context.TODO(), spec, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = services.Create(context.TODO(), spec, v1.CreateOptions{})
	}
	return errors.Trace(err)
}

// deleteService deletes a service resource.
func (a *app) deleteService(serviceName string) error {
	services := a.client.CoreV1().Services(a.namespace)
	err := services.Delete(context.TODO(), serviceName, v1.DeleteOptions{})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (a *app) ensureStatefulSet(spec *appsv1.StatefulSet, existingPodSpec corev1.PodSpec) error {
	_, err := a.createStatefulSet(spec)
	if errors.IsNotValid(err) {
		return errors.Annotatef(err, "ensuring stateful set %q", spec.GetName())
	} else if errors.IsAlreadyExists(err) {
		// continue
	} else if err != nil {
		return errors.Trace(err)
	} else {
		return nil
	}
	// The statefulset already exists so all we are allowed to update is replicas,
	// template, update strategy. Juju may hand out info with a slightly different
	// requested volume size due to trying to adapt the unit model to the k8s world.
	existing, err := a.getStatefulSet(spec.GetName())
	if err != nil {
		return errors.Trace(err)
	}
	existing.Spec.Replicas = spec.Spec.Replicas
	// TODO(caas) - allow storage `request` configurable - currently we only allow `limit`.
	existing.Spec.Template.Spec.Containers = existingPodSpec.Containers
	existing.Spec.Template.Spec.ServiceAccountName = existingPodSpec.ServiceAccountName
	existing.Spec.Template.Spec.AutomountServiceAccountToken = existingPodSpec.AutomountServiceAccountToken
	// NB: we can't update the Spec.ServiceName as it is immutable.
	_, err = a.updateStatefulSet(existing)
	return errors.Trace(err)
}

func (a *app) createStatefulSet(spec *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	out, err := a.client.AppsV1().StatefulSets(a.namespace).Create(context.TODO(), spec, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("stateful set %q", spec.GetName())
	}
	if k8serrors.IsInvalid(err) {
		return nil, errors.NotValidf("stateful set %q", spec.GetName())
	}
	return out, errors.Trace(err)
}

func (a *app) updateStatefulSet(spec *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	out, err := a.client.AppsV1().StatefulSets(a.namespace).Update(context.TODO(), spec, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("stateful set %q", spec.GetName())
	}
	if k8serrors.IsInvalid(err) {
		return nil, errors.NotValidf("stateful set %q", spec.GetName())
	}
	return out, errors.Trace(err)
}

func (a *app) getStatefulSet(name string) (*appsv1.StatefulSet, error) {
	out, err := a.client.AppsV1().StatefulSets(a.namespace).Get(context.TODO(), name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("stateful set %q", name)
	}
	return out, errors.Trace(err)
}

// deleteStatefulSet deletes a statefulset resource.
func (a *app) deleteStatefulSet(name string) error {
	err := a.client.AppsV1().StatefulSets(a.namespace).Delete(context.TODO(), name, v1.DeleteOptions{})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (a *app) ensureSecret(sec *corev1.Secret) (func(), error) {
	cleanUp := func() {}
	out, err := a.createSecret(sec)
	if err == nil {
		logger.Debugf("secret %q created", out.GetName())
		cleanUp = func() { _ = a.deleteSecret(out.GetName(), out.GetUID()) }
		return cleanUp, nil
	}
	if !errors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}
	_, err = a.listSecrets(sec.GetLabels())
	if err != nil {
		if errors.IsNotFound(err) {
			// sec.Name is already used for an existing secret.
			return cleanUp, errors.AlreadyExistsf("secret %q", sec.GetName())
		}
		return cleanUp, errors.Trace(err)
	}
	err = a.updateSecret(sec)
	logger.Debugf("updating secret %q", sec.GetName())
	return cleanUp, errors.Trace(err)
}

// updateSecret updates a secret resource.
func (a *app) updateSecret(sec *corev1.Secret) error {
	_, err := a.client.CoreV1().Secrets(a.namespace).Update(context.TODO(), sec, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("secret %q", sec.GetName())
	}
	return errors.Trace(err)
}

// getSecret return a secret resource.
func (a *app) getSecret(secretName string) (*corev1.Secret, error) {
	secret, err := a.client.CoreV1().Secrets(a.namespace).Get(context.TODO(), secretName, v1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errors.NotFoundf("secret %q", secretName)
		}
		return nil, errors.Trace(err)
	}
	return secret, nil
}

// createSecret creates a secret resource.
func (a *app) createSecret(secret *corev1.Secret) (*corev1.Secret, error) {
	utils.PurifyResource(secret)
	out, err := a.client.CoreV1().Secrets(a.namespace).Create(context.TODO(), secret, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("secret %q", secret.GetName())
	}
	return out, errors.Trace(err)
}

// deleteSecret deletes a secret resource.
func (a *app) deleteSecret(secretName string, uid types.UID) error {
	err := a.client.CoreV1().Secrets(a.namespace).Delete(context.TODO(), secretName, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (a *app) listSecrets(labels map[string]string) ([]corev1.Secret, error) {
	listOps := v1.ListOptions{
		LabelSelector: utils.LabelSetToSelector(labels).String(),
	}
	secList, err := a.client.CoreV1().Secrets(a.namespace).List(context.TODO(), listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(secList.Items) == 0 {
		return nil, errors.NotFoundf("secret with labels %v", labels)
	}
	return secList.Items, nil
}
