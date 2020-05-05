// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewRemoveSaasCommand returns a command which removes a consumed application.
func NewRemoveSaasCommand() cmd.Command {
	cmd := &removeSaasCommand{}
	cmd.newAPIFunc = func() (RemoveSaasAPI, error) {
		root, err := cmd.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return application.NewClient(root), nil
	}
	return modelcmd.Wrap(cmd)
}

// removeSaasCommand causes an existing remote application to be destroyed.
type removeSaasCommand struct {
	modelcmd.ModelCommandBase
	SaasNames []string

	newAPIFunc func() (RemoveSaasAPI, error)

	Force  bool
	NoWait bool
	fs     *gnuflag.FlagSet
}

var helpSummaryRmSaas = `
Remove consumed applications (SAAS) from the model.`[1:]

var helpDetailsRmSaas = `
Removing a consumed (SAAS) application will terminate any relations that
application has, potentially leaving any related local applications
in a non-functional state.

Examples:
    juju remove-saas hosted-mysql
    juju remove-saas -m test-model hosted-mariadb`[1:]

func (c *removeSaasCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove-saas",
		Args:    "<saas-application-name> [<saas-application-name>...]",
		Aliases: []string{"remove-consumed-application"},
		Purpose: helpSummaryRmSaas,
		Doc:     helpDetailsRmSaas,
	})
}

func (c *removeSaasCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("no SAAS application names specified")
	}
	for _, arg := range args {
		if !names.IsValidApplication(arg) {
			return errors.Errorf("invalid SAAS application name %q", arg)
		}
	}
	c.SaasNames = args
	return nil
}

func (c *removeSaasCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.Force, "force", false, "Completely remove an application and all its dependencies")
	f.BoolVar(&c.NoWait, "no-wait", false, "Rush through application removal without waiting for each individual step to complete")
	c.fs = f
}

// RemoveSaasAPI defines the API methods that the remove-saas command uses.
type RemoveSaasAPI interface {
	Close() error
	BestAPIVersion() int
	DestroyConsumedApplication(application.DestroyConsumedApplicationParams) ([]params.ErrorResult, error)
}

func (c *removeSaasCommand) Run(ctx *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

	apiVersion := client.BestAPIVersion()
	if apiVersion < 5 {
		return errors.New("remove-saas is not supported by this version of Juju")
	} else if apiVersion < 10 {
		if c.Force {
			return errors.New("--force is not supported by this version of Juju")
		}
		if c.NoWait {
			return errors.New("--no-wait is not supported by this version of Juju")
		}
	}

	if c.NoWait && !c.Force {
		return errors.New("--no-wait requires --force")
	}

	return c.removeSaass(ctx, client)
}

func (c *removeSaasCommand) removeSaass(
	ctx *cmd.Context,
	client RemoveSaasAPI,
) error {
	var maxWait *time.Duration
	if c.Force {
		if c.NoWait {
			zeroSec := 0 * time.Second
			maxWait = &zeroSec
		}
	}
	params := application.DestroyConsumedApplicationParams{
		Force:     c.Force,
		MaxWait:   maxWait,
		SaasNames: c.SaasNames,
	}
	results, err := client.DestroyConsumedApplication(params)
	if err := block.ProcessBlockedError(err, block.BlockRemove); err != nil {
		return errors.Trace(err)
	}
	anyFailed := false
	for i, name := range c.SaasNames {
		result := results[i]
		if result.Error != nil {
			ctx.Infof("removing SAAS application %s failed: %s", name, result.Error)
			anyFailed = true
			continue
		}
	}
	if anyFailed {
		return cmd.ErrSilent
	}
	return nil
}
