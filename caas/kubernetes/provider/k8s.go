// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/retry"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"
	"k8s.io/client-go/kubernetes"
	k8serrors "k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/v1"
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

// EnsureOperator creates a new operator for appName, replacing any existing operator.
func (k *kubernetesClient) EnsureOperator(appName, agentPath string, newConfig caas.NewOperatorConfigFunc) error {
	if exists, err := k.operatorExists(appName); err != nil {
		return errors.Trace(err)
	} else if exists {
		logger.Debugf("%s operator already deployed - deleting", appName)
		if err := k.deleteOperator(appName); err != nil {
			return errors.Trace(err)
		}
	}
	logger.Debugf("deploying %s operator", appName)

	// TODO(caas) use secrets for storing agent password?
	configMapName, err := k.ensureConfigMap(appName, newConfig)
	if err != nil {
		return errors.Trace(err)
	}

	return k.deployOperator(appName, agentPath, configMapName)
}

func (k *kubernetesClient) ensureConfigMap(appName string, newConfig caas.NewOperatorConfigFunc) (string, error) {
	mapName := configMapName(appName)

	config, err := newConfig()
	if err != nil {
		return "", errors.Annotate(err, "creating config")
	}
	if err := k.updateConfigMap(mapName, config); err != nil {
		return "", errors.Annotate(err, "creating or updating ConfigMap")
	}
	return mapName, nil
}

func (k *kubernetesClient) updateConfigMap(configMapName string, config *caas.OperatorConfig) error {
	_, err := k.CoreV1().ConfigMaps(namespace).Update(&v1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name: configMapName,
		},
		Data: map[string]string{
			"agent.conf": string(config.AgentConf),
		},
	})
	if k8serrors.IsNotFound(err) {
		return k.createConfigMap(configMapName, config)
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) createConfigMap(configMapName string, config *caas.OperatorConfig) error {
	_, err := k.CoreV1().ConfigMaps(namespace).Create(&v1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name: configMapName,
		},
		Data: map[string]string{
			"agent.conf": string(config.AgentConf),
		},
	})
	return errors.Trace(err)
}

func (k *kubernetesClient) operatorExists(appName string) (bool, error) {
	_, err := k.CoreV1().Pods(namespace).Get(podName(appName))
	if k8serrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, errors.Trace(err)
	}
	return true, nil
}

func (k *kubernetesClient) deleteOperator(appName string) error {
	podName := podName(appName)
	orphanDependents := false
	err := k.CoreV1().Pods(namespace).Delete(
		podName,
		&v1.DeleteOptions{OrphanDependents: &orphanDependents})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}

	// Wait for operator to be deleted.
	existsErr := errors.New("exists")
	retryArgs := retry.CallArgs{
		Clock: clock.WallClock,
		IsFatalError: func(err error) bool {
			return errors.Cause(err) != existsErr
		},
		Func: func() error {
			exists, err := k.operatorExists(appName)
			if err != nil {
				return err
			}
			if !exists {
				return nil
			}
			return existsErr
		},
		Delay:       5 * time.Second,
		MaxDuration: time.Minute,
	}
	return retry.Call(retryArgs)
}

func (k *kubernetesClient) deployOperator(appName, agentPath string, configMapName string) error {
	configVolName := configMapName + "-volume"

	appTag := names.NewApplicationTag(appName)
	spec := &v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: podName(appName),
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{
				Name:            "juju-operator",
				ImagePullPolicy: v1.PullIfNotPresent,
				// TODO(caas) use proper image
				Image: "wallyworld/caas-jujud-operator:latest",
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
	_, err := k.CoreV1().Pods(namespace).Create(spec)
	return errors.Trace(err)
}

func podName(appName string) string {
	return "juju-operator-" + appName
}

func configMapName(appName string) string {
	return podName(appName) + "-config"
}
