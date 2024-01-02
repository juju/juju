// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/client/application"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
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

`[1:]

const helpExamplesRmSaas = `
    juju remove-saas hosted-mysql
    juju remove-saas -m test-model hosted-mariadb

`

func (c *removeSaasCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-saas",
		Args:     "<saas-application-name> [<saas-application-name>...]",
		Purpose:  helpSummaryRmSaas,
		Doc:      helpDetailsRmSaas,
		Examples: helpExamplesRmSaas,
		SeeAlso: []string{
			"consume",
			"offer",
		},
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
	f.BoolVar(&c.Force, "force", false, "Completely remove a SAAS and all its dependencies")
	f.BoolVar(&c.NoWait, "no-wait", false, "Rush through SAAS removal without waiting for each individual step to complete")
	c.fs = f
}

// RemoveSaasAPI defines the API methods that the remove-saas command uses.
type RemoveSaasAPI interface {
	Close() error
	DestroyConsumedApplication(application.DestroyConsumedApplicationParams) ([]params.ErrorResult, error)
}

func (c *removeSaasCommand) Run(ctx *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

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
