// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/proxy"
	"github.com/juju/retry"
	"github.com/juju/utils/v2"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	k8sproxy "github.com/juju/juju/caas/kubernetes/provider/proxy"
	k8sutils "github.com/juju/juju/caas/kubernetes/provider/utils"
	providerutils "github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/podcfg"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/mongo"
)

const (
	// JujuControllerStackName is the juju CAAS controller stack name.
	JujuControllerStackName = "controller"

	// ControllerServiceFQDNTemplate is the FQDN of the controller service using the cluster DNS.
	ControllerServiceFQDNTemplate = "controller-service.controller-%s.svc.cluster.local"

	proxyResourceName = "proxy"
)

var (
	// TemplateFileNameServerPEM is the template server.pem file name.
	TemplateFileNameServerPEM = "template-" + mongo.FileNameDBSSLKey
)

const (
	mongoDBContainerName   = "mongodb"
	apiServerContainerName = "api-server"
)

type controllerServiceSpec struct {
	// ServiceType is required.
	ServiceType core.ServiceType

	// ExternalName is optional.
	ExternalName string

	// ExternalIP is optional.
	ExternalIP string

	// ExternalIPs is optional.
	ExternalIPs []string

	// Annotations is optional.
	Annotations k8sannotations.Annotation
}

func getDefaultControllerServiceSpecs(cloudType string) *controllerServiceSpec {
	specs := map[string]*controllerServiceSpec{
		k8s.K8sCloudAzure: {
			ServiceType: core.ServiceTypeLoadBalancer,
		},
		k8s.K8sCloudEC2: {
			ServiceType: core.ServiceTypeLoadBalancer,
			Annotations: k8sannotations.New(nil).
				Add("service.beta.kubernetes.io/aws-load-balancer-backend-protocol", "tcp"),
		},
		k8s.K8sCloudGCE: {
			ServiceType: core.ServiceTypeLoadBalancer,
		},
		k8s.K8sCloudMicrok8s: {
			ServiceType: core.ServiceTypeClusterIP,
		},
		k8s.K8sCloudOpenStack: {
			ServiceType: core.ServiceTypeLoadBalancer,
		},
		k8s.K8sCloudMAAS: {
			ServiceType: core.ServiceTypeLoadBalancer, // TODO(caas): test and verify this.
		},
		k8s.K8sCloudLXD: {
			ServiceType: core.ServiceTypeClusterIP, // TODO(caas): test and verify this.
		},
		k8s.K8sCloudOther: {
			ServiceType: core.ServiceTypeClusterIP, // Default svc spec for any other cloud is not listed above.
		},
	}
	if out, ok := specs[cloudType]; ok {
		return out
	}
	return specs[k8s.K8sCloudOther]
}

type controllerStack struct {
	ctx environs.BootstrapContext

	stackName        string
	selectorLabels   map[string]string
	stackLabels      map[string]string
	stackAnnotations map[string]string
	broker           *kubernetesClient

	pcfg        *podcfg.ControllerPodConfig
	agentConfig agent.ConfigSetterWriter

	storageClass               string
	storageSize                resource.Quantity
	portMongoDB, portAPIServer int

	fileNameSharedSecret, fileNameBootstrapParams,
	fileNameSSLKey, fileNameSSLKeyMount,
	fileNameAgentConf, fileNameAgentConfMount string

	resourceNameStatefulSet, resourceNameService,
	resourceNameConfigMap, resourceNameSecret,
	pvcNameControllerPodStorage,
	resourceNameVolSharedSecret, resourceNameVolSSLKey, resourceNameVolBootstrapParams, resourceNameVolAgentConf string

	containerCount int

	cleanUps []func()
}

type controllerStacker interface {
	// Deploy creates all resources for controller stack.
	Deploy() error
}

func controllerCorelation(broker *kubernetesClient) (string, error) {
	// ensure controller specific annotations.
	controllerUUIDKey := k8sutils.AnnotationControllerIsControllerKey(false)
	_ = broker.addAnnotations(controllerUUIDKey, "true")

	ns, err := broker.listNamespacesByAnnotations(broker.GetAnnotations())
	if errors.IsNotFound(err) || ns == nil {
		// No existing controller found on the cluster.
		// A controller must be bootstrapping now.
		// It will reply on setControllerNamespace in controller stack to set namespace name.
		return "", errors.NewNotFound(err, "controller")
	}
	if err != nil {
		return "", errors.Trace(err)
	}
	return ns[0].GetName(), nil
}

// DecideControllerNamespace decides the namespace name to use for a new controller.
func DecideControllerNamespace(controllerName string) string {
	return "controller-" + controllerName
}

