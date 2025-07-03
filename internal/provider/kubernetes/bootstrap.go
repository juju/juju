// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	"github.com/juju/retry"
	"gopkg.in/yaml.v3"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"

	"github.com/juju/juju/agent"
	agentconstants "github.com/juju/juju/agent/constants"
	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	environsbootstrap "github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry"
	"github.com/juju/juju/internal/featureflag"
	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/internal/provider/kubernetes/application"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/pebble"
	k8sproxy "github.com/juju/juju/internal/provider/kubernetes/proxy"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	providerutils "github.com/juju/juju/internal/provider/kubernetes/utils"
	"github.com/juju/juju/internal/service/pebble/plan"
	"github.com/juju/juju/juju/osenv"
)

const (
	proxyResourceName           = "proxy"
	storageName                 = "storage"
	mongoScratchStorageName     = "mongo-scratch"
	apiServerScratchStorageName = "apiserver-scratch"
)

var (
	// TemplateFileNameServerPEM is the template server.pem file name.
	TemplateFileNameServerPEM = "template-" + mongo.FileNameDBSSLKey
)

const (
	mongoDBContainerName   = "mongodb"
	apiServerContainerName = "api-server"

	// startupGraceTime is the number of seconds afforded to startup probes to
	// become successful before considering them a failure.
	startupGraceTime = 600

	apiServerStartupProbeInitialDelay = 3
	apiServerStartupProbeTimeout      = 3
	apiServerStartupProbePeriod       = 3
	apiServerStartupProbeSuccess      = 1
	apiServerStartupProbeFailure      = startupGraceTime / apiServerStartupProbePeriod

	apiServerLivenessProbeInitialDelay = 1
	apiServerLivenessProbeTimeout      = 3
	apiServerLivenessProbePeriod       = 5
	apiServerLivenessProbeSuccess      = 1
	apiServerLivenessProbeFailure      = 2

	mongoDBStartupProbeInitialDelay = 1
	mongoDBStartupProbeTimeout      = 1
	mongoDBStartupProbePeriod       = 5
	mongoDBStartupProbeSuccess      = 1
	mongoDBStartupProbeFailure      = startupGraceTime / mongoDBStartupProbePeriod
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
	logger environs.BootstrapLogger

	stackName        string
	selectorLabels   map[string]string
	stackLabels      map[string]string
	stackAnnotations map[string]string
	broker           *kubernetesClient
	timeout          time.Duration

	pcfg *podcfg.ControllerPodConfig
	// agentConfig is the controller api server config.
	agentConfig agent.ConfigSetterWriter
	// unitAgentConfig is the controller charm agent config.
	unitAgentConfig agent.ConfigSetterWriter

	storageClass               string
	storageSize                resource.Quantity
	portMongoDB, portAPIServer int
	portSSHServer              int

	resourceNameService,
	resourceNameConfigMap,
	resourceNameSecret, resourceNamedockerSecret,
	resourceNameVolSharedSecret, resourceNameVolSSLKey,
	resourceNameVolBootstrapParams, resourceNameVolAgentConf string

	dockerAuthSecretData []byte

	cleanUps []func()
}

type controllerStacker interface {
	// Deploy creates all resources for controller stack.
	Deploy(ctx context.Context) error
}

// findControllerNamespace is used for finding a controller's namespace based on
// its model name and controller uuid. This function really shouldn't exist
// and should be removed in 3.0. We have it here as we are still trying to use
// Kubernetes annotations as database selectors in some parts of Juju.
func findControllerNamespace(
	ctx context.Context,
	client kubernetes.Interface,
	controllerUUID string,
) (*core.Namespace, error) {
	// First we are going to start off by listing namespaces that are not using
	// legacy labels as that is the direction we are moving towards and hence
	// should be the quickest operation
	namespaces, err := client.CoreV1().Namespaces().List(
		ctx,
		metav1.ListOptions{
			LabelSelector: labels.Set{
				constants.LabelJujuModelName: environsbootstrap.ControllerModelName,
			}.String(),
		},
	)

	if err != nil {
		return nil, errors.Annotate(err, "finding controller namespace with non legacy labels")
	}

	for _, ns := range namespaces.Items {
		if ns.Annotations[providerutils.AnnotationControllerUUIDKey(constants.LabelVersion1)] == controllerUUID {
			return &ns, nil
		}
	}

	// We didn't find anything using new labels so lets try the old ones.
	namespaces, err = client.CoreV1().Namespaces().List(
		ctx,
		metav1.ListOptions{
			LabelSelector: labels.Set{
				constants.LegacyLabelModelName: environsbootstrap.ControllerModelName,
			}.String(),
		},
	)

	if err != nil {
		return nil, errors.Annotate(err, "finding controller namespace with legacy labels")
	}

	for _, ns := range namespaces.Items {
		if ns.Annotations[providerutils.AnnotationControllerUUIDKey(constants.LegacyLabelVersion)] == controllerUUID {
			return &ns, nil
		}
	}

	return nil, errors.NotFoundf(
		"controller namespace not found for model %q and controller uuid %q",
		environsbootstrap.ControllerModelName,
		controllerUUID,
	)
}

// DecideControllerNamespace decides the namespace name to use for a new controller.
func DecideControllerNamespace(controllerName string) string {
	return "controller-" + controllerName
}

