// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/application"
	apicharms "github.com/juju/juju/api/charms"
	apicloud "github.com/juju/juju/api/cloud"
	commoncharm "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/cloudspec"
	jujussh "github.com/juju/juju/network/ssh"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/ssh_container_mock.go github.com/juju/juju/cmd/juju/commands CloudCredentialAPI,ApplicationAPI,ModelAPI,CharmsAPI
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/context_mock.go github.com/juju/juju/cmd/juju/commands Context
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/k8s_exec_mock.go github.com/juju/juju/caas/kubernetes/provider/exec Executor

// sshContainer implements functionality shared by sshCommand, SCPCommand
// and DebugHooksCommand for CAAS model.
type sshContainer struct {
	// remote indicates if it should target to the operator or workload pod.
	remote    bool
	target    string
	container string
	args      []string
	modelUUID string

	cloudCredentialAPI CloudCredentialAPI
	modelAPI           ModelAPI
	applicationAPI     ApplicationAPI
	charmsAPI          CharmsAPI
	execClientGetter   func(string, cloudspec.CloudSpec) (k8sexec.Executor, error)
	execClient         k8sexec.Executor
	statusAPIGetter    func() (StatusAPI, error)
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
	Close() error
	UnitsInfo(units []names.UnitTag) ([]application.UnitInfo, error)
}

// ModelAPI defines model related APIs.
type ModelAPI interface {
	Close() error
	ModelInfo([]names.ModelTag) ([]params.ModelInfoResult, error)
}

type CharmsAPI interface {
	Close() error
	CharmInfo(charmURL string) (*commoncharm.CharmInfo, error)
}

// SetFlags sets up options and flags for the command.
func (c *sshContainer) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.remote, "remote", false, "Target on the workload or operator pod (k8s-only)")
	f.StringVar(&c.container, "container", "", "the container name of the target pod")
}

func (c *sshContainer) setHostChecker(checker jujussh.ReachableChecker) {}

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

// initRun initializes the API connection if required. It must be called
// at the top of the command's Run method.
func (c *sshContainer) initRun(mc ModelCommand) (err error) {
	if len(c.modelUUID) == 0 {
		_, mDetails, err := mc.ModelDetails()
		if err != nil {
			return err
		}
		c.modelUUID = mDetails.ModelUUID
	}

	if c.cloudCredentialAPI == nil || c.modelAPI == nil {
		cAPI, err := mc.NewControllerAPIRoot()
		if err != nil {
			return errors.Trace(err)
		}
		c.cloudCredentialAPI = apicloud.NewClient(cAPI)
		c.modelAPI = modelmanager.NewClient(cAPI)
	}

	if c.execClientGetter == nil {
		c.execClientGetter = k8sexec.NewForJujuCloudSpec
	}
	if c.execClient == nil {
		if c.execClient, err = c.getExecClient(); err != nil {
			return errors.Trace(err)
		}
	}

	if c.applicationAPI == nil {
		root, err := mc.NewAPIRoot()
		if err != nil {
			return errors.Trace(err)
		}
		c.applicationAPI = application.NewClient(root)
	}

	if c.statusAPIGetter == nil {
		c.statusAPIGetter = func() (StatusAPI, error) {
			return mc.NewAPIClient()
		}
	}

	if c.charmsAPI == nil {
		root, err := mc.NewAPIRoot()
		if err != nil {
			return errors.Trace(err)
		}
		c.charmsAPI = apicharms.NewClient(root)
	}

	return nil
}

// cleanupRun closes API connections.
func (c *sshContainer) cleanupRun() {
	if c.cloudCredentialAPI != nil {
		_ = c.cloudCredentialAPI.Close()
		c.cloudCredentialAPI = nil
	}
	if c.modelAPI != nil {
		_ = c.modelAPI.Close()
		c.modelAPI = nil
	}
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
}

func (c *sshContainer) resolveTarget(target string) (*resolvedTarget, error) {
	// If the user specified a leader unit, try to resolve it to the
	// appropriate unit name and override the requested target name.
	resolvedTargetName, err := maybeResolveLeaderUnit(c.statusAPIGetter, target)
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
	meta := charmInfo.Charm().Meta()

	var providerID string
	if meta.Format() == charm.FormatV1 && !c.remote {
		// We don't want to introduce CaaS broker here, but only use exec client.
		podAPI := c.execClient.RawClient().CoreV1().Pods(c.execClient.NameSpace())
		providerID, err = k8sprovider.GetOperatorPodName(
			podAPI,
			c.execClient.RawClient().CoreV1().Namespaces(),
			appName,
			c.execClient.NameSpace(),
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

	if meta.Format() == charm.FormatV2 {
		if c.container == "" {
			c.container = "charm"
		} else if _, ok := meta.Containers[c.container]; !ok {
			containers := []string{"charm"}
			for k := range meta.Containers {
				containers = append(containers, k)
			}
			return nil, errors.New(fmt.Sprintf("container %q must be one of %s", c.container, strings.Join(containers, ", ")))
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
		args = []string{"sh"}
	}
	cancel, stop := getInterruptAbortChan(ctx)
	defer stop()
	return c.execClient.Exec(
		k8sexec.ExecParams{
			PodName:       target.entity,
			ContainerName: c.container,
			Commands:      args,
			Stdout:        ctx.GetStdout(),
			Stderr:        ctx.GetStderr(),
			Stdin:         ctx.GetStdin(),
			TTY:           enablePty,
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

func (c *sshContainer) getExecClient() (k8sexec.Executor, error) {
	if v := c.cloudCredentialAPI.BestAPIVersion(); v < 2 {
		return nil, errors.NotSupportedf("credential content lookup on the controller in Juju v%d", v)
	}

	modelTag := names.NewModelTag(c.modelUUID)
	mInfoResults, err := c.modelAPI.ModelInfo([]names.ModelTag{modelTag})
	if err != nil {
		return nil, err
	}
	mInfo := mInfoResults[0]
	if mInfo.Error != nil {
		return nil, errors.Annotatef(mInfo.Error, "getting model information")
	}
	credentialTag, err := names.ParseCloudCredentialTag(mInfo.Result.CloudCredentialTag)
	if err != nil {
		return nil, err
	}
	remoteContents, err := c.cloudCredentialAPI.CredentialContents(credentialTag.Cloud().Id(), credentialTag.Name(), true)
	if err != nil {
		return nil, err
	}
	cred := remoteContents[0]
	if cred.Error != nil {
		return nil, errors.Annotatef(cred.Error, "getting credential")
	}
	if cred.Result.Content.Valid != nil && !*cred.Result.Content.Valid {
		return nil, errors.NewNotValid(nil, fmt.Sprintf("model credential %q is not valid", cred.Result.Content.Name))
	}

	jujuCred := jujucloud.NewCredential(jujucloud.AuthType(cred.Result.Content.AuthType), cred.Result.Content.Attributes)
	cloud, err := c.cloudCredentialAPI.Cloud(names.NewCloudTag(cred.Result.Content.Cloud))
	if err != nil {
		return nil, err
	}
	if !jujucloud.CloudIsCAAS(cloud) {
		return nil, errors.NewNotValid(nil, fmt.Sprintf("cloud %q is not kubernetes cloud type", cloud.Name))
	}
	cloudSpec, err := cloudspec.MakeCloudSpec(cloud, "", &jujuCred)
	if err != nil {
		return nil, err
	}
	return c.execClientGetter(mInfo.Result.Name, cloudSpec)
}
