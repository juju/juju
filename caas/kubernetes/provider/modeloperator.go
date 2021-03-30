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
	rbac "k8s.io/api/rbac/v1"
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
	EnsureConfigMap(*core.ConfigMap) ([]func(), error)

	// EnsureClusterRole ensures that the provided cluster role exists in the
	// Kubernetes cluster
	EnsureClusterRole(*rbac.ClusterRole) ([]func(), error)

	// EnsureClusterRoleBinding ensures that the provided cluster role binding
	// exists in the Kubernetes cluster
	EnsureClusterRoleBinding(*rbac.ClusterRoleBinding) ([]func(), error)

	// EnsureDeployment ensures the supplied kubernetes deployment object exists
	// in the targeted cluster. Error returned if this action is not able to be
	// performed.
	EnsureDeployment(*apps.Deployment) ([]func(), error)

	// EnsureRole ensures the supplied kubernetes role object exists in the
	// targeted clusters namespace
	EnsureRole(*rbac.Role) ([]func(), error)

	// EnsureRoleBinding ensures the supplied kubernetes role binding object
	// exists in the targetes clusters namespace
	EnsureRoleBinding(*rbac.RoleBinding) ([]func(), error)

	// EnsureService ensures the spplied kubernetes service object exists in the
	// targeted cluster. Error returned if the action is not able to be
	// performed.
	EnsureService(*core.Service) ([]func(), error)

	// EnsureServiceAccount ensures the supplied the kubernetes service account
	// exists in the targets cluster.
	EnsureServiceAccount(*core.ServiceAccount) ([]func(), error)

	// Model returns the name of the current model being deployed to for the
	// broker
	Model() string

	// Namespace returns the current default namespace targeted by this broker.
	Namespace() string

	// IsLegacyLabels indicates if this provider is operating on a legacy label schema
	IsLegacyLabels() bool
}

// modelOperatorBrokerBridge provides a pluggable struct of funcs to implement
// the ModelOperatorBroker interface
type modelOperatorBrokerBridge struct {
	ensureClusterRole        func(*rbac.ClusterRole) ([]func(), error)
	ensureClusterRoleBinding func(*rbac.ClusterRoleBinding) ([]func(), error)
	ensureConfigMap          func(*core.ConfigMap) ([]func(), error)
	ensureDeployment         func(*apps.Deployment) ([]func(), error)
	ensureRole               func(*rbac.Role) ([]func(), error)
	ensureRoleBinding        func(*rbac.RoleBinding) ([]func(), error)
	ensureService            func(*core.Service) ([]func(), error)
	ensureServiceAccount     func(*core.ServiceAccount) ([]func(), error)
	model                    func() string
	namespace                func() string
	isLegacyLabels           func() bool
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

//EnsureClusterRole implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) EnsureClusterRole(cr *rbac.ClusterRole) ([]func(), error) {
	if m.ensureClusterRole == nil {
		return []func(){}, errors.NotImplementedf("ensure cluster role bridge")
	}
	return m.ensureClusterRole(cr)
}

// EnsureClusterRoleBinding implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) EnsureClusterRoleBinding(
	crb *rbac.ClusterRoleBinding,
) ([]func(), error) {
	if m.ensureClusterRoleBinding == nil {
		return []func(){}, errors.NotImplementedf("ensure cluster role binding bridge")
	}
	return m.ensureClusterRoleBinding(crb)
}

// EnsureConfigMap implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) EnsureConfigMap(c *core.ConfigMap) ([]func(), error) {
	if m.ensureConfigMap == nil {
		return []func(){}, errors.NotImplementedf("ensure config map bridge")
	}
	return m.ensureConfigMap(c)
}

// EnsureDeployment implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) EnsureDeployment(d *apps.Deployment) ([]func(), error) {
	if m.ensureDeployment == nil {
		return []func(){}, errors.NotImplementedf("ensure deployment bridge")
	}
	return m.ensureDeployment(d)
}

// EnsureRole implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) EnsureRole(r *rbac.Role) ([]func(), error) {
	if m.ensureRole == nil {
		return []func(){}, errors.NotImplementedf("ensure role bridge")
	}
	return m.ensureRole(r)
}

