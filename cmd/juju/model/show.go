// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package model

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
)

const showModelCommandDoc = `Show information about the current or specified model`

func NewShowCommand() cmd.Command {
	return modelcmd.Wrap(&showModelCommand{})
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
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelmanager.NewClient(api), nil
}

// Info implements Command.Info.
func (c *showModelCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show-model",
		Purpose: "shows information about the current or specified model",
		Doc:     showModelCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *showModelCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Run implements Command.Run.
func (c *showModelCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.getAPI()
	if err != nil {
		return err
	}
	defer api.Close()

	store := c.ClientStore()
	modelDetails, err := store.ModelByName(
		c.ControllerName(),
		c.AccountName(),
		c.ModelName(),
	)
	if err != nil {
		return errors.Annotate(err, "getting model details")
	}

	modelTag := names.NewModelTag(modelDetails.ModelUUID)
	results, err := api.ModelInfo([]names.ModelTag{modelTag})
	if err != nil {
		return err
	}
	if results[0].Error != nil {
		return results[0].Error
	}
	infoMap, err := c.apiModelInfoToModelInfoMap([]params.ModelInfo{*results[0].Result})
	if err != nil {
		return errors.Trace(err)
	}
	return c.out.Write(ctx, infoMap)
}

func (c *showModelCommand) apiModelInfoToModelInfoMap(modelInfo []params.ModelInfo) (map[string]common.ModelInfo, error) {
	// TODO(perrito666) 2016-05-02 lp:1558657
	now := time.Now()
	output := make(map[string]common.ModelInfo)
	for _, info := range modelInfo {
		out, err := common.ModelInfoFromParams(info, now)
		if err != nil {
			return nil, errors.Trace(err)
		}
		output[out.Name] = out
	}
	return output, nil
}
