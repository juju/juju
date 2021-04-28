// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/environs/cloudspec"
)

type (
	SSHContainer   = sshContainer
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

type SSHContainerInterfaceForTest interface {
	CleanupRun()
	ResolveTarget(string) (*resolvedTarget, error)
	SSH(Context, bool, *resolvedTarget) error
	Copy(ctx Context) error
	GetExecClient() (k8sexec.Executor, error)
	ModelName() string

	SetArgs([]string)
}

func NewSSHContainer(
	modelUUID, modelName string,
	cloudCredentialAPI CloudCredentialAPI,
	modelAPI ModelAPI,
	applicationAPI ApplicationAPI,
	charmsAPI CharmsAPI,
	execClient k8sexec.Executor,
	remote bool,
	containerName string,
) SSHContainerInterfaceForTest {
	return &sshContainer{
		modelUUID:          modelUUID,
		modelName:          modelName,
		cloudCredentialAPI: cloudCredentialAPI,
		modelAPI:           modelAPI,
		applicationAPI:     applicationAPI,
		charmsAPI:          charmsAPI,
		execClient:         execClient,
		execClientGetter: func(string, cloudspec.CloudSpec) (k8sexec.Executor, error) {
			return execClient, nil
		},
		remote:    remote,
		container: containerName,
	}
}