// EnsureRoleBinding implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) EnsureRoleBinding(r *rbac.RoleBinding) ([]func(), error) {
	if m.ensureRoleBinding == nil {
		return []func(){}, errors.NotImplementedf("ensure role binding bridge")
	}
	return m.ensureRoleBinding(r)
}

// EnsureService implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) EnsureService(s *core.Service) ([]func(), error) {
	if m.ensureService == nil {
		return []func(){}, errors.NotImplementedf("ensure service bridge")
	}
	return m.ensureService(s)
}

// EnsureServiceAccount implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) EnsureServiceAccount(s *core.ServiceAccount) ([]func(), error) {
	if m.ensureServiceAccount == nil {
		return []func(){}, errors.NotImplementedf("ensure service account bridge")
	}
	return m.ensureServiceAccount(s)
}

// Model implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) Model() string {
	if m.model == nil {
		return ""
	}
	return m.model()
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
	broker ModelOperatorBroker) (err error) {

	operatorName := modelOperatorName
	modelTag := names.NewModelTag(modelUUID)

	selectorLabels := modelOperatorLabels(operatorName, broker.IsLegacyLabels())
	labels := selectorLabels
	if !broker.IsLegacyLabels() {
		labels = utils.LabelsMerge(labels, utils.LabelsJuju)
	}

	cleanUpFuncs := []func(){}
	defer func() {
		if err != nil {
			utils.RunCleanUps(cleanUpFuncs)
		}
	}()

	configMap := modelOperatorConfigMap(
		broker.Namespace(),
		operatorName,
		labels,
		config.AgentConf)

	c, err := broker.EnsureConfigMap(configMap)
	cleanUpFuncs = append(cleanUpFuncs, c...)
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

	saName, c, err := ensureModelOperatorRBAC(
		broker,
		operatorName,
		labels,
	)
	cleanUpFuncs = append(cleanUpFuncs, c...)
	if err != nil {
		err = errors.Trace(err)
		return
	}

	service := modelOperatorService(
		operatorName, broker.Namespace(), labels, selectorLabels, config.Port)
	c, err = broker.EnsureService(service)
	cleanUpFuncs = append(cleanUpFuncs, c...)
	if err != nil {
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
		saName,
		volumes,
		volumeMounts)
	if err != nil {
		return errors.Annotate(err, "building juju model operator deployment")
	}

	c, err = broker.EnsureDeployment(deployment)
	cleanUpFuncs = append(cleanUpFuncs, c...)
	if err != nil {
		return errors.Annotate(err, "ensuring juju model operator deployment")
	}

	return nil
}

