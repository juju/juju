// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"github.com/juju/names/v6"

	"github.com/juju/juju/state"
)

// Backend defines the State API used by the sshclient facade.
type Backend interface {
	GetSSHHostKeys(names.MachineTag) (state.SSHHostKeys, error)
}
