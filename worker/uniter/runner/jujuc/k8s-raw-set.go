// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/featureflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
)

// K8sRawSetCommand implements the k8s-raw-set command.
type K8sRawSetCommand struct {
	cmd.CommandBase
	ctx Context

	specFile cmd.FileVar
}

var isK8sRawSpecEnabled = featureflag.Enabled(feature.RawK8sSpec)

func checkK8sRawSpecEnabled(cmdName string) error {
	if featureflag.Enabled(feature.RawK8sSpec) {
		return nil
	}
	return errors.NewNotSupported(nil,
		fmt.Sprintf("%q command is under feature flag. To use it, please set %q=%q", cmdName, osenv.JujuFeatureFlagEnvKey, feature.RawK8sSpec),
	)
}

// NewK8sRawSetCommand makes a k8s-raw-set command.
func NewK8sRawSetCommand(ctx Context) (cmd.Command, error) {
	return &K8sRawSetCommand{ctx: ctx}, nil
}

func (c *K8sRawSetCommand) Info() *cmd.Info {
	doc := `
Sets configuration data in k8s raw format to use for k8s resources.
The spec applies to all units for the application.
`
	return jujucmd.Info(&cmd.Info{
		Name:    "k8s-raw-set",
		Args:    "--file <core spec file>",
		Purpose: "set k8s spec information",
		Doc:     doc,
	})
}

func (c *K8sRawSetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.specFile.SetStdin()
	c.specFile.Path = "-"
	f.Var(&c.specFile, "file", "file containing k8s raw spec")
}

func (c *K8sRawSetCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *K8sRawSetCommand) Run(ctx *cmd.Context) error {
	if err := checkK8sRawSpecEnabled(c.Info().Name); err != nil {
		return errors.Trace(err)
	}
	specData, err := c.handleSpecFile(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	return c.ctx.SetRawK8sSpec(specData)
}

func (c *K8sRawSetCommand) handleSpecFile(ctx *cmd.Context) (string, error) {
	specData, err := c.specFile.Read(ctx)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(specData) == 0 {
		return "", errors.New("no k8s raw spec specified: pipe k8s raw spec to command, or specify a file with --file")
	}
	return string(specData), nil
}
