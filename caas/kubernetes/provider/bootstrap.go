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
	// cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/mongo"
)

const (
	portMongoDB             = 37017
	portAPIServer           = 17070
	stackName               = "juju-controller"
	storageSizeAPIServerRaw = "10Gi" // TODO: parse from constrains?
	storageSizeMongoDBRaw   = "10Gi"
	fileNameSharedSecret    = "shared-secret"
	fileNameSSLKey          = "server.pem"
	fileNameBootstrapParams = "bootstrap-params"
	fileNameAgentConf       = "agent.conf"
)

var (
	stackLabels                 = map[string]string{"app": stackName}
	resourceNameSharedSecret    = getResourceName(fileNameSharedSecret)
	resourceNameSSLKey          = getResourceName(fileNameSSLKey)
	resourceNameBootstrapParams = getResourceName(fileNameBootstrapParams)
	resourceNameAgentConf       = getResourceName(fileNameAgentConf)
	pvcNameMongoStorage         = getResourceName("mongo-storage")
	pvcNameLogDirStorage        = getResourceName("jujud-log-storage")
	pvcNameAPIServerStorage     = getResourceName("jujud-storage")
)

func getResourceName(name string) string {
	return stackName + "-" + strings.Replace(name, ".", "-", -1)
}

func (k *kubernetesClient) createControllerService() error {
	spec := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      stackName,
			Labels:    stackLabels,
			Namespace: k.namespace,
		},
		Spec: core.ServiceSpec{
			Selector: stackLabels,
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

func (k *kubernetesClient) createControllerSecretShared(agentConfig agent.ConfigSetterWriter) error {
	// ensureServerParams, err := cmdutil.NewEnsureServerParams(agentConfig)
	si, ok := agentConfig.StateServingInfo()
	if !ok {
		return errors.NewNotValid(nil, "agent config has no state serving info")
	}
	return k.createSecret(
		resourceNameSharedSecret,
		stackLabels,
		core.SecretTypeOpaque,
		map[string][]byte{
			fileNameSharedSecret: []byte(si.SharedSecret),
		},
	)
}

func (k *kubernetesClient) createControllerSecretServerPem(agentConfig agent.ConfigSetterWriter) error {
	si, ok := agentConfig.StateServingInfo()
	if !ok || si.CAPrivateKey == "" {
		// No certificate information exists yet, nothing to do.
		return errors.NewNotValid(nil, "certificate is empty")
	}
	return k.createSecret(
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

func (k *kubernetesClient) createControllerConfigmapBootstrapParams(pcfg *podcfg.ControllerPodConfig) error {
	bootstrapParamsFileContent, err := pcfg.Bootstrap.StateInitializationParams.Marshal()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("bootstrapParams file content: \n%s", string(bootstrapParamsFileContent))
	// return k.createSecret(
	// 	resourceNameBootstrapParams,
	// 	stackLabels,
	// 	core.SecretTypeOpaque,
	// 	map[string][]byte{
	// 		fileNameBootstrapParams: bootstrapParamsFileContent,
	// 	},
	// )

	spec := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      resourceNameBootstrapParams,
			Labels:    stackLabels,
			Namespace: k.namespace,
		},
		Data: map[string]string{
			fileNameBootstrapParams: string(bootstrapParamsFileContent),
		},
	}
	logger.Debugf("creating bootstrap-params configmap: \n%+v", spec)
	_, err = k.CoreV1().ConfigMaps(k.namespace).Create(spec)
	return errors.Trace(err)
}

func (k *kubernetesClient) createControllerConfigmapAgentConf(agentConfig agent.ConfigSetterWriter) error {
	agentConfigFileContent, err := agentConfig.Render()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("agentConfig file content: \n%s", string(agentConfigFileContent))

	spec := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      resourceNameAgentConf,
			Labels:    stackLabels,
			Namespace: k.namespace,
		},
		Data: map[string]string{
			fileNameAgentConf: string(agentConfigFileContent),
		},
	}
	logger.Debugf("creating agent.conf configmap: \n%+v", spec)
	_, err = k.CoreV1().ConfigMaps(k.namespace).Create(spec)
	return errors.Trace(err)
}

