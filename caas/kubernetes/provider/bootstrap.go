// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/ssh"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	k8sstorage "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/podcfg"
	config "github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
)

const (
	// JujuControllerStackName is the juju CAAS controller stack name.
	JujuControllerStackName = "juju-controller"

	portMongoDB             = 37017
	portAPIServer           = 17070
	fileNameSharedSecret    = "shared-secret"
	fileNameSSLKey          = "server.pem"
	fileNameBootstrapParams = "bootstrap-params"
	fileNameAgentConf       = "agent.conf"

	storageSizeControllerRaw = "20Gi" // TODO: parse from constrains?
)

var (
	stackLabelsGetter                       = func(stackName string) map[string]string { return map[string]string{labelApplication: stackName} }
	resourceNameGetterService               = func(stackName string) string { return stackName }
	resourceNameGetterStatefulSet           = resourceNameGetterService
	resourceNameGetterVolumeSharedSecret    = resourceNameGetter(fileNameSharedSecret)
	resourceNameGetterVolumeSSLKey          = resourceNameGetter(fileNameSSLKey)
	resourceNameGetterVolumeBootstrapParams = resourceNameGetter(fileNameBootstrapParams)
	resourceNameGetterVolumeAgentConf       = resourceNameGetter(fileNameAgentConf)
	resourceNameGetterVolumeSystemIdentity  = resourceNameGetter(agent.SystemIdentity)
	resourceNameGetterConfigMap             = resourceNameGetter("configmap")
	resourceNameGetterSecret                = resourceNameGetter("secret")
	pvcNameGetterLogDirStorage              = resourceNameGetter("jujud-log-storage")
	pvcNameGetterControllerPodStorage       = resourceNameGetter("juju-controller-storage")
)

func resourceNameGetter(name string) func(string) string {
	return func(stackName string) string {
		return stackName + "-" + strings.Replace(name, ".", "-", -1)
	}
}

func createControllerService(client bootstrapBroker) error {
	spec := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      resourceNameGetterService(JujuControllerStackName),
			Labels:    stackLabelsGetter(JujuControllerStackName),
			Namespace: client.GetCurrentNamespace(),
		},
		Spec: core.ServiceSpec{
			Selector: stackLabelsGetter(JujuControllerStackName),
			Type:     core.ServiceType("NodePort"), // TODO: NodePort works for single node only like microk8s.
			Ports: []core.ServicePort{
				{
					Name:       "mongodb",
					TargetPort: intstr.FromInt(portMongoDB),
					Port:       portMongoDB,
					Protocol:   "TCP",
				},
				{
					Name:       "api-server",
					TargetPort: intstr.FromInt(portAPIServer),
					Port:       portAPIServer,
				},
			},
		},
	}
	logger.Debugf("ensuring controller service: \n%+v", spec)
	return errors.Trace(client.ensureService(spec))
}

func getControllerSecret(broker bootstrapBroker) (secret *core.Secret, err error) {
	defer func() {
		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
	}()

	secretName := resourceNameGetterSecret(JujuControllerStackName)
	secret, err = broker.getSecret(secretName)
	if err == nil {
		return secret, nil
	}
	if errors.IsNotFound(err) {
		err = broker.createSecret(&core.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:      secretName,
				Labels:    stackLabelsGetter(JujuControllerStackName),
				Namespace: broker.GetCurrentNamespace(),
			},
			Type: core.SecretTypeOpaque,
		})
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return broker.getSecret(secretName)
}

func createControllerSecretSharedSecret(client bootstrapBroker, agentConfig agent.ConfigSetterWriter) error {
	si, ok := agentConfig.StateServingInfo()
	if !ok {
		return errors.NewNotValid(nil, "agent config has no state serving info")
	}
	if si.SharedSecret == "" {
		// Generate a shared secret for the Mongo replica set, and write it out.
		sharedSecret, err := mongo.GenerateSharedSecret()
		if err != nil {
			return errors.Trace(err)
		}
		si.SharedSecret = sharedSecret
		agentConfig.SetStateServingInfo(si)
	}

	secret, err := getControllerSecret(client)
	if err != nil {
		return errors.Trace(err)
	}
	secret.Data[fileNameSharedSecret] = []byte(si.SharedSecret)
	logger.Debugf("ensuring shared secret: \n%+v", secret)
	return client.ensureSecret(secret)
}

