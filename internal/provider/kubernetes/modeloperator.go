// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/paths"
	coreresources "github.com/juju/juju/core/resource"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/proxy"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
)

// ModelOperatorBroker defines a broker for Executing Kubernetes ensure
// commands. This interfaces is scoped down to the exact components needed by
// the ensure model operator routines.
type ModelOperatorBroker interface {
	// Client returns the core Kubernetes client to use for model operator actions.
	Client() kubernetes.Interface

	// ExtendedClient returns the extended Kubernetes client to use for model operator actions.
	ExtendedClient() clientset.Interface

	// EnsureConfigMap ensures the supplied kubernetes config map exists in the
	// targeted cluster. Error returned if this action is not able to be
	// performed.
	EnsureConfigMap(context.Context, *core.ConfigMap) ([]func(), error)

	// EnsureDeployment ensures the supplied kubernetes deployment object exists
	// in the targeted cluster. Error returned if this action is not able to be
	// performed.
	EnsureDeployment(context.Context, *apps.Deployment) ([]func(), error)

	// EnsureRole ensures the supplied kubernetes role object exists in the
	// targeted clusters namespace
	EnsureRole(context.Context, *rbac.Role) ([]func(), error)

	// EnsureRoleBinding ensures the supplied kubernetes role binding object
	// exists in the targetes clusters namespace
	EnsureRoleBinding(context.Context, *rbac.RoleBinding) ([]func(), error)

	// EnsureService ensures the spplied kubernetes service object exists in the
	// targeted cluster. Error returned if the action is not able to be
	// performed.
	EnsureService(context.Context, *core.Service) ([]func(), error)

	// EnsureServiceAccount ensures the supplied the kubernetes service account
	// exists in the targets cluster.
	EnsureServiceAccount(context.Context, *core.ServiceAccount) ([]func(), error)

	// ModelName returns the name of the current model being deployed to for the
	// broker
	ModelName() string

	// ModelUUID returns the uuid of the current model being deployed to for the
	// broker.
	ModelUUID() string

	// Namespace returns the current default namespace targeted by this broker.
	Namespace() string

	// LabelVersion returns the detected label version for k8s resources created
	// for this model.
	LabelVersion() constants.LabelVersion
}

// modelOperatorBrokerBridge provides a pluggable struct of funcs to implement
// the ModelOperatorBroker interface
type modelOperatorBrokerBridge struct {
	client         kubernetes.Interface
	extendedClient clientset.Interface

	ensureConfigMap      func(context.Context, *core.ConfigMap) ([]func(), error)
	ensureDeployment     func(context.Context, *apps.Deployment) ([]func(), error)
	ensureRole           func(context.Context, *rbac.Role) ([]func(), error)
	ensureRoleBinding    func(context.Context, *rbac.RoleBinding) ([]func(), error)
	ensureService        func(context.Context, *core.Service) ([]func(), error)
	ensureServiceAccount func(context.Context, *core.ServiceAccount) ([]func(), error)

	modelName      string
	modelUUID      string
	controllerUUID string
	namespace      string
	labelVersion   constants.LabelVersion
}

const (
	modelOperatorPortLabel = "api"

	EnvModelAgentCAASServiceName      = "SERVICE_NAME"
	EnvModelAgentCAASServiceNamespace = "SERVICE_NAMESPACE"
	EnvModelAgentHTTPPort             = "HTTP_PORT"

	OperatorModelTarget = "model"
)

var (
	// modelOperatorName is the model operator stack name used for deployment, service, RBAC resources.
	modelOperatorName = "modeloperator"

	// ExecRBACResourceName is the model's exec RBAC resource name.
	ExecRBACResourceName = "model-exec"
)

// Client implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) Client() kubernetes.Interface {
	return m.client
}

// ExtendedClient implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) ExtendedClient() clientset.Interface {
	return m.extendedClient
}

// EnsureConfigMap implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) EnsureConfigMap(ctx context.Context, c *core.ConfigMap) ([]func(), error) {
	if m.ensureConfigMap == nil {
		return []func(){}, errors.NotImplementedf("ensure config map bridge")
	}
	return m.ensureConfigMap(ctx, c)
}

// EnsureDeployment implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) EnsureDeployment(ctx context.Context, d *apps.Deployment) ([]func(), error) {
	if m.ensureDeployment == nil {
		return []func(){}, errors.NotImplementedf("ensure deployment bridge")
	}
	return m.ensureDeployment(ctx, d)
}

// EnsureRole implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) EnsureRole(ctx context.Context, r *rbac.Role) ([]func(), error) {
	if m.ensureRole == nil {
		return []func(){}, errors.NotImplementedf("ensure role bridge")
	}
	return m.ensureRole(ctx, r)
}

