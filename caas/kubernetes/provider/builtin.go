// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"bytes"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/utils/v2/exec"

	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/cloud"
	jujucloud "github.com/juju/juju/cloud"
)

func attemptMicroK8sCloud(cmdRunner CommandRunner) (cloud.Cloud, jujucloud.Credential, string, error) {
	return attemptMicroK8sCloudInternal(cmdRunner, KubeCloudParams{
		ClusterName:   k8s.MicroK8sClusterName,
		CloudName:     k8s.K8sCloudMicrok8s,
		CredentialUID: k8s.K8sCloudMicrok8s,
		CaasType:      constants.CAASProviderType,
		ClientConfigGetter: func(caasType string) (clientconfig.ClientConfigFunc, error) {
			return clientconfig.NewClientConfigReader(caasType)
		},
		Clock: jujuclock.WallClock,
	})
}

func attemptMicroK8sCloudInternal(
	cmdRunner CommandRunner,
	kubeCloudParams KubeCloudParams,
) (cloud.Cloud, jujucloud.Credential, string, error) {
	var newCloud cloud.Cloud
	configContent, err := getLocalMicroK8sConfig(cmdRunner)
	if err != nil {
		return newCloud, jujucloud.Credential{}, "", err
	}

	rdr := bytes.NewReader(configContent)
	newCloud, credential, err := CloudFromKubeConfig(rdr, kubeCloudParams)
	if err != nil {
		return newCloud, jujucloud.Credential{}, "", err
	}
	newCloud.Regions = []jujucloud.Region{{
		Name: k8s.Microk8sRegion,
	}}
	newCloud.Description = cloud.DefaultCloudDescription(cloud.CloudTypeCAAS)
	return newCloud, credential, credential.Label, nil
}

func getLocalMicroK8sConfig(cmdRunner CommandRunner) ([]byte, error) {
	result, err := cmdRunner.RunCommands(exec.RunParams{
		Commands: "which microk8s.config",
	})
	if err != nil || result.Code != 0 {
		return []byte{}, errors.NotFoundf("microk8s")
	}
	execParams := exec.RunParams{
		Commands: "microk8s.config",
	}
	result, err = cmdRunner.RunCommands(execParams)
	if err != nil {
		return []byte{}, err
	}
	if result.Code != 0 {
		return []byte{}, errors.New(string(result.Stderr))
	}
	return result.Stdout, nil
}
