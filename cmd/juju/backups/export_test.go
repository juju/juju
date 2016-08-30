// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

const (
	NotSet          = notset
	DownloadWarning = downloadWarning
)

var (
	NewAPIClient = &newAPIClient
)

type CreateCommand struct {
	*createCommand
}

type DownloadCommand struct {
	*downloadCommand
}

func NewCreateCommandForTest() (cmd.Command, *CreateCommand) {
	c := &createCommand{}
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c), &CreateCommand{c}
}

func NewDownloadCommandForTest() (cmd.Command, *DownloadCommand) {
	c := &downloadCommand{}
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c), &DownloadCommand{c}
}

func NewListCommandForTest() cmd.Command {
	c := &listCommand{}
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c)
}

func NewShowCommandForTest() cmd.Command {
	c := &showCommand{}
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c)
}

func NewUploadCommandForTest() cmd.Command {
	c := &uploadCommand{}
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c)
}

func NewRemoveCommandForTest() cmd.Command {
	c := &removeCommand{}
	c.Log = &cmd.Log{}
	return modelcmd.Wrap(c)
}

func NewRestoreCommandForTest(
	store jujuclient.ClientStore,
	api RestoreAPI,
	getArchive func(string) (ArchiveReader, *params.BackupsMetadataResult, error),
	newEnviron func(environs.OpenParams) (environs.Environ, error),
	getRebootstrapParams func(string, *params.BackupsMetadataResult) (*restoreBootstrapParams, error),
) cmd.Command {
	c := &restoreCommand{
		getArchiveFunc:           getArchive,
		newEnvironFunc:           newEnviron,
		getRebootstrapParamsFunc: getRebootstrapParams,
		newAPIClientFunc: func() (RestoreAPI, error) {
			return api, nil
		},
		waitForAgentFunc: func(ctx *cmd.Context, c *modelcmd.ModelCommandBase, controllerName, hostedModelName string) error {
			return nil
		},
	}
	if getRebootstrapParams == nil {
		c.getRebootstrapParamsFunc = c.getRebootstrapParams
	}
	if newEnviron == nil {
		c.newEnvironFunc = environs.New
	}
	c.Log = &cmd.Log{}
	c.SetClientStore(store)
	return modelcmd.Wrap(c)
}

func GetEnvironFunc(e environs.Environ) func(environs.OpenParams) (environs.Environ, error) {
	return func(environs.OpenParams) (environs.Environ, error) {
		return e, nil
	}
}

func GetRebootstrapParamsFunc(cloud string) func(string, *params.BackupsMetadataResult) (*restoreBootstrapParams, error) {
	return func(string, *params.BackupsMetadataResult) (*restoreBootstrapParams, error) {
		return &restoreBootstrapParams{
			ControllerConfig: testing.FakeControllerConfig(),
			Cloud: environs.CloudSpec{
				Type:   "lxd",
				Name:   cloud,
				Region: "a-region",
			},
		}, nil
	}
}

func GetRebootstrapParamsFuncWithError() func(string, *params.BackupsMetadataResult) (*restoreBootstrapParams, error) {
	return func(string, *params.BackupsMetadataResult) (*restoreBootstrapParams, error) {
		return nil, errors.New("failed")
	}
}
