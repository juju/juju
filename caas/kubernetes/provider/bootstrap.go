// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/retry"
	"gopkg.in/juju/names.v2"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/environs"
	environsbootstrap "github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/mongo"
)

const (
	// JujuControllerStackName is the juju CAAS controller stack name.
	JujuControllerStackName = "controller"
)

var (
	// TemplateFileNameServerPEM is the template server.pem file name.
	TemplateFileNameServerPEM = "template-" + mongo.FileNameDBSSLKey
	// TemplateFileNameAgentConf is the template agent.conf file name.
	TemplateFileNameAgentConf = "template-" + agent.AgentConfigFilename
)

type controllerStack struct {
	ctx environs.BootstrapContext

	stackName   string
	stackLabels map[string]string
	broker      *kubernetesClient

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

	cleanUps []func()
}

type controllerStacker interface {
	// Deploy creates all resources for controller stack.
	Deploy() error
}

func controllerCorelation(broker *kubernetesClient) (*kubernetesClient, error) {
	if broker.Config().Name() != environsbootstrap.ControllerModelName {
		return broker, nil
	}
	// ensure controller specific annotations.
	_ = broker.addAnnotations(annotationControllerIsControllerKey, "true")

	ns, err := broker.listNamespacesByAnnotations(broker.GetAnnotations())
	if errors.IsNotFound(err) || ns == nil {
		// No existing controller found on the cluster.
		// A controller must be bootstrapping now.
		// It will reply on setControllerNamespace in controller stack to set namespace name.
		return broker, nil
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	// This is an existing controller.
	// Now, link it back.
	// It should be always one record here based on current annotation design.
	broker.SetNamespace(ns[0].GetName())
	return broker, nil
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
	// TODO(caas): parse from constrains?
	storageSizeControllerRaw := "20Gi"
	storageSize, err := resource.ParseQuantity(storageSizeControllerRaw)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// TODO(bootstrap): we'll need a different tag type other than machine tag.
	var agentConfig agent.ConfigSetterWriter
	agentConfig, err = pcfg.AgentConfig(names.NewMachineTag(pcfg.MachineId))
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

	cs := controllerStack{
		ctx:         ctx,
		stackName:   stackName,
		stackLabels: map[string]string{labelApplication: stackName},
		broker:      broker,

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
		fileNameAgentConfMount:  TemplateFileNameAgentConf,

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

func (c controllerStack) getResourceName(name string) string {
	return c.stackName + "-" + strings.Replace(name, ".", "-", -1)
}

func (c controllerStack) getControllerSecret() (secret *core.Secret, err error) {
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
		err = c.broker.createSecret(&core.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:      c.resourceNameSecret,
				Labels:    c.stackLabels,
				Namespace: c.broker.GetCurrentNamespace(),
			},
			Type: core.SecretTypeOpaque,
		})
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.broker.getSecret(c.resourceNameSecret)
}

func (c controllerStack) getControllerConfigMap() (cm *core.ConfigMap, err error) {
	defer func() {
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
	}()

	cm, err = c.broker.getConfigMap(c.resourceNameConfigMap)
	if err == nil {
		return cm, nil
	}
	if errors.IsNotFound(err) {
		err = c.broker.createConfigMap(&core.ConfigMap{
			ObjectMeta: v1.ObjectMeta{
				Name:      c.resourceNameConfigMap,
				Labels:    c.stackLabels,
				Namespace: c.broker.GetCurrentNamespace(),
			},
		})
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.broker.getConfigMap(c.resourceNameConfigMap)
}

func (c controllerStack) doCleanUp() {
	logger.Debugf("bootstrap failed, removing %d resources.", len(c.cleanUps))
	for _, f := range c.cleanUps {
		f()
	}
}

// Deploy creates all resources for controller stack.
func (c controllerStack) Deploy() (err error) {
	// creating namespace for controller stack, this namespace will be removed by broker.DestroyController if bootstrap failed.
	nsName := c.broker.GetCurrentNamespace()
	c.ctx.Infof("Creating k8s resources for controller %q", nsName)
	if err = c.broker.createNamespace(nsName); err != nil {
		return errors.Annotate(err, "creating namespace for controller stack")
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

	// create shared-secret secret for controller pod.
	if err = c.createControllerSecretSharedSecret(); err != nil {
		return errors.Annotate(err, "creating shared-secret secret for controller")
	}

	// create server.pem secret for controller pod.
	if err = c.createControllerSecretServerPem(); err != nil {
		return errors.Annotate(err, "creating server.pem secret for controller")
	}

	// create mongo admin account secret for controller pod.
	if err = c.createControllerSecretMongoAdmin(); err != nil {
		return errors.Annotate(err, "creating mongo admin account secret for controller")
	}

	// create bootstrap-params configmap for controller pod.
	if err = c.ensureControllerConfigmapBootstrapParams(); err != nil {
		return errors.Annotate(err, "creating bootstrap-params configmap for controller")
	}

	// Note: create agent config configmap for controller pod lastly because agentConfig has been updated in previous steps.
	if err = c.ensureControllerConfigmapAgentConf(); err != nil {
		return errors.Annotate(err, "creating agent config configmap for controller")
	}

	// create statefulset to ensure controller stack.
	if err = c.createControllerStatefulset(); err != nil {
		return errors.Annotate(err, "creating statefulset for controller")
	}
	// fake message for better user experience.
	c.ctx.Infof("Downloading Juju agent OCI image")

	return nil
}

func (c controllerStack) createControllerService() error {
	svcName := c.resourceNameService
	spec := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      svcName,
			Labels:    c.stackLabels,
			Namespace: c.broker.GetCurrentNamespace(),
		},
		Spec: core.ServiceSpec{
			Selector: c.stackLabels,
			Type:     core.ServiceType(c.pcfg.Bootstrap.ControllerServiceType),
			Ports: []core.ServicePort{
				{
					Name:       "api-server",
					TargetPort: intstr.FromInt(c.portAPIServer),
					Port:       int32(c.portAPIServer),
				},
			},
		},
	}

	logger.Debugf("creating controller service: \n%+v", spec)
	if err := c.broker.ensureK8sService(spec); err != nil {
		return errors.Trace(err)
	}

	c.addCleanUp(func() {
		logger.Debugf("deleting %q", svcName)
		c.broker.deleteService(svcName)
	})

	publicAddressPoller := func() error {
		// get the service by app name;
		svc, err := c.broker.GetService(c.stackName, false)
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
			logger.Debugf("polling k8s controller svc DNS, in %d attempt, got error %v", attempt, err)
		},
	}
	return errors.Trace(retry.Call(retryCallArgs))
}