// EnsureRoleBinding implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) EnsureRoleBinding(ctx context.Context, r *rbac.RoleBinding) ([]func(), error) {
	if m.ensureRoleBinding == nil {
		return []func(){}, errors.NotImplementedf("ensure role binding bridge")
	}
	return m.ensureRoleBinding(ctx, r)
}

// EnsureService implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) EnsureService(ctx context.Context, s *core.Service) ([]func(), error) {
	if m.ensureService == nil {
		return []func(){}, errors.NotImplementedf("ensure service bridge")
	}
	return m.ensureService(ctx, s)
}

// EnsureServiceAccount implements ModelOperatorBroker
func (m *modelOperatorBrokerBridge) EnsureServiceAccount(ctx context.Context, s *core.ServiceAccount) ([]func(), error) {
	if m.ensureServiceAccount == nil {
		return []func(){}, errors.NotImplementedf("ensure service account bridge")
	}
	return m.ensureServiceAccount(ctx, s)
}

// ModelName implements ModelOperatorBroker.
func (m *modelOperatorBrokerBridge) ModelName() string {
	return m.modelName
}

// ModelUUID implements ModelOperatorBroker.
func (m *modelOperatorBrokerBridge) ModelUUID() string {
	return m.modelUUID
}

// ControllerUUID implements ModelOperatorBroker.
func (m *modelOperatorBrokerBridge) ControllerUUID() string {
	return m.controllerUUID
}

// Namespace implements ModelOperatorBroker.
func (m *modelOperatorBrokerBridge) Namespace() string {
	return m.namespace
}

// LabelVersion implements ModelOperatorBroker.
func (m *modelOperatorBrokerBridge) LabelVersion() constants.LabelVersion {
	return m.labelVersion
}

func ensureModelOperator(
	ctx context.Context,
	modelUUID,
	agentPath string,
	clock jujuclock.Clock,
	config *caas.ModelOperatorConfig,
	broker ModelOperatorBroker,
) (err error) {

	operatorName := modelOperatorName
	modelTag := names.NewModelTag(modelUUID)

	selectorLabels := modelOperatorLabels(operatorName, broker.LabelVersion())
	labels := selectorLabels
	if broker.LabelVersion() != constants.LegacyLabelVersion {
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

	c, err := broker.EnsureConfigMap(ctx, configMap)
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
		ctx,
		broker,
		clock,
		operatorName,
		labels,
	)
	cleanUpFuncs = append(cleanUpFuncs, c...)
	if err != nil {
		return errors.Trace(err)
	}

	service := modelOperatorService(
		operatorName, broker.Namespace(), labels, selectorLabels, config.Port)
	c, err = broker.EnsureService(ctx, service)
	cleanUpFuncs = append(cleanUpFuncs, c...)
	if err != nil {
		return errors.Annotate(err, "ensuring model operater service")
	}

	deployment, err := modelOperatorDeployment(
		operatorName,
		broker.Namespace(),
		labels,
		selectorLabels,
		config.ImageDetails,
		config.Port,
		modelUUID,
		service.Name,
		saName,
		volumes,
		volumeMounts)
	if err != nil {
		return errors.Annotate(err, "building juju model operator deployment")
	}

	c, err = broker.EnsureDeployment(ctx, deployment)
	cleanUpFuncs = append(cleanUpFuncs, c...)
	if err != nil {
		return errors.Annotate(err, "ensuring juju model operator deployment")
	}

	return nil
}

// EnsureModelOperator implements caas broker's interface. Function ensures that
// a model operator for this broker's namespace exists within Kubernetes.
func (k *kubernetesClient) EnsureModelOperator(
	ctx context.Context,
	modelUUID,
	agentPath string,
	config *caas.ModelOperatorConfig,
) error {
	if k.client() == nil {
		return errors.New("kubernetes client cannot be nil")
	}

	bridge := &modelOperatorBrokerBridge{
		client:         k.client(),
		extendedClient: k.extendedClient(),
		ensureConfigMap: func(ctx context.Context, c *core.ConfigMap) ([]func(), error) {
			cleanUp, err := k.ensureConfigMap(ctx, c)
			return []func(){cleanUp}, err
		},
		ensureDeployment: func(ctx context.Context, d *apps.Deployment) ([]func(), error) {
			return []func(){}, k.ensureDeployment(ctx, d)
		},
		ensureRole: func(ctx context.Context, r *rbac.Role) ([]func(), error) {
			_, c, err := k.ensureRole(ctx, r)
			return c, err
		},
		ensureRoleBinding: func(ctx context.Context, rb *rbac.RoleBinding) ([]func(), error) {
			_, c, err := k.ensureRoleBinding(ctx, rb)
			return c, err
		},
		ensureService: func(ctx context.Context, svc *core.Service) ([]func(), error) {
			c, err := k.ensureK8sService(ctx, svc)
			return []func(){c}, err
		},
		ensureServiceAccount: func(ctx context.Context, sa *core.ServiceAccount) ([]func(), error) {
			_, c, err := k.ensureServiceAccount(ctx, sa)
			return c, err
		},
		modelUUID:      k.ModelUUID(),
		modelName:      k.ModelName(),
		controllerUUID: k.ControllerUUID(),
		namespace:      k.Namespace(),
		labelVersion:   k.LabelVersion(),
	}

	return ensureModelOperator(ctx, modelUUID, agentPath, k.clock, config, bridge)
}