func newcontrollerStack(
	ctx environs.BootstrapContext,
	stackName string,
	storageClass string,
	broker *kubernetesClient,
	pcfg *podcfg.ControllerPodConfig,
) (controllerStacker, error) {
	storageSizeControllerRaw := "20Gi"
	if rootDiskSize := pcfg.Bootstrap.BootstrapMachineConstraints.RootDisk; rootDiskSize != nil {
		storageSizeControllerRaw = fmt.Sprintf("%dMi", *rootDiskSize)
	}
	storageSize, err := resource.ParseQuantity(storageSizeControllerRaw)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var agentConfig agent.ConfigSetterWriter
	agentConfig, err = pcfg.AgentConfig(names.NewControllerAgentTag(pcfg.ControllerId))
	if err != nil {
		return nil, errors.Trace(err)
	}

	si, ok := agentConfig.StateServingInfo()
	if !ok {
		return nil, errors.NewNotValid(nil, "agent config has no state serving info")
	}

	// ensures shared-secret content.
	if si.SharedSecret == "" {
		// Generate a shared secret for the Mongo replica set.
		sharedSecret, err := mongo.GenerateSharedSecret()
		if err != nil {
			return nil, errors.Trace(err)
		}
		si.SharedSecret = sharedSecret
	}

	agentConfig.SetStateServingInfo(si)
	pcfg.Bootstrap.StateServingInfo = si

	selectorLabels := providerutils.SelectorLabelsForApp(stackName, false)
	labels := providerutils.LabelsForApp(stackName, false)

	controllerUUIDKey := k8sutils.AnnotationControllerUUIDKey(false)
	cs := &controllerStack{
		ctx:              ctx,
		stackName:        stackName,
		selectorLabels:   selectorLabels,
		stackLabels:      labels,
		stackAnnotations: map[string]string{controllerUUIDKey: pcfg.ControllerTag.Id()},
		broker:           broker,

		pcfg:        pcfg,
		agentConfig: agentConfig,

		storageSize:   storageSize,
		storageClass:  storageClass,
		portMongoDB:   pcfg.Bootstrap.ControllerConfig.StatePort(),
		portAPIServer: pcfg.Bootstrap.ControllerConfig.APIPort(),

		fileNameSharedSecret:    mongo.SharedSecretFile,
		fileNameSSLKey:          mongo.FileNameDBSSLKey,
		fileNameSSLKeyMount:     TemplateFileNameServerPEM,
		fileNameBootstrapParams: cloudconfig.FileNameBootstrapParams,
		fileNameAgentConf:       agent.AgentConfigFilename,
		fileNameAgentConfMount:  constants.TemplateFileNameAgentConf,

		resourceNameStatefulSet: stackName,
	}
	cs.resourceNameService = cs.getResourceName("service")
	cs.resourceNameConfigMap = cs.getResourceName("configmap")
	cs.resourceNameSecret = cs.getResourceName("secret")

	cs.resourceNameVolSharedSecret = cs.getResourceName(cs.fileNameSharedSecret)
	cs.resourceNameVolSSLKey = cs.getResourceName(cs.fileNameSSLKey)
	cs.resourceNameVolBootstrapParams = cs.getResourceName(cs.fileNameBootstrapParams)
	cs.resourceNameVolAgentConf = cs.getResourceName(cs.fileNameAgentConf)

	cs.pvcNameControllerPodStorage = "storage"
	return cs, nil
}

func getBootstrapResourceName(stackName string, name string) string {
	return stackName + "-" + strings.Replace(name, ".", "-", -1)
}

func (c *controllerStack) getResourceName(name string) string {
	return getBootstrapResourceName(c.stackName, name)
}

func (c *controllerStack) getControllerSecret() (secret *core.Secret, err error) {
	defer func() {
		if err == nil && secret != nil && secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
	}()

	secret, err = c.broker.getSecret(c.resourceNameSecret)
	if err == nil {
		return secret, nil
	}
	if errors.IsNotFound(err) {
		_, err = c.broker.createSecret(&core.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:        c.resourceNameSecret,
				Labels:      c.stackLabels,
				Namespace:   c.broker.GetCurrentNamespace(),
				Annotations: c.stackAnnotations,
			},
			Type: core.SecretTypeOpaque,
		})
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.broker.getSecret(c.resourceNameSecret)
}

