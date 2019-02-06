// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"strings"
	"path/filepath"

	"github.com/juju/errors"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	
	context "github.com/juju/juju/environs/context"
	environs "github.com/juju/juju/environs"
	"github.com/juju/juju/agent"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/cloudconfig/podcfg"
)

const (
	portMongoDB   = 37017
	portAPIServer = 17070
	stackName     = "juju-controller"
	storageSizeAPIServerRaw = "10Gi" // TODO: parse from constrains?
	storageSizeMongoDBRaw   = "10Gi"
	fileNameSharedSecret = "shared-secret"
	fileNameSSLKey = "server.pem"
	fileNameBootstrapParams = "bootstrap-params"
	fileNameAgentConf ="agent.conf"
)

var (
	stackLabels = map[string]string{"app": stackName}
	resourceNameSharedSecret = getResourceName(fileNameSharedSecret)
	resourceNameSSLKey = getResourceName(fileNameSSLKey)
	resourceNameBootstrapParams = getResourceName(fileNameBootstrapParams)
	resourceNameAgentConf = getResourceName(fileNameAgentConf)
	pvcNameMongoStorage = getResourceName("mongo-storage")
	pvcNameLogDirStorage = getResourceName("jujud-log-storage")
	pvcNameAPIServerStorage = getResourceName("jujud-storage")
)

func getResourceName(name string) string{
	return stackName + "-" + strings.Replace(name, ".", "-", -1)
}

func (k *kubernetesClient) createControllerService() error {
	spec := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   stackName,
			Labels: stackLabels,
		},
		Spec: core.ServiceSpec{
			Selector: stackLabels,
			Type:     core.ServiceType("NodePort"), // TODO: NodePort works for single node only like microk8s.
			Ports: []core.ServicePort{
				{
					Name:       "mongodb",
					TargetPort: intstr.FromInt(portMongoDB),
					Port:       portMongoDB,
					// Protocol:   "TCP",
				},
				{
					Name:       "apiServer",
					TargetPort: intstr.FromInt(portAPIServer),
					Port:       portAPIServer,
					Protocol:   "TCP",
				},
			},
		},
	}
	_, err := k.CoreV1().Services(k.namespace).Create(spec)
	return errors.Trace(err)
}

func (k *kubernetesClient) createControllerSecretShared(agentConfig agent.ConfigSetterWriter) error {
	ensureServerParams, err := cmdutil.NewEnsureServerParams(agentConfig)
	if err != nil {
		return errors.Trace(err)
	}
	return createSecret(
		resourceNameSharedSecret,
		stackLabels,
		core.SecretTypeOpaque,
		map[string][]byte{
			fileNameSharedSecret: []byte(ensureServerParams.SharedSecret),
		},
	)
}

func (k *kubernetesClient) createControllerSecretServerPem(agentConfig agent.ConfigSetterWriter) error {
	si, ok := agentConfig.StateServingInfo()
	if !ok || si.CAPrivateKey == "" {
		// No certificate information exists yet, nothing to do.
		return errors.NewNotValid(nil, "certificate is empty")
	}
	return createSecret(
		resourceNameSSLKey,
		stackLabels,
		core.SecretTypeOpaque,
		map[string][]byte{
			fileNameSSLKey: []byte(mongo.GenerateSSLKey(si.Cert, si.PrivateKey)),
		},
	)
}

func (k *kubernetesClient) createControllerSecretMongoAdmin(agentConfig agent.ConfigSetterWriter) error {
	// TODO: for mongo side car container, it's currently disabled.
	return nil
}

func (k *kubernetesClient) createControllerSecretBootstrapParams(pcfg *podcfg.ControllerPodConfig) error {
	bootstrapParamsFileContent, err := pcfg.Bootstrap.StateInitializationParams.Marshal()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("bootstrapParams file content: \n%s", string(bootstrapParamsFileContent))
	return createSecret(
		resourceNameBootstrapParams,
		stackLabels,
		core.SecretTypeOpaque,
		map[string][]byte{
			fileNameBootstrapParams: bootstrapParamsFileContent,
		},
	)
}

