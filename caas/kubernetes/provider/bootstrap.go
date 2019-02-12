// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/mongo"
)

const (
	// JujuControllerStackName is the juju CAAS controller stack name.
	JujuControllerStackName = "juju-controller"

	portMongoDB             = 37017
	portAPIServer           = 17070
	storageSizeAPIServerRaw = "10Gi" // TODO: parse from constrains?
	storageSizeMongoDBRaw   = "10Gi"
	fileNameSharedSecret    = "shared-secret"
	fileNameSSLKey          = "server.pem"
	fileNameBootstrapParams = "bootstrap-params"
	fileNameAgentConf       = "agent.conf"
)

var (
	stackLabelsGetter                       = func(stackName string) map[string]string { return map[string]string{labelApplication: stackName} }
	resourceNameGetterService               = func(stackName string) string { return stackName }
	resourceNameGetterStatefulSet           = resourceNameGetterService
	resourceNameGetterVolumeSharedSecret    = resourceNameGetter(fileNameSharedSecret)
	resourceNameGetterVolumeSSLKey          = resourceNameGetter(fileNameSSLKey)
	resourceNameGetterVolumeBootstrapParams = resourceNameGetter(fileNameBootstrapParams)
	resourceNameGetterVolumeAgentConf       = resourceNameGetter(fileNameAgentConf)
	resourceNameGetterConfigMap             = resourceNameGetter("configmap")
	resourceNameGetterSecret                = resourceNameGetter("secret")
	pvcNameGetterMongoStorage               = resourceNameGetter("mongo-storage")
	pvcNameGetterLogDirStorage              = resourceNameGetter("jujud-log-storage")
	pvcNameGetterAPIServerStorage           = resourceNameGetter("jujud-storage")
)

func resourceNameGetter(name string) func(string) string {
	return func(stackName string) string {
		return stackName + "-" + strings.Replace(name, ".", "-", -1)
	}
}

func (k *kubernetesClient) createControllerService() error {
	spec := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      resourceNameGetterService(JujuControllerStackName),
			Labels:    stackLabelsGetter(JujuControllerStackName),
			Namespace: k.namespace,
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
	logger.Debugf("creating controller service: \n%+v", spec)
	_, err := k.CoreV1().Services(k.namespace).Create(spec)
	return errors.Trace(err)
}

type secretEnsurer interface {
	ensureSecret(Secret *core.Secret) error
	getSecret(secretName string) (*core.Secret, error)
	GetCurrentNamespace() string
}

