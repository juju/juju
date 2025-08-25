// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"strconv"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/client/application"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
)

// NewScaleApplicationCommand returns a command which scales an application's units.
func NewScaleApplicationCommand() modelcmd.ModelCommand {
	cmd := &scaleApplicationCommand{}
	cmd.newAPIFunc = func() (scaleApplicationAPI, error) {
		root, err := cmd.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return application.NewClient(root), nil
	}
	return modelcmd.Wrap(cmd)
}

// scaleApplicationCommand is responsible for destroying application units.
type scaleApplicationCommand struct {
	modelcmd.ModelCommandBase
	modelcmd.CAASOnlyCommand

	newAPIFunc      func() (scaleApplicationAPI, error)
	applicationName string
	scale           int
}

const scaleApplicationDoc = `
Scale a Kubernetes application by specifying how many units there should be.
The new number of units can be greater or less than the current number, thus
allowing both scale up and scale down.
`

const scaleApplicationExamples = `
    juju scale-application mariadb 2
`

// Info implements cmd.Command.
func (c *scaleApplicationCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "scale-application",
		Args:     "<application> <scale>",
		Purpose:  "Set the desired number of k8s application units.",
		Doc:      scaleApplicationDoc,
		Examples: scaleApplicationExamples,
		SeeAlso: []string{
			"remove-application",
			"add-unit",
			"remove-unit",
		},
	})
}

func (c *scaleApplicationCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("no application specified")
	}
	c.applicationName = args[0]
	if !names.IsValidApplication(c.applicationName) {
		return errors.Errorf("invalid application name %q", c.applicationName)
	}
	if len(args) == 1 {
		return errors.Errorf("no scale specified")
	}
	var err error
	c.scale, err = strconv.Atoi(args[1])
	if err != nil {
		return errors.Annotatef(err, "invalid scale %q", args[1])
	}
	if c.scale < 0 {
		return errors.New("scale must be a positive integer")
	}
	return cmd.CheckEmpty(args[2:])
}

type scaleApplicationAPI interface {
	Close() error
	ScaleApplication(application.ScaleApplicationParams) (params.ScaleApplicationResult, error)
}

// Run implements cmd.Command.
func (c *scaleApplicationCommand) Run(ctx *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.ScaleApplication(application.ScaleApplicationParams{
		ApplicationName: c.applicationName,
		Scale:           c.scale,
	})
	if err != nil {
		return block.ProcessBlockedError(errors.Annotatef(err, "could not scale application %q", c.applicationName), block.BlockChange)

	}
	if err := result.Error; err != nil {
		return err
	}
	ctx.Infof("%v scaled to %d units", c.applicationName, result.Info.Scale)
	return nil
}
