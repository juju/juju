// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"io"
	"os"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/application"
	apicloud "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/cloudspec"
	jujussh "github.com/juju/juju/network/ssh"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/ssh_container_mock.go github.com/juju/juju/cmd/juju/commands CloudCredentialAPI,ApplicationAPI,ModelAPI
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/context_mock.go github.com/juju/juju/cmd/juju/commands Context
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/k8s_exec_mock.go github.com/juju/juju/caas/kubernetes/provider/exec Executor

// sshContainer implements functionality shared by sshCommand, SCPCommand
// and DebugHooksCommand for CAAS model.
type sshContainer struct {
	// remote indicates if it should target to the operator or workload pod.
	remote    bool
	target    string
	args      []string
	modelUUID string

	cloudCredentialAPI CloudCredentialAPI
	modelAPI           ModelAPI
	applicationAPI     ApplicationAPI
	execClientGetter   func(string, cloudspec.CloudSpec) (k8sexec.Executor, error)
	execClient         k8sexec.Executor
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

// SetFlags sets up options and flags for the command.
func (c *sshContainer) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.remote, "remote", false, "Target on the workload or operator pod (k8s-only)")
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
func (c *sshContainer) initRun(mc modelcmd.ModelCommandBase) (err error) {
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
}

func (c *sshContainer) resolveTarget(target string) (*resolvedTarget, error) {
	if !names.IsValidUnit(target) {
		return nil, errors.Errorf("invalid unit name %q", target)
	}
	unitTag := names.NewUnitTag(target)
	var providerID string
	if !c.remote {
		appName, err := names.UnitApplication(unitTag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		// We don't want to introduce CaaS broker here, but only use exec client.
		podAPI := c.execClient.RawClient().CoreV1().Pods(c.execClient.NameSpace())
		if providerID, err = k8sprovider.GetOperatorPodName(podAPI, appName); err != nil {
			return nil, errors.Trace(err)
		}
		if len(providerID) == 0 {
			return nil, errors.New(fmt.Sprintf("operator pod for unit %q is not ready yet", unitTag.Id()))
		}
	} else {
		results, err := c.applicationAPI.UnitsInfo([]names.UnitTag{unitTag})
		if err != nil {
			return nil, errors.Trace(err)
		}
		unit := results[0]
		if unit.Error != nil {
			return nil, errors.Annotatef(unit.Error, "getting unit %q", target)
		}
		if len(unit.ProviderId) == 0 {
			return nil, errors.New(fmt.Sprintf("container for unit %q is not ready yet", unitTag.Id()))
		}
		providerID = unit.ProviderId
	}
	return &resolvedTarget{
		entity: providerID,
	}, nil
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
	ch := make(chan os.Signal, 1)
	defer close(ch)
	cancel := make(chan struct{})
	ctx.InterruptNotify(ch)
	defer ctx.StopInterruptNotify(ch)

	go func() {
		select {
		case <-ch:
			close(cancel)
		}
	}()
	args := c.args
	if len(args) == 0 {
		args = []string{"sh"}
	}
	return c.execClient.Exec(
		k8sexec.ExecParams{
			PodName:  target.entity,
			Commands: args,
			Stdout:   ctx.GetStdout(),
			Stderr:   ctx.GetStderr(),
			Stdin:    ctx.GetStdin(),
			TTY:      enablePty,
		},
		cancel,
	)
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
