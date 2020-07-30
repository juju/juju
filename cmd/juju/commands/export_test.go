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

func (c *sshContainer) GetExecClient() (k8sexec.Executor, error) {
	return c.getExecClient()
}

type SSHContainerInterfaceForTest interface {
	CleanupRun()
	ResolveTarget(string) (*resolvedTarget, error)
	SSH(Context, bool, *resolvedTarget) error
	GetExecClient() (k8sexec.Executor, error)

	SetArgs(Args []string)
}

func NewSSHContainer(
	modelUUID string,
	cloudCredentialAPI CloudCredentialAPI,
	modelAPI ModelAPI,
	applicationAPI ApplicationAPI,
	execClientGetter func(string, cloudspec.CloudSpec) (k8sexec.Executor, error),
) SSHContainerInterfaceForTest {
	return &sshContainer{
		modelUUID:          modelUUID,
		cloudCredentialAPI: cloudCredentialAPI,
		modelAPI:           modelAPI,
		applicationAPI:     applicationAPI,
		execClientGetter:   execClientGetter,
	}
}