func getControllerSecret(broker secretEnsurer) (secret *core.Secret, err error) {
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
		err = broker.ensureSecret(&core.Secret{
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

func (k *kubernetesClient) createControllerSecretSharedSecret(agentConfig agent.ConfigSetterWriter) error {
	si, ok := agentConfig.StateServingInfo()
	if !ok {
		return errors.NewNotValid(nil, "agent config has no state serving info")
	}
	if si.SharedSecret == "" {
		// Generate a shared secret for the Mongo replica set, and write it out.
		sharedSecret, err := mongo.GenerateSharedSecret()
		if err != nil {
			return err
		}
		si.SharedSecret = sharedSecret
		agentConfig.SetStateServingInfo(si)
	}

	secret, err := getControllerSecret(k)
	if err != nil {
		return errors.Trace(err)
	}
	secret.Data[fileNameSharedSecret] = []byte(si.SharedSecret)
	logger.Debugf("ensuring shared secret: \n%+v", secret)
	return k.ensureSecret(secret)
}

func (k *kubernetesClient) createControllerSecretServerPem(agentConfig agent.ConfigSetterWriter) error {
	si, ok := agentConfig.StateServingInfo()
	if !ok || si.CAPrivateKey == "" {
		// No certificate information exists yet, nothing to do.
		return errors.NewNotValid(nil, "certificate is empty")
	}

	secret, err := getControllerSecret(k)
	if err != nil {
		return errors.Trace(err)
	}
	secret.Data[fileNameSSLKey] = []byte(mongo.GenerateSSLKey(si.Cert, si.PrivateKey))
	logger.Debugf("ensuring server.pem secret: \n%+v", secret)
	return k.ensureSecret(secret)
}

func (k *kubernetesClient) createControllerSecretMongoAdmin(agentConfig agent.ConfigSetterWriter) error {
	// TODO: for mongo side car container, it's currently disabled.
	return nil
}

type configMapEnsurer interface {
	ensureConfigMap(configMap *core.ConfigMap) error
	getConfigMap(cmName string) (*core.ConfigMap, error)
	GetCurrentNamespace() string
}

func getControllerConfigMap(broker configMapEnsurer) (cm *core.ConfigMap, err error) {
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
		err = broker.ensureConfigMap(&core.ConfigMap{
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

func (k *kubernetesClient) ensureControllerConfigmapBootstrapParams(pcfg *podcfg.ControllerPodConfig) error {
	bootstrapParamsFileContent, err := pcfg.Bootstrap.StateInitializationParams.Marshal()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("bootstrapParams file content: \n%s", string(bootstrapParamsFileContent))

	cm, err := getControllerConfigMap(k)
	if err != nil {
		return errors.Trace(err)
	}
	cm.Data[fileNameBootstrapParams] = string(bootstrapParamsFileContent)
	logger.Debugf("creating bootstrap-params configmap: \n%+v", cm)
	return k.ensureConfigMap(cm)
}

func (k *kubernetesClient) ensureControllerConfigmapAgentConf(agentConfig agent.ConfigSetterWriter) error {
	agentConfigFileContent, err := agentConfig.Render()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("agentConfig file content: \n%s", string(agentConfigFileContent))

	cm, err := getControllerConfigMap(k)
	if err != nil {
		return errors.Trace(err)
	}
	cm.Data[fileNameAgentConf] = string(agentConfigFileContent)
	logger.Debugf("ensuring agent.conf configmap: \n%+v", cm)
	return k.ensureConfigMap(cm)
}

func (k *kubernetesClient) createControllerStatefulset(pcfg *podcfg.ControllerPodConfig) error {
	numberOfPods := int32(1) // TODO: HA mode!
	spec := &apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      resourceNameGetterStatefulSet(JujuControllerStackName),
			Labels:    stackLabelsGetter(JujuControllerStackName),
			Namespace: k.namespace,
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
					Namespace: k.namespace,
				},
				Spec: core.PodSpec{
					RestartPolicy: core.RestartPolicyAlways,
				},
			},
		},
	}

	storageclass, err := k.getDefaultStorageClass()
	if err != nil {
		return errors.Trace(err)
	}
	if err := buildStorageSpecForController(spec, storageclass.GetName()); err != nil {
		return errors.Trace(err)
	}

	if err := buildContainerSpecForController(spec, *pcfg); err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("creating controller statefulset: \n%+v", spec)
	_, err = k.AppsV1().StatefulSets(k.namespace).Create(spec)
	return errors.Trace(err)
}

func buildStorageSpecForController(statefulset *apps.StatefulSet, storageClassName string) error {
	storageSizeAPIServer, err := resource.ParseQuantity(storageSizeAPIServerRaw)
	if err != nil {
		return errors.Trace(err)
	}
	storageSizeMongoDB, err := resource.ParseQuantity(storageSizeMongoDBRaw)
	if err != nil {
		return errors.Trace(err)
	}

	// build persistent volume claim.
	statefulset.Spec.VolumeClaimTemplates = []core.PersistentVolumeClaim{
		{
			ObjectMeta: v1.ObjectMeta{
				Name:   pvcNameGetterMongoStorage(JujuControllerStackName),
				Labels: stackLabelsGetter(JujuControllerStackName),
			},
			Spec: core.PersistentVolumeClaimSpec{
				StorageClassName: &storageClassName,
				AccessModes:      []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceStorage: storageSizeMongoDB,
					},
				},
			},
		},
		{
			ObjectMeta: v1.ObjectMeta{
				Name:   pvcNameGetterAPIServerStorage(JujuControllerStackName),
				Labels: stackLabelsGetter(JujuControllerStackName),
			},
			Spec: core.PersistentVolumeClaimSpec{
				StorageClassName: &storageClassName,
				AccessModes:      []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceStorage: storageSizeAPIServer,
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

func buildContainerSpecForController(statefulset *apps.StatefulSet, pcfg podcfg.ControllerPodConfig) error {
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
				Name:      pvcNameGetterMongoStorage(JujuControllerStackName),
				MountPath: filepath.Join(pcfg.DataDir, "db"),
			},
			{
				Name:      resourceNameGetterVolumeAgentConf(JujuControllerStackName),
				MountPath: filepath.Join(pcfg.DataDir, "agents", "machine-"+pcfg.MachineId, "template-agent.conf"),
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
				Name:      pvcNameGetterAPIServerStorage(JujuControllerStackName),
				MountPath: pcfg.DataDir,
			},
			{
				Name:      pvcNameGetterLogDirStorage(JujuControllerStackName),
				MountPath: pcfg.LogDir,
			},
			{
				Name:      resourceNameGetterVolumeAgentConf(JujuControllerStackName),
				MountPath: filepath.Join(pcfg.DataDir, "agents", "machine-"+pcfg.MachineId, "template-agent.conf"),
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
		},
	})
	statefulset.Spec.Template.Spec.Containers = containerSpec
	return nil
}