func (c *controllerStack) getControllerConfigMap() (cm *core.ConfigMap, err error) {
	defer func() {
		if cm != nil && cm.Data == nil {
			cm.Data = map[string]string{}
		}
	}()

	cm, err = c.broker.getConfigMap(c.resourceNameConfigMap)
	if err == nil {
		return cm, nil
	}
	if errors.IsNotFound(err) {
		_, err = c.broker.createConfigMap(&core.ConfigMap{
			ObjectMeta: v1.ObjectMeta{
				Name:        c.resourceNameConfigMap,
				Labels:      c.stackLabels,
				Namespace:   c.broker.GetCurrentNamespace(),
				Annotations: c.stackAnnotations,
			},
		})
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.broker.getConfigMap(c.resourceNameConfigMap)
}

func (c *controllerStack) doCleanUp() {
	logger.Debugf("bootstrap failed, removing %d resources.", len(c.cleanUps))
	for _, f := range c.cleanUps {
		f()
	}
}

// Deploy creates all resources for the controller stack.
func (c *controllerStack) Deploy() (err error) {
	// creating namespace for controller stack, this namespace will be removed by broker.DestroyController if bootstrap failed.
	nsName := c.broker.GetCurrentNamespace()
	c.ctx.Infof("Creating k8s resources for controller %q", nsName)
	if err = c.broker.createNamespace(nsName); err != nil {
		return errors.Annotate(err, "creating namespace for controller stack")
	}

	// Check context manually for cancellation between each step (not ideal,
	// but it avoids wiring context absolutely everywhere).
	isDone := func() bool {
		select {
		case <-c.ctx.Context().Done():
			return true
		default:
			return false
		}
	}
	if isDone() {
		return bootstrap.Cancelled()
	}

	defer func() {
		if err != nil {
			c.doCleanUp()
		}
	}()

	// create service for controller pod.
	if err = c.createControllerService(); err != nil {
		return errors.Annotate(err, "creating service for controller")
	}
	if isDone() {
		return bootstrap.Cancelled()
	}

	// create the proxy resources for services of type cluster ip
	if err = c.createControllerProxy(); err != nil {
		return errors.Annotate(err, "creating controller service proxy for controller")
	}

	// create shared-secret secret for controller pod.
	if err = c.createControllerSecretSharedSecret(); err != nil {
		return errors.Annotate(err, "creating shared-secret secret for controller")
	}
	if isDone() {
		return bootstrap.Cancelled()
	}

	// create server.pem secret for controller pod.
	if err = c.createControllerSecretServerPem(); err != nil {
		return errors.Annotate(err, "creating server.pem secret for controller")
	}
	if isDone() {
		return bootstrap.Cancelled()
	}

	// create mongo admin account secret for controller pod.
	if err = c.createControllerSecretMongoAdmin(); err != nil {
		return errors.Annotate(err, "creating mongo admin account secret for controller")
	}
	if isDone() {
		return bootstrap.Cancelled()
	}

	// create bootstrap-params configmap for controller pod.
	if err = c.ensureControllerConfigmapBootstrapParams(); err != nil {
		return errors.Annotate(err, "creating bootstrap-params configmap for controller")
	}
	if isDone() {
		return bootstrap.Cancelled()
	}

	// Note: create agent config configmap for controller pod lastly because agentConfig has been updated in previous steps.
	if err = c.ensureControllerConfigmapAgentConf(); err != nil {
		return errors.Annotate(err, "creating agent config configmap for controller")
	}
	if isDone() {
		return bootstrap.Cancelled()
	}

	// create service account for local cluster/provider connections.
	if err = c.ensureControllerServiceAccount(); err != nil {
		return errors.Annotate(err, "creating service account for controller")
	}
	if isDone() {
		return bootstrap.Cancelled()
	}

	// create statefulset to ensure controller stack.
	if err = c.createControllerStatefulset(); err != nil {
		return errors.Annotate(err, "creating statefulset for controller")
	}
	if isDone() {
		return bootstrap.Cancelled()
	}

	return nil
}

func (c *controllerStack) getControllerSvcSpec(cloudType string, cfg *podcfg.BootstrapConfig) (spec *controllerServiceSpec, err error) {
	defer func() {
		if spec != nil && len(spec.ServiceType) == 0 {
			// ServiceType is required.
			err = errors.NotValidf("service type is empty for %q", cloudType)
		}
	}()

	spec = getDefaultControllerServiceSpecs(cloudType)
	if cfg == nil {
		return spec, nil
	}
	if len(cfg.ControllerServiceType) > 0 {
		if spec.ServiceType, err = caasServiceToK8s(caas.ServiceType(cfg.ControllerServiceType)); err != nil {
			return nil, errors.Trace(err)
		}
	}

	spec.ExternalIPs = append([]string(nil), cfg.ControllerExternalIPs...)

	switch spec.ServiceType {
	case core.ServiceTypeExternalName:
		spec.ExternalName = cfg.ControllerExternalName
	case core.ServiceTypeLoadBalancer:
		if len(cfg.ControllerExternalName) > 0 {
			return nil, errors.NewNotValid(nil, fmt.Sprintf(
				"external name %q provided but service type was set to %q",
				cfg.ControllerExternalName, spec.ServiceType,
			))
		}
		if len(cfg.ControllerExternalIPs) > 0 {
			spec.ExternalIP = cfg.ControllerExternalIPs[0]
		}
	}
	return spec, nil
}

func (c *controllerStack) createControllerProxy() error {
	if c.pcfg.Bootstrap.IgnoreProxy {
		return nil
	}

	// Lets first take a look at what will be deployed for a service.
	// If the service type is clusterip then we will setup the proxy

	cloudType, _, _ := cloud.SplitHostCloudRegion(c.pcfg.Bootstrap.ControllerCloud.HostCloudRegion)
	controllerSvcSpec, err := c.getControllerSvcSpec(cloudType, c.pcfg.Bootstrap)
	if err != nil {
		return errors.Trace(err)
	}

	if controllerSvcSpec.ServiceType != core.ServiceTypeClusterIP {
		// Not a cluster ip service so we don't need to setup a k8s proxy
		return nil
	}

	k8sClient := c.broker.client()

	remotePort := intstr.FromInt(c.portAPIServer)
	config := k8sproxy.ControllerProxyConfig{
		Name:          c.getResourceName(proxyResourceName),
		Namespace:     c.broker.GetCurrentNamespace(),
		RemotePort:    remotePort.String(),
		TargetService: c.resourceNameService,
	}

	err = k8sproxy.CreateControllerProxy(
		config,
		c.stackLabels,
		k8sClient.CoreV1().ConfigMaps(c.broker.GetCurrentNamespace()),
		k8sClient.RbacV1().Roles(c.broker.GetCurrentNamespace()),
		k8sClient.RbacV1().RoleBindings(c.broker.GetCurrentNamespace()),
		k8sClient.CoreV1().ServiceAccounts(c.broker.GetCurrentNamespace()),
	)

	return errors.Trace(err)
}

func (c *controllerStack) createControllerService() error {
	svcName := c.resourceNameService

	cloudType, _, _ := cloud.SplitHostCloudRegion(c.pcfg.Bootstrap.ControllerCloud.HostCloudRegion)
	controllerSvcSpec, err := c.getControllerSvcSpec(cloudType, c.pcfg.Bootstrap)
	if err != nil {
		return errors.Trace(err)
	}

	spec := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:        svcName,
			Labels:      c.stackLabels,
			Namespace:   c.broker.GetCurrentNamespace(),
			Annotations: c.stackAnnotations,
		},
		Spec: core.ServiceSpec{
			Selector: c.selectorLabels,
			Type:     controllerSvcSpec.ServiceType,
			Ports: []core.ServicePort{
				{
					Name:       "api-server",
					TargetPort: intstr.FromInt(c.portAPIServer),
					Port:       int32(c.portAPIServer),
				},
			},
			ExternalName:   controllerSvcSpec.ExternalName,
			ExternalIPs:    controllerSvcSpec.ExternalIPs,
			LoadBalancerIP: controllerSvcSpec.ExternalIP,
		},
	}

	if controllerSvcSpec.Annotations != nil {
		spec.SetAnnotations(controllerSvcSpec.Annotations.ToMap())
	}

	logger.Debugf("creating controller service: \n%+v", spec)
	if _, err := c.broker.ensureK8sService(spec); err != nil {
		return errors.Trace(err)
	}

	c.addCleanUp(func() {
		logger.Debugf("deleting %q", svcName)
		_ = c.broker.deleteService(svcName)
	})

	publicAddressPoller := func() error {
		// get the service by app name;
		svc, err := c.broker.GetService(c.stackName, caas.ModeWorkload, false)
		if err != nil {
			return errors.Annotate(err, "getting controller service")
		}
		if len(svc.Addresses) == 0 {
			return errors.NotProvisionedf("controller service address")
		}
		// we need to ensure svc DNS has been provisioned already here because
		// we do Not want bootstrap-state cmd wait instead.
		return nil
	}

	retryCallArgs := retry.CallArgs{
		Attempts:    60,
		Delay:       3 * time.Second,
		MaxDuration: 3 * time.Minute,
		Clock:       c.broker.clock,
		Func:        publicAddressPoller,
		IsFatalError: func(err error) bool {
			return !errors.IsNotProvisioned(err)
		},
		NotifyFunc: func(err error, attempt int) {
			logger.Debugf("polling k8s controller svc DNS, in %d attempt, %v", attempt, err)
		},
	}

	return errors.Trace(retry.Call(retryCallArgs))
}

