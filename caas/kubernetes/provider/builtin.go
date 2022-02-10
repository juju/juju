// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"bytes"
	"strings"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/utils/v3/exec"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	jujucloud "github.com/juju/juju/cloud"
)

func attemptMicroK8sCloud(cmdRunner CommandRunner) (jujucloud.Cloud, error) {
	microk8sConfig, err := getLocalMicroK8sConfig(cmdRunner)
	if err != nil {
		return jujucloud.Cloud{}, err
	}

	k8sCloud, err := k8scloud.CloudFromKubeConfigClusterReader(
		caas.MicroK8sClusterName,
		bytes.NewReader(microk8sConfig),
		k8scloud.CloudParamaters{
			Description: jujucloud.DefaultCloudDescription(jujucloud.CloudTypeCAAS),
			Name:        caas.K8sCloudMicrok8s,
			Regions: []jujucloud.Region{{
				Name: caas.Microk8sRegion,
			}},
		},
	)

	return k8sCloud, err
}

func attemptMicroK8sCredential(cmdRunner CommandRunner) (jujucloud.Credential, error) {
	microk8sConfig, err := getLocalMicroK8sConfig(cmdRunner)
	if err != nil {
		return jujucloud.Credential{}, err
	}

	k8sConfig, err := k8scloud.ConfigFromReader(bytes.NewReader(microk8sConfig))
	if err != nil {
		return jujucloud.Credential{}, errors.Annotate(err, "processing microk8s config to make juju credentials")
	}

	contextName, err := k8scloud.PickContextByClusterName(k8sConfig, caas.MicroK8sClusterName)
	if err != nil {
		return jujucloud.Credential{}, errors.Trace(err)
	}

	context := k8sConfig.Contexts[contextName]
	resolver := clientconfig.GetJujuAdminServiceAccountResolver(jujuclock.WallClock)
	conf, err := resolver(caas.K8sCloudMicrok8s, k8sConfig, contextName)
	if err != nil {
		return jujucloud.Credential{}, errors.Annotate(err, "resolving microk8s credentials")
	}

	return k8scloud.CredentialFromKubeConfig(context.AuthInfo, conf)
}

func getLocalMicroK8sConfig(cmdRunner CommandRunner) ([]byte, error) {
	_, err := cmdRunner.LookPath("microk8s")
	if err != nil {
		return []byte{}, errors.NotFoundf("microk8s")
	}
	execParams := exec.RunParams{
		Commands: "microk8s config",
	}
	result, err := cmdRunner.RunCommands(execParams)
	if err != nil {
		return []byte{}, err
	}
	if result.Code != 0 {
		// TODO - confined snaps can't execute other commands.
		if strings.HasSuffix(strings.ToLower(string(result.Stderr)), "permission denied") {
			return []byte{}, errors.NotFoundf("microk8s")
		}
		return []byte{}, errors.New(string(result.Stderr))
	} else {
		if strings.HasPrefix(strings.ToLower(string(result.Stdout)), "microk8s is not running") {
			return []byte{}, errors.NotFoundf("microk8s is not running")
		}
	}
	return result.Stdout, nil
}
