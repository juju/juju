// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"
	"github.com/juju/retry"

	"github.com/juju/juju/api/client/application"
	apicharms "github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/sshclient"
	controllerapi "github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	environsbootstrap "github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/cloudspec"
	jujussh "github.com/juju/juju/internal/network/ssh"
	"github.com/juju/juju/rpc/params"
)

// sshContainer implements functionality shared by sshCommand, SCPCommand
// and DebugHooksCommand for CAAS model.
type sshContainer struct {
	leaderResolver
	target         string
	container      string
	args           []string
	modelUUID      string
	controllerUUID string
	modelName      string
	namespace      string

	applicationAPI   ApplicationAPI
	charmAPI         CharmAPI
	execClientGetter func(string, cloudspec.CloudSpec) (k8sexec.Executor, error)
	execClient       k8sexec.Executor
	sshClient        SSHClientAPI
	controllerAPI    SSHControllerAPI
}

// SetFlags sets up options and flags for the command.
func (c *sshContainer) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.container, "container", "", "the container name of the target pod")
}

func (c *sshContainer) setHostChecker(_ jujussh.ReachableChecker) {}

func (c *sshContainer) setLeaderAPI(ctx context.Context, leaderAPI LeaderAPI) {
	c.leaderAPI = leaderAPI
}

// getTarget returns the target.
func (c *sshContainer) getTarget() string {
	return c.target
}

// setTarget sets the target.
func (c *sshContainer) setTarget(target string) {
	c.target = target
}

// getArgs returns the args.
func (c *sshContainer) getArgs() []string {
	return c.args
}

// setArgs sets the args.
func (c *sshContainer) setArgs(args []string) {
	c.args = args
}

func (c *sshContainer) setRetryStrategy(_ retry.CallArgs) {}

func (c *sshContainer) setPublicKeyRetryStrategy(_ retry.CallArgs) {}