func (c *controllerStack) addCleanUp(cleanUp func()) {
	c.cleanUps = append(c.cleanUps, cleanUp)
}

func (c *controllerStack) createControllerSecretSharedSecret() error {
	si, ok := c.agentConfig.StateServingInfo()
	if !ok {
		return errors.NewNotValid(nil, "agent config has no state serving info")
	}

	secret, err := c.getControllerSecret()
	if err != nil {
		return errors.Trace(err)
	}
	secret.Data[c.fileNameSharedSecret] = []byte(si.SharedSecret)
	logger.Debugf("ensuring shared secret: \n%+v", secret)
	c.addCleanUp(func() {
		logger.Debugf("deleting %q shared-secret", secret.Name)
		_ = c.broker.deleteSecret(secret.GetName(), secret.GetUID())
	})
	return c.broker.updateSecret(secret)
}

func (c *controllerStack) createControllerSecretServerPem() error {
	si, ok := c.agentConfig.StateServingInfo()
	if !ok || si.CAPrivateKey == "" {
		// No certificate information exists yet, nothing to do.
		return errors.NewNotValid(nil, "certificate is empty")
	}

	secret, err := c.getControllerSecret()
	if err != nil {
		return errors.Trace(err)
	}
	secret.Data[c.fileNameSSLKey] = []byte(mongo.GenerateSSLKey(si.Cert, si.PrivateKey))

	logger.Debugf("ensuring server.pem secret: \n%+v", secret)
	c.addCleanUp(func() {
		logger.Debugf("deleting %q server.pem", secret.Name)
		_ = c.broker.deleteSecret(secret.GetName(), secret.GetUID())
	})
	return c.broker.updateSecret(secret)
}

func (c *controllerStack) createControllerSecretMongoAdmin() error {
	return nil
}

func (c *controllerStack) ensureControllerConfigmapBootstrapParams() error {
	bootstrapParamsFileContent, err := c.pcfg.Bootstrap.StateInitializationParams.Marshal()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("bootstrapParams file content: \n%s", string(bootstrapParamsFileContent))

	cm, err := c.getControllerConfigMap()
	if err != nil {
		return errors.Trace(err)
	}
	cm.Data[c.fileNameBootstrapParams] = string(bootstrapParamsFileContent)

	logger.Debugf("creating bootstrap-params configmap: \n%+v", cm)

	cleanUp, err := c.broker.ensureConfigMap(cm)
	c.addCleanUp(func() {
		logger.Debugf("deleting %q bootstrap-params", cm.Name)
		cleanUp()
	})
	return errors.Trace(err)
}

func (c *controllerStack) ensureControllerConfigmapAgentConf() error {
	agentConfigFileContent, err := c.agentConfig.Render()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("agentConfig file content: \n%s", string(agentConfigFileContent))

	cm, err := c.getControllerConfigMap()
	if err != nil {
		return errors.Trace(err)
	}
	cm.Data[c.fileNameAgentConf] = string(agentConfigFileContent)

	logger.Debugf("ensuring agent.conf configmap: \n%+v", cm)
	cleanUp, err := c.broker.ensureConfigMap(cm)
	c.addCleanUp(func() {
		logger.Debugf("deleting %q template-agent.conf", cm.Name)
		cleanUp()
	})
	return errors.Trace(err)
}

