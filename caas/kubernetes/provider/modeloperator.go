// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/paths"
)

const (
	labelModelOperator     = "juju-modeloperator"
	modelOperatorPortLabel = "api"
)

// EnsureModelOperator implements caas broker's interface. Function ensures that
// a model operator for this broker's namespace exists within Kubernetes.
func (k *kubernetesClient) EnsureModelOperator(
	modelUUID,
	agentPath string,
	config *caas.ModelOperatorConfig,
) error {
	operatorName := modelOperatorName(k.CurrentModel())
	modelTag := names.NewModelTag(modelUUID)

	configMap := modelOperatorConfigMap(
		k.namespace,
		operatorName,
		map[string]string{},
		config.AgentConf)

	_, err := k.ensureConfigMap(configMap)
	if err != nil {
		return errors.Annotate(err, "ensuring model operator config map")
	}

	volumes := []core.Volume{{
		Name: configMap.Name,
		VolumeSource: core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				LocalObjectReference: core.LocalObjectReference{
					Name: configMap.Name,
				},
				Items: []core.KeyToPath{
					{
						Key:  modelOperatorConfigMapAgentConfKey(operatorName),
						Path: TemplateFileNameAgentConf,
					},
				},
			},
		},
	}}

	volumeMounts := []core.VolumeMount{
		{
			Name:      configMap.Name,
			MountPath: filepath.Join(agent.Dir(agentPath, modelTag), TemplateFileNameAgentConf),
			SubPath:   TemplateFileNameAgentConf,
		},
	}

	service := modelOperatorService(
		operatorName, k.namespace, map[string]string{}, config.Port)
	if err := k.ensureK8sService(service); err != nil {
		return errors.Annotate(err, "ensuring model operater service")
	}

	deployment, err := modelOperatorDeployment(
		operatorName,
		k.namespace,
		map[string]string{},
		config.OperatorImagePath,
		config.Port,
		modelUUID,
		volumes,
		volumeMounts)
	if err != nil {
		return errors.Annotate(err, "building juju model operator deployment")
	}

	return k.ensureDeployment(deployment)
}

// ModelOperator return the model operator config used to create the current
// model operator for this broker
func (k *kubernetesClient) ModelOperator() (*caas.ModelOperatorConfig, error) {
	operatorName := modelOperatorName(k.CurrentModel())
	exists, err := k.ModelOperatorExists()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !exists {
		return nil, errors.NotFoundf("model operator %s", operatorName)
	}

	modelOperatorCfg := caas.ModelOperatorConfig{}
	cm, err := k.client().CoreV1().ConfigMaps(k.namespace).
		Get(operatorName, meta.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if cm != nil {
		if agentConf, ok := cm.Data[modelOperatorConfigMapAgentConfKey(operatorName)]; ok {
			modelOperatorCfg.AgentConf = []byte(agentConf)
		}
	}

	return &modelOperatorCfg, nil
}

func modelOperatorConfigMap(
	namespace,
	operatorName string,
	labels map[string]string,
	agentConf []byte,
) *core.ConfigMap {
	moLabels := modelOperatorLabels(operatorName)

	return &core.ConfigMap{
		ObjectMeta: meta.ObjectMeta{
			Name:      operatorName,
			Namespace: namespace,
			Labels:    AppendLabels(labels, moLabels),
		},
		Data: map[string]string{
			modelOperatorConfigMapAgentConfKey(operatorName): string(agentConf),
		},
	}
}

func modelOperatorDeployment(
	operatorName,
	namespace string,
	labels map[string]string,
	operatorImagePath string,
	port int32,
	modelUUID string,
	volumes []core.Volume,
	volumeMounts []core.VolumeMount,
) (*apps.Deployment, error) {

	moLabels := modelOperatorLabels(operatorName)

	jujudCmd := fmt.Sprintf("$JUJU_TOOLS_DIR/jujud model --model-uuid=%s", modelUUID)
	jujuDataDir, err := paths.DataDir("kubernetes")
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &apps.Deployment{
		ObjectMeta: meta.ObjectMeta{
			Name:      operatorName,
			Namespace: namespace,
			Labels:    AppendLabels(labels, moLabels),
		},
		Spec: apps.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &meta.LabelSelector{
				MatchLabels: moLabels,
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: meta.ObjectMeta{
					Labels: moLabels,
				},
				Spec: core.PodSpec{
					Containers: []core.Container{{
						Image:           operatorImagePath,
						ImagePullPolicy: core.PullIfNotPresent,
						Name:            operatorContainerName,
						WorkingDir:      jujuDataDir,
						Command: []string{
							"/bin/sh",
						},
						Args: []string{
							"-c",
							fmt.Sprintf(
								caas.JujudStartUpSh,
								jujuDataDir,
								"tools",
								jujudCmd,
							),
						},
						Ports: []core.ContainerPort{
							{
								ContainerPort: port,
								Name:          modelOperatorPortLabel,
								Protocol:      core.ProtocolTCP,
							},
						},
						VolumeMounts: volumeMounts,
					}},
					Volumes: volumes,
				},
			},
		},
	}, nil
}

// ModelOperatorExists indicates if the model operator for the given broker
// exists
func (k *kubernetesClient) ModelOperatorExists() (bool, error) {
	operatorName := modelOperatorName(k.CurrentModel())
	exists, err := k.modelOperatorDeploymentExists(operatorName)
	if err != nil {
		return false, errors.Trace(err)
	}
	return exists, nil
}

func (k *kubernetesClient) modelOperatorDeploymentExists(operatorName string) (bool, error) {
	_, err := k.client().AppsV1().Deployments(k.namespace).
		Get(operatorName, meta.GetOptions{})

	if k8serrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	return true, nil
}

func modelOperatorLabels(operatorName string) map[string]string {
	return map[string]string{
		labelModelOperator: operatorName,
	}
}

func modelOperatorName(modelName string) string {
	return modelName + "-modeloperator"
}

func modelOperatorService(
	operatorName,
	namespace string,
	labels map[string]string,
	port int32,
) *core.Service {
	moLabels := modelOperatorLabels(operatorName)

	return &core.Service{
		ObjectMeta: meta.ObjectMeta{
			Name:      operatorName,
			Namespace: namespace,
			Labels:    moLabels,
		},
		Spec: core.ServiceSpec{
			Selector: moLabels,
			Type:     core.ServiceTypeClusterIP,
			Ports: []core.ServicePort{
				{
					Protocol:   core.ProtocolTCP,
					Port:       port,
					TargetPort: intstr.FromString(modelOperatorPortLabel),
				},
			},
		},
	}
}

func modelOperatorConfigMapAgentConfKey(operatorName string) string {
	return operatorName + "-agent.conf"
}
