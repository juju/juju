// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/modelcmd"
)

// NewDisableCommandForTest returns a new disable command with the
// apiFunc specified to return the args.
func NewDisableCommandForTest(api blockClientAPI, err error) cmd.Command {
	return modelcmd.Wrap(&disableCommand{
		apiFunc: func(c newAPIRoot) (blockClientAPI, error) {
			return api, err
		},
	})
}

// NewEnableCommandForTest returns a new enable command with the
// apiFunc specified to return the args.
func NewEnableCommandForTest(api unblockClientAPI, err error) cmd.Command {
	return modelcmd.Wrap(&enableCommand{
		apiFunc: func(c newAPIRoot) (unblockClientAPI, error) {
			return api, err
		},
	})
}

// NewListCommandForTest returns a new list command with the
// apiFunc specified to return the args.
func NewListCommandForTest(api blockListAPI, err error) cmd.Command {
	return modelcmd.Wrap(&listCommand{
		apiFunc: func(c newAPIRoot) (blockListAPI, error) {
			return api, err
		},
	})
}
