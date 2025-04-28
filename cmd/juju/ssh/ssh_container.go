// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"
	"github.com/juju/retry"

	"github.com/juju/juju/api/client/application"
	apicharms "github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/sshclient"
	commoncharm "github.com/juju/juju/api/common/charms"
	controllerapi "github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	environsbootstrap "github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/cloudspec"
	jujussh "github.com/juju/juju/network/ssh"
	"github.com/juju/juju/rpc/params"
)

// sshContainer implements functionality shared by sshCommand, SCPCommand
// and DebugHooksCommand for CAAS model.
type sshContainer struct {
	leaderResolver
	// remote indicates if it should target to the operator or workload pod.
	remote         bool
	target         string
	container      string
	args           []string
	modelUUID      string
	controllerUUID string
	modelName      string
	namespace      string

	applicationAPI   ApplicationAPI
	charmsAPI        CharmsAPI
	execClientGetter func(string, cloudspec.CloudSpec) (k8sexec.Executor, error)
	execClient       k8sexec.Executor
	sshClient        SSHClientAPI
	controllerAPI    SSHControllerAPI
}

// CloudCredentialAPI defines cloud credential related APIs.
type CloudCredentialAPI interface {
	Cloud(tag names.CloudTag) (jujucloud.Cloud, error)
	CredentialContents(cloud, credential string, withSecrets bool) ([]params.CredentialContentResult, error)
	BestAPIVersion() int
	Close() error
}

// ApplicationAPI defines application related APIs.
type ApplicationAPI interface {
	Leader(string) (string, error)
	Close() error
	UnitsInfo(units []names.UnitTag) ([]application.UnitInfo, error)
}

// SSHClientAPI defines ssh client related APIs.
type SSHClientAPI interface {
	Close() error
	ModelCredentialForSSH() (cloudspec.CloudSpec, error)
}

type CharmsAPI interface {
	Close() error
	CharmInfo(charmURL string) (*commoncharm.CharmInfo, error)
}

type SSHControllerAPI interface {
	ControllerConfig() (controller.Config, error)
}

// SetFlags sets up options and flags for the command.
func (c *sshContainer) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.remote, "remote", false, "Target on the workload or operator pod (k8s-only)")
}

func (c *sshContainer) setHostChecker(_ jujussh.ReachableChecker) {}

func (c *sshContainer) setLeaderAPI(leaderAPI LeaderAPI) {
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
func (c *sshContainer) initRun(mc ModelCommand) (err error) {
	if c.modelName, err = mc.ModelIdentifier(); err != nil {
		return errors.Trace(err)
	}

	if len(c.modelUUID) == 0 {
		_, mDetails, err := mc.ModelDetails()
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

	cAPI, err := mc.NewControllerAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	root, err := mc.NewAPIRoot()
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
		controllerCfg, err := c.controllerAPI.ControllerConfig()
		if err != nil {
			return errors.Trace(err)
		}
		c.namespace = provider.DecideControllerNamespace(controllerCfg.ControllerName())
	}

	if c.execClient == nil {
		if c.execClient, err = c.getExecClient(); err != nil {
			return errors.Trace(err)
		}
	}

	defer func() {
		c.leaderAPI = c.applicationAPI
	}()
	if c.applicationAPI == nil {
		c.applicationAPI = application.NewClient(root)
	}

	if c.charmsAPI == nil {
		c.charmsAPI = apicharms.NewClient(root)
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
	if c.charmsAPI != nil {
		_ = c.charmsAPI.Close()
		c.charmsAPI = nil
	}
	if c.sshClient != nil {
		_ = c.sshClient.Close()
		c.sshClient = nil
	}
}

const charmContainerName = "charm"

func (c *sshContainer) resolveTarget(target string) (*resolvedTarget, error) {
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
	resolvedTargetName, err := c.maybeResolveLeaderUnit(target)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if !names.IsValidUnit(resolvedTargetName) {
		return nil, errors.Errorf("invalid unit name %q", resolvedTargetName)
	}
	unitTag := names.NewUnitTag(resolvedTargetName)
	appName, err := names.UnitApplication(unitTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}

	unitInfoResults, err := c.applicationAPI.UnitsInfo([]names.UnitTag{unitTag})
	if err != nil {
		return nil, errors.Trace(err)
	}
	unit := unitInfoResults[0]
	if unit.Error != nil {
		return nil, errors.Annotatef(unit.Error, "getting unit %q", resolvedTargetName)
	}

	charmInfo, err := c.charmsAPI.CharmInfo(unit.Charm)
	if err != nil {
		return nil, errors.Annotatef(err, "getting charm info for %q", resolvedTargetName)
	}

	isMetaV2 := (charm.MetaFormat(charmInfo.Charm()) == charm.FormatV2)
	var providerID string
	if !isMetaV2 && !c.remote {
		// We don't want to introduce CaaS broker here, but only use exec client.
		podAPI := c.execClient.RawClient().CoreV1().Pods(c.execClient.NameSpace())
		modelName := modelNameWithoutUsername(c.modelName)
		// Model name should always be set, but just in case...
		if modelName == "" {
			modelName = c.execClient.NameSpace()
		}
		providerID, err = k8sprovider.GetOperatorPodName(
			podAPI,
			c.execClient.RawClient().CoreV1().Namespaces(),
			appName,
			c.execClient.NameSpace(),
			modelName,
			c.modelUUID,
			c.controllerUUID,
		)

		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(providerID) == 0 {
			return nil, errors.New(fmt.Sprintf("operator pod for unit %q is not ready yet", unitTag.Id()))
		}
	} else {
		if len(unit.ProviderId) == 0 {
			return nil, errors.New(fmt.Sprintf("container for unit %q is not ready yet", unitTag.Id()))
		}
		providerID = unit.ProviderId
	}

	if isMetaV2 {
		meta := charmInfo.Charm().Meta()
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
	}

	return &resolvedTarget{entity: providerID}, nil
}

// Context defines methods for command context.
type Context interface {
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

	srcSpec, err := c.expandSCPArg(args[0])
	if err != nil {
		return err
	}
	destSpec, err := c.expandSCPArg(args[1])
	if err != nil {
		return err
	}

	cancel, stop := getInterruptAbortChan(ctx)
	defer stop()
	return c.execClient.Copy(k8sexec.CopyParams{Src: srcSpec, Dest: destSpec}, cancel)
}

func (c *sshContainer) expandSCPArg(arg string) (o k8sexec.FileResource, err error) {
	if i := strings.Index(arg, ":"); i == -1 {
		return k8sexec.FileResource{Path: arg}, nil
	} else if i > 0 {
		o.Path = arg[i+1:]

		resolvedTarget, err := c.resolveTarget(arg[:i])
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

func (c *sshContainer) getExecClient() (k8sexec.Executor, error) {
	cloudSpec, err := c.sshClient.ModelCredentialForSSH()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.execClientGetter(c.namespace, cloudSpec)
}

func (c *sshContainer) maybePopulateTargetViaField(_ *resolvedTarget, _ func(*client.StatusArgs) (*params.FullStatus, error)) error {
	return nil
}