func (c *controllerStack) ensureControllerServiceAccount() error {
	sa := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "controller",
			Namespace: c.broker.GetCurrentNamespace(),
			Labels: providerutils.LabelsMerge(
				c.stackLabels,
				providerutils.LabelsJujuModelOperatorDisableWebhook,
			),
			Annotations: c.stackAnnotations,
		},
		AutomountServiceAccountToken: boolPtr(true),
	}

	logger.Debugf("ensuring controller service account: \n%+v", sa)
	_, cleanUps, err := c.broker.ensureServiceAccount(sa)
	c.addCleanUp(func() {
		logger.Debugf("deleting %q service account", sa.Name)
		for _, v := range cleanUps {
			v()
		}
	})
	if err != nil {
		return errors.Trace(err)
	}

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:        c.broker.GetCurrentNamespace(), // name cluster role binding after the controller namespace.
			Labels:      providerutils.LabelsForModel("controller", false),
			Annotations: c.stackAnnotations,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      "controller",
			Namespace: c.broker.GetCurrentNamespace(),
		}},
	}

	_, crbCleanUps, err := c.broker.ensureClusterRoleBinding(crb)
	c.addCleanUp(func() {
		logger.Debugf("deleting %q cluster role binding", crb.Name)
		for _, v := range crbCleanUps {
			v()
		}
	})
	return errors.Trace(err)
}

