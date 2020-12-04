// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/core/paths"
)

// ModelOperatorBroker defines a broker for Executing Kubernetes ensure
// commands. This interfaces is scoped down to the exact components needed by
// the ensure model operator routines.
type ModelOperatorBroker interface {
	// EnsureConfigMap ensures the supplied kubernetes config map exists in the
	// targeted cluster. Error returned if this action is not able to be
	// performed.
	EnsureConfigMap(*core.ConfigMap) error

	// EnsureDeployment ensures the supplied kubernetes deployment object exists
	// in the targeted cluster. Error returned if this action is not able to be
	// performed.
	EnsureDeployment(*apps.Deployment) error

	// EnsureService ensures the spplied kubernetes service object exists in the
	// targeted cluster. Error returned if the action is not able to be
	// performed.
	EnsureService(*core.Service) error

	// Namespace returns the current default namespace targeted by this broker.
	Namespace() string

	// IsLegacyLabels indicates if this provider is operating on a legacy label schema
	IsLegacyLabels() bool
}

// modelOperatorBrokerBridge provides a pluggable struct of funcs to implement
// the ModelOperatorBroker interface
type modelOperatorBrokerBridge struct {
	ensureConfigMap  func(*core.ConfigMap) error
	ensureDeployment func(*apps.Deployment) error
	ensureService    func(*core.Service) error
	namespace        func() string
	isLegacyLabels   func() bool
}

const (
	modelOperatorPortLabel = "api"

	EnvModelAgentCAASServiceName      = "SERVICE_NAME"
	EnvModelAgentCAASServiceNamespace = "SERVICE_NAMESPACE"
	EnvModelAgentHTTPPort             = "HTTP_PORT"

	OperatorModelTarget = "model"
)

var (
	modelOperatorName = "modeloperator"
)

// EnsureConfigMap implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) EnsureConfigMap(c *core.ConfigMap) error {
	if m.ensureConfigMap == nil {
		return errors.New("ensure config map bridge not configured")
	}
	return m.ensureConfigMap(c)
}

// EnsureDeployment implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) EnsureDeployment(d *apps.Deployment) error {
	if m.ensureDeployment == nil {
		return errors.New("ensure deployment bridge not configured")
	}
	return m.ensureDeployment(d)
}

// EnsureService implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) EnsureService(s *core.Service) error {
	if m.ensureService == nil {
		return errors.New("ensure service bridge not configured")
	}
	return m.ensureService(s)
}

// Namespace implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) Namespace() string {
	if m.namespace == nil {
		return ""
	}
	return m.namespace()
}

func (m *modelOperatorBrokerBridge) IsLegacyLabels() bool {
	if m.isLegacyLabels == nil {
		return true
	}
	return m.isLegacyLabels()
}

func ensureModelOperator(
	modelUUID,
	agentPath string,
	config *caas.ModelOperatorConfig,
	broker ModelOperatorBroker) error {

	operatorName := modelOperatorName
	modelTag := names.NewModelTag(modelUUID)

	selectorLabels := modelOperatorLabels(operatorName, broker.IsLegacyLabels())
	labels := selectorLabels
	if !broker.IsLegacyLabels() {
		labels = utils.LabelsMerge(labels, utils.LabelsJuju)
	}

	configMap := modelOperatorConfigMap(
		broker.Namespace(),
		operatorName,
		labels,
		config.AgentConf)

	if err := broker.EnsureConfigMap(configMap); err != nil {
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
						Key:  modelOperatorConfigMapAgentConfKey(modelOperatorName),
						Path: constants.TemplateFileNameAgentConf,
					},
				},
			},
		},
	}}

	volumeMounts := []core.VolumeMount{
		{
			Name:      configMap.Name,
			MountPath: filepath.Join(agent.Dir(agentPath, modelTag), constants.TemplateFileNameAgentConf),
			SubPath:   constants.TemplateFileNameAgentConf,
		},
	}

	service := modelOperatorService(
		operatorName, broker.Namespace(), labels, selectorLabels, config.Port)
	if err := broker.EnsureService(service); err != nil {
		return errors.Annotate(err, "ensuring model operater service")
	}

	deployment, err := modelOperatorDeployment(
		operatorName,
		broker.Namespace(),
		labels,
		selectorLabels,
		config.OperatorImagePath,
		config.Port,
		modelUUID,
		service.Name,
		volumes,
		volumeMounts)
	if err != nil {
		return errors.Annotate(err, "building juju model operator deployment")
	}

	return broker.EnsureDeployment(deployment)
}

