// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"github.com/juju/cmd/v4"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

// NewDisableCommandForTest returns a new disable command with the
// apiFunc specified to return the args.
func NewDisableCommandForTest(store jujuclient.ClientStore, api blockClientAPI, err error) cmd.Command {
	cmd := &disableCommand{
		apiFunc: func(_ newAPIRoot) (blockClientAPI, error) {
			return api, err
		},
	}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewEnableCommandForTest returns a new enable command with the
// apiFunc specified to return the args.
func NewEnableCommandForTest(store jujuclient.ClientStore, api unblockClientAPI, err error) cmd.Command {
	cmd := &enableCommand{
		apiFunc: func(_ newAPIRoot) (unblockClientAPI, error) {
			return api, err
		},
	}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

type listMockAPI interface {
	blockListAPI
	// Can't include two interfaces that specify the same method
	ListBlockedModels() ([]params.ModelBlockInfo, error)
}

// NewListCommandForTest returns a new list command with the
// apiFunc specified to return the args.
func NewListCommandForTest(store jujuclient.ClientStore, api listMockAPI, err error) cmd.Command {
	cmd := &listCommand{
		apiFunc: func(_ newAPIRoot) (blockListAPI, error) {
			return api, err
		},
		controllerAPIFunc: func(_ newControllerAPIRoot) (controllerListAPI, error) {
			return api, err
		},
	}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}
