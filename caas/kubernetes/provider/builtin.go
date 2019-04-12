// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"bytes"

	"github.com/juju/errors"
	"github.com/juju/utils/exec"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/cloud"
	jujucloud "github.com/juju/juju/cloud"
)

func attemptMicroK8sCloud(cmdRunner CommandRunner) (cloud.Cloud, jujucloud.Credential, string, error) {
	var newCloud cloud.Cloud
	configContent, err := getLocalMicroK8sConfig(cmdRunner)
	if err != nil {
		return newCloud, jujucloud.Credential{}, "", err
	}

	rdr := bytes.NewReader(configContent)

	cloudParams := KubeCloudParams{
		ClusterName: caas.MicroK8sClusterName,
		CaasName:    caas.K8sCloudMicrok8s,
		CaasType:    CAASProviderType,
		ClientConfigGetter: func(caasType string) (clientconfig.ClientConfigFunc, error) {
			return clientconfig.NewClientConfigReader(caasType)
		},
	}
	newCloud, credential, credentialName, err := CloudFromKubeConfig(rdr, cloudParams)
	if err != nil {
		return newCloud, jujucloud.Credential{}, "", err
	}
	newCloud.Regions = []jujucloud.Region{{
		Name: caas.Microk8sRegion,
	}}
	newCloud.Description = cloud.DefaultCloudDescription(cloud.CloudTypeCAAS)
	return newCloud, credential, credentialName, nil
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