func createControllerSecretServerPem(client bootstrapBroker, agentConfig agent.ConfigSetterWriter) error {
	si, ok := agentConfig.StateServingInfo()
	if !ok || si.CAPrivateKey == "" {
		// No certificate information exists yet, nothing to do.
		return errors.NewNotValid(nil, "certificate is empty")
	}

	secret, err := getControllerSecret(client)
	if err != nil {
		return errors.Trace(err)
	}
	secret.Data[fileNameSSLKey] = []byte(mongo.GenerateSSLKey(si.Cert, si.PrivateKey))
	logger.Debugf("ensuring server.pem secret: \n%+v", secret)
	return client.ensureSecret(secret)
}

func createControllerSecretSystemIdentity(client bootstrapBroker, agentConfig agent.ConfigSetterWriter, pcfg *podcfg.ControllerPodConfig) error {
	si, ok := agentConfig.StateServingInfo()
	if !ok {
		return errors.NewNotValid(nil, "StateServingInfo is empty")
	}
	privateKey, _, err := ssh.GenerateKey(config.JujuSystemKey)
	if err != nil {
		return errors.Trace(err)
	}
	si.SystemIdentity = privateKey
	agentConfig.SetStateServingInfo(si)

	// TODO: should we set to `default` rather than low.?
	mmprof, err := mongo.NewMemoryProfile(pcfg.Controller.Config.MongoMemoryProfile())
	if err != nil {
		logger.Errorf("could not set requested memory profile: %v", err)
	} else {
		agentConfig.SetMongoMemoryProfile(mmprof)
	}

	secret, err := getControllerSecret(client)
	if err != nil {
		return errors.Trace(err)
	}
	secret.Data[agent.SystemIdentity] = []byte(privateKey)
	logger.Debugf("ensuring server.pem secret: \n%+v", secret)
	return client.ensureSecret(secret)
}

func createControllerSecretMongoAdmin(client bootstrapBroker, agentConfig agent.ConfigSetterWriter) error {
	// TODO: for mongo side car container, it's currently disabled.
	return nil
}

