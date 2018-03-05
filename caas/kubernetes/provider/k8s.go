// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/retry"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"
	"k8s.io/client-go/kubernetes"
	k8serrors "k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/util/intstr"
	"k8s.io/client-go/pkg/util/yaml"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
	"github.com/juju/juju/version"
	"github.com/juju/juju/watcher"
)

var logger = loggo.GetLogger("juju.kubernetes.provider")

const (
	labelOperator    = "juju-operator"
	labelVersion     = "juju-version"
	labelApplication = "juju-application"
	labelUnit        = "juju-unit"
)

// TODO(caas) - add unit tests

type kubernetesClient struct {
	*kubernetes.Clientset

	// namespace is the k8s namespace to use when
	// creating k8s resources.
	namespace string
}

// NewK8sProvider returns a kubernetes client for the specified cloud.
func NewK8sProvider(cloudSpec environs.CloudSpec, namespace string) (caas.Broker, error) {
	config, err := newK8sConfig(cloudSpec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &kubernetesClient{Clientset: client, namespace: namespace}, nil
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
		Username: credentialAttrs["Username"],
		Password: credentialAttrs["Password"],
		TLSClientConfig: rest.TLSClientConfig{
			CertData: []byte(credentialAttrs["ClientCertificateData"]),
			KeyData:  []byte(credentialAttrs["ClientKeyData"]),
			CAData:   CAData,
		},
	}, nil
}

// EnsureNamespace ensures this broker's namespace is created.
func (k *kubernetesClient) EnsureNamespace() error {
	ns := &v1.Namespace{ObjectMeta: v1.ObjectMeta{Name: k.namespace}}
	namespaces := k.CoreV1().Namespaces()
	_, err := namespaces.Update(ns)
	if k8serrors.IsNotFound(err) {
		_, err = namespaces.Create(ns)
	}
	return errors.Trace(err)
}

