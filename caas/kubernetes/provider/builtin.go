// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"bytes"

	"github.com/juju/errors"
	"github.com/juju/utils/exec"

	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/cloud"
	jujucloud "github.com/juju/juju/cloud"
)

const (
	builtinMicroK8sClusterName = "microk8s-cluster"
	builtinMicroK8sName        = "microk8s"
	builtinMicroK8sRegion      = "localhost"
)

func attemptMicroK8sCloud(cmdRunner CommandRunner) (cloud.Cloud, jujucloud.Credential, string, error) {
	var newCloud cloud.Cloud
	configContent, err := getLocalMicroK8sConfig(cmdRunner)
	if err != nil {
		return newCloud, jujucloud.Credential{}, "", err
	}

	rdr := bytes.NewReader(configContent)

	cloudParams := KubeCloudParams{
		ClusterName: builtinMicroK8sClusterName,
		CaasName:    builtinMicroK8sName,
		CaasType:    CAASProviderType,

		ClientConfigGetter: func(caasType string) (clientconfig.ClientConfigFunc, error) {
			return clientconfig.NewClientConfigReader(caasType)
		},
	}
	return CloudFromKubeConfig(rdr, cloudParams)
}

func getLocalMicroK8sConfig(cmdRunner CommandRunner) ([]byte, error) {
	execParams := exec.RunParams{
		Commands: "microk8s.config",
	}
	result, err := cmdRunner.RunCommands(execParams)
	if err != nil {
		return []byte{}, err
	}
	if result.Code != 0 {
		return []byte{}, errors.New(string(result.Stderr))
	}
	return result.Stdout, nil
}
