// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewDisableCommandForTest returns a new disable command with the
// apiFunc specified to return the args.
func NewDisableCommandForTest(api blockClientAPI, err error) cmd.Command {
	return modelcmd.Wrap(&disableCommand{
		apiFunc: func(_ newAPIRoot) (blockClientAPI, error) {
			return api, err
		},
	})
}

// NewEnableCommandForTest returns a new enable command with the
// apiFunc specified to return the args.
func NewEnableCommandForTest(api unblockClientAPI, err error) cmd.Command {
	return modelcmd.Wrap(&enableCommand{
		apiFunc: func(_ newAPIRoot) (unblockClientAPI, error) {
			return api, err
		},
	})
}

type listMockAPI interface {
	blockListAPI
	// Can't include two interfaces that specify the same method
	ListBlockedModels() ([]params.ModelBlockInfo, error)
}

// NewListCommandForTest returns a new list command with the
// apiFunc specified to return the args.
func NewListCommandForTest(api listMockAPI, err error) cmd.Command {
	return modelcmd.Wrap(&listCommand{
		apiFunc: func(_ newAPIRoot) (blockListAPI, error) {
			return api, err
		},
		controllerAPIFunc: func(_ newControllerAPIRoot) (controllerListAPI, error) {
			return api, err
		},
	})
}