func getControllerConfigMap(broker bootstrapBroker) (cm *core.ConfigMap, err error) {
	defer func() {
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
	}()

	cmName := resourceNameGetterConfigMap(JujuControllerStackName)
	cm, err = broker.getConfigMap(cmName)
	if err == nil {
		return cm, nil
	}
	if errors.IsNotFound(err) {
		err = broker.createConfigMap(&core.ConfigMap{
			ObjectMeta: v1.ObjectMeta{
				Name:      cmName,
				Labels:    stackLabelsGetter(JujuControllerStackName),
				Namespace: broker.GetCurrentNamespace(),
			},
		})
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return broker.getConfigMap(cmName)
}

func ensureControllerConfigmapBootstrapParams(client bootstrapBroker, pcfg *podcfg.ControllerPodConfig) error {
	bootstrapParamsFileContent, err := pcfg.Bootstrap.StateInitializationParams.Marshal()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("bootstrapParams file content: \n%s", string(bootstrapParamsFileContent))

	cm, err := getControllerConfigMap(client)
	if err != nil {
		return errors.Trace(err)
	}
	cm.Data[fileNameBootstrapParams] = string(bootstrapParamsFileContent)
	logger.Debugf("creating bootstrap-params configmap: \n%+v", cm)
	return client.ensureConfigMap(cm)
}

func ensureControllerConfigmapAgentConf(client bootstrapBroker, agentConfig agent.ConfigSetterWriter) error {
	agentConfigFileContent, err := agentConfig.Render()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("agentConfig file content: \n%s", string(agentConfigFileContent))

	cm, err := getControllerConfigMap(client)
	if err != nil {
		return errors.Trace(err)
	}
	cm.Data[fileNameAgentConf] = string(agentConfigFileContent)
	logger.Debugf("ensuring agent.conf configmap: \n%+v", cm)
	return client.ensureConfigMap(cm)
}

type bootstrapBroker interface {
	createConfigMap(configMap *core.ConfigMap) error
	getConfigMap(cmName string) (*core.ConfigMap, error)
	ensureConfigMap(configMap *core.ConfigMap) error

	createSecret(Secret *core.Secret) error
	getSecret(secretName string) (*core.Secret, error)
	ensureSecret(sec *core.Secret) error

	ensureService(spec *core.Service) error

	createStatefulSet(spec *apps.StatefulSet) error

	GetCurrentNamespace() string
	EnsureNamespace() error
	getDefaultStorageClass() (*k8sstorage.StorageClass, error)
}

func createControllerStack(client bootstrapBroker, pcfg *podcfg.ControllerPodConfig, agentConfig agent.ConfigSetterWriter) error {

	// create namespace for controller stack.
	if err := client.EnsureNamespace(); err != nil {
		// create but not ensure to avoid reuse an existing namespace.
		return errors.Annotate(err, "creating namespace for controller stack")
	}

	// create service for controller pod.
	if err := createControllerService(client); err != nil {
		return errors.Annotate(err, "creating service for controller")
	}

	// create shared-secret secret for controller pod.
	if err := createControllerSecretSharedSecret(client, agentConfig); err != nil {
		return errors.Annotate(err, "creating shared-secret secret for controller")
	}

	// create server.pem secret for controller pod.
	if err := createControllerSecretServerPem(client, agentConfig); err != nil {
		return errors.Annotate(err, "creating server.pem secret for controller")
	}

	// create system-identity secret for controller pod.
	if err := createControllerSecretSystemIdentity(client, agentConfig, pcfg); err != nil {
		return errors.Annotate(err, "creating system-identity secret for controller")
	}

	// create mongo admin account secret for controller pod.
	if err := createControllerSecretMongoAdmin(client, agentConfig); err != nil {
		return errors.Annotate(err, "creating mongo admin account secret for controller")
	}

	// create bootstrap-params configmap for controller pod.
	if err := ensureControllerConfigmapBootstrapParams(client, pcfg); err != nil {
		return errors.Annotate(err, "creating bootstrap-params configmap for controller")
	}

	// Note: create agent config configmap for controller pod lastly because agentConfig has been updated in previous steps.
	if err := ensureControllerConfigmapAgentConf(client, agentConfig); err != nil {
		return errors.Annotate(err, "creating agent config configmap for controller")
	}

	// create statefulset to ensure controller stack.
	return errors.Annotate(
		createControllerStatefulset(client, pcfg, agentConfig),
		"creating statefulset for controller",
	)
}

func createControllerStatefulset(client bootstrapBroker, pcfg *podcfg.ControllerPodConfig, agentConfig agent.ConfigSetterWriter) error {
	numberOfPods := int32(1) // TODO: HA mode!
	spec := &apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      resourceNameGetterStatefulSet(JujuControllerStackName),
			Labels:    stackLabelsGetter(JujuControllerStackName),
			Namespace: client.GetCurrentNamespace(),
		},
		Spec: apps.StatefulSetSpec{
			ServiceName: resourceNameGetterService(JujuControllerStackName),
			Replicas:    &numberOfPods,
			Selector: &v1.LabelSelector{
				MatchLabels: stackLabelsGetter(JujuControllerStackName),
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels:    stackLabelsGetter(JujuControllerStackName),
					Namespace: client.GetCurrentNamespace(),
				},
				Spec: core.PodSpec{
					RestartPolicy: core.RestartPolicyAlways,
				},
			},
		},
	}

	storageclass, err := client.getDefaultStorageClass()
	if err != nil {
		return errors.Trace(err)
	}
	if err := buildStorageSpecForController(spec, storageclass.GetName()); err != nil {
		return errors.Trace(err)
	}

	if err := buildContainerSpecForController(spec, *pcfg, agentConfig); err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("creating controller statefulset: \n%+v", spec)
	return errors.Trace(client.createStatefulSet(spec))
}