func (c controllerStack) addCleanUp(cleanUp func()) {
	c.cleanUps = append(c.cleanUps, cleanUp)
}

func (c controllerStack) createControllerSecretSharedSecret() error {
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
		c.broker.deleteSecret(secret.Name)
	})
	return c.broker.updateSecret(secret)
}

func (c controllerStack) createControllerSecretServerPem() error {
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
		c.broker.deleteSecret(secret.Name)
	})
	return c.broker.updateSecret(secret)
}

func (c controllerStack) createControllerSecretMongoAdmin() error {
	return nil
}

func (c controllerStack) ensureControllerConfigmapBootstrapParams() error {
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
	c.addCleanUp(func() {
		logger.Debugf("deleting %q bootstrap-params", cm.Name)
		c.broker.deleteConfigMap(cm.Name)
	})
	return c.broker.ensureConfigMap(cm)
}

func (c controllerStack) ensureControllerConfigmapAgentConf() error {
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
	c.addCleanUp(func() {
		logger.Debugf("deleting %q template-agent.conf", cm.Name)
		c.broker.deleteConfigMap(cm.Name)
	})
	return c.broker.ensureConfigMap(cm)
}

func (c controllerStack) createControllerStatefulset() error {
	numberOfPods := int32(1) // TODO: HA mode!
	spec := &apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      c.resourceNameStatefulSet,
			Labels:    c.stackLabels,
			Namespace: c.broker.GetCurrentNamespace(),
		},
		Spec: apps.StatefulSetSpec{
			ServiceName: c.resourceNameService,
			Replicas:    &numberOfPods,
			Selector: &v1.LabelSelector{
				MatchLabels: c.stackLabels,
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels:    c.stackLabels,
					Namespace: c.broker.GetCurrentNamespace(),
				},
				Spec: core.PodSpec{
					RestartPolicy: core.RestartPolicyAlways,
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

	logger.Debugf("creating controller statefulset: \n%+v", spec)
	c.addCleanUp(func() {
		logger.Debugf("deleting %q statefulset", spec.Name)
		c.broker.deleteStatefulSet(spec.Name)
	})
	return errors.Trace(c.broker.createStatefulSet(spec))
}

func (c controllerStack) buildStorageSpecForController(statefulset *apps.StatefulSet) error {
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
				Name:   c.pvcNameControllerPodStorage,
				Labels: c.stackLabels,
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

func (c controllerStack) buildContainerSpecForController(statefulset *apps.StatefulSet) error {
	var wiredTigerCacheSize float32
	if c.pcfg.Controller.Config.MongoMemoryProfile() == string(mongo.MemoryProfileLow) {
		wiredTigerCacheSize = mongo.Mongo34LowCacheSize
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
			Name:            "mongodb",
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
			Name:            "api-server",
			ImagePullPolicy: core.PullIfNotPresent,
			Image:           c.pcfg.GetControllerImagePath(),
			Command: []string{
				"/bin/sh",
			},
			Args: []string{
				"-c",
				fmt.Sprintf(caas.JujudStartUpSh, jujudCmd),
			},
			WorkingDir: jujudToolDir,
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
						"machine-"+c.pcfg.MachineId,
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
		return containerSpec
	}

	loggingOption := "--show-log"
	if loggo.GetLogger("").LogLevel() == loggo.DEBUG {
		// If the bootstrap command was requested with --debug, then the root
		// logger will be set to DEBUG. If it is, then we use --debug here too.
		loggingOption = "--debug"
	}

	agentCfgPath := filepath.Join(
		c.pcfg.DataDir,
		"agents",
		"machine-"+c.pcfg.MachineId,
		c.fileNameAgentConf,
	)
	var jujudCmd string
	if c.pcfg.MachineId == "0" {
		// only do bootstrap-state on the bootstrap machine - machine-0.
		jujudCmd += "\n" + fmt.Sprintf(
			"test -e %s || ./jujud bootstrap-state %s --data-dir %s %s --timeout %s",
			agentCfgPath,
			filepath.Join(c.pcfg.DataDir, c.fileNameBootstrapParams),
			c.pcfg.DataDir,
			loggingOption,
			c.pcfg.Bootstrap.Timeout.String(),
		)
	}
	jujudCmd += "\n" + fmt.Sprintf(
		"./jujud machine --data-dir %s --machine-id %s %s",
		c.pcfg.DataDir,
		c.pcfg.MachineId,
		loggingOption,
	)
	statefulset.Spec.Template.Spec.Containers = generateContainerSpecs(jujudCmd)
	return nil
}
