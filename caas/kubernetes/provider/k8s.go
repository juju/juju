// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/retry"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"
	"k8s.io/client-go/kubernetes"
	k8serrors "k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/util/yaml"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
)

var logger = loggo.GetLogger("juju.kubernetes.provider")

// TODO(caas) should be using a juju specific namespace
const namespace = "default"

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

// EnsureUnit creates or updates a unit pod with the given unit name and spec.
func (k *kubernetesClient) EnsureUnit(unitName, spec string) error {
	logger.Debugf("creating/updating %s", unitName)
	unitSpec, err := parseUnitSpec(spec)
	if err != nil {
		return errors.Annotatef(err, "parsing spec for %s", unitName)
	}
	podName := unitPodName(unitName)
	if err := k.deletePod(podName); err != nil {
		return errors.Trace(err)
	}
	pod := &v1.Pod{
		ObjectMeta: v1.ObjectMeta{Name: podName},
		Spec:       unitSpec.Pod,
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

func parseUnitSpec(in string) (*unitSpec, error) {
	var spec unitSpec
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in))
	if err := decoder.Decode(&spec); err != nil {
		return nil, errors.Trace(err)
	}
	return &spec, nil
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