// EnsureModelOperator implements caas broker's interface. Function ensures that
// a model operator for this broker's namespace exists within Kubernetes.
func (k *kubernetesClient) EnsureModelOperator(
	modelUUID,
	agentPath string,
	config *caas.ModelOperatorConfig,
) error {
	bridge := &modelOperatorBrokerBridge{
		ensureClusterRole: func(cr *rbac.ClusterRole) ([]func(), error) {
			_, c, err := k.ensureClusterRole(cr)
			return c, err
		},
		ensureClusterRoleBinding: func(crb *rbac.ClusterRoleBinding) ([]func(), error) {
			_, c, err := k.ensureClusterRoleBinding(crb)
			return c, err
		},
		ensureConfigMap: func(c *core.ConfigMap) ([]func(), error) {
			cleanUp, err := k.ensureConfigMap(c)
			return []func(){cleanUp}, err
		},
		ensureDeployment: func(d *apps.Deployment) ([]func(), error) {
			return []func(){}, k.ensureDeployment(d)
		},
		ensureRole: func(r *rbac.Role) ([]func(), error) {
			_, c, err := k.ensureRole(r)
			return c, err
		},
		ensureRoleBinding: func(rb *rbac.RoleBinding) ([]func(), error) {
			_, c, err := k.ensureRoleBinding(rb)
			return c, err
		},
		ensureService: func(svc *core.Service) ([]func(), error) {
			c, err := k.ensureK8sService(svc)
			return []func(){c}, err
		},
		ensureServiceAccount: func(sa *core.ServiceAccount) ([]func(), error) {
			_, c, err := k.ensureServiceAccount(sa)
			return c, err
		},
		namespace:      func() string { return k.namespace },
		model:          func() string { return k.CurrentModel() },
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
	modelUUID,
	serviceName,
	serviceAccountName string,
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
			Replicas: utils.Int32Ptr(1),
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
					ServiceAccountName:           serviceAccountName,
					AutomountServiceAccountToken: boolPtr(true),
					Volumes:                      volumes,
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

func modelOperatorGlobalScopedName(model, operatorName string) string {
	if model == "" {
		return operatorName
	}
	return fmt.Sprintf("%s-%s", model, operatorName)
}

func ensureModelOperatorRBAC(
	broker ModelOperatorBroker,
	operatorName string,
	labels map[string]string,
) (string, []func(), error) {
	cleanUpFuncs := []func(){}

	globalName := modelOperatorGlobalScopedName(broker.Model(), operatorName)

	sa := &core.ServiceAccount{
		ObjectMeta: meta.ObjectMeta{
			Name:      operatorName,
			Namespace: broker.Namespace(),
			Labels:    labels,
		},
		AutomountServiceAccountToken: boolPtr(true),
	}

	c, err := broker.EnsureServiceAccount(sa)
	cleanUpFuncs = append(cleanUpFuncs, c...)
	if err != nil {
		return sa.Name, cleanUpFuncs, errors.Annotate(err, "ensuring service account")
	}

	clusterRole := &rbac.ClusterRole{
		ObjectMeta: meta.ObjectMeta{
			Name:   globalName,
			Labels: labels,
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"namespaces"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{"admissionregistration.k8s.io"},
				Resources: []string{"mutatingwebhookconfigurations"},
				Verbs: []string{
					"create",
					"delete",
					"get",
					"list",
					"update",
				},
			},
		},
	}

	c, err = broker.EnsureClusterRole(clusterRole)
	cleanUpFuncs = append(cleanUpFuncs, c...)
	if err != nil {
		return sa.Name, cleanUpFuncs, errors.Annotate(err, "ensuring cluster role")
	}

	clusterRoleBinding := &rbac.ClusterRoleBinding{
		ObjectMeta: meta.ObjectMeta{
			Name:   globalName,
			Labels: labels,
		},
		RoleRef: rbac.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRole.Name,
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      sa.Name,
				Namespace: sa.Namespace,
			},
		},
	}

	c, err = broker.EnsureClusterRoleBinding(clusterRoleBinding)
	cleanUpFuncs = append(cleanUpFuncs, c...)
	if err != nil {
		return sa.Name, cleanUpFuncs, errors.Annotate(err, "ensuring cluster role binding")
	}

	role := &rbac.Role{
		ObjectMeta: meta.ObjectMeta{
			Name:      operatorName,
			Namespace: broker.Namespace(),
			Labels:    labels,
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"serviceaccounts"},
				Verbs: []string{
					"get",
					"list",
					"watch",
				},
			},
		},
	}

	c, err = broker.EnsureRole(role)
	cleanUpFuncs = append(cleanUpFuncs, c...)
	if err != nil {
		return sa.Name, cleanUpFuncs, errors.Annotate(err, "ensuring role")
	}

	roleBinding := &rbac.RoleBinding{
		ObjectMeta: meta.ObjectMeta{
			Name:      operatorName,
			Namespace: broker.Namespace(),
			Labels:    labels,
		},
		RoleRef: rbac.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role.Name,
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      sa.Name,
				Namespace: sa.Namespace,
			},
		},
	}

	c, err = broker.EnsureRoleBinding(roleBinding)
	cleanUpFuncs = append(cleanUpFuncs, c...)
	if err != nil {
		return sa.Name, cleanUpFuncs, errors.Annotate(err, "ensuring role binding")
	}

	return sa.Name, cleanUpFuncs, nil
}

func modelOperatorConfigMapAgentConfKey(operatorName string) string {
	return operatorName + "-agent.conf"
}
