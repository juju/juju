// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"github.com/juju/retry"
	"github.com/juju/utils/v3"

	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	jujussh "github.com/juju/juju/network/ssh"
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
	charmsAPI CharmAPI,
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
		charmAPI:       charmsAPI,
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

func clientStore() jujuclient.ClientStore {
	store := jujuclienttesting.MinimalStore()
	models := store.Models["arthur"]
	models.Models["admin/controller"] = jujuclient.ModelDetails{
		ModelUUID: utils.MustNewUUID().String(),
		ModelType: model.IAAS,
	}
	store.Models["arthur"] = models
	store.Models["arthur"].CurrentModel = "controller"
	store.Accounts["arthur"] = jujuclient.AccountDetails{User: "admin"}
	return store
}

func NewSSHCommandForTest(
	applicationAPI ApplicationAPI,
	sshClient SSHClientAPI,
	statusClient StatusClientAPI,
	hostChecker jujussh.ReachableChecker,
	isTerminal func(interface{}) bool,
	retryStrategy retry.CallArgs,
) *sshCommand {
	c := &sshCommand{
		hostChecker:   hostChecker,
		isTerminal:    isTerminal,
		retryStrategy: retryStrategy,
	}
	c.sshMachine.sshClient = sshClient
	c.sshMachine.leaderAPI = applicationAPI
	c.statusClient = statusClient
	c.apiAddr = "localhost:6666"
	c.SetClientStore(clientStore())
	return c
}

func NewSCPCommandForTest(
	applicationAPI ApplicationAPI,
	sshClient SSHClientAPI,
	statusClient StatusClientAPI,
	hostChecker jujussh.ReachableChecker,
	retryStrategy retry.CallArgs,
) *scpCommand {
	c := &scpCommand{
		hostChecker:   hostChecker,
		retryStrategy: retryStrategy,
	}
	c.sshMachine.sshClient = sshClient
	c.sshMachine.leaderAPI = applicationAPI
	c.statusClient = statusClient
	c.apiAddr = "localhost:6666"
	c.SetClientStore(clientStore())
	return c
}

func NewDebugHooksCommandForTest(
	applicationAPI ApplicationAPI,
	sshClient SSHClientAPI,
	statusClient StatusClientAPI,
	charmAPI CharmAPI,
	hostChecker jujussh.ReachableChecker,
	retryStrategy retry.CallArgs,
) *debugHooksCommand {
	c := &debugHooksCommand{
		sshCommand: sshCommand{
			hostChecker:   hostChecker,
			retryStrategy: retryStrategy,
			sshContainer: sshContainer{
				applicationAPI: applicationAPI,
				charmAPI:       charmAPI,
			},
		},
	}
	c.sshMachine.sshClient = sshClient
	c.sshMachine.leaderAPI = applicationAPI
	c.statusClient = statusClient
	c.apiAddr = "localhost:6666"
	c.SetClientStore(clientStore())
	return c
}

func NewDebugCodeCommandForTest(
	applicationAPI ApplicationAPI,
	sshClient SSHClientAPI,
	statusClient StatusClientAPI,
	charmAPI CharmAPI,
	hostChecker jujussh.ReachableChecker,
	retryStrategy retry.CallArgs,
) *debugCodeCommand {
	c := &debugCodeCommand{
		debugHooksCommand: debugHooksCommand{
			sshCommand: sshCommand{
				hostChecker:   hostChecker,
				retryStrategy: retryStrategy,
				sshContainer: sshContainer{
					applicationAPI: applicationAPI,
					charmAPI:       charmAPI,
				},
			},
		},
	}
	c.sshMachine.sshClient = sshClient
	c.sshMachine.leaderAPI = applicationAPI
	c.statusClient = statusClient
	c.apiAddr = "localhost:6666"
	c.SetClientStore(clientStore())
	return c
}
