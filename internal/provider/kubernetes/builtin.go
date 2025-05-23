// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/exec"

	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/version"
	envtools "github.com/juju/juju/environs/tools"
)

func attemptMicroK8sCloud(cmdRunner CommandRunner, getKubeConfigDir func() (string, error)) (jujucloud.Cloud, error) {
	microk8sConfig, err := getLocalMicroK8sConfig(cmdRunner, getKubeConfigDir)
	if err != nil {
		return jujucloud.Cloud{}, err
	}

	k8sCloud, err := k8scloud.CloudFromKubeConfigClusterReader(
		k8s.MicroK8sClusterName,
		bytes.NewReader(microk8sConfig),
		k8scloud.CloudParamaters{
			Description: jujucloud.DefaultCloudDescription(jujucloud.CloudTypeKubernetes),
			Name:        k8s.K8sCloudMicrok8s,
			Regions: []jujucloud.Region{{
				Name: k8s.Microk8sRegion,
			}},
		},
	)

	return k8sCloud, err
}

func attemptMicroK8sCredential(ctx context.Context, cmdRunner CommandRunner, getKubeConfigDir func() (string, error)) (jujucloud.Credential, error) {
	microk8sConfig, err := getLocalMicroK8sConfig(cmdRunner, getKubeConfigDir)
	if err != nil {
		return jujucloud.Credential{}, err
	}

	k8sConfig, err := k8scloud.ConfigFromReader(bytes.NewReader(microk8sConfig))
	if err != nil {
		return jujucloud.Credential{}, errors.Annotate(err, "processing microk8s config to make juju credentials")
	}

	contextName, err := k8scloud.PickContextByClusterName(k8sConfig, k8s.MicroK8sClusterName)
	if err != nil {
		return jujucloud.Credential{}, errors.Trace(err)
	}

	context := k8sConfig.Contexts[contextName]
	resolver := clientconfig.GetJujuAdminServiceAccountResolver(ctx, jujuclock.WallClock)
	conf, err := resolver(k8s.K8sCloudMicrok8s, k8sConfig, contextName)
	if err != nil {
		return jujucloud.Credential{}, errors.Annotate(err, "resolving microk8s credentials")
	}

	return k8scloud.CredentialFromKubeConfig(context.AuthInfo, conf)
}

// For testing.
var CheckJujuOfficial = envtools.JujudVersion

func decideKubeConfigDir() (string, error) {
	jujuDir, err := envtools.ExistingJujuLocation()
	if err != nil {
		return "", errors.Annotate(err, "cannot find juju binary")
	}
	_, isOffical, err := CheckJujuOfficial(jujuDir)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return "", errors.Trace(err)
	}
	if isOffical {
		return filepath.Join(os.Getenv("SNAP_DATA"), "microk8s", "credentials", "client.config"), nil
	}
	return filepath.Join("/var/snap/microk8s/current/", "credentials", "client.config"), nil
}

var microk8sGroupError = `
Insufficient permissions to access MicroK8s.
You can either try again with sudo or add the user %s to the 'snap_microk8s' group:

    sudo usermod -a -G snap_microk8s %s

After this, reload the user groups either via a reboot or by running 'newgrp snap_microk8s'.
`[1:]

func getLocalMicroK8sConfig(cmdRunner CommandRunner, getKubeConfigDir func() (string, error)) ([]byte, error) {
	if runtime.GOOS != "linux" {
		return getLocalMicroK8sConfigNonLinux(cmdRunner)
	}

	notSupportErr := errors.NewNotSupported(nil, fmt.Sprintf("juju %q can only work with strictly confined microk8s", version.Current))
	clientConfigPath, err := getKubeConfigDir()
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Tracef(context.TODO(), "reading kubeconfig %q", clientConfigPath)
	content, err := os.ReadFile(clientConfigPath)
	if os.IsNotExist(err) {
		return nil, errors.Annotatef(notSupportErr, "%q does not exist", clientConfigPath)
	}
	if os.IsPermission(err) {
		user, err := utils.LocalUsername()
		if err != nil {
			user = "<user>"
		}
		return nil, errors.Errorf(microk8sGroupError, user, user)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot read %q", clientConfigPath)
	}
	return content, nil
}

func getLocalMicroK8sConfigNonLinux(cmdRunner CommandRunner) ([]byte, error) {
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
		errMessage := strings.ReplaceAll(string(result.Stderr), "\n", "")
		if strings.HasSuffix(strings.ToLower(errMessage), "permission denied") {
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