// EnsureModelOperator implements caas broker's interface. Function ensures that
// a model operator for this broker's namespace exists within Kubernetes.
func (k *kubernetesClient) EnsureModelOperator(
	modelUUID,
	agentPath string,
	config *caas.ModelOperatorConfig,
) error {
	bridge := &modelOperatorBrokerBridge{
		ensureConfigMap: func(c *core.ConfigMap) error {
			_, err := k.ensureConfigMap(c)
			return err
		},
		ensureDeployment: k.ensureDeployment,
		ensureService: func(svc *core.Service) error {
			_, err := k.ensureK8sService(svc)
			return err
		},
		namespace:      func() string { return k.namespace },
		isLegacyLabels: k.IsLegacyLabels,
	}

	return ensureModelOperator(modelUUID, agentPath, config, bridge)
}

// ModelOperator return the model operator config used to create the current
// model operator for this broker
func (k *kubernetesClient) ModelOperator() (*caas.ModelOperatorConfig, error) {
	operatorName := modelOperatorName
	exists, err := k.ModelOperatorExists()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !exists {
		return nil, errors.NotFoundf("model operator %s", operatorName)
	}

	modelOperatorCfg := caas.ModelOperatorConfig{}
	cm, err := k.client().CoreV1().ConfigMaps(k.namespace).
		Get(context.TODO(), operatorName, meta.GetOptions{})
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

	return &core.ConfigMap{
		ObjectMeta: meta.ObjectMeta{
			Name:      operatorName,
			Namespace: namespace,
			Labels:    labels,
		},
		Data: map[string]string{
			modelOperatorConfigMapAgentConfKey(operatorName): string(agentConf),
		},
	}
}

func modelOperatorDeployment(
	operatorName,
	namespace string,
	labels,
	selectorLabels map[string]string,
	operatorImagePath string,
	port int32,
	modelUUID string,
	serviceName string,
	volumes []core.Volume,
	volumeMounts []core.VolumeMount,
) (*apps.Deployment, error) {
	jujudCmd := fmt.Sprintf("$JUJU_TOOLS_DIR/jujud model --model-uuid=%s", modelUUID)
	jujuDataDir := paths.DataDir(paths.OSUnixLike)

	return &apps.Deployment{
		ObjectMeta: meta.ObjectMeta{
			Name:      operatorName,
			Namespace: namespace,
			Labels: utils.LabelsMerge(
				labels,
				utils.LabelsJujuModelOperatorDisableWebhook,
			),
		},
		Spec: apps.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &meta.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: meta.ObjectMeta{
					Labels: utils.LabelsMerge(
						selectorLabels,
						utils.LabelsJujuModelOperatorDisableWebhook,
					),
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
						Env: []core.EnvVar{
							{
								Name:  EnvModelAgentHTTPPort,
								Value: strconv.Itoa(int(port)),
							},
							{
								Name:  EnvModelAgentCAASServiceName,
								Value: serviceName,
							},
							{
								Name:  EnvModelAgentCAASServiceNamespace,
								Value: namespace,
							},
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
	operatorName := modelOperatorName
	exists, err := k.modelOperatorDeploymentExists(operatorName)
	if err != nil {
		return false, errors.Trace(err)
	}
	return exists, nil
}

func (k *kubernetesClient) modelOperatorDeploymentExists(operatorName string) (bool, error) {
	_, err := k.client().AppsV1().Deployments(k.namespace).
		Get(context.TODO(), operatorName, meta.GetOptions{})

	if k8serrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	return true, nil
}

func modelOperatorLabels(operatorName string, legacy bool) labels.Set {
	if legacy {
		return utils.LabelForKeyValue(constants.LegacyLabelModelOperator, operatorName)
	}
	return utils.LabelsForOperator(operatorName, OperatorModelTarget, legacy)
}

func modelOperatorService(
	operatorName,
	namespace string,
	labels,
	selectorLabels map[string]string,
	port int32,
) *core.Service {
	return &core.Service{
		ObjectMeta: meta.ObjectMeta{
			Name:      operatorName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: core.ServiceSpec{
			Selector: selectorLabels,
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