func (k *kubernetesClient) createControllerConfigmapAgentConf(agentConfig agent.ConfigSetterWriter) error {
	agentConfigFileContent, err := agentConfig.Render()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("agentConfig file content: \n%s", string(agentConfigFileContent))

	spec := core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:   resourceNameAgentConf),
			Labels: stackLabels,
		},
		Data: map[string]string{
			fileNameAgentConf: string(agentConfigFileContent),
		},
	}

	_, err := k.CoreV1().ConfigMaps(k.namespace).Create(spec)
	return errors.Trace(err)
}

func (k *kubernetesClient) createControllerStatefulset(pcfg *podcfg.ControllerPodConfig) error {

	spec := &apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   stackName,
			Labels: stackLabels,
		},
		Spec: apps.StatefulSetSpec{
			serviceName: stackName,
			Replicas:    1,  // TODO: HA mode!
			Selector: &v1.LabelSelector{
				MatchLabels: stackLabels,
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: stackLabels,
				},
				Spec: core.PodSpec{
					// TerminationGracePeriodSeconds: 10,
					RestartPolicy: core.RestartPolicyAlways,
				},
			},
		},
	}

	if err := buildStorageSpecForController(spec); err != nil {
		return errors.Trace(err)
	}

	if err := buildContainerSpecForController(spec, *pcfg); err != nil {
		return errors.Trace(err)
	}
	_, err := k.AppsV1().StatefulSets(k.namespace).Create(spec)
	return errors.Trace(err)
}

func buildStorageSpecForController(statefulset *apps.StatefulSet) error {
	storageclass, err := k.getDefaultStorageClass()
	if err != nil {
		return errors.Trace(err)
	}

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
				Name:   pvcNameMongoStorage,
				Labels: stackLabels,
			},
			Spec: core.PersistentVolumeClaimSpec{
				StorageClassName: storageclass.GetName(),
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
				Name:   pvcNameAPIServerStorage,
				Labels: stackLabels,
			},
			Spec: core.PersistentVolumeClaimSpec{
				StorageClassName: storageclass.GetName(),
				AccessModes:      []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceStorage: storageSizeMongoDB,
					},
				},
			},
		},
	}

	statefulset.Spec.Template.Spec.Volumes = []core.Volume{
		{
			Name: pvcNameLogDirStorage,
			volumeSource: core.VolumeSource{
				EmptyDir: core.EmptyDirVolumeSource{} // TODO: 
			},
		},
		{
			Name: resourceNameAgentConf,
			volumeSource: core.VolumeSource{
				ConfigMap: &core.ConfigMapVolumeSource{
					Name: resourceNameAgentConf,
					Items: []KeyToPath{
						{
							Key: fileNameAgentConf,
							Path: "template" + fileNameAgentConf  // TODO: 
						},
					},
				},
			},
		},
		{
			Name: resourceNameBootstrapParams,
			volumeSource: core.VolumeSource{
				ConfigMap: &core.ConfigMapVolumeSource{
					Name: resourceNameBootstrapParams,
					Items: []KeyToPath{
						{
							Key: fileNameBootstrapParams,
							Path: fileNameBootstrapParams  // TODO: 
						},
					},
				},
			},
		},
		{
			Name: resourceNameSSLKey,
			volumeSource: core.VolumeSource{
				Secret: &core.SecretVolumeSource{
					SecretName: resourceNameSSLKey,
					DefaultMode: 256,
					// Items: []KeyToPath{
					// 	{
					// 		Key: fileNameSSLKey,
					// 		Path: fileNameSSLKey
					// 	},
					// },
				},
			},
		},
		{
			Name: resourceNameSharedSecret,
			volumeSource: core.VolumeSource{
				Secret: &core.SecretVolumeSource{
					SecretName: resourceNameSharedSecret,
					DefaultMode: 256,
					Items: []KeyToPath{
						{
							Key: fileNameSharedSecret,
							Path: fileNameSharedSecret
						},
					},
				},
			},
		},
	}
	return nil
}