// DeleteNamespace deletes this broker's namespace.
func (k *kubernetesClient) DeleteNamespace() error {
	orphanDependents := false
	namespaces := k.CoreV1().Namespaces()
	err := namespaces.Delete(k.namespace, &v1.DeleteOptions{
		OrphanDependents: &orphanDependents,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(err)
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
	if err := k.ensureConfigMap(operatorConfigMap(appName, config)); err != nil {
		return errors.Annotate(err, "creating or updating ConfigMap")
	}
	pod := operatorPod(appName, agentPath)
	// See if we are able to just update the pod image, otherwise we'll need to
	// delete and create as without deployment controller that's all we can do.
	// TODO(caas) - consider using a deployment controller for operator for easier management
	if err := k.maybeUpdatePodImage(
		operatorSelector(appName), version.Current.String(), pod.Spec.Containers[0].Image); err != nil {
		return k.ensurePod(pod)
	}
	return nil
}

// DeleteOperator deletes the specified operator.
func (k *kubernetesClient) DeleteOperator(appName string) (err error) {
	logger.Debugf("deleting %s operator", appName)
	podName := operatorPodName(appName)
	if err := k.deletePod(podName); err != nil {
		return errors.Trace(err)
	}

	// TODO(caas) - this is a stop gap until we implement a CAAS model manager worker
	// If there are no more resources left running, we can delete the namespace.
	// When an application is deleted, the operator is the last to go so we use that
	// as an opportunity to see if there's anything left running.
	deployments := k.ExtensionsV1beta1().Deployments(k.namespace)
	deploymentsList, err := deployments.List(v1.ListOptions{})
	if err != nil {
		return errors.Trace(err)
	}
	if len(deploymentsList.Items) > 0 {
		return nil
	}

	pods := k.CoreV1().Pods(k.namespace)
	podsList, err := pods.List(v1.ListOptions{})
	if err != nil {
		return errors.Trace(err)
	}
	allTerminating := len(podsList.Items) == 0
	for _, pod := range podsList.Items {
		allTerminating = allTerminating && pod.DeletionTimestamp != nil
		if !allTerminating {
			break
		}
	}
	if allTerminating {
		err = k.DeleteNamespace()
	}
	return err
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
	return errors.Trace(k.deleteDeployment(appName))
}

// EnsureService creates or updates a service for pods with the given spec.
func (k *kubernetesClient) EnsureService(
	appName string, spec *caas.ContainerSpec, numUnits int, config application.ConfigAttributes,
) (err error) {
	logger.Debugf("creating/updating application %s", appName)

	if numUnits < 0 {
		return errors.Errorf("number of units must be >= 0")
	}
	if spec == nil {
		return errors.Errorf("missing container spec")
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

	unitSpec, err := makeUnitSpec(spec)
	if err != nil {
		return errors.Annotatef(err, "parsing unit spec for %s", appName)
	}

	// See if a deployment controller is required. If num units is > 0 then
	// a deployment controller set to create that number of units is required.
	if numUnits > 0 {
		numPods := int32(numUnits)
		if err := k.configureDeployment(appName, unitSpec, spec.Files, &numPods); err != nil {
			return errors.Annotate(err, "creating or updating deployment controller")
		}
		cleanups = append(cleanups, func() { k.deleteDeployment(appName) })
	}

	var ports []v1.ContainerPort
	for _, c := range unitSpec.Pod.Containers {
		for _, p := range c.Ports {
			if p.ContainerPort == 0 {
				continue
			}
			ports = append(ports, p)
		}
	}
	if err := k.configureService(appName, ports, config); err != nil {
		return errors.Annotatef(err, "creating or updating service for %v", appName)
	}
	return nil
}

type configMapNameFunc func(fileSetName string) string

func (k *kubernetesClient) configurePodFiles(podSpec *v1.PodSpec, files []caas.FileSet, cfgMapName configMapNameFunc) error {
	for _, fileSet := range files {
		cfgName := cfgMapName(fileSet.Name)
		vol := v1.Volume{Name: cfgName}
		if fileSet.Secret == nil {
			if err := k.ensureConfigMap(filesetConfigMap(cfgName, &fileSet)); err != nil {
				return errors.Annotatef(err, "creating or updating ConfigMap for file set %v", cfgName)
			}
			vol.ConfigMap = &v1.ConfigMapVolumeSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: cfgName,
				},
			}
		} else {
			vol.Secret = &v1.SecretVolumeSource{SecretName: fileSet.Secret.Name}
			for _, kp := range fileSet.Secret.SecretKeys {
				vol.Secret.Items = append(vol.Secret.Items, v1.KeyToPath{Key: kp.Key, Path: kp.Path})
			}
		}
		podSpec.Volumes = []v1.Volume{vol}
		podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, v1.VolumeMount{
			Name:      cfgName,
			MountPath: fileSet.MountPath,
			ReadOnly:  fileSet.Secret != nil,
		})
	}
	return nil
}

func (k *kubernetesClient) configureDeployment(appName string, unitSpec *unitSpec, files []caas.FileSet, replicas *int32) error {
	logger.Debugf("creating/updating deployment for %s", appName)

	// Add the specified file to the pod spec.
	cfgName := func(fileSetName string) string {
		return applicationConfigMapName(appName, fileSetName)
	}
	podSpec := unitSpec.Pod
	if err := k.configurePodFiles(&podSpec, files, cfgName); err != nil {
		return errors.Trace(err)
	}

	namePrefix := resourceNamePrefix(appName)
	deployment := &v1beta1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   deploymentName(appName),
			Labels: map[string]string{labelApplication: appName}},
		Spec: v1beta1.DeploymentSpec{
			Replicas: replicas,
			Selector: &unversioned.LabelSelector{
				MatchLabels: map[string]string{labelApplication: appName},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: namePrefix,
					Labels:       map[string]string{labelApplication: appName},
				},
				Spec: podSpec,
			},
		},
	}
	return k.ensureDeployment(deployment)
}