func (k *kubernetesClient) createControllerStatefulset(pcfg *podcfg.ControllerPodConfig) error {
	numberOfPods := int32(1) // TODO: HA mode!
	spec := &apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      stackName,
			Labels:    stackLabels,
			Namespace: k.namespace,
		},
		Spec: apps.StatefulSetSpec{
			ServiceName: stackName,
			Replicas:    &numberOfPods,
			Selector: &v1.LabelSelector{
				MatchLabels: stackLabels,
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels:    stackLabels,
					Namespace: k.namespace,
				},
				Spec: core.PodSpec{
					// TerminationGracePeriodSeconds: 10,
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
				Name:   pvcNameMongoStorage,
				Labels: stackLabels,
				// Namespace: k.namespace,
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
				Name:   pvcNameAPIServerStorage,
				Labels: stackLabels,
				// Namespace: k.namespace,
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
		Name: pvcNameLogDirStorage,
		VolumeSource: core.VolumeSource{
			EmptyDir: &core.EmptyDirVolumeSource{}, // TODO:
		},
	})
	// add volume server.pem secret.
	vols = append(vols, core.Volume{
		Name: resourceNameSSLKey,
		VolumeSource: core.VolumeSource{
			Secret: &core.SecretVolumeSource{
				SecretName:  resourceNameSSLKey,
				DefaultMode: &fileMode,
				// Items: []core.KeyToPath{
				// 	{
				// 		Key: fileNameSSLKey,
				// 		Path: fileNameSSLKey
				// 	},
				// },
			},
		},
	})
	// add volume shared secret.
	vols = append(vols, core.Volume{
		Name: resourceNameSharedSecret,
		VolumeSource: core.VolumeSource{
			Secret: &core.SecretVolumeSource{
				SecretName:  resourceNameSharedSecret,
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
	// add volume agent.conf comfigmap.
	volAgentConf := core.Volume{
		Name: resourceNameAgentConf,
		VolumeSource: core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				// Name: resourceNameAgentConf,
				Items: []core.KeyToPath{
					{
						Key:  fileNameAgentConf,
						Path: "template" + fileNameAgentConf, // TODO:
					},
				},
			},
		},
	}
	volAgentConf.VolumeSource.ConfigMap.Name = resourceNameAgentConf
	vols = append(vols, volAgentConf)
	// add volume bootstrap-params comfigmap.
	volBootstrapParams := core.Volume{
		Name: resourceNameBootstrapParams,
		VolumeSource: core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				// Name: resourceNameBootstrapParams,
				Items: []core.KeyToPath{
					{
						Key:  fileNameBootstrapParams,
						Path: fileNameBootstrapParams, // TODO:
					},
				},
			},
		},
	}
	volBootstrapParams.VolumeSource.ConfigMap.Name = resourceNameBootstrapParams
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
			"--replSet=juju", // TODO
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
				Name:      pvcNameLogDirStorage,
				MountPath: pcfg.LogDir,
			},
			{
				Name:      pvcNameMongoStorage,
				MountPath: filepath.Join(pcfg.DataDir, "db"),
			},
			{
				Name:      resourceNameAgentConf,
				MountPath: filepath.Join(pcfg.DataDir, "agents", "machine-"+pcfg.MachineId, "template-agent.conf"), // TODO:
				SubPath:   "template-agent.conf",
			},
			{
				Name:      resourceNameSSLKey,
				MountPath: filepath.Join(pcfg.DataDir, fileNameSSLKey),
				SubPath:   fileNameSSLKey,
				ReadOnly:  true,
			},
			{
				Name:      resourceNameSharedSecret,
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
		ImagePullPolicy: core.PullAlways,                      // TODO: for debug
		Image:           "ycliuhw/jujud-controller:2.6-beta1", // TODO:
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
				MountPath: filepath.Join(pcfg.DataDir, "agents", "machine-"+pcfg.MachineId, "template-agent.conf"), // TODO:
				SubPath:   "template-agent.conf",
			},
			{
				Name:      resourceNameSSLKey,
				MountPath: filepath.Join(pcfg.DataDir, fileNameSSLKey),
				SubPath:   fileNameSSLKey,
				ReadOnly:  true,
			},
			{
				Name:      resourceNameSharedSecret,
				MountPath: filepath.Join(pcfg.DataDir, fileNameSharedSecret),
				SubPath:   fileNameSharedSecret,
				ReadOnly:  true,
			},
			{
				Name:      resourceNameBootstrapParams,
				MountPath: filepath.Join(pcfg.DataDir, fileNameBootstrapParams),
				SubPath:   fileNameBootstrapParams,
				ReadOnly:  true,
			},
		},
	})
	statefulset.Spec.Template.Spec.Containers = containerSpec
	return nil
}