func newControllerStack(
	logger environs.BootstrapLogger,
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

	agentConfig.SetStateServingInfo(si)
	pcfg.Bootstrap.StateServingInfo = si

	unitAgentConfig, err := pcfg.UnitAgentConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}

	selectorLabels := providerutils.SelectorLabelsForApp(stackName, constants.LastLabelVersion)
	labels := providerutils.LabelsForApp(stackName, constants.LastLabelVersion)

	controllerUUIDKey := providerutils.AnnotationControllerUUIDKey(constants.LastLabelVersion)
	cs := &controllerStack{
		logger:           logger,
		stackName:        stackName,
		selectorLabels:   selectorLabels,
		stackLabels:      labels,
		stackAnnotations: map[string]string{controllerUUIDKey: pcfg.ControllerTag.Id()},
		broker:           broker,
		timeout:          pcfg.Bootstrap.Timeout,

		pcfg:            pcfg,
		agentConfig:     agentConfig,
		unitAgentConfig: unitAgentConfig,

		storageSize:   storageSize,
		storageClass:  storageClass,
		portMongoDB:   pcfg.Bootstrap.ControllerConfig.StatePort(),
		portAPIServer: pcfg.Bootstrap.ControllerConfig.APIPort(),
		portSSHServer: pcfg.Bootstrap.ControllerConfig.SSHServerPort(),
	}
	cs.resourceNameService = cs.getResourceName("service")
	cs.resourceNameConfigMap = cs.getResourceName("configmap")
	cs.resourceNameSecret = cs.getResourceName("secret")
	cs.resourceNamedockerSecret = constants.CAASImageRepoSecretName

	cs.resourceNameVolSharedSecret = cs.getResourceName(mongo.SharedSecretFile)
	cs.resourceNameVolSSLKey = cs.getResourceName(mongo.FileNameDBSSLKey)
	cs.resourceNameVolBootstrapParams = cs.getResourceName(cloudconfig.FileNameBootstrapParams)
	cs.resourceNameVolAgentConf = cs.getResourceName(agentconstants.AgentConfigFilename)

	// Initialize registry.
	repoDetails, err := docker.NewImageRepoDetails(pcfg.Controller.CAASImageRepo())
	if err != nil {
		return nil, errors.Annotatef(err, "parsing %s", controller.CAASImageRepo)
	}
	if !repoDetails.Empty() {
		reg, err := registry.New(repoDetails)
		if err != nil {
			return nil, errors.Trace(err)
		}
		defer func() { _ = reg.Close() }()
		err = reg.RefreshAuth()
		if err != nil {
			return nil, errors.Trace(err)
		}
		err = reg.Ping()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if cs.dockerAuthSecretData, err = reg.ImageRepoDetails().SecretData(); err != nil {
			return nil, errors.Trace(err)
		}
	}

	return cs, nil
}

func (c *controllerStack) isPrivateRepo() bool {
	return len(c.dockerAuthSecretData) > 0
}

func getBootstrapResourceName(stackName string, name string) string {
	return stackName + "-" + strings.Replace(name, ".", "-", -1)
}

func (c *controllerStack) getResourceName(name string) string {
	return getBootstrapResourceName(c.stackName, name)
}

func (c *controllerStack) pathJoin(elem ...string) string {
	// Setting series for bootstrapping to kubernetes is currently not supported.
	// We always use forward-slash because Linux is the only OS we support now.
	pathSeparator := "/"
	return strings.Join(elem, pathSeparator)
}

func (c *controllerStack) getControllerSecret(ctx context.Context) (secret *core.Secret, err error) {
	defer func() {
		if err == nil && secret != nil && secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
	}()

	secret, err = c.broker.getSecret(ctx, c.resourceNameSecret)
	if err == nil {
		return secret, nil
	}
	if errors.Is(err, errors.NotFound) {
		_, err = c.broker.createSecret(ctx, &core.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        c.resourceNameSecret,
				Labels:      c.stackLabels,
				Namespace:   c.broker.Namespace(),
				Annotations: c.stackAnnotations,
			},
			Type: core.SecretTypeOpaque,
		})
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.broker.getSecret(ctx, c.resourceNameSecret)
}

func (c *controllerStack) getControllerConfigMap(ctx context.Context) (cm *core.ConfigMap, err error) {
	defer func() {
		if cm != nil && cm.Data == nil {
			cm.Data = map[string]string{}
		}
	}()

	cm, err = c.broker.getConfigMap(ctx, c.resourceNameConfigMap)
	if err == nil {
		return cm, nil
	}
	if errors.Is(err, errors.NotFound) {
		_, err = c.broker.createConfigMap(ctx, &core.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:        c.resourceNameConfigMap,
				Labels:      c.stackLabels,
				Namespace:   c.broker.Namespace(),
				Annotations: c.stackAnnotations,
			},
		})
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.broker.getConfigMap(ctx, c.resourceNameConfigMap)
}

func (c *controllerStack) doCleanUp() {
	logger.Debugf(context.TODO(), "bootstrap failed, removing %d resources.", len(c.cleanUps))
	for _, f := range c.cleanUps {
		f()
	}
}