// ModelOperator return the model operator config used to create the current
// model operator for this broker
func (k *kubernetesClient) ModelOperator(ctx context.Context) (*caas.ModelOperatorConfig, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	operatorName := modelOperatorName
	exists, err := k.ModelOperatorExists(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !exists {
		return nil, errors.NotFoundf("model operator %s", operatorName)
	}

	modelOperatorCfg := caas.ModelOperatorConfig{}
	cm, err := k.client().CoreV1().ConfigMaps(k.namespace).
		Get(ctx, operatorName, meta.GetOptions{})
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
	operatorImageDetails coreresources.DockerImageDetails,
	port int32,
	modelUUID,
	serviceName,
	serviceAccountName string,
	volumes []core.Volume,
	volumeMounts []core.VolumeMount,
) (o *apps.Deployment, err error) {
	jujudCmd := fmt.Sprintf("exec $JUJU_TOOLS_DIR/jujud model --model-uuid=%s", modelUUID)
	jujuDataDir := paths.DataDir(paths.OSUnixLike)

	o = &apps.Deployment{
		ObjectMeta: meta.ObjectMeta{
			Name:      operatorName,
			Namespace: namespace,
			Labels: utils.LabelsMerge(
				labels,
				utils.LabelsJujuModelOperatorDisableWebhook,
			),
		},
		Spec: apps.DeploymentSpec{
			Replicas: pointer.Int32Ptr(1),
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
						Image:           operatorImageDetails.RegistryPath,
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
	}
	if operatorImageDetails.IsPrivate() {
		o.Spec.Template.Spec.ImagePullSecrets = []core.LocalObjectReference{
			{Name: constants.CAASImageRepoSecretName},
		}
	}
	return o, nil
}

// ModelOperatorExists indicates if the model operator for the given broker
// exists
func (k *kubernetesClient) ModelOperatorExists(ctx context.Context) (bool, error) {
	operatorName := modelOperatorName
	exists, err := k.modelOperatorDeploymentExists(ctx, operatorName)
	if err != nil {
		return false, errors.Trace(err)
	}
	return exists, nil
}

func (k *kubernetesClient) modelOperatorDeploymentExists(ctx context.Context, operatorName string) (bool, error) {
	if k.namespace == "" {
		return false, errNoNamespace
	}
	_, err := k.client().AppsV1().Deployments(k.namespace).
		Get(ctx, operatorName, meta.GetOptions{})

	if k8serrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	return true, nil
}

func modelOperatorLabels(operatorName string, labelVersion constants.LabelVersion) labels.Set {
	if labelVersion == constants.LegacyLabelVersion {
		return utils.LabelForKeyValue(constants.LegacyLabelModelOperator, operatorName)
	}
	return utils.LabelsForOperator(operatorName, OperatorModelTarget, labelVersion)
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

func modelOperatorGlobalScopedName(modelName, operatorName string) string {
	if modelName == "" {
		return operatorName
	}
	return fmt.Sprintf("%s-%s", modelName, operatorName)
}

func ensureModelOperatorRBAC(
	ctx context.Context,
	broker ModelOperatorBroker,
	clock jujuclock.Clock,
	operatorName string,
	labels map[string]string,
) (string, []func(), error) {
	cleanUpFuncs := []func(){}

	objMetaGlobal := meta.ObjectMeta{
		Name:   modelOperatorGlobalScopedName(broker.ModelName(), operatorName),
		Labels: labels,
	}
	objMetaNamespaced := meta.ObjectMeta{
		Name:      operatorName,
		Labels:    labels,
		Namespace: broker.Namespace(),
	}

	sa := &core.ServiceAccount{
		ObjectMeta:                   objMetaNamespaced,
		AutomountServiceAccountToken: boolPtr(true),
	}

	c, err := broker.EnsureServiceAccount(ctx, sa)
	cleanUpFuncs = append(cleanUpFuncs, c...)
	if err != nil {
		return sa.Name, cleanUpFuncs, errors.Annotate(err, "ensuring service account")
	}

	clusterRole := resources.NewClusterRole(broker.Client().RbacV1().ClusterRoles(), objMetaGlobal.GetName(), &rbac.ClusterRole{
		ObjectMeta: objMetaGlobal,
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
	})

	c, err = clusterRole.Ensure(
		ctx,
		resources.ClaimJujuOwnership,
	)
	cleanUpFuncs = append(cleanUpFuncs, c...)
	if err != nil {
		return sa.Name, cleanUpFuncs, errors.Annotate(err, "ensuring cluster role")
	}

	clusterRoleBinding := resources.NewClusterRoleBinding(broker.Client().RbacV1().ClusterRoleBindings(), objMetaGlobal.GetName(), &rbac.ClusterRoleBinding{
		ObjectMeta: objMetaGlobal,
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
	})

	c, err = clusterRoleBinding.Ensure(ctx, resources.ClaimJujuOwnership)
	cleanUpFuncs = append(cleanUpFuncs, c...)
	if err != nil {
		return sa.Name, cleanUpFuncs, errors.Annotate(err, "ensuring cluster role binding")
	}

	role := &rbac.Role{
		ObjectMeta: objMetaNamespaced,
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

	c, err = broker.EnsureRole(ctx, role)
	cleanUpFuncs = append(cleanUpFuncs, c...)
	if err != nil {
		return sa.Name, cleanUpFuncs, errors.Annotate(err, "ensuring role")
	}

	roleBinding := &rbac.RoleBinding{
		ObjectMeta: objMetaNamespaced,
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

	c, err = broker.EnsureRoleBinding(ctx, roleBinding)
	cleanUpFuncs = append(cleanUpFuncs, c...)
	if err != nil {
		return sa.Name, cleanUpFuncs, errors.Annotate(err, "ensuring role binding")
	}

	err = ensureExecRBACResources(ctx, objMetaNamespaced, clock, broker)
	return sa.Name, cleanUpFuncs, errors.Trace(err)
}

func ensureExecRBACResources(ctx context.Context, objMeta meta.ObjectMeta, clock jujuclock.Clock, broker ModelOperatorBroker) error {
	objMeta.SetName(ExecRBACResourceName)

	sa := &core.ServiceAccount{
		ObjectMeta:                   objMeta,
		AutomountServiceAccountToken: boolPtr(true),
	}
	_, err := broker.EnsureServiceAccount(ctx, sa)
	if err != nil {
		return errors.Annotatef(err, "ensuring service account %q", sa.GetName())
	}

	role := &rbac.Role{
		ObjectMeta: objMeta,
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"namespaces"},
				Verbs: []string{
					"get",
					"list",
				},
				ResourceNames: []string{
					objMeta.Namespace,
				},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs: []string{
					"get",
					"list",
				},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods/exec"},
				Verbs: []string{
					"create",
				},
			},
		},
	}
	_, err = broker.EnsureRole(ctx, role)
	if err != nil {
		return errors.Annotatef(err, "ensuring role %q", role.GetName())
	}

	roleBinding := &rbac.RoleBinding{
		ObjectMeta: objMeta,
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

	_, err = broker.EnsureRoleBinding(ctx, roleBinding)
	if err != nil {
		return errors.Annotatef(err, "ensuring role binding %q", roleBinding.Name)
	}

	_, err = proxy.EnsureSecretForServiceAccount(
		ctx, sa.GetName(), objMeta, clock,
		broker.Client().CoreV1().Secrets(objMeta.GetNamespace()),
		broker.Client().CoreV1().ServiceAccounts(objMeta.GetNamespace()),
	)
	return errors.Trace(err)
}

func modelOperatorConfigMapAgentConfKey(operatorName string) string {
	return operatorName + "-agent.conf"
}

// GetModelOperatorDeploymentImage retrieves the container image used by the model operator deployment
// in the configured namespace (e.g. "ghcr.io/juju/jujud-operator:3.6.9").
// The following error types can be expected to be returned:
// - [errors.NotFound]: When the deployment is missing or no containers can be found.
func (k *kubernetesClient) GetModelOperatorDeploymentImage(ctx context.Context) (string, error) {
	api := k.client().AppsV1().Deployments(k.namespace)
	res, err := api.Get(ctx, modelOperatorName, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return "", errors.NewNotFound(err, fmt.Sprintf("k8s %q deployment not found", modelOperatorName))
	} else if err != nil {
		return "", errors.Trace(err)
	}

	containers := res.Spec.Template.Spec.Containers
	if len(containers) == 0 {
		return "", errors.NotFoundf("no containers found in model operator deployment %q", modelOperatorName)
	}

	image := containers[0].Image
	logger.Tracef(ctx, "model operator %q deployment image: %s", modelOperatorName, image)
	return image, nil
}