func buildStorageSpecForController(statefulset *apps.StatefulSet, storageClassName string) error {
	storageSizeController, err := resource.ParseQuantity(storageSizeControllerRaw)
	if err != nil {
		return errors.Trace(err)
	}

	// build persistent volume claim.
	statefulset.Spec.VolumeClaimTemplates = []core.PersistentVolumeClaim{
		{
			ObjectMeta: v1.ObjectMeta{
				Name:   pvcNameGetterControllerPodStorage(JujuControllerStackName),
				Labels: stackLabelsGetter(JujuControllerStackName),
			},
			Spec: core.PersistentVolumeClaimSpec{
				StorageClassName: &storageClassName,
				AccessModes:      []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceStorage: storageSizeController,
					},
				},
			},
		},
	}

	fileMode := int32(256)
	var vols []core.Volume

	// add volume log dir.
	vols = append(vols, core.Volume{
		Name: pvcNameGetterLogDirStorage(JujuControllerStackName),
		VolumeSource: core.VolumeSource{
			EmptyDir: &core.EmptyDirVolumeSource{}, // TODO: setup log dir.
		},
	})
	secretName := resourceNameGetterSecret(JujuControllerStackName)
	// add volume server.pem secret.
	vols = append(vols, core.Volume{
		Name: resourceNameGetterVolumeSSLKey(JujuControllerStackName),
		VolumeSource: core.VolumeSource{
			Secret: &core.SecretVolumeSource{
				SecretName:  secretName,
				DefaultMode: &fileMode,
				Items: []core.KeyToPath{
					{
						Key:  fileNameSSLKey,
						Path: fileNameSSLKey,
					},
				},
			},
		},
	})
	// add volume shared secret.
	vols = append(vols, core.Volume{
		Name: resourceNameGetterVolumeSharedSecret(JujuControllerStackName),
		VolumeSource: core.VolumeSource{
			Secret: &core.SecretVolumeSource{
				SecretName:  secretName,
				DefaultMode: &fileMode,
				Items: []core.KeyToPath{
					{
						Key:  fileNameSharedSecret,
						Path: fileNameSharedSecret,
					},
				},
			},
		},
	})
	// add volume system-identity.
	vols = append(vols, core.Volume{
		Name: resourceNameGetterVolumeSystemIdentity(JujuControllerStackName),
		VolumeSource: core.VolumeSource{
			Secret: &core.SecretVolumeSource{
				SecretName:  secretName,
				DefaultMode: &fileMode,
				Items: []core.KeyToPath{
					{
						Key:  agent.SystemIdentity,
						Path: agent.SystemIdentity,
					},
				},
			},
		},
	})
	cmName := resourceNameGetterConfigMap(JujuControllerStackName)
	// add volume agent.conf comfigmap.
	volAgentConf := core.Volume{
		Name: resourceNameGetterVolumeAgentConf(JujuControllerStackName),
		VolumeSource: core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				Items: []core.KeyToPath{
					{
						Key:  fileNameAgentConf,
						Path: "template" + "-" + fileNameAgentConf,
					},
				},
			},
		},
	}
	volAgentConf.VolumeSource.ConfigMap.Name = cmName
	vols = append(vols, volAgentConf)
	// add volume bootstrap-params comfigmap.
	volBootstrapParams := core.Volume{
		Name: resourceNameGetterVolumeBootstrapParams(JujuControllerStackName),
		VolumeSource: core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				Items: []core.KeyToPath{
					{
						Key:  fileNameBootstrapParams,
						Path: fileNameBootstrapParams,
					},
				},
			},
		},
	}
	volBootstrapParams.VolumeSource.ConfigMap.Name = cmName
	vols = append(vols, volBootstrapParams)

	statefulset.Spec.Template.Spec.Volumes = vols
	return nil
}