// initRun initializes the API connection if required. It must be called
// at the top of the command's Run method.
func (c *sshContainer) initRun(ctx context.Context, mc ModelCommand) (err error) {
	if c.modelName, err = mc.ModelIdentifier(); err != nil {
		return errors.Trace(err)
	}

	if len(c.modelUUID) == 0 {
		_, mDetails, err := mc.ModelDetails(ctx)
		if err != nil {
			return err
		}
		c.modelUUID = mDetails.ModelUUID
	}

	if len(c.controllerUUID) == 0 {
		controllerDetails, err := mc.ControllerDetails()
		if err != nil {
			return err
		}
		c.controllerUUID = controllerDetails.ControllerUUID
	}

	cAPI, err := mc.NewControllerAPIRoot(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	root, err := mc.NewAPIRoot(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	if c.sshClient == nil {
		c.sshClient = sshclient.NewFacade(root)
	}

	if c.execClientGetter == nil {
		c.execClientGetter = k8sexec.NewForJujuCloudSpec
	}

	c.namespace = modelNameWithoutUsername(c.modelName)
	if c.namespace == environsbootstrap.ControllerModelName {
		if c.controllerAPI == nil {
			c.controllerAPI = controllerapi.NewClient(cAPI)
		}
		controllerCfg, err := c.controllerAPI.ControllerConfig(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		c.namespace = provider.DecideControllerNamespace(controllerCfg.ControllerName())
	}

	if c.execClient == nil {
		if c.execClient, err = c.getExecClient(ctx); err != nil {
			return errors.Trace(err)
		}
	}

	defer func() {
		c.leaderAPI = c.applicationAPI
	}()
	if c.applicationAPI == nil {
		c.applicationAPI = application.NewClient(root)
	}

	if c.charmAPI == nil {
		c.charmAPI = apicharms.NewClient(root)
	}

	return nil
}

// cleanupRun closes API connections.
func (c *sshContainer) cleanupRun() {
	if c.execClientGetter != nil {
		c.execClientGetter = nil
	}
	if c.execClient != nil {
		c.execClient = nil
	}
	if c.applicationAPI != nil {
		_ = c.applicationAPI.Close()
		c.applicationAPI = nil
	}
	if c.charmAPI != nil {
		_ = c.charmAPI.Close()
		c.charmAPI = nil
	}
	if c.sshClient != nil {
		_ = c.sshClient.Close()
		c.sshClient = nil
	}
}

const charmContainerName = "charm"

func (c *sshContainer) resolveTarget(ctx context.Context, target string) (*resolvedTarget, error) {
	if modelNameWithoutUsername(c.modelName) == environsbootstrap.ControllerModelName && names.IsValidMachine(target) {
		// TODO(caas): change here to controller unit tag once we refactored controller to an application.
		if target != "0" {
			// HA is not enabled on CaaS controller yet.
			return nil, errors.NotFoundf("target %q", target)
		}
		return &resolvedTarget{entity: fmt.Sprintf("%s-%s", environsbootstrap.ControllerModelName, target)}, nil
	}
	// If the user specified a leader unit, try to resolve it to the
	// appropriate unit name and override the requested target name.
	resolvedTargetName, err := c.maybeResolveLeaderUnit(ctx, target)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if !names.IsValidUnit(resolvedTargetName) {
		return nil, errors.Errorf("invalid unit name %q", resolvedTargetName)
	}
	unitTag := names.NewUnitTag(resolvedTargetName)

	unitInfoResults, err := c.applicationAPI.UnitsInfo(ctx, []names.UnitTag{unitTag})
	if err != nil {
		return nil, errors.Trace(err)
	}
	unit := unitInfoResults[0]
	if unit.Error != nil {
		return nil, errors.Annotatef(unit.Error, "getting unit %q", resolvedTargetName)
	}

	charmInfo, err := c.charmAPI.CharmInfo(ctx, unit.Charm)
	if err != nil {
		return nil, errors.Annotatef(err, "getting charm info for %q", resolvedTargetName)
	}

	if len(unit.ProviderId) == 0 {
		return nil, errors.New(fmt.Sprintf("container for unit %q is not ready yet", unitTag.Id()))
	}

	meta := charmInfo.Meta
	if c.container == "" {
		c.container = charmContainerName
	}
	if _, ok := meta.Containers[c.container]; !ok && c.container != charmContainerName {
		containers := []string{charmContainerName}
		for k := range meta.Containers {
			containers = append(containers, k)
		}
		return nil, errors.New(
			fmt.Sprintf("container %q must be one of %s", c.container, strings.Join(containers, ", ")))
	}

	return &resolvedTarget{entity: unit.ProviderId}, nil
}

// Context defines methods for command context.
type Context interface {
	context.Context
	InterruptNotify(c chan<- os.Signal)
	StopInterruptNotify(c chan<- os.Signal)
	GetStdout() io.Writer
	GetStderr() io.Writer
	GetStdin() io.Reader
}

func (c *sshContainer) ssh(ctx Context, enablePty bool, target *resolvedTarget) (err error) {
	args := c.args
	if len(args) == 0 {
		args = []string{"exec", "sh"}
	}
	cancel, stop := getInterruptAbortChan(ctx)
	defer stop()
	var env []string
	if enablePty {
		if term := os.Getenv("TERM"); term != "" {
			env = append(env, "TERM="+term)
		}
	}
	return c.execClient.Exec(
		ctx,
		k8sexec.ExecParams{
			PodName:       target.entity,
			ContainerName: c.container,
			Commands:      args,
			Stdout:        ctx.GetStdout(),
			Stderr:        ctx.GetStderr(),
			Stdin:         ctx.GetStdin(),
			TTY:           enablePty,
			Env:           env,
		},
		cancel,
	)
}

func getInterruptAbortChan(ctx Context) (<-chan struct{}, func()) {
	ch := make(chan os.Signal, 1)
	cancel := make(chan struct{})
	ctx.InterruptNotify(ch)

	cleanUp := func() {
		ctx.StopInterruptNotify(ch)
		close(ch)
	}

	go func() {
		select {
		case <-ch:
			close(cancel)
		}
	}()
	return cancel, cleanUp
}

func (c *sshContainer) copy(ctx Context) error {
	args := c.getArgs()
	if len(args) < 2 {
		return errors.New("source and destination are required")
	}
	if len(args) > 2 {
		return errors.New("only one source and one destination are allowed for a k8s application")
	}

	srcSpec, err := c.expandSCPArg(ctx, args[0])
	if err != nil {
		return err
	}
	destSpec, err := c.expandSCPArg(ctx, args[1])
	if err != nil {
		return err
	}

	cancel, stop := getInterruptAbortChan(ctx)
	defer stop()
	return c.execClient.Copy(ctx, k8sexec.CopyParams{Src: srcSpec, Dest: destSpec}, cancel)
}

func (c *sshContainer) expandSCPArg(ctx context.Context, arg string) (o k8sexec.FileResource, err error) {
	if i := strings.Index(arg, ":"); i == -1 {
		return k8sexec.FileResource{Path: arg}, nil
	} else if i > 0 {
		o.Path = arg[i+1:]

		resolvedTarget, err := c.resolveTarget(ctx, arg[:i])
		if err != nil {
			return o, err
		}
		o.PodName = resolvedTarget.entity
		o.ContainerName = c.container
		return o, nil
	}
	return o, errors.New("target must match format: [pod[/container]:]path")
}

func modelNameWithoutUsername(modelName string) string {
	parts := strings.Split(modelName, "/")
	if len(parts) == 2 {
		modelName = parts[1]
	}
	return modelName
}

func (c *sshContainer) getExecClient(ctx context.Context) (k8sexec.Executor, error) {
	cloudSpec, err := c.sshClient.ModelCredentialForSSH(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.execClientGetter(c.namespace, cloudSpec)
}

func (c *sshContainer) maybePopulateTargetViaField(ctx context.Context, _ *resolvedTarget, _ func(context.Context, *client.StatusArgs) (*params.FullStatus, error)) error {
	return nil
}