// Deploy creates all resources for the controller stack.
func (c *controllerStack) Deploy(ctx context.Context) (err error) {
	// creating namespace for controller stack, this namespace will be removed by broker.DestroyController if bootstrap failed.
	nsName := c.broker.Namespace()
	c.logger.Infof("Creating k8s resources for controller %q", nsName)
	if err = c.broker.createNamespace(ctx, nsName); err != nil {
		return errors.Annotate(err, "creating namespace for controller stack")
	}

	if environsbootstrap.IsContextDone(ctx) {
		return environsbootstrap.Cancelled()
	}

	defer func() {
		if err != nil {
			c.doCleanUp()
		}
	}()

	// create service for controller pod.
	if err = c.createControllerService(ctx); err != nil {
		return errors.Annotate(err, "creating service for controller")
	}
	if environsbootstrap.IsContextDone(ctx) {
		return environsbootstrap.Cancelled()
	}

	// create the proxy resources for services of type cluster ip
	if err = c.createControllerProxy(ctx); err != nil {
		return errors.Annotate(err, "creating controller service proxy for controller")
	}

	if environsbootstrap.IsContextDone(ctx) {
		return environsbootstrap.Cancelled()
	}

	// create server.pem secret for controller pod.
	if err = c.createControllerSecretServerPem(ctx); err != nil {
		return errors.Annotate(err, "creating server.pem secret for controller")
	}
	if environsbootstrap.IsContextDone(ctx) {
		return environsbootstrap.Cancelled()
	}

	// create bootstrap-params configmap for controller pod.
	if err = c.ensureControllerConfigmapBootstrapParams(ctx); err != nil {
		return errors.Annotate(err, "creating bootstrap-params configmap for controller")
	}
	if environsbootstrap.IsContextDone(ctx) {
		return environsbootstrap.Cancelled()
	}

	// Note: create agent config configmap for controller pod lastly because agentConfig has been updated in previous steps.
	if err = c.ensureControllerConfigmapAgentConf(ctx); err != nil {
		return errors.Annotate(err, "creating agent config configmap for controller")
	}
	if environsbootstrap.IsContextDone(ctx) {
		return environsbootstrap.Cancelled()
	}

	if err = c.ensureControllerApplicationSecret(ctx); err != nil {
		return errors.Annotate(err, "creating secret for controller application")
	}
	if environsbootstrap.IsContextDone(ctx) {
		return environsbootstrap.Cancelled()
	}

	// create service account for local cluster/provider connections.
	saName, saCleanUps, err := ensureControllerServiceAccount(
		ctx,
		c.broker.client(),
		c.broker.Namespace(),
		c.broker.ControllerUUID(),
		c.stackLabels,
		c.stackAnnotations,
	)
	c.addCleanUp(func() {
		logger.Debugf(context.TODO(), "delete controller service accounts")
		for _, v := range saCleanUps {
			v()
		}
	})
	if err != nil {
		return errors.Annotate(err, "creating service account for controller")
	}
	if environsbootstrap.IsContextDone(ctx) {
		return environsbootstrap.Cancelled()
	}

	if err = c.patchServiceAccountForImagePullSecret(ctx, saName); err != nil {
		return errors.Annotate(err, "patching image pull secret for controller service account")
	}
	if environsbootstrap.IsContextDone(ctx) {
		return environsbootstrap.Cancelled()
	}

	// create statefulset to ensure controller stack.
	if err = c.createControllerStatefulset(ctx); err != nil {
		return errors.Annotate(err, "creating statefulset for controller")
	}
	if environsbootstrap.IsContextDone(ctx) {
		return environsbootstrap.Cancelled()
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
		if spec.ServiceType, err = CaasServiceToK8s(caas.ServiceType(cfg.ControllerServiceType)); err != nil {
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

func (c *controllerStack) createControllerProxy(ctx context.Context) error {
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
		Namespace:     c.broker.Namespace(),
		RemotePort:    remotePort.String(),
		TargetService: c.resourceNameService,
	}

	err = k8sproxy.CreateControllerProxy(
		ctx,
		config,
		c.stackLabels,
		c.broker.clock,
		k8sClient.CoreV1().ConfigMaps(c.broker.Namespace()),
		k8sClient.RbacV1().Roles(c.broker.Namespace()),
		k8sClient.RbacV1().RoleBindings(c.broker.Namespace()),
		k8sClient.CoreV1().ServiceAccounts(c.broker.Namespace()),
		k8sClient.CoreV1().Secrets(c.broker.Namespace()),
	)

	return errors.Trace(err)
}

func (c *controllerStack) createControllerService(ctx context.Context) error {
	svcName := c.resourceNameService

	cloudType, _, _ := cloud.SplitHostCloudRegion(c.pcfg.Bootstrap.ControllerCloud.HostCloudRegion)
	controllerSvcSpec, err := c.getControllerSvcSpec(cloudType, c.pcfg.Bootstrap)
	if err != nil {
		return errors.Trace(err)
	}

	spec := &core.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        svcName,
			Labels:      c.stackLabels,
			Namespace:   c.broker.Namespace(),
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
				{
					Name:       "ssh-server",
					TargetPort: intstr.FromInt(c.portSSHServer),
					Port:       int32(c.portSSHServer),
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

	logger.Debugf(context.TODO(), "creating controller service: \n%+v", spec)
	if _, err := c.broker.ensureK8sService(ctx, spec); err != nil {
		return errors.Trace(err)
	}

	c.addCleanUp(func() {
		logger.Debugf(context.TODO(), "deleting %q", svcName)
		_ = c.broker.deleteService(ctx, svcName)
	})

	publicAddressPoller := func() error {
		// get the service by app name;
		svc, err := c.broker.GetService(ctx, c.stackName, false)
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
		Attempts: -1,
		Delay:    3 * time.Second,
		Stop:     ctx.Done(),
		Clock:    c.broker.clock,
		Func:     publicAddressPoller,
		IsFatalError: func(err error) bool {
			return !errors.Is(err, errors.NotProvisioned)
		},
		NotifyFunc: func(err error, attempt int) {
			logger.Debugf(context.TODO(), "polling k8s controller svc DNS, in %d attempt, %v", attempt, err)
		},
	}
	err = retry.Call(retryCallArgs)
	if retry.IsDurationExceeded(err) || (retry.IsRetryStopped(err) && ctx.Err() == context.DeadlineExceeded) {
		return errors.Timeoutf("waiting for controller service address fully provisioned")
	}
	return errors.Trace(err)
}

func (c *controllerStack) addCleanUp(cleanUp func()) {
	c.cleanUps = append(c.cleanUps, cleanUp)
}

func (c *controllerStack) createDockerSecret(ctx context.Context) (string, error) {
	if len(c.dockerAuthSecretData) == 0 {
		return "", errors.NotValidf("empty docker secret data")
	}
	name := c.resourceNamedockerSecret
	logger.Debugf(context.TODO(), "ensuring docker secret %q", name)
	cleanUp, err := c.broker.ensureOCIImageSecret(
		ctx, name, c.stackLabels, c.dockerAuthSecretData, c.stackAnnotations,
	)
	c.addCleanUp(func() {
		logger.Debugf(context.TODO(), "deleting %q", name)
		cleanUp()
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return name, nil
}

func (c *controllerStack) patchServiceAccountForImagePullSecret(ctx context.Context, saName string) error {
	if !c.isPrivateRepo() {
		return nil
	}
	dockerSecretName, err := c.createDockerSecret(ctx)
	if err != nil {
		return errors.Annotate(err, "creating docker secret for controller")
	}
	sa, err := c.broker.getServiceAccount(ctx, saName)
	if err != nil {
		return errors.Trace(err)
	}
	sa.ImagePullSecrets = append(
		sa.ImagePullSecrets,
		core.LocalObjectReference{Name: dockerSecretName},
	)
	_, err = c.broker.updateServiceAccount(ctx, sa)
	return errors.Trace(err)
}

func (c *controllerStack) createControllerSecretServerPem(ctx context.Context) error {
	si, ok := c.agentConfig.StateServingInfo()
	if !ok || si.CAPrivateKey == "" {
		// No certificate information exists yet, nothing to do.
		return errors.NewNotValid(nil, "certificate is empty")
	}

	secret, err := c.getControllerSecret(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	secret.Data[mongo.FileNameDBSSLKey] = []byte(mongo.GenerateSSLKey(si.Cert, si.PrivateKey))

	logger.Tracef(context.TODO(), "ensuring server.pem secret: \n%+v", secret)
	c.addCleanUp(func() {
		logger.Debugf(context.TODO(), "deleting %q server.pem", secret.Name)
		_ = c.broker.deleteSecret(ctx, secret.GetName(), secret.GetUID())
	})
	return c.broker.updateSecret(ctx, secret)
}

func (c *controllerStack) ensureControllerConfigmapBootstrapParams(ctx context.Context) error {
	bootstrapParamsFileContent, err := c.pcfg.Bootstrap.StateInitializationParams.Marshal()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Tracef(context.TODO(), "bootstrapParams file content: \n%s", string(bootstrapParamsFileContent))

	cm, err := c.getControllerConfigMap(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	cm.Data[cloudconfig.FileNameBootstrapParams] = string(bootstrapParamsFileContent)

	logger.Tracef(context.TODO(), "creating bootstrap-params configmap: \n%+v", cm)

	cleanUp, err := c.broker.ensureConfigMap(ctx, cm)
	c.addCleanUp(func() {
		logger.Debugf(context.TODO(), "deleting %q bootstrap-params", cm.Name)
		cleanUp()
	})
	return errors.Trace(err)
}

func (c *controllerStack) ensureControllerConfigmapAgentConf(ctx context.Context) error {
	agentConfigFileContent, err := c.agentConfig.Render()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Tracef(context.TODO(), "controller agentConfig file content: \n%s", string(agentConfigFileContent))

	unitAgentConfigFileContent, err := c.unitAgentConfig.Render()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Tracef(context.TODO(), "controller unit agentConfig file content: \n%s", string(unitAgentConfigFileContent))

	cm, err := c.getControllerConfigMap(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	cm.Data[constants.ControllerAgentConfigFilename] = string(agentConfigFileContent)
	cm.Data[constants.ControllerUnitAgentConfigFilename] = string(unitAgentConfigFileContent)

	logger.Tracef(context.TODO(), "ensuring agent.conf configmap: \n%+v", cm)
	cleanUp, err := c.broker.ensureConfigMap(ctx, cm)
	c.addCleanUp(func() {
		logger.Debugf(context.TODO(), "deleting %q template-agent.conf", cm.Name)
		cleanUp()
	})
	return errors.Trace(err)
}

func (c *controllerStack) ensureControllerApplicationSecret(ctx context.Context) error {
	controllerUnitPassword := c.unitAgentConfig.OldPassword()
	apiInfo, ok := c.unitAgentConfig.APIInfo()
	if ok {
		controllerUnitPassword = apiInfo.Password
	}

	secret := &core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        c.appSecretName(),
			Namespace:   c.broker.namespace,
			Labels:      c.stackLabels,
			Annotations: c.stackAnnotations,
		},
		Type: core.SecretTypeOpaque,
		Data: map[string][]byte{
			constants.EnvJujuK8sUnitPassword: []byte(controllerUnitPassword),
		},
	}
	cleanUp, err := c.broker.ensureSecret(ctx, secret)
	c.addCleanUp(func() {
		logger.Debugf(context.TODO(), "deleting %q secret", c.appSecretName())
		cleanUp()
	})
	return errors.Trace(err)
}

// ensureControllerServiceAccount is responsible for making sure the in cluster
// service account for the controller exists and is upto date. Returns the name
// of the service account create, cleanup functions and any errors.
func ensureControllerServiceAccount(
	ctx context.Context,
	client kubernetes.Interface,
	namespace string,
	controllerUUID string,
	labels map[string]string,
	annotations map[string]string,
) (string, []func(), error) {
	sa := resources.NewServiceAccount(environsbootstrap.ControllerApplicationName, namespace, &core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Labels: providerutils.LabelsMerge(
				labels,
				providerutils.LabelsJujuModelOperatorDisableWebhook,
			),
			Annotations: annotations,
		},
		AutomountServiceAccountToken: boolPtr(true),
	})

	cleanUps, err := sa.Ensure(ctx, client)
	if err != nil {
		return sa.Name, cleanUps, errors.Trace(err)
	}

	// name cluster role binding after the controller namespace.
	clusterRoleBindingName := namespace
	crb := resources.NewClusterRoleBinding(clusterRoleBindingName, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleBindingName,
			Labels: providerutils.LabelsForModel(environsbootstrap.ControllerModelName, "",
				controllerUUID, constants.LastLabelVersion),
			Annotations: annotations,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      environsbootstrap.ControllerApplicationName,
			Namespace: namespace,
		}},
	})

	crbCleanUps, err := crb.Ensure(ctx, client)
	cleanUps = append(cleanUps, crbCleanUps...)
	return sa.Name, cleanUps, errors.Trace(err)
}

func (c *controllerStack) createControllerStatefulset(ctx context.Context) error {
	numberOfPods := int32(1) // TODO(caas): HA mode!
	controllerStatefulSet := &apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: c.stackName,
			Labels: providerutils.LabelsMerge(
				c.stackLabels,
				providerutils.LabelsJujuModelOperatorDisableWebhook,
			),
			Namespace:   c.broker.Namespace(),
			Annotations: c.stackAnnotations,
		},
		Spec: apps.StatefulSetSpec{
			ServiceName: c.resourceNameService,
			Replicas:    &numberOfPods,
			Selector: &metav1.LabelSelector{
				MatchLabels: c.selectorLabels,
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: providerutils.LabelsMerge(
						c.selectorLabels,
						providerutils.LabelsJujuModelOperatorDisableWebhook,
					),
					Name:        c.pcfg.GetPodName(), // This really should not be set.
					Namespace:   c.broker.Namespace(),
					Annotations: c.stackAnnotations,
				},
			},
		},
	}

	controllerSpec, err := c.buildContainerSpecForController()
	if err != nil {
		return errors.Trace(err)
	}
	controllerStatefulSet.Spec.Template.Spec = *controllerSpec
	if err := c.buildStorageSpecForController(ctx, controllerStatefulSet); err != nil {
		return errors.Trace(err)
	}

	logger.Tracef(context.TODO(), "creating controller statefulset: \n%+v", controllerStatefulSet)
	c.addCleanUp(func() {
		logger.Debugf(context.TODO(), "deleting %q statefulset", controllerStatefulSet.Name)
		_ = c.broker.deleteStatefulSet(ctx, controllerStatefulSet.Name)
	})
	w, err := c.broker.WatchUnits(c.stackName)
	if err != nil {
		return errors.Trace(err)
	}
	defer w.Kill()

	if _, err = c.broker.createStatefulSet(ctx, controllerStatefulSet); err != nil {
		return errors.Trace(err)
	}

	for i := int32(0); i < numberOfPods; i++ {
		podName := c.pcfg.GetPodName() // TODO(caas): HA mode!
		if err = c.waitForPod(ctx, w, podName); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (c *controllerStack) waitForPod(ctx context.Context, podWatcher watcher.NotifyWatcher, podName string) error {
	timeout := c.broker.clock.NewTimer(c.timeout)

	podEventWatcher, err := c.broker.watchEvents(podName, "Pod")
	if err != nil {
		return errors.Trace(err)
	}
	defer podEventWatcher.Kill()

	printedMsg := set.NewStrings()
	printPodEvents := func() error {
		events, err := c.broker.getEvents(ctx, podName, "Pod")
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
				logger.Debugf(context.TODO(), evt.Message)
				if evt.Reason == PullingImage {
					c.logger.Infof(evt.Message)
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
				pvc, err := c.broker.getPVC(ctx, pvcSource.ClaimName)
				if err != nil {
					return errors.Annotatef(err, "failed to get pvc %s", pvcSource.ClaimName)
				}
				if pvc.Status.Phase == core.ClaimPending {
					events, err := c.broker.getEvents(ctx, pvc.Name, "PersistentVolumeClaim")
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

	pendingReason := func(ctx context.Context) error {
		pod, err := c.broker.getPod(ctx, podName)
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

	checkStatus := func(ctx context.Context, pod *core.Pod) (bool, error) {
		switch pod.Status.Phase {
		case core.PodRunning:
			return true, nil
		case core.PodFailed:
			return false, errors.Annotate(pendingReason(ctx), "controller pod failed")
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
			pod, err := c.broker.getPod(ctx, podName)
			if errors.Is(err, errors.NotFound) {
				logger.Debugf(context.TODO(), "pod %q is not provisioned yet", podName)
				continue
			}
			if err != nil {
				return errors.Annotate(err, "fetching pods' status for controller")
			}
			done, err := checkStatus(ctx, pod)
			if err != nil {
				return errors.Trace(err)
			}
			if done {
				c.logger.Infof("Starting controller pod")
				return nil
			}
		case <-podEventWatcher.Changes():
			_ = printPodEvents()
		case <-timeout.Chan():
			err := pendingReason(ctx)
			if err != nil {
				return errors.Annotatef(err, "timed out waiting for controller pod")
			}
			return errors.Timeoutf("timed out waiting for controller pod")
		}
	}
}

func (c *controllerStack) buildStorageSpecForController(ctx context.Context, statefulset *apps.StatefulSet) error {
	sc, err := c.broker.getStorageClass(ctx, c.storageClass)
	if err != nil {
		return errors.Trace(err)
	}
	// try to find <namespace>-<c.storageClass>,
	// if it's not found, then fallback to c.storageClass.
	c.storageClass = sc.GetName()

	// build persistent volume claim.
	statefulset.Spec.VolumeClaimTemplates = append(statefulset.Spec.VolumeClaimTemplates, core.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        storageName,
			Labels:      c.stackLabels,
			Annotations: c.stackAnnotations,
		},
		Spec: core.PersistentVolumeClaimSpec{
			StorageClassName: &c.storageClass,
			AccessModes:      []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
			Resources: core.VolumeResourceRequirements{
				Requests: core.ResourceList{
					core.ResourceStorage: c.storageSize,
				},
			},
		},
	})

	vols := []core.Volume{{
		Name: mongoScratchStorageName,
		VolumeSource: core.VolumeSource{
			EmptyDir: &core.EmptyDirVolumeSource{},
		},
	}, {
		Name: apiServerScratchStorageName,
		VolumeSource: core.VolumeSource{
			EmptyDir: &core.EmptyDirVolumeSource{},
		},
	}, {
		// add volume server.pem secret.
		Name: c.resourceNameVolSSLKey,
		VolumeSource: core.VolumeSource{
			Secret: &core.SecretVolumeSource{
				SecretName:  c.resourceNameSecret,
				DefaultMode: pointer.Int32(0400),
				Items: []core.KeyToPath{
					{
						Key:  mongo.FileNameDBSSLKey,
						Path: TemplateFileNameServerPEM,
					},
				},
			},
		},
	}, {
		// add volume shared secret.
		Name: c.resourceNameVolSharedSecret,
		VolumeSource: core.VolumeSource{
			Secret: &core.SecretVolumeSource{
				SecretName:  c.resourceNameSecret,
				DefaultMode: pointer.Int32(0660),
				Items: []core.KeyToPath{
					{
						Key:  mongo.SharedSecretFile,
						Path: mongo.SharedSecretFile,
					},
				},
			},
		},
	}, {
		// add volume agent.conf configmap.
		Name: c.resourceNameVolAgentConf,
		VolumeSource: core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				LocalObjectReference: core.LocalObjectReference{
					Name: c.resourceNameConfigMap,
				},
				Items: []core.KeyToPath{
					{
						Key:  constants.ControllerAgentConfigFilename,
						Path: constants.ControllerAgentConfigFilename,
					}, {
						Key:  constants.ControllerUnitAgentConfigFilename,
						Path: constants.ControllerUnitAgentConfigFilename,
					},
				},
			},
		},
	}, {
		// add volume bootstrap-params configmap.
		Name: c.resourceNameVolBootstrapParams,
		VolumeSource: core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				LocalObjectReference: core.LocalObjectReference{
					Name: c.resourceNameConfigMap,
				},
				Items: []core.KeyToPath{
					{
						Key:  cloudconfig.FileNameBootstrapParams,
						Path: cloudconfig.FileNameBootstrapParams,
					},
				},
			},
		},
	}}

	statefulset.Spec.Template.Spec.Volumes = append(statefulset.Spec.Template.Spec.Volumes, vols...)
	return nil
}

