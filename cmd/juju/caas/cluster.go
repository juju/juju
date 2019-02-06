// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/utils"
	"github.com/juju/utils/exec"
)

//go:generate mockgen -package mocks -destination mocks/runner_mock.go github.com/juju/juju/cmd/juju/caas CommandRunner

type CommandRunner interface {
	RunCommands(run exec.RunParams) (*exec.ExecResponse, error)
}

type defaultRunner struct{}

func (*defaultRunner) RunCommands(run exec.RunParams) (*exec.ExecResponse, error) {
	return exec.RunCommands(run)
}

func getEnv(key string) string {
	result := os.Getenv(strings.ToUpper(key))
	if result == "" {
		result = os.Getenv(strings.ToLower(key))
	}
	return result
}

func kubeconfig() string {
	kubeconfig := getEnv("kubeconfig")
	if kubeconfig == "" {
		kubeconfig = filepath.Join(utils.Home(), ".kube", "config")
	}
	return kubeconfig
}

var runCommand = func(runner CommandRunner, params []string, kubeconfig string) (*exec.ExecResponse, error) {
	cmd := strings.Join(params, " ")

	path := getEnv("path")
	execParams := exec.RunParams{
		Commands:    cmd,
		Environment: []string{"KUBECONFIG=" + kubeconfig, "PATH=" + path},
	}
	return runner.RunCommands(execParams)
}

type clusterParams struct {
	name       string
	project    string
	region     string
	credential string
}

type cluster struct {
	name   string
	region string
}

type k8sCluster interface {
	CommandRunner
	getKubeConfig(p *clusterParams) (io.ReadCloser, string, error)
	interactiveParams(ctx *cmd.Context, p *clusterParams) (*clusterParams, error)
	cloud() string
}
