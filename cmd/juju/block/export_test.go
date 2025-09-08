// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"context"

	"github.com/juju/juju/api/jujuclient"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/rpc/params"
)

// NewDisableCommandForTest returns a new disable command with the
// apiFunc specified to return the args.
func NewDisableCommandForTest(store jujuclient.ClientStore, api blockClientAPI, err error) cmd.Command {
	cmd := &disableCommand{
		apiFunc: func(ctx context.Context, _ newAPIRoot) (blockClientAPI, error) {
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
		apiFunc: func(ctx context.Context, _ newAPIRoot) (unblockClientAPI, error) {
			return api, err
		},
	}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

type listMockAPI interface {
	blockListAPI
	// Can't include two interfaces that specify the same method
	ListBlockedModels(context.Context) ([]params.ModelBlockInfo, error)
}

// NewListCommandForTest returns a new list command with the
// apiFunc specified to return the args.
func NewListCommandForTest(store jujuclient.ClientStore, api listMockAPI, err error) cmd.Command {
	cmd := &listCommand{
		apiFunc: func(ctx context.Context, _ newAPIRoot) (blockListAPI, error) {
			return api, err
		},
		controllerAPIFunc: func(ctx context.Context, _ newControllerAPIRoot) (controllerListAPI, error) {
			return api, err
		},
	}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}