func buildContainerSpecForController(statefulset *apps.StatefulSet, pcfg podcfg.ControllerPodConfig) error {
	probCmds := &core.ExecAction{
		Command: {
			"mongo",
			fmt.Sprintf("--port=%s", portMongoDB),
			"--ssl",
			"--sslAllowInvalidHostnames",
			"--sslAllowInvalidCertificates",
			fmt.Sprintf("--sslPEMKeyFile=%s/server.pem", pcfg.DataDir),
			"--eval",
			"db.adminCommand('ping')",
		},
	}
	containerSpec := []core.Container{
		{
			Name:            "mongoDB",
			ImagePullPolicy: core.PullIfNotPresent,
			Image:           "mongo:3.6.6",  // TODO:
			Command: []string{
				"mongod",
			},
			Args: []string{
				fmt.Sprintf("--dbpath=%s/db", pcfg.DataDir),
				fmt.Sprintf("--sslPEMKeyFile=%s/server.pem", pcfg.DataDir),
				"--sslPEMKeyPassword=ignored",
				"--sslMode=requireSSL",
				fmt.Sprintf("--port=%s", portMongoDB),
				"--journal",
				"--replSet=juju",  // TODO
				"--quiet",
				"--oplogSize=1024",   // TODO
				"--ipv6",
				"--auth",
				fmt.Sprintf("--keyFile=%s/shared-secret", pcfg.DataDir),
				"--storageEngine=wiredTiger",
				"--wiredTigerCacheSizeGB=0.25",   // TODO
				"--bind_ip_all",

			},
			Ports: []core.ContainerPort{
				{
					Name: "mongoDB",
					ContainerPort: portMongoDB,
					Protocol: "TCP",
				},
			},
			ReadinessProbe: &core.Probe{
				Handler: core.Handler{
					Exec: probCmds,
				},
				FailureThreshold: 3,
				InitialDelaySeconds: 5,
				PeriodSeconds: 10,
				SuccessThreshold: 1,
				TimeoutSeconds: 1,
			},
			LivenessProb: &core.Probe{
				Handler: core.Handler{
					Exec: probCmds,
				},
				FailureThreshold: 3,
				InitialDelaySeconds: 30,
				PeriodSeconds: 10,
				SuccessThreshold: 1,
				TimeoutSeconds: 5,
			},
			VolumeMounts: []core.VolumeMount{
				{
					Name:      pvcNameLogDirStorage,
					MountPath: pcfg.LogDir, 
				},
				{
					Name:      pvcNameMongoStorage,
					MountPath: filepath.Join(pcfg.DataDir, "db"),
				},
				{
					Name:      resourceNameAgentConf,
					MountPath: filepath.Join(pcfg.DataDir, "agents", "machine-"+pcfg.MachineId, "template-agent.conf"),  // TODO:
					SubPath:   "template-agent.conf",
				},
				{
					Name:      resourceNameSSLKey,
					MountPath: filepath.Join(pcfg.DataDir, fileNameSSLKey)
					SubPath:   fileNameSSLKey,
					ReadOnly: true,
				},
				{
					Name:      resourceNameSharedSecret,
					MountPath: filepath.Join(pcfg.DataDir, fileNameSharedSecret)
					SubPath:   fileNameSharedSecret,
					ReadOnly: true,
				},
			},
			{
				Name: "api-server",
				// ImagePullPolicy: core.PullIfNotPresent,
				ImagePullPolicy: core.PullAlways, // TODO: for debug
				Image:           "ycliuhw/jujud-controller:2.5-beta1-bionic-amd64-2a3577c0b9",  // TODO:
				
			},
			VolumeMounts: []core.VolumeMount{
				{
					Name:      pvcNameAPIServerStorage,
					MountPath: pcfg.DataDir, 
				},
				{
					Name:      pvcNameLogDirStorage,
					MountPath: pcfg.LogDir, 
				},
				{
					Name:      resourceNameAgentConf,
					MountPath: filepath.Join(pcfg.DataDir, "agents", "machine-"+pcfg.MachineId, "template-agent.conf"),  // TODO:
					SubPath:   "template-agent.conf",
				},
				{
					Name:      resourceNameSSLKey,
					MountPath: filepath.Join(pcfg.DataDir, fileNameSSLKey)
					SubPath:   fileNameSSLKey,
					ReadOnly: true,
				},
				{
					Name:      resourceNameSharedSecret,
					MountPath: filepath.Join(pcfg.DataDir, fileNameSharedSecret)
					SubPath:   fileNameSharedSecret,
					ReadOnly: true,
				},
				{
					Name:      resourceNameBootstrapParams,
					MountPath: filepath.Join(pcfg.DataDir, fileNameBootstrapParams)
					SubPath:   fileNameBootstrapParams,
					ReadOnly: true,
				},
				
			},
		},
	}
	statefulset.Spec.Template.Spec.Containers = containerSpec
	return nil
}
