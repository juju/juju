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
	"github.com/juju/juju/status"
	"github.com/juju/juju/watcher"
)

var logger = loggo.GetLogger("juju.kubernetes.provider")

const (
	// TODO(caas) should be using a juju specific namespace
	namespace = "default"

	labelApplication = "juju-application"
	labelUnit        = "juju-unit"
)

// TODO(caas) - add unit tests

type kubernetesClient struct {
	*kubernetes.Clientset
}

// NewK8sProvider returns a kubernetes client for the specified cloud.
func NewK8sProvider(cloudSpec environs.CloudSpec) (caas.Broker, error) {
	config, err := newK8sConfig(cloudSpec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &kubernetesClient{client}, nil
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

// EnsureOperator creates or updates an operator pod with the given application
// name, agent path, and operator config.
func (k *kubernetesClient) EnsureOperator(appName, agentPath string, config *caas.OperatorConfig) error {
	logger.Debugf("creating/updating %s operator", appName)

	// TODO(caas) use secrets for storing agent password?
	if err := k.ensureConfigMap(operatorConfigMap(appName, config)); err != nil {
		return errors.Annotate(err, "creating or updating ConfigMap")
	}
	pod := operatorPod(appName, agentPath)
	if err := k.deletePod(pod.Name); err != nil {
		return errors.Trace(err)
	}
	return k.createPod(pod)
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

	if numUnits <= 0 {
		return errors.Errorf("number of units must be > 0")
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
	numPods := int32(numUnits)
	if err := k.configureDeployment(appName, unitSpec, &numPods); err != nil {
		return errors.Annotate(err, "creating or updating deployment controller")
	}
	cleanups = append(cleanups, func() { k.deleteDeployment(appName) })

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

func (k *kubernetesClient) configureDeployment(appName string, unitSpec *unitSpec, replicas *int32) error {
	logger.Debugf("creating/updating deployment for %s", appName)

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
				Spec: unitSpec.Pod,
			},
		},
	}
	return k.ensureDeployment(deployment)
}

func (k *kubernetesClient) ensureDeployment(spec *v1beta1.Deployment) error {
	deployments := k.ExtensionsV1beta1().Deployments(namespace)
	_, err := deployments.Update(spec)
	if k8serrors.IsNotFound(err) {
		_, err = deployments.Create(spec)
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteDeployment(appName string) error {
	orphanDependents := false
	deployments := k.ExtensionsV1beta1().Deployments(namespace)
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
	services := k.CoreV1().Services(namespace)
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
	services := k.CoreV1().Services(namespace)
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

	svc, err := k.CoreV1().Services(namespace).Get(deploymentName(appName))
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
	ingress := k.ExtensionsV1beta1().Ingresses(namespace)
	_, err := ingress.Update(spec)
	if k8serrors.IsNotFound(err) {
		_, err = ingress.Create(spec)
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteIngress(appName string) error {
	orphanDependents := false
	ingress := k.ExtensionsV1beta1().Ingresses(namespace)
	err := ingress.Delete(deploymentName(appName), &v1.DeleteOptions{OrphanDependents: &orphanDependents})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func applicationSelector(appName string) string {
	return fmt.Sprintf("%v==%v", labelApplication, appName)
}

// WatchUnits returns a watcher which notifies when there
// are changes to units of the specified application.
func (k *kubernetesClient) WatchUnits(appName string) (watcher.NotifyWatcher, error) {
	pods := k.CoreV1().Pods(namespace)
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
	pods := k.CoreV1().Pods(namespace)
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
		dying := p.DeletionTimestamp != nil
		if dying {
			continue
		}
		result = append(result, caas.Unit{
			Id:      string(p.UID),
			Address: p.Status.PodIP,
			Ports:   ports,
			Status: status.StatusInfo{
				Status:  k.jujuStatus(p.Status.Phase),
				Message: p.Status.Message,
				Since:   &now,
			},
		})
	}
	return result, nil
}

func (k *kubernetesClient) jujuStatus(podPhase v1.PodPhase) status.Status {
	switch podPhase {
	case v1.PodRunning:
		return status.Running
	case v1.PodFailed:
		return status.Error
	default:
		return status.Allocating
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
	if err := k.deletePod(podName); err != nil {
		return errors.Trace(err)
	}
	pod := &v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: podName,
			Labels: map[string]string{
				labelApplication: appName,
				labelUnit:        unitName}},
		Spec: unitSpec.Pod,
	}
	return k.createPod(pod)
}

func (k *kubernetesClient) ensureConfigMap(configMap *v1.ConfigMap) error {
	configMaps := k.CoreV1().ConfigMaps(namespace)
	_, err := configMaps.Update(configMap)
	if k8serrors.IsNotFound(err) {
		_, err = configMaps.Create(configMap)
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) createPod(spec *v1.Pod) error {
	pods := k.CoreV1().Pods(namespace)
	_, err := pods.Create(spec)
	return errors.Trace(err)
}

func (k *kubernetesClient) deletePod(podName string) error {
	orphanDependents := false
	pods := k.CoreV1().Pods(namespace)
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
		MaxDuration: time.Minute,
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
		ObjectMeta: v1.ObjectMeta{Name: podName},
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
    {{if .Config}}
    env:
    {{- range $k, $v := .Config }}
        - name: {{$k}}
          value: {{$v}}
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

func unitPodName(unitName string) string {
	return "juju-" + names.NewUnitTag(unitName).String()
}

func deploymentName(appName string) string {
	return "juju-" + appName
}

func resourceNamePrefix(appName string) string {
	return "juju-" + names.NewApplicationTag(appName).String() + "-"
}