func (c *controllerStack) appSecretName() string {
	return c.stackName + "-application-config"
}

func (c *controllerStack) controllerContainers(setupCmd, machineCmd, controllerImage string, jujudEnv map[string]string) ([]core.Container, error) {
	var containerSpec []core.Container
	// add container mongoDB.
	// TODO(bootstrap): refactor mongo package to make it usable for IAAS and CAAS,
	// then generate mongo config from EnsureServerParams.
	tlsPrivateKeyPath := c.pathJoin(c.pcfg.DataDir, mongo.FileNameDBSSLKey)
	probeCmds := &core.ExecAction{
		Command: []string{
			"mongo",
			fmt.Sprintf("--port=%d", c.portMongoDB),
			"--eval",
			"db.adminCommand('ping')",
		},
	}
	args := []string{
		fmt.Sprintf("--dbpath=%s", c.pathJoin(c.pcfg.DataDir, "db")),
		fmt.Sprintf("--port=%d", c.portMongoDB),
		"--journal",
		fmt.Sprintf("--replSet=%s", mongo.ReplicaSetName),
		"--quiet",
		"--oplogSize=1024",
		"--noauth",
		"--storageEngine=wiredTiger",
		"--bind_ip_all",
	}

	// Create the script used to start mongo.
	const mongoSh = "/tmp/mongo.sh"
	mongoStartup := fmt.Sprintf(caas.MongoStartupShTemplate, strings.Join(args, " "),
		c.pathJoin(c.pcfg.DataDir, mongo.SharedSecretFile+".temp"),
		c.pathJoin(c.pcfg.DataDir, mongo.SharedSecretFile),
		tlsPrivateKeyPath)
	// Write it to a file so it can be executed.
	mongoStartup = strings.ReplaceAll(mongoStartup, "\n", "\\n")
	makeMongoCmd := fmt.Sprintf("printf '%s'>%s", mongoStartup, mongoSh)
	mongoArgs := fmt.Sprintf("%[1]s && chmod a+x %[2]s && exec %[2]s", makeMongoCmd, mongoSh)
	logger.Debugf(context.TODO(), "mongodb container args:\n%s", mongoArgs)

	dbImage, err := c.pcfg.GetJujuDbOCIImagePath()
	if err != nil {
		return nil, errors.Trace(err)
	}
	containerSpec = append(containerSpec, core.Container{
		Name:            mongoDBContainerName,
		ImagePullPolicy: core.PullIfNotPresent,
		Image:           dbImage,
		Command: []string{
			"/bin/sh",
		},
		Args: []string{
			"-c",
			mongoArgs,
		},
		SecurityContext: &core.SecurityContext{
			RunAsUser:              pointer.Int64(constants.JujuUserID),
			RunAsGroup:             pointer.Int64(constants.JujuGroupID),
			ReadOnlyRootFilesystem: pointer.Bool(true),
		},
		Ports: []core.ContainerPort{
			{
				Name:          "mongodb",
				ContainerPort: int32(c.portMongoDB),
				Protocol:      "TCP",
			},
		},
		ReadinessProbe: &core.Probe{
			ProbeHandler: core.ProbeHandler{
				Exec: probeCmds,
			},
			FailureThreshold:    3,
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
			SuccessThreshold:    1,
			TimeoutSeconds:      1,
		},
		LivenessProbe: &core.Probe{
			ProbeHandler: core.ProbeHandler{
				Exec: probeCmds,
			},
			FailureThreshold:    3,
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
			SuccessThreshold:    1,
			TimeoutSeconds:      5,
		},
		StartupProbe: &core.Probe{
			ProbeHandler: core.ProbeHandler{
				Exec: probeCmds,
			},
			FailureThreshold:    mongoDBStartupProbeFailure,
			InitialDelaySeconds: mongoDBStartupProbeInitialDelay,
			PeriodSeconds:       mongoDBStartupProbePeriod,
			SuccessThreshold:    mongoDBStartupProbeSuccess,
			TimeoutSeconds:      mongoDBStartupProbeTimeout,
		},
		VolumeMounts: []core.VolumeMount{
			{
				Name:      mongoScratchStorageName,
				MountPath: "/var/log",
				SubPath:   "var/log",
			},
			{
				Name:      mongoScratchStorageName,
				MountPath: "/tmp",
				SubPath:   "tmp",
			},
			{
				Name:      storageName,
				MountPath: c.pcfg.DataDir,
			},
			{
				Name:      storageName,
				MountPath: c.pathJoin(c.pcfg.DataDir, "db"),
				SubPath:   "db",
			},
			{
				Name:      c.resourceNameVolSSLKey,
				MountPath: c.pathJoin(c.pcfg.DataDir, TemplateFileNameServerPEM),
				SubPath:   TemplateFileNameServerPEM,
				ReadOnly:  true,
			},
			{
				Name:      c.resourceNameVolSharedSecret,
				MountPath: c.pathJoin(c.pcfg.DataDir, mongo.SharedSecretFile+".temp"),
				SubPath:   mongo.SharedSecretFile,
				ReadOnly:  true,
			},
		},
	})

	// add container API server.
	pebbleLayer, err := jujudPebbleLayer(machineCmd, jujudEnv)
	if err != nil {
		return nil, errors.Annotate(err, "writing jujud pebble layer")
	}
	apiContainer := core.Container{
		Name:            apiServerContainerName,
		ImagePullPolicy: core.PullIfNotPresent,
		Image:           controllerImage,
		Command: []string{
			"/bin/sh",
		},
		Args: []string{
			"-c",
			fmt.Sprintf(
				caas.APIServerStartUpSh,
				c.pcfg.DataDir,
				setupCmd,
				pebbleLayer,
				pebble.ApiServerHealthCheckPort,
			),
		},
		WorkingDir: c.pcfg.DataDir,
		EnvFrom: []core.EnvFromSource{{
			SecretRef: &core.SecretEnvSource{
				LocalObjectReference: core.LocalObjectReference{
					Name: c.appSecretName(),
				},
			},
		}},
		Env: []core.EnvVar{{
			Name:  "JUJU_CONTAINER_NAME",
			Value: apiServerContainerName,
		}, {
			Name:  "PEBBLE_SOCKET",
			Value: "/charm/container/pebble.socket",
		}},
		StartupProbe: &core.Probe{
			ProbeHandler:        pebble.StartupHandler(pebble.ApiServerHealthCheckPort),
			InitialDelaySeconds: apiServerStartupProbeInitialDelay,
			TimeoutSeconds:      apiServerStartupProbeTimeout,
			PeriodSeconds:       apiServerStartupProbePeriod,
			SuccessThreshold:    apiServerStartupProbeSuccess,
			FailureThreshold:    apiServerStartupProbeFailure,
		},
		LivenessProbe: &core.Probe{
			ProbeHandler:        pebble.LivenessHandler(pebble.ApiServerHealthCheckPort),
			InitialDelaySeconds: apiServerLivenessProbeInitialDelay,
			TimeoutSeconds:      apiServerLivenessProbeTimeout,
			PeriodSeconds:       apiServerLivenessProbePeriod,
			SuccessThreshold:    apiServerLivenessProbeSuccess,
			FailureThreshold:    apiServerLivenessProbeFailure,
		},
		ReadinessProbe: &core.Probe{
			ProbeHandler:        pebble.ReadinessHandler(pebble.ApiServerHealthCheckPort),
			InitialDelaySeconds: apiServerLivenessProbeInitialDelay,
			TimeoutSeconds:      apiServerLivenessProbeTimeout,
			PeriodSeconds:       apiServerLivenessProbePeriod,
			SuccessThreshold:    apiServerLivenessProbeSuccess,
			FailureThreshold:    apiServerLivenessProbeFailure,
		},
		SecurityContext: &core.SecurityContext{
			RunAsUser:              pointer.Int64(constants.JujuUserID),
			RunAsGroup:             pointer.Int64(constants.JujuGroupID),
			ReadOnlyRootFilesystem: pointer.Bool(true),
		},
		VolumeMounts: []core.VolumeMount{
			{
				Name:      apiServerScratchStorageName,
				MountPath: "/tmp",
				SubPath:   "tmp",
			},
			{
				Name:      apiServerScratchStorageName,
				MountPath: "/var/lib/pebble",
				SubPath:   "var/lib/pebble",
			},
			{
				Name:      apiServerScratchStorageName,
				MountPath: "/var/log/juju",
				SubPath:   "var/log/juju",
			},
			{
				Name:      storageName,
				MountPath: c.pcfg.DataDir,
			},
			{
				Name: storageName,
				MountPath: c.pathJoin(
					c.pcfg.DataDir,
					"agents",
					"controller-"+c.pcfg.ControllerId,
				),
				SubPath: c.pathJoin("agents",
					"controller-"+c.pcfg.ControllerId,
				),
			},
			{
				Name: c.resourceNameVolAgentConf,
				MountPath: c.pathJoin(
					c.pcfg.DataDir,
					"agents",
					"controller-"+c.pcfg.ControllerId,
					constants.TemplateFileNameAgentConf,
				),
				SubPath:  constants.ControllerAgentConfigFilename,
				ReadOnly: true,
			},
			{
				Name:      c.resourceNameVolSSLKey,
				MountPath: c.pathJoin(c.pcfg.DataDir, TemplateFileNameServerPEM),
				SubPath:   TemplateFileNameServerPEM,
				ReadOnly:  true,
			},
			{
				Name:      c.resourceNameVolSharedSecret,
				MountPath: c.pathJoin(c.pcfg.DataDir, mongo.SharedSecretFile),
				SubPath:   mongo.SharedSecretFile,
				ReadOnly:  true,
			},
			{
				Name:      c.resourceNameVolBootstrapParams,
				MountPath: c.pathJoin(c.pcfg.DataDir, cloudconfig.FileNameBootstrapParams),
				SubPath:   cloudconfig.FileNameBootstrapParams,
				ReadOnly:  true,
			},
			{
				Name:      constants.CharmVolumeName,
				MountPath: "/charm/container",
				SubPath:   fmt.Sprintf("charm/containers/%s", apiServerContainerName),
			},
			{
				Name:      constants.CharmVolumeName,
				MountPath: "/etc/profile.d/juju-introspection.sh",
				SubPath:   "containeragent/etc/profile.d/juju-introspection.sh",
				ReadOnly:  true,
			},
			{
				Name:      constants.CharmVolumeName,
				MountPath: paths.JujuIntrospect(paths.OSUnixLike),
				SubPath:   "charm/bin/containeragent",
				ReadOnly:  true,
			},
			{
				Name:      constants.CharmVolumeName,
				MountPath: paths.JujuExec(paths.OSUnixLike),
				SubPath:   "charm/bin/containeragent",
				ReadOnly:  true,
			},
			{
				Name:      constants.CharmVolumeName,
				MountPath: paths.JujuDumpLogs(paths.OSUnixLike),
				SubPath:   "charm/bin/containeragent",
				ReadOnly:  true,
			},
		},
	}
	if features := featureflag.AsEnvironmentValue(); features != "" {
		apiContainer.Env = []core.EnvVar{{
			Name:  osenv.JujuFeatureFlagEnvKey,
			Value: features,
		}}
	}

	containerSpec = append(containerSpec, apiContainer)
	return containerSpec, nil
}