func (c *controllerStack) createControllerStatefulset() error {
	numberOfPods := int32(1) // TODO(caas): HA mode!
	spec := &apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name: c.resourceNameStatefulSet,
			Labels: providerutils.LabelsMerge(
				c.stackLabels,
				providerutils.LabelsJujuModelOperatorDisableWebhook,
			),
			Namespace:   c.broker.GetCurrentNamespace(),
			Annotations: c.stackAnnotations,
		},
		Spec: apps.StatefulSetSpec{
			ServiceName: c.resourceNameService,
			Replicas:    &numberOfPods,
			Selector: &v1.LabelSelector{
				MatchLabels: c.selectorLabels,
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: providerutils.LabelsMerge(
						c.selectorLabels,
						providerutils.LabelsJujuModelOperatorDisableWebhook,
					),
					Name:        c.pcfg.GetPodName(), // This really should not be set.
					Namespace:   c.broker.GetCurrentNamespace(),
					Annotations: c.stackAnnotations,
				},
				Spec: core.PodSpec{
					RestartPolicy:                core.RestartPolicyAlways,
					ServiceAccountName:           "controller",
					AutomountServiceAccountToken: boolPtr(true),
				},
			},
		},
	}

	if err := c.buildStorageSpecForController(spec); err != nil {
		return errors.Trace(err)
	}
	if err := c.buildContainerSpecForController(spec); err != nil {
		return errors.Trace(err)
	}

	if err := processConstraints(&spec.Spec.Template.Spec, c.stackName, c.pcfg.Bootstrap.BootstrapMachineConstraints); err != nil {
		return errors.Trace(err)
	}

	logger.Debugf("creating controller statefulset: \n%+v", spec)
	c.addCleanUp(func() {
		logger.Debugf("deleting %q statefulset", spec.Name)
		_ = c.broker.deleteStatefulSet(spec.Name)
	})
	w, err := c.broker.WatchUnits(c.resourceNameStatefulSet, caas.ModeWorkload)
	if err != nil {
		return errors.Trace(err)
	}
	defer w.Kill()

	if _, err = c.broker.createStatefulSet(spec); err != nil {
		return errors.Trace(err)
	}

	for i := int32(0); i < numberOfPods; i++ {
		podName := c.pcfg.GetPodName() // TODO(caas): HA mode!
		if err = c.waitForPod(w, podName); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (c *controllerStack) waitForPod(podWatcher watcher.NotifyWatcher, podName string) error {
	timeout := c.broker.clock.NewTimer(c.pcfg.Bootstrap.Timeout)

	podEventWatcher, err := c.broker.watchEvents(podName, "Pod")
	if err != nil {
		return errors.Trace(err)
	}
	defer podEventWatcher.Kill()

	printedMsg := set.NewStrings()
	printPodEvents := func() error {
		events, err := c.broker.getEvents(podName, "Pod")
		if err != nil {
			return errors.Trace(err)
		}
		for _, evt := range events {
			// clean the messages to prevent duplicated records.
			// we don't care which image is been pulling/pulled and this reason should be printed once only.
			switch evt.Reason {
			case PullingImage:
				evt.Message = "Downloading images"
			case PulledImage:
				evt.Message = "Pulled images"
			case StartedContainer:
				if evt.InvolvedObject.FieldPath == fmt.Sprintf("spec.containers{%s}", mongoDBContainerName) {
					evt.Message = "Started mongodb container"
				} else if evt.InvolvedObject.FieldPath == fmt.Sprintf("spec.containers{%s}", apiServerContainerName) {
					evt.Message = "Started controller container"
				}
			}
			if evt.Type == core.EventTypeNormal && !printedMsg.Contains(evt.Message) {
				printedMsg.Add(evt.Message)
				logger.Debugf(evt.Message)
				if evt.Reason == PullingImage {
					c.ctx.Infof(evt.Message)
				}
			}
		}
		return nil
	}

	unschedulableReason := func(pod *core.Pod) error {
		// TODO: handle reason for unschedulable state such as node taints (HA)
		// Volumes
		for _, volume := range pod.Spec.Volumes {
			if pvcSource := volume.PersistentVolumeClaim; pvcSource != nil {
				pvc, err := c.broker.getPVC(pvcSource.ClaimName)
				if err != nil {
					return errors.Annotatef(err, "failed to get pvc %s", pvcSource.ClaimName)
				}
				if pvc.Status.Phase == core.ClaimPending {
					events, err := c.broker.getEvents(pvc.Name, "PersistentVolumeClaim")
					if err != nil {
						return errors.Annotate(err, "failed to get pvc events")
					}
					numEvents := len(events)
					if numEvents > 0 {
						lastEvent := events[numEvents-1]
						return errors.Errorf("pvc %s pending due to %s - %s",
							pvc.Name, lastEvent.Reason, lastEvent.Message)
					}
				}
			}
		}
		return nil
	}

	pendingReason := func() error {
		pod, err := c.broker.getPod(podName)
		if err != nil {
			return errors.Trace(err)
		}
		for _, cond := range pod.Status.Conditions {
			switch cond.Type {
			case core.PodScheduled:
				if cond.Reason == core.PodReasonUnschedulable {
					err := unschedulableReason(pod)
					if err != nil {
						return errors.Annotate(err, "unschedulable")
					}
					return errors.Errorf("unschedulable: %v", cond.Message)
				}
			}
		}
		if pod.Status.Phase == core.PodPending {
			return errors.Errorf("pending: %v - %v", pod.Status.Reason, pod.Status.Message)
		}
		return nil
	}

	checkStatus := func(pod *core.Pod) (bool, error) {
		switch pod.Status.Phase {
		case core.PodRunning:
			return true, nil
		case core.PodFailed:
			return false, errors.Annotate(pendingReason(), "controller pod failed")
		case core.PodSucceeded:
			return false, errors.Errorf("controller pod terminated unexpectedly")
		}
		return false, nil
	}

	_ = printPodEvents()
	for {
		select {
		case <-podWatcher.Changes():
			_ = printPodEvents()
			pod, err := c.broker.getPod(podName)
			if errors.IsNotFound(err) {
				logger.Debugf("pod %q is not provisioned yet", podName)
				continue
			}
			if err != nil {
				return errors.Annotate(err, "fetching pods' status for controller")
			}
			done, err := checkStatus(pod)
			if err != nil {
				return errors.Trace(err)
			}
			if done {
				c.ctx.Infof("Starting controller pod")
				return nil
			}
		case <-podEventWatcher.Changes():
			_ = printPodEvents()
		case <-timeout.Chan():
			err := pendingReason()
			if err != nil {
				return errors.Annotatef(err, "timed out waiting for controller pod")
			}
			return errors.Timeoutf("timed out waiting for controller pod")
		}
	}
}

func (c *controllerStack) buildStorageSpecForController(statefulset *apps.StatefulSet) error {
	sc, err := c.broker.getStorageClass(c.storageClass)
	if err != nil {
		return errors.Trace(err)
	}
	// try to find <namespace>-<c.storageClass>,
	// if it's not found, then fallback to c.storageClass.
	c.storageClass = sc.GetName()

	// build persistent volume claim.
	statefulset.Spec.VolumeClaimTemplates = []core.PersistentVolumeClaim{
		{
			ObjectMeta: v1.ObjectMeta{
				Name:        c.pvcNameControllerPodStorage,
				Labels:      c.stackLabels,
				Annotations: c.stackAnnotations,
			},
			Spec: core.PersistentVolumeClaimSpec{
				StorageClassName: &c.storageClass,
				AccessModes:      []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceStorage: c.storageSize,
					},
				},
			},
		},
	}

	fileMode := int32(256)
	var vols []core.Volume
	// add volume server.pem secret.
	vols = append(vols, core.Volume{
		Name: c.resourceNameVolSSLKey,
		VolumeSource: core.VolumeSource{
			Secret: &core.SecretVolumeSource{
				SecretName:  c.resourceNameSecret,
				DefaultMode: &fileMode,
				Items: []core.KeyToPath{
					{
						Key:  c.fileNameSSLKey,
						Path: c.fileNameSSLKeyMount,
					},
				},
			},
		},
	})
	// add volume shared secret.
	vols = append(vols, core.Volume{
		Name: c.resourceNameVolSharedSecret,
		VolumeSource: core.VolumeSource{
			Secret: &core.SecretVolumeSource{
				SecretName:  c.resourceNameSecret,
				DefaultMode: &fileMode,
				Items: []core.KeyToPath{
					{
						Key:  c.fileNameSharedSecret,
						Path: c.fileNameSharedSecret,
					},
				},
			},
		},
	})
	// add volume agent.conf comfigmap.
	volAgentConf := core.Volume{
		Name: c.resourceNameVolAgentConf,
		VolumeSource: core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				Items: []core.KeyToPath{
					{
						Key:  c.fileNameAgentConf,
						Path: c.fileNameAgentConfMount,
					},
				},
			},
		},
	}
	volAgentConf.VolumeSource.ConfigMap.Name = c.resourceNameConfigMap
	vols = append(vols, volAgentConf)
	// add volume bootstrap-params comfigmap.
	volBootstrapParams := core.Volume{
		Name: c.resourceNameVolBootstrapParams,
		VolumeSource: core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				Items: []core.KeyToPath{
					{
						Key:  c.fileNameBootstrapParams,
						Path: c.fileNameBootstrapParams,
					},
				},
			},
		},
	}
	volBootstrapParams.VolumeSource.ConfigMap.Name = c.resourceNameConfigMap
	vols = append(vols, volBootstrapParams)

	statefulset.Spec.Template.Spec.Volumes = vols
	return nil
}

