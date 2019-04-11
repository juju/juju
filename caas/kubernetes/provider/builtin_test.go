// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/cloud"
)

var (
	_ = gc.Suite(&builtinSuite{})
)

var microk8sConfig = `
apiVersion: v1
clusters:
- cluster:
    server: http://1.1.1.1:8080
  name: microk8s-cluster
contexts:
- context:
    cluster: microk8s-cluster
    user: admin
  name: microk8s
current-context: microk8s
kind: Config
preferences: {}
users:
- name: admin
  user:
    username: admin

`

type builtinSuite struct {
	runner dummyRunner
}

func (s *builtinSuite) SetUpTest(c *gc.C) {
	var logger loggo.Logger
	s.runner = dummyRunner{CallMocker: testing.NewCallMocker(logger)}
}

func (s *builtinSuite) TestGetLocalMicroK8sConfigNotInstalled(c *gc.C) {
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "which microk8s.config"}).Returns(&exec.ExecResponse{Code: 1}, nil)

	result, err := provider.GetLocalMicroK8sConfig(s.runner)
	c.Assert(err, gc.ErrorMatches, `microk8s not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(result, gc.HasLen, 0)
}

func (s *builtinSuite) TestGetLocalMicroK8sConfigCallFails(c *gc.C) {
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "which microk8s.config"}).Returns(&exec.ExecResponse{Code: 0}, nil)
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "microk8s.config"}).Returns(&exec.ExecResponse{Code: 1, Stderr: []byte("cannot find config")}, nil)
	result, err := provider.GetLocalMicroK8sConfig(s.runner)
	c.Assert(err, gc.ErrorMatches, `cannot find config`)
	c.Assert(result, gc.HasLen, 0)
}

func (s *builtinSuite) TestGetLocalMicroK8sConfig(c *gc.C) {
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "which microk8s.config"}).Returns(&exec.ExecResponse{Code: 0}, nil)
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "microk8s.config"}).Returns(&exec.ExecResponse{Code: 0, Stdout: []byte("a bunch of config")}, nil)

	result, err := provider.GetLocalMicroK8sConfig(s.runner)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(result), gc.Equals, "a bunch of config")
}

func (s *builtinSuite) TestAttemptMicroK8sCloud(c *gc.C) {
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "which microk8s.config"}).Returns(&exec.ExecResponse{Code: 0}, nil)
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "microk8s.config"}).Returns(&exec.ExecResponse{Code: 0, Stdout: []byte(microk8sConfig)}, nil)

	k8sCloud, credential, credentialName, err := provider.AttemptMicroK8sCloud(s.runner)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(k8sCloud, gc.DeepEquals, cloud.Cloud{
		Name:           caas.K8sCloudMicrok8s,
		Endpoint:       "http://1.1.1.1:8080",
		Type:           cloud.CloudTypeCAAS,
		AuthTypes:      []cloud.AuthType{cloud.UserPassAuthType},
		CACertificates: []string{""},
		Description:    cloud.DefaultCloudDescription(cloud.CloudTypeCAAS),
		Regions: []cloud.Region{{
			Name: "localhost",
		}},
	})
	c.Assert(credential, gc.DeepEquals, getDefaultCredential())
	c.Assert(credentialName, gc.Equals, "admin")
}

func (s *builtinSuite) TestAttemptMicroK8sCloudErrors(c *gc.C) {
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "which microk8s.config"}).Returns(&exec.ExecResponse{Code: 1}, nil)
	k8sCloud, credential, credentialName, err := provider.AttemptMicroK8sCloud(s.runner)
	c.Assert(err, gc.ErrorMatches, `microk8s not found`)
	c.Assert(k8sCloud, gc.DeepEquals, cloud.Cloud{})
	c.Assert(credential, gc.DeepEquals, cloud.Credential{})
	c.Assert(credentialName, gc.Equals, "")
}
