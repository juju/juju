// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package model

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

const showModelCommandDoc = `Show information about the current or specified model.`

func NewShowCommand() cmd.Command {
	showCmd := &showModelCommand{}
	return modelcmd.Wrap(showCmd, modelcmd.WrapSkipModelFlags)
}

// showModelCommand shows all the users with access to the current model.
type showModelCommand struct {
	modelcmd.ModelCommandBase
	out cmd.Output
	api ShowModelAPI
}

// ShowModelAPI defines the methods on the client API that the
// users command calls.
type ShowModelAPI interface {
	Close() error
	ModelInfo([]names.ModelTag) ([]params.ModelInfoResult, error)
}

func (c *showModelCommand) getAPI() (ShowModelAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	api, err := c.NewControllerAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelmanager.NewClient(api), nil
}

// Info implements Command.Info.
func (c *showModelCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "show-model",
		Args:    "<model name>",
		Purpose: "Shows information about the current or specified model.",
		Doc:     showModelCommandDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *showModelCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

// Init implements Command.Init.
func (c *showModelCommand) Init(args []string) error {
	modelName := ""
	if len(args) > 0 {
		modelName = args[0]
		args = args[1:]
	}
	if err := c.SetModelIdentifier(modelName, true); err != nil {
		return errors.Trace(err)
	}
	if err := c.ModelCommandBase.Init(args); err != nil {
		return err
	}
	return nil
}

// Run implements Command.Run.
func (c *showModelCommand) Run(ctx *cmd.Context) error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	_, modelDetails, err := c.ModelDetails()
	if err != nil {
		return errors.Trace(err)
	}
	modelTag := names.NewModelTag(modelDetails.ModelUUID)

	api, err := c.getAPI()
	if err != nil {
		return err
	}
	defer api.Close()

	results, err := api.ModelInfo([]names.ModelTag{modelTag})
	if err != nil {
		return err
	}
	if results[0].Error != nil {
		return maybeEmitRedirectError(results[0].Error)
	}
	infoMap, err := c.apiModelInfoToModelInfoMap([]params.ModelInfo{*results[0].Result}, controllerName)
	if err != nil {
		return errors.Trace(err)
	}
	return c.out.Write(ctx, infoMap)
}

func (c *showModelCommand) apiModelInfoToModelInfoMap(modelInfo []params.ModelInfo, controllerName string) (map[string]common.ModelInfo, error) {
	// TODO(perrito666) 2016-05-02 lp:1558657
	now := time.Now()
	output := make(map[string]common.ModelInfo)
	for _, info := range modelInfo {
		out, err := common.ModelInfoFromParams(info, now)
		if err != nil {
			return nil, errors.Trace(err)
		}
		out.ControllerName = controllerName
		output[out.ShortName] = out
	}
	return output, nil
}

func maybeEmitRedirectError(err error) error {
	pErr, ok := errors.Cause(err).(*params.Error)
	if !ok {
		return err
	}

	var redirInfo params.RedirectErrorInfo
	if err := pErr.UnmarshalInfo(&redirInfo); err == nil && redirInfo.CACert != "" && len(redirInfo.Servers) != 0 {
		return &api.RedirectError{
			Servers:         params.ToMachineHostsPorts(redirInfo.Servers),
			CACert:          redirInfo.CACert,
			ControllerAlias: redirInfo.ControllerAlias,
			FollowRedirect:  false, // user-action required
		}
	}
	return err
}