func (c *controllerStack) buildContainerSpecForController(statefulset *apps.StatefulSet) error {
	var wiredTigerCacheSize float32
	if c.pcfg.Controller.Config.MongoMemoryProfile() == string(mongo.MemoryProfileLow) {
		wiredTigerCacheSize = mongo.LowCacheSize
	}
	generateContainerSpecs := func(jujudCmd string) []core.Container {
		var containerSpec []core.Container
		// add container mongoDB.
		// TODO(bootstrap): refactor mongo package to make it usable for IAAS and CAAS,
		// then generate mongo config from EnsureServerParams.
		probCmds := &core.ExecAction{
			Command: []string{
				"mongo",
				fmt.Sprintf("--port=%d", c.portMongoDB),
				"--ssl",
				"--sslAllowInvalidHostnames",
				"--sslAllowInvalidCertificates",
				fmt.Sprintf("--sslPEMKeyFile=%s/%s", c.pcfg.DataDir, c.fileNameSSLKey),
				"--eval",
				"db.adminCommand('ping')",
			},
		}
		args := []string{
			fmt.Sprintf("--dbpath=%s/db", c.pcfg.DataDir),
			fmt.Sprintf("--sslPEMKeyFile=%s/%s", c.pcfg.DataDir, c.fileNameSSLKey),
			"--sslPEMKeyPassword=ignored",
			"--sslMode=requireSSL",
			fmt.Sprintf("--port=%d", c.portMongoDB),
			"--journal",
			fmt.Sprintf("--replSet=%s", mongo.ReplicaSetName),
			"--quiet",
			"--oplogSize=1024",
			"--ipv6",
			"--auth",
			fmt.Sprintf("--keyFile=%s/%s", c.pcfg.DataDir, c.fileNameSharedSecret),
			"--storageEngine=wiredTiger",
			"--bind_ip_all",
		}
		if wiredTigerCacheSize > 0 {
			args = append(args, fmt.Sprintf("--wiredTigerCacheSizeGB=%v", wiredTigerCacheSize))
		}
		containerSpec = append(containerSpec, core.Container{
			Name:            mongoDBContainerName,
			ImagePullPolicy: core.PullIfNotPresent,
			Image:           c.pcfg.GetJujuDbOCIImagePath(),
			Command: []string{
				"mongod",
			},
			Args: args,
			Ports: []core.ContainerPort{
				{
					Name:          "mongodb",
					ContainerPort: int32(c.portMongoDB),
					Protocol:      "TCP",
				},
			},
			ReadinessProbe: &core.Probe{
				Handler: core.Handler{
					Exec: probCmds,
				},
				FailureThreshold:    3,
				InitialDelaySeconds: 5,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				TimeoutSeconds:      1,
			},
			LivenessProbe: &core.Probe{
				Handler: core.Handler{
					Exec: probCmds,
				},
				FailureThreshold:    3,
				InitialDelaySeconds: 30,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				TimeoutSeconds:      5,
			},
			VolumeMounts: []core.VolumeMount{
				{
					Name:      c.pvcNameControllerPodStorage,
					MountPath: c.pcfg.DataDir,
				},
				{
					Name:      c.pvcNameControllerPodStorage,
					MountPath: filepath.Join(c.pcfg.DataDir, "db"),
					SubPath:   "db",
				},
				{
					Name:      c.resourceNameVolSSLKey,
					MountPath: filepath.Join(c.pcfg.DataDir, c.fileNameSSLKeyMount),
					SubPath:   c.fileNameSSLKeyMount,
					ReadOnly:  true,
				},
				{
					Name:      c.resourceNameVolSharedSecret,
					MountPath: filepath.Join(c.pcfg.DataDir, c.fileNameSharedSecret),
					SubPath:   c.fileNameSharedSecret,
					ReadOnly:  true,
				},
			},
		})

		// add container API server.
		containerSpec = append(containerSpec, core.Container{
			Name:            apiServerContainerName,
			ImagePullPolicy: core.PullIfNotPresent,
			Image:           c.pcfg.GetControllerImagePath(),
			Command: []string{
				"/bin/sh",
			},
			Args: []string{
				"-c",
				fmt.Sprintf(
					caas.JujudStartUpSh,
					c.pcfg.DataDir,
					"tools",
					jujudCmd,
				),
			},
			WorkingDir: c.pcfg.DataDir,
			VolumeMounts: []core.VolumeMount{
				{
					Name:      c.pvcNameControllerPodStorage,
					MountPath: c.pcfg.DataDir,
				},
				{
					Name: c.resourceNameVolAgentConf,
					MountPath: filepath.Join(
						c.pcfg.DataDir,
						"agents",
						"controller-"+c.pcfg.ControllerId,
						c.fileNameAgentConfMount,
					),
					SubPath: c.fileNameAgentConfMount,
				},
				{
					Name:      c.resourceNameVolSSLKey,
					MountPath: filepath.Join(c.pcfg.DataDir, c.fileNameSSLKeyMount),
					SubPath:   c.fileNameSSLKeyMount,
					ReadOnly:  true,
				},
				{
					Name:      c.resourceNameVolSharedSecret,
					MountPath: filepath.Join(c.pcfg.DataDir, c.fileNameSharedSecret),
					SubPath:   c.fileNameSharedSecret,
					ReadOnly:  true,
				},
				{
					Name:      c.resourceNameVolBootstrapParams,
					MountPath: filepath.Join(c.pcfg.DataDir, c.fileNameBootstrapParams),
					SubPath:   c.fileNameBootstrapParams,
					ReadOnly:  true,
				},
			},
		})
		c.containerCount = len(containerSpec)
		return containerSpec
	}

	loggingOption := "--show-log"
	if loggo.GetLogger("").LogLevel() == loggo.DEBUG {
		// If the bootstrap command was requested with --debug, then the root
		// logger will be set to DEBUG. If it is, then we use --debug here too.
		loggingOption = "--debug"
	}

	agentConfigRelativePath := filepath.Join(
		"agents",
		fmt.Sprintf("controller-%s", c.pcfg.ControllerId),
		c.fileNameAgentConf,
	)
	var jujudCmd string
	if c.pcfg.ControllerId == agent.BootstrapControllerId {
		dashboardCmd, err := c.setUpDashboardCommand()
		if err != nil {
			return errors.Trace(err)
		}
		if dashboardCmd != "" {
			jujudCmd += "\n" + dashboardCmd
		}
		// only do bootstrap-state on the bootstrap controller - controller-0.
		jujudCmd += "\n" + fmt.Sprintf(
			"test -e $JUJU_DATA_DIR/%s || $JUJU_TOOLS_DIR/jujud bootstrap-state $JUJU_DATA_DIR/%s --data-dir $JUJU_DATA_DIR %s --timeout %s",
			agentConfigRelativePath,
			c.fileNameBootstrapParams,
			loggingOption,
			c.pcfg.Bootstrap.Timeout.String(),
		)
	}
	jujudCmd += "\n" + fmt.Sprintf(
		"$JUJU_TOOLS_DIR/jujud machine --data-dir $JUJU_DATA_DIR --controller-id %s --log-to-stderr %s",
		c.pcfg.ControllerId,
		loggingOption,
	)
	statefulset.Spec.Template.Spec.Containers = generateContainerSpecs(jujudCmd)
	return nil
}

