// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostkeyreporter

import (
	"context"

	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// Facade provides access to the HostKeyReporter API facade.
type Facade struct {
	caller base.FacadeCaller
}

// NewFacade creates a new client-side HostKeyReporter facade.
func NewFacade(caller base.APICaller) *Facade {
	return &Facade{
		caller: base.NewFacadeCaller(caller, "HostKeyReporter"),
	}
}

// ReportKeys reports the public SSH host keys for a machine to the
// controller. The keys should be in the same format as the sshd host
// key files, one entry per key.
func (f *Facade) ReportKeys(machineId string, publicKeys []string) error {
	args := params.SSHHostKeySet{EntityKeys: []params.SSHHostKeys{{
		Tag:        names.NewMachineTag(machineId).String(),
		PublicKeys: publicKeys,
	}}}
	var result params.ErrorResults
	err := f.caller.FacadeCall(context.TODO(), "ReportKeys", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}