// jujudPebbleLayer returns the Pebble layer yaml for running the jujud
// service. This will be written to a file in the Pebble layers directory.
func jujudPebbleLayer(machineCmd string, env map[string]string) ([]byte, error) {
	layer := plan.Layer{
		Summary: "jujud service",
		Services: map[string]*plan.Service{
			"jujud": {
				Override: plan.ReplaceOverride,
				Summary:  "Juju controller agent",
				Command:  machineCmd,
				Startup:  plan.StartupEnabled,
			},
		},
	}
	if env != nil {
		layer.Services["jujud"].Environment = env
	}

	return yaml.Marshal(layer)
}

func (c *controllerStack) buildContainerSpecForController() (*core.PodSpec, error) {
	loggingOption := "--show-log"
	if loggo.GetLogger("").LogLevel() == loggo.DEBUG {
		// If the bootstrap command was requested with --debug, then the root
		// logger will be set to DEBUG. If it is, then we use --debug here too.
		loggingOption = "--debug"
	}

	agentConfigRelativePath := c.pathJoin(
		"agents",
		fmt.Sprintf("controller-%s", c.pcfg.ControllerId),
		agentconstants.AgentConfigFilename,
	)

	var jujudEnv map[string]string = nil
	featureFlags := featureflag.AsEnvironmentValue()
	if featureFlags != "" {
		jujudEnv = map[string]string{osenv.JujuFeatureFlagEnvKey: featureFlags}
	}

	setupCmd := ""
	if c.pcfg.ControllerId == agent.BootstrapControllerId {
		// only do bootstrap-state on the bootstrap controller - controller-0.
		bootstrapStateCmd := fmt.Sprintf(
			"%s bootstrap-state --data-dir $JUJU_DATA_DIR %s --timeout %s",
			c.pathJoin("$JUJU_TOOLS_DIR", "jujud"),
			loggingOption,
			c.timeout.String(),
		)
		if featureFlags != "" {
			bootstrapStateCmd = fmt.Sprintf("%s=%s %s", osenv.JujuFeatureFlagEnvKey, featureFlags, bootstrapStateCmd)
		}
		setupCmd = fmt.Sprintf(
			"test -e %s || %s",
			c.pathJoin("$JUJU_DATA_DIR", agentConfigRelativePath),
			bootstrapStateCmd,
		)
	}

	machineCmd := fmt.Sprintf(
		"%s machine --data-dir $JUJU_DATA_DIR --controller-id %s --log-to-stderr %s",
		c.pathJoin("$JUJU_TOOLS_DIR", "jujud"),
		c.pcfg.ControllerId,
		loggingOption,
	)

	return c.buildContainerSpecForCommands(setupCmd, machineCmd, jujudEnv)
}