func (k *kubernetesClient) ensureDeployment(spec *v1beta1.Deployment) error {
	deployments := k.ExtensionsV1beta1().Deployments(k.namespace)
	_, err := deployments.Update(spec)
	if k8serrors.IsNotFound(err) {
		_, err = deployments.Create(spec)
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteDeployment(appName string) error {
	orphanDependents := false
	deployments := k.ExtensionsV1beta1().Deployments(k.namespace)
	err := deployments.Delete(deploymentName(appName), &v1.DeleteOptions{OrphanDependents: &orphanDependents})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) configureService(appName string, containerPorts []v1.ContainerPort, config application.ConfigAttributes) error {
	logger.Debugf("creating/updating service for %s", appName)

	var ports []v1.ServicePort
	for i, cp := range containerPorts {
		// We normally expect a single container port for most use cases.
		// We allow the user to specify what first service port should be,
		// otherwise it just defaults to the container port.
		// TODO(caas) - consider allowing all service ports to be specified
		var targetPort intstr.IntOrString
		if i == 0 {
			targetPort = intstr.FromInt(config.GetInt(serviceTargetPortConfigKey, int(cp.ContainerPort)))
		}
		ports = append(ports, v1.ServicePort{
			Protocol:   cp.Protocol,
			Port:       cp.ContainerPort,
			TargetPort: targetPort,
		})
	}

	serviceType := v1.ServiceType(config.GetString(serviceTypeConfigKey, defaultServiceType))
	service := &v1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   deploymentName(appName),
			Labels: map[string]string{labelApplication: appName}},
		Spec: v1.ServiceSpec{
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

func (k *kubernetesClient) ensureService(spec *v1.Service) error {
	services := k.CoreV1().Services(k.namespace)
	// Set any immutable fields if the service already exists.
	existing, err := services.Get(spec.Name)
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
	orphanDependents := false
	services := k.CoreV1().Services(k.namespace)
	err := services.Delete(deploymentName(appName), &v1.DeleteOptions{OrphanDependents: &orphanDependents})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

// ExposeService sets up external access to the specified application.
func (k *kubernetesClient) ExposeService(appName string, config application.ConfigAttributes) error {
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

	svc, err := k.CoreV1().Services(k.namespace).Get(deploymentName(appName))
	if err != nil {
		return errors.Trace(err)
	}
	if len(svc.Spec.Ports) == 0 {
		return errors.Errorf("cannot create ingress rule for service %q without a port", svc.Name)
	}
	spec := &v1beta1.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Name:   deploymentName(appName),
			Labels: map[string]string{labelApplication: appName},
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
	orphanDependents := false
	ingress := k.ExtensionsV1beta1().Ingresses(k.namespace)
	err := ingress.Delete(deploymentName(appName), &v1.DeleteOptions{OrphanDependents: &orphanDependents})
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
	return newKubernetesWatcher(w, appName)
}

// Units returns all units of the specified application.
func (k *kubernetesClient) Units(appName string) ([]caas.Unit, error) {
	pods := k.CoreV1().Pods(k.namespace)
	podsList, err := pods.List(v1.ListOptions{
		LabelSelector: applicationSelector(appName),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result []caas.Unit
	now := time.Now()
	for _, p := range podsList.Items {
		var ports []string
		for _, c := range p.Spec.Containers {
			for _, p := range c.Ports {
				ports = append(ports, fmt.Sprintf("%v/%v", p.ContainerPort, p.Protocol))
			}
		}
		terminated := p.DeletionTimestamp != nil
		unitInfo := caas.Unit{
			Id:      string(p.UID),
			Address: p.Status.PodIP,
			Ports:   ports,
			Dying:   terminated,
			Status: status.StatusInfo{
				Status:  k.jujuStatus(p.Status.Phase, terminated),
				Message: p.Status.Message,
				Since:   &now,
			},
		}
		// If the pod is a Juju unit label, it was created directly
		// by Juju an we can extract the unit tag to include on the result.
		unitLabel := p.Labels[labelUnit]
		if strings.HasPrefix(unitLabel, "juju-") {
			unitTag, err := names.ParseUnitTag(unitLabel[5:])
			if err == nil {
				unitInfo.UnitTag = unitTag.String()
			}
		}
		result = append(result, unitInfo)
	}
	return result, nil
}

func (k *kubernetesClient) jujuStatus(podPhase v1.PodPhase, terminated bool) status.Status {
	if terminated {
		return status.Terminated
	}
	switch podPhase {
	case v1.PodRunning:
		return status.Running
	case v1.PodFailed:
		return status.Error
	case v1.PodPending:
		return status.Allocating
	default:
		return status.Unknown
	}
}

// EnsureUnit creates or updates a unit pod with the given unit name and spec.
func (k *kubernetesClient) EnsureUnit(appName, unitName string, spec *caas.ContainerSpec) error {
	logger.Debugf("creating/updating unit %s", unitName)
	unitSpec, err := makeUnitSpec(spec)
	if err != nil {
		return errors.Annotatef(err, "parsing spec for %s", unitName)
	}
	podName := unitPodName(unitName)
	pod := v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: podName,
			Labels: map[string]string{
				labelApplication: appName,
				labelUnit:        podName}},
		Spec: unitSpec.Pod,
	}

	// Add the specified file to the pod spec.
	cfgName := func(fileSetName string) string {
		return unitConfigMapName(unitName, fileSetName)
	}
	if err := k.configurePodFiles(&pod.Spec, spec.Files, cfgName); err != nil {
		return errors.Trace(err)
	}
	return k.ensurePod(&pod)
}

// filesetConfigMap returns a *v1.ConfigMap for a pod
// of the specified unit, with the specified files.
func filesetConfigMap(configMapName string, files *caas.FileSet) *v1.ConfigMap {
	result := &v1.ConfigMap{
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

// DeleteUnit deletes a unit pod with the given unit name.
func (k *kubernetesClient) DeleteUnit(unitName string) error {
	logger.Debugf("deleting unit %s", unitName)
	podName := unitPodName(unitName)
	return k.deletePod(podName)
}

func (k *kubernetesClient) ensureConfigMap(configMap *v1.ConfigMap) error {
	configMaps := k.CoreV1().ConfigMaps(k.namespace)
	_, err := configMaps.Update(configMap)
	if k8serrors.IsNotFound(err) {
		_, err = configMaps.Create(configMap)
	}
	return errors.Trace(err)
}

// maybeUpdatePodImage updates the pod image for the selector so long as its Juju version
// label matches the given version.
func (k *kubernetesClient) maybeUpdatePodImage(selector, jujuVersion, image string) error {
	pods := k.CoreV1().Pods(k.namespace)
	podList, err := pods.List(v1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return errors.Trace(err)
	}
	if len(podList.Items) == 0 {
		return errors.NotFoundf("pod %q", selector)
	}
	pod := podList.Items[0]

	// The the pod is for the same Juju version, we know the pod spec
	// will not have changed so can just update the image.
	if pod.Labels[jujuVersion] != jujuVersion {
		return errors.New("version mismatch")
	}
	pod.Spec.Containers[0].Image = image
	_, err = pods.Update(&pod)
	return errors.Trace(err)
}

func (k *kubernetesClient) ensurePod(pod *v1.Pod) error {
	// Kubernetes doesn't support updating a pod except under specific
	// circumstances so we need to delete and create.
	pods := k.CoreV1().Pods(k.namespace)
	if err := k.deletePod(pod.Name); err != nil {
		return errors.Trace(err)
	}
	_, err := pods.Create(pod)
	return errors.Trace(err)
}

func (k *kubernetesClient) deletePod(podName string) error {
	orphanDependents := false
	pods := k.CoreV1().Pods(k.namespace)
	err := pods.Delete(podName, &v1.DeleteOptions{
		OrphanDependents: &orphanDependents,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}

	// Wait for pod to be deleted.
	//
	// TODO(caas) if we even need to wait,
	// consider using pods.Watch.
	errExists := errors.New("exists")
	retryArgs := retry.CallArgs{
		Clock: clock.WallClock,
		IsFatalError: func(err error) bool {
			return errors.Cause(err) != errExists
		},
		Func: func() error {
			_, err := pods.Get(podName)
			if err == nil {
				return errExists
			}
			if k8serrors.IsNotFound(err) {
				return nil
			}
			return errors.Trace(err)
		},
		Delay:       5 * time.Second,
		MaxDuration: 2 * time.Minute,
	}
	return retry.Call(retryArgs)
}

// operatorPod returns a *v1.Pod for the operator pod
// of the specified application.
func operatorPod(appName, agentPath string) *v1.Pod {
	podName := operatorPodName(appName)
	configMapName := operatorConfigMapName(appName)
	configVolName := configMapName + "-volume"

	appTag := names.NewApplicationTag(appName)
	return &v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:   podName,
			Labels: map[string]string{labelOperator: appName, labelVersion: version.Current.String()},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{
				Name:            "juju-operator",
				ImagePullPolicy: v1.PullIfNotPresent,
				Image:           "jujusolutions/caas-jujud-operator:latest",
				Env: []v1.EnvVar{
					{Name: "JUJU_APPLICATION", Value: appName},
				},
				VolumeMounts: []v1.VolumeMount{{
					Name:      configVolName,
					MountPath: agent.Dir(agentPath, appTag) + "/agent.conf",
					SubPath:   "agent.conf",
				}},
			}},
			Volumes: []v1.Volume{{
				Name: configVolName,
				VolumeSource: v1.VolumeSource{
					ConfigMap: &v1.ConfigMapVolumeSource{
						LocalObjectReference: v1.LocalObjectReference{
							Name: configMapName,
						},
						Items: []v1.KeyToPath{{
							Key:  "agent.conf",
							Path: "agent.conf",
						}},
					},
				},
			}},
		},
	}
}

// operatorConfigMap returns a *v1.ConfigMap for the operator pod
// of the specified application, with the specified configuration.
func operatorConfigMap(appName string, config *caas.OperatorConfig) *v1.ConfigMap {
	configMapName := operatorConfigMapName(appName)
	return &v1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name: configMapName,
		},
		Data: map[string]string{
			"agent.conf": string(config.AgentConf),
		},
	}
}

type unitSpec struct {
	Pod v1.PodSpec `json:"pod"`
}

var defaultPodTemplate = `
pod:
  containers:
  - name: {{.Name}}
    image: {{.ImageName}}
    {{if .Ports}}
    ports:
    {{- range .Ports }}
        - containerPort: {{.ContainerPort}}
          {{if .Protocol}}protocol: {{.Protocol}}{{end}}
    {{- end}}
    {{end}}
    {{if or .Config .ConfigSecrets}}
    env:
    {{- range $k, $v := .Config }}
        - name: {{$k}}
          value: {{$v}}
    {{- end}}
    {{- range $k, $v := .ConfigSecrets }}
        - name: {{$k}}
          valueFrom:
            secretKeyRef:
              name: {{$v.SecretName}}
              key: {{$v.Key}}
    {{- end}}
    {{end}}
`[1:]

func makeUnitSpec(containerSpec *caas.ContainerSpec) (*unitSpec, error) {
	tmpl := template.Must(template.New("").Parse(defaultPodTemplate))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, containerSpec); err != nil {
		return nil, errors.Trace(err)
	}
	unitSpecString := buf.String()

	var unitSpec unitSpec
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(unitSpecString), len(unitSpecString))
	if err := decoder.Decode(&unitSpec); err != nil {
		return nil, errors.Trace(err)
	}
	return &unitSpec, nil
}

func operatorPodName(appName string) string {
	return "juju-operator-" + appName
}

func operatorConfigMapName(appName string) string {
	return operatorPodName(appName) + "-config"
}

func applicationConfigMapName(appName, fileSetName string) string {
	return fmt.Sprintf("%v-%v-config", deploymentName(appName), fileSetName)
}

func unitConfigMapName(unitName, fileSetName string) string {
	return fmt.Sprintf("%v-%v-config", unitPodName(unitName), fileSetName)
}

func unitPodName(unitName string) string {
	return "juju-" + names.NewUnitTag(unitName).String()
}

func deploymentName(appName string) string {
	return "juju-" + appName
}

func resourceNamePrefix(appName string) string {
	return "juju-" + names.NewApplicationTag(appName).String() + "-"
}
