// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package hostkeyreporter implements the API facade used by the
// hostkeyreporter worker.
package hostkeyreporter

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// Backend defines the State API used by the hostkeyreporter facade.
type Backend interface {
	SetSSHHostKeys(names.MachineTag, state.SSHHostKeys) error
}

// Facade implements the API required by the hostkeyreporter worker.
type Facade struct {
	backend      Backend
	getCanModify common.GetAuthFunc
}

// New returns a new API facade for the hostkeyreporter worker.
func New(backend Backend, _ facade.Resources, authorizer facade.Authorizer) (*Facade, error) {
	return &Facade{
		backend: backend,
		getCanModify: func() (common.AuthFunc, error) {
			return authorizer.AuthOwner, nil
		},
	}, nil
}

// ReportKeys sets the SSH host keys for one or more entities.
func (facade *Facade) ReportKeys(args params.SSHHostKeySet) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.EntityKeys)),
	}

	canModify, err := facade.getCanModify()
	if err != nil {
		return results, err
	}

	for i, arg := range args.EntityKeys {
		tag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canModify(tag) {
			err = facade.backend.SetSSHHostKeys(tag, state.SSHHostKeys(arg.PublicKeys))
		}
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}