func (c *controllerStack) setUpDashboardCommand() (string, error) {
	if c.pcfg.Bootstrap.Dashboard == nil {
		return "", nil
	}
	var dashboardCmds []string
	u, err := url.Parse(c.pcfg.Bootstrap.Dashboard.URL)
	if err != nil {
		return "", errors.Annotate(err, "cannot parse Juju Dashboard URL")
	}
	dashboardJson, err := json.Marshal(c.pcfg.Bootstrap.Dashboard)
	if err != nil {
		return "", errors.Trace(err)
	}
	dashboardDir := agenttools.SharedDashboardDir(c.pcfg.DataDir)
	dashboardCmds = append(dashboardCmds,
		"echo Installing Dashboard...",
		"export dashboard="+utils.ShQuote(dashboardDir),
		"mkdir -p $dashboard",
	)
	// Download the Dashboard from simplestreams.
	command := "curl -sSf -o $dashboard/dashboard.tar.bz2 --retry 10"
	if c.pcfg.DisableSSLHostnameVerification {
		command += " --insecure"
	}

	curlProxyArgs := formatCurlProxyArguments(u.String(), c.pcfg.ProxySettings)
	command += curlProxyArgs
	command += " " + utils.ShQuote(u.String())
	// A failure in fetching the Juju Dashboard archive should not prevent the
	// model to be bootstrapped. Better no Dashboard than no Juju at all.
	command += " || echo Unable to retrieve Juju Dashboard"
	dashboardCmds = append(dashboardCmds, command)
	dashboardCmds = append(dashboardCmds,
		"[ -f $dashboard/dashboard.tar.bz2 ] && sha256sum $dashboard/dashboard.tar.bz2 > $dashboard/jujudashboard.sha256",
		fmt.Sprintf(
			`[ -f $dashboard/jujudashboard.sha256 ] && (grep '%s' $dashboard/jujudashboard.sha256 && printf %%s %s > $dashboard/downloaded-dashboard.txt || echo Juju Dashboard checksum mismatch)`,
			c.pcfg.Bootstrap.Dashboard.SHA256, utils.ShQuote(string(dashboardJson))),
	)
	return strings.Join(dashboardCmds, "\n"), nil
}

func formatCurlProxyArguments(dashboardURL string, proxySettings proxy.Settings) (proxyArgs string) {
	if strings.HasPrefix(dashboardURL, "http://") && proxySettings.Http != "" {
		proxyUrl := proxySettings.Http
		proxyArgs += fmt.Sprintf(" --proxy %s", proxyUrl)
	} else if strings.HasPrefix(dashboardURL, "https://") && proxySettings.Https != "" {
		proxyUrl := proxySettings.Https
		// curl automatically uses HTTP CONNECT for URLs containing HTTPS
		proxyArgs += fmt.Sprintf(" --proxy %s", proxyUrl)
	}
	if proxySettings.NoProxy != "" {
		proxyArgs += fmt.Sprintf(" --noproxy %s", proxySettings.NoProxy)
	}
	return
}
