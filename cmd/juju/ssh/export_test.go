// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/environs/cloudspec"
)

type (
	ResolvedTarget = resolvedTarget
)

var (
	GetInterruptAbortChan = getInterruptAbortChan
)

func (r resolvedTarget) GetEntity() string {
	return r.entity
}

func (r *resolvedTarget) SetEntity(entity string) {
	r.entity = entity
}

func (c *sshContainer) CleanupRun() {
	c.cleanupRun()
}

func (c *sshContainer) ResolveTarget(target string) (*resolvedTarget, error) {
	return c.resolveTarget(target)
}

func (c *sshContainer) SSH(ctx Context, enablePty bool, target *resolvedTarget) error {
	return c.ssh(ctx, enablePty, target)
}

func (c *sshContainer) Copy(ctx Context) error {
	return c.copy(ctx)
}

func (c *sshContainer) GetExecClient() (k8sexec.Executor, error) {
	return c.getExecClient()
}

func (c *sshContainer) ModelName() string {
	return c.modelName
}

func (c *sshContainer) SetArgs(args []string) {
	c.setArgs(args)
}

func (c *sshContainer) InitRun(mc ModelCommand) (err error) {
	return c.initRun(mc)
}

func (c *sshContainer) Namespace() string {
	return c.namespace
}

type SSHContainerInterfaceForTest interface {
	CleanupRun()
	ResolveTarget(string) (*resolvedTarget, error)
	SSH(Context, bool, *resolvedTarget) error
	Copy(ctx Context) error
	GetExecClient() (k8sexec.Executor, error)
	ModelName() string
	SetArgs([]string)
	InitRun(mc ModelCommand) (err error)
	Namespace() string
}

func NewSSHContainer(
	modelUUID, modelName string,
	applicationAPI ApplicationAPI,
	charmsAPI CharmsAPI,
	execClient k8sexec.Executor,
	sshClient SSHClientAPI,
	remote bool,
	containerName string,
	controllerAPI SSHControllerAPI,
) SSHContainerInterfaceForTest {
	return &sshContainer{
		modelUUID:      modelUUID,
		modelName:      modelName,
		applicationAPI: applicationAPI,
		charmsAPI:      charmsAPI,
		execClient:     execClient,
		sshClient:      sshClient,
		execClientGetter: func(string, cloudspec.CloudSpec) (k8sexec.Executor, error) {
			return execClient, nil
		},
		remote:        remote,
		container:     containerName,
		controllerAPI: controllerAPI,
	}
}
