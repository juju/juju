// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	"gopkg.in/juju/names.v2"
	"k8s.io/client-go/kubernetes"
	k8serrors "k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
)

// TODO(caas) should be using a juju specific namespace
const namespace = "default"

type kubernetesClient struct {
	*kubernetes.Clientset
}

func newK8sClient(facade CAASProvisionerFacade) (CAASClient, error) {
	config, err := newK8sConfig(facade)
	if err != nil {
		return nil, errors.Trace(err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &kubernetesClient{client}, nil
}

func newK8sConfig(facade CAASProvisionerFacade) (*rest.Config, error) {
	config, err := facade.ConnectionConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var CAData []byte
	for _, cacert := range config.CACertificates {
		CAData = append(CAData, cacert...)
	}

	return &rest.Config{
		Host:     config.Endpoint,
		Username: config.Username,
		Password: config.Password,
		TLSClientConfig: rest.TLSClientConfig{
			CertData: config.CertData,
			KeyData:  config.KeyData,
			CAData:   CAData,
		},
	}, nil
}

// EnsureOperator creates a new operator for appName if it doesn't exist.
func (k *kubernetesClient) EnsureOperator(appName, agentPath string, newConfig NewOperatorConfigFunc) error {
	if exists, err := k.operatorExists(appName); err != nil {
		return errors.Trace(err)
	} else if exists {
		logger.Debugf("%s operator already deployed", appName)
		return nil
	}
	logger.Infof("deploying %s operator", appName)

	configMapName, err := k.ensureConfigMap(appName, newConfig)
	if err != nil {
		return errors.Trace(err)
	}

	return k.deployOperator(appName, agentPath, configMapName)
}

func (k *kubernetesClient) ensureConfigMap(appName string, newConfig NewOperatorConfigFunc) (string, error) {
	mapName := podName(appName) + "-config"

	exists, err := k.configMapExists(mapName)
	if err != nil {
		return "", errors.Trace(err)
	}
	if exists {
		logger.Infof("ConfigMap %s already exists", mapName)
	} else {
		config, err := newConfig(appName)
		if err != nil {
			return "", errors.Annotate(err, "creating config")
		}
		if err := k.createConfigMap(mapName, config); err != nil {
			return "", errors.Annotate(err, "creating ConfigMap")
		}
	}
	return mapName, nil
}

func (k *kubernetesClient) configMapExists(configMapName string) (bool, error) {
	_, err := k.CoreV1().ConfigMaps(namespace).Get(configMapName)
	if k8serrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, errors.Trace(err)
	}
	return true, nil
}

func (k *kubernetesClient) createConfigMap(configMapName string, config []byte) error {
	_, err := k.CoreV1().ConfigMaps(namespace).Create(&v1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name: configMapName,
		},
		Data: map[string]string{
			"agent.conf": string(config),
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
				Image:   "ubuntu:16.04",
				Command: []string{"sleep"},
				Args:    []string{"infinity"},
				//Args:    []string{"caasoperator", "--application-name", appName, "--debug"},

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