func (c *controllerStack) buildContainerSpecForCommands(setupCmd, machineCmd string, jujudEnv map[string]string) (*core.PodSpec, error) {
	controllerImage, err := c.pcfg.GetControllerImagePath()
	if err != nil {
		return nil, errors.Trace(err)
	}

	containers, err := c.controllerContainers(setupCmd, machineCmd, controllerImage, jujudEnv)
	if err != nil {
		return nil, errors.Trace(err)
	}

	controllerApp := application.NewApplication(
		environsbootstrap.ControllerApplicationName,
		c.broker.namespace,
		c.broker.ModelUUID(),
		environsbootstrap.ControllerModelName,
		constants.LastLabelVersion,
		caas.DeploymentStateful,
		c.broker.client(),
		c.broker.newWatcher,
		c.broker.clock,
		c.broker.randomPrefix,
	)

	defaultBase := version.DefaultSupportedLTSBase()
	repo, err := docker.NewImageRepoDetails(c.pcfg.Controller.CAASImageRepo())
	if err != nil {
		return nil, errors.Annotatef(err, "parsing %s", controller.CAASImageRepo)
	}
	charmBaseImage, err := podcfg.ImageForBase(repo.Repository, charm.Base{
		Name: strings.ToLower(defaultBase.OS),
		Channel: charm.Channel{
			Track: defaultBase.Channel.Track,
			Risk:  charm.Stable,
		},
	})
	if err != nil {
		return nil, errors.Annotate(err, "getting image for base")
	}

	cfg := caas.ApplicationConfig{
		AgentVersion:         c.pcfg.JujuVersion,
		AgentImagePath:       controllerImage,
		CharmBaseImagePath:   charmBaseImage,
		IsPrivateImageRepo:   repo.IsPrivate(),
		CharmModifiedVersion: 0,
		InitialScale:         1,
		Constraints:          c.pcfg.Bootstrap.BootstrapMachineConstraints,
		ExistingContainers:   []string{apiServerContainerName},
		// TODO(wallyworld) - use storage so the volumes don't need to be manually set up
		//Filesystems: nil,
		CharmUser: caas.RunAsNonRoot,
	}
	spec, err := controllerApp.ApplicationPodSpec(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "creating controller pod spec")
	}
	spec.Containers = append(spec.Containers, containers...)

	agentConfigMount := core.VolumeMount{
		Name: c.resourceNameVolAgentConf,
		MountPath: c.pathJoin(
			c.pcfg.DataDir,
			constants.TemplateFileNameAgentConf,
		),
		SubPath: constants.ControllerUnitAgentConfigFilename,
	}
	dataDirMount := core.VolumeMount{
		Name:      storageName,
		MountPath: c.pcfg.DataDir,
	}

	for i, ct := range spec.InitContainers {
		if ct.Name != constants.ApplicationInitContainer {
			continue
		}
		ct.VolumeMounts = append(ct.VolumeMounts, agentConfigMount)
		ct.Args = append(ct.Args, "--controller")
		spec.InitContainers[i] = ct
	}
	for i, ct := range spec.Containers {
		// Modify "charm" container spec
		if ct.Name != constants.ApplicationCharmContainer {
			continue
		}

		// Replace the /var/lib/juju mount
		for j, mount := range ct.VolumeMounts {
			if mount.MountPath == c.pcfg.DataDir {
				ct.VolumeMounts = append(ct.VolumeMounts[:j], ct.VolumeMounts[j+1:]...)
				break
			}
		}
		ct.VolumeMounts = append(ct.VolumeMounts, agentConfigMount, dataDirMount)

		// Remove probes to prevent controller death.
		ct.LivenessProbe = nil
		ct.ReadinessProbe = nil
		ct.StartupProbe = nil
		for j, env := range ct.Env {
			if env.Name == constants.EnvAgentHTTPProbePort {
				ct.Env = append(ct.Env[:j], ct.Env[j+1:]...)
				break
			}
		}
		spec.Containers[i] = ct
	}
	return spec, nil
}