func buildContainerSpecForController(statefulset *apps.StatefulSet, pcfg podcfg.ControllerPodConfig, agentConfig agent.ConfigSetterWriter) error {
	probCmds := &core.ExecAction{
		Command: []string{
			"mongo",
			fmt.Sprintf("--port=%d", portMongoDB),
			"--ssl",
			"--sslAllowInvalidHostnames",
			"--sslAllowInvalidCertificates",
			fmt.Sprintf("--sslPEMKeyFile=%s/server.pem", pcfg.DataDir),
			"--eval",
			"db.adminCommand('ping')",
		},
	}
	var containerSpec []core.Container
	// add container mongoDB.
	// TODO(caas): refactor mongo package to make it usable for IAAS and CAAS,
	// then generate mongo config from EnsureServerParams.
	containerSpec = append(containerSpec, core.Container{
		Name:            "mongodb",
		ImagePullPolicy: core.PullIfNotPresent,
		Image:           "mongo:3.6.6", // TODO:
		Command: []string{
			"mongod",
		},
		Args: []string{
			fmt.Sprintf("--dbpath=%s/db", pcfg.DataDir),
			fmt.Sprintf("--sslPEMKeyFile=%s/server.pem", pcfg.DataDir),
			"--sslPEMKeyPassword=ignored",
			"--sslMode=requireSSL",
			fmt.Sprintf("--port=%d", portMongoDB),
			"--journal",
			fmt.Sprintf("--replSet=%s", mongo.ReplicaSetName), // TODO
			"--quiet",
			"--oplogSize=1024", // TODO
			"--ipv6",
			"--auth",
			fmt.Sprintf("--keyFile=%s/shared-secret", pcfg.DataDir),
			"--storageEngine=wiredTiger",
			"--wiredTigerCacheSizeGB=0.25", // TODO
			"--bind_ip_all",
		},
		Ports: []core.ContainerPort{
			{
				Name:          "mongodb",
				ContainerPort: portMongoDB,
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
				Name:      pvcNameGetterLogDirStorage(JujuControllerStackName),
				MountPath: pcfg.LogDir,
			},
			{
				Name:      pvcNameGetterControllerPodStorage(JujuControllerStackName),
				MountPath: filepath.Join(pcfg.DataDir, "db"),
				SubPath:   "db",
			},
			{
				Name:      resourceNameGetterVolumeSSLKey(JujuControllerStackName),
				MountPath: filepath.Join(pcfg.DataDir, fileNameSSLKey),
				SubPath:   fileNameSSLKey,
				ReadOnly:  true,
			},
			{
				Name:      resourceNameGetterVolumeSharedSecret(JujuControllerStackName),
				MountPath: filepath.Join(pcfg.DataDir, fileNameSharedSecret),
				SubPath:   fileNameSharedSecret,
				ReadOnly:  true,
			},
			{
				Name:      resourceNameGetterVolumeSystemIdentity(JujuControllerStackName),
				MountPath: agentConfig.SystemIdentityPath(),
				SubPath:   agent.SystemIdentity,
				ReadOnly:  true,
			},
		},
	})

	// add container API server.
	containerSpec = append(containerSpec, core.Container{
		Name: "api-server",
		// ImagePullPolicy: core.PullIfNotPresent,
		ImagePullPolicy: core.PullAlways, // TODO: for debug
		Image:           pcfg.GetControllerImagePath(),
		VolumeMounts: []core.VolumeMount{
			{
				Name:      pvcNameGetterControllerPodStorage(JujuControllerStackName),
				MountPath: pcfg.DataDir,
			},
			{
				Name:      pvcNameGetterLogDirStorage(JujuControllerStackName),
				MountPath: pcfg.LogDir,
			},
			{
				Name:      resourceNameGetterVolumeAgentConf(JujuControllerStackName),
				MountPath: filepath.Join(pcfg.DataDir, "agents", ("machine-" + pcfg.MachineId), "template-agent.conf"),
				SubPath:   "template-agent.conf",
			},
			{
				Name:      resourceNameGetterVolumeSSLKey(JujuControllerStackName),
				MountPath: filepath.Join(pcfg.DataDir, fileNameSSLKey),
				SubPath:   fileNameSSLKey,
				ReadOnly:  true,
			},
			{
				Name:      resourceNameGetterVolumeSharedSecret(JujuControllerStackName),
				MountPath: filepath.Join(pcfg.DataDir, fileNameSharedSecret),
				SubPath:   fileNameSharedSecret,
				ReadOnly:  true,
			},
			{
				Name:      resourceNameGetterVolumeBootstrapParams(JujuControllerStackName),
				MountPath: filepath.Join(pcfg.DataDir, fileNameBootstrapParams),
				SubPath:   fileNameBootstrapParams,
				ReadOnly:  true,
			},
			{
				Name:      resourceNameGetterVolumeSystemIdentity(JujuControllerStackName),
				MountPath: agentConfig.SystemIdentityPath(),
				SubPath:   agent.SystemIdentity,
				ReadOnly:  true,
			},
		},
	})
	statefulset.Spec.Template.Spec.Containers = containerSpec
	return nil
}
